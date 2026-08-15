// Package transport serves the MCP server over stdio or streamable HTTP.
package transport

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// Paths the HTTP transport answers on. The MCP path is mcp-go's default; health sits
// beside it and needs no token, because the image is built FROM scratch and a container
// healthcheck has no shell to read one with.
const (
	MCPPath    = "/mcp"
	HealthPath = "/healthz"

	// DiscoveryQuery and DiscoveryHeader are how a connection says it can cope with a tool
	// list that grows.
	//
	// It has to be the client's decision rather than the server's, because it depends on
	// the client: one that re-reads its list when told to gets a small set and asks for the
	// rest, one that does not would be left unable to call anything it was not given at
	// connection time. Saying nothing gets everything, which is what a client that cannot
	// answer for itself should get.
	//
	// Both spellings exist because clients differ in what they let a person configure: some
	// take only a URL, some take headers as well.
	DiscoveryQuery  = "discovery"
	DiscoveryHeader = "X-Gdocs-Discovery"
)

// WantsDiscovery says whether a request asked for the growing tool list.
//
// Anything but an explicit off counts as on once the switch is present at all: a client
// configured with ?discovery=1, =on or =true means the same thing, and guessing at the
// spelling is not something a person should have to do to make their editor work.
func WantsDiscovery(r *http.Request) bool {
	for _, value := range []string{r.URL.Query().Get(DiscoveryQuery), r.Header.Get(DiscoveryHeader)} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "":
			continue
		case "0", "off", "false", "no":
			return false
		default:
			return true
		}
	}

	return false
}

// ServeStdio hands the server to the stdio transport.
func ServeStdio(mcpServer *server.MCPServer) error {
	return server.ServeStdio(mcpServer)
}

// ServeHTTP starts the streamable HTTP transport on address, behind bearer auth. Extra
// handlers are mounted beside it — the sign-in pages, which a browser reaches and which
// therefore cannot carry an Authorization header.
func ServeHTTP(servers, discovery map[string]*server.MCPServer, address, token string, extra map[string]http.Handler) error {
	httpServer := &http.Server{
		Addr:    address,
		Handler: NewHandler(servers, discovery, token, extra),
		// Only the header deadline is set. Read and write deadlines would cut off the
		// long-lived SSE streams this transport is built on.
		ReadHeaderTimeout: 10 * time.Second,
	}

	return httpServer.ListenAndServe()
}

// NewHandler wires the MCP endpoints behind bearer auth and adds the health endpoint.
//
// There is more than one endpoint because one process serves several sets of tools: /mcp
// is everything the configuration allows, and /mcp/slides, /mcp/sheets and the rest are
// the same process narrowed to one family. A project that needs slides connects to the
// slides path and never sees the other ninety tool descriptions — which is the whole point,
// since every name in the listing costs the agent context on every session.
//
// Every path has a second server behind it for connections that asked for discovery: the
// same tools, offered a few at a time instead of all at once. Which one answers is decided
// per request, so two clients on the same path can be given different things.
//
// The sign-in, the token and the token store are shared: it is one server, seen through
// several windows.
func NewHandler(servers, discovery map[string]*server.MCPServer, token string, extra map[string]http.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})

	for path, handler := range extra {
		mux.Handle(path, handler)
	}

	for path, mcpServer := range servers {
		// The streamable transport is told its own path: it builds the session
		// endpoints from it, and a server mounted at /mcp/docs that believes it lives
		// at /mcp hands the client an address that answers something else.
		endpoint := server.NewStreamableHTTPServer(mcpServer, server.WithEndpointPath(path))

		var handler http.Handler = endpoint
		if narrow, ok := discovery[path]; ok {
			handler = switchOnDiscovery(endpoint,
				server.NewStreamableHTTPServer(narrow, server.WithEndpointPath(path)))
		}

		mux.Handle(path, RequireBearer(handler, token))
	}

	return mux
}

// switchOnDiscovery sends a request to whichever server the connection asked for.
//
// The switch is read on every request rather than once per session because that is all this
// layer can see; the session itself is kept by whichever transport answers, and a client that
// sets the parameter in its configuration sets it on all of its requests. A client that sets
// it on some and not others gets two sessions, which is what it asked for.
func switchOnDiscovery(full, narrow http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if WantsDiscovery(r) {
			narrow.ServeHTTP(w, r)
			return
		}

		full.ServeHTTP(w, r)
	})
}

// RequireBearer rejects everything that does not carry the expected token.
//
// This server acts as the person who signed in, so whatever reaches its port inherits
// that reach; on loopback that is any process running as the same account. Comparison is
// constant-time so a wrong token leaks nothing through response timing.
func RequireBearer(next http.Handler, token string) http.Handler {
	expected := []byte(token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented, ok := BearerToken(r.Header.Get("Authorization"))
		if !ok || subtle.ConstantTimeCompare([]byte(presented), expected) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="mcp-gdocs"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// BearerToken pulls the token out of an Authorization header value. The scheme is
// matched case-insensitively, as the HTTP spec requires.
func BearerToken(header string) (string, bool) {
	const scheme = "bearer "

	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}

	token := strings.TrimSpace(header[len(scheme):])
	if token == "" {
		return "", false
	}

	return token, true
}
