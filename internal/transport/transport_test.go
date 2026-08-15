package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestBearerToken(t *testing.T) {
	for _, test := range []struct {
		header string
		token  string
		ok     bool
	}{
		{"Bearer secret", "secret", true},
		// The scheme is case-insensitive per the HTTP spec, and clients differ.
		{"bearer secret", "secret", true},
		{"BEARER  secret  ", "secret", true},
		{"Basic secret", "", false},
		{"Bearer", "", false},
		{"Bearer ", "", false},
		{"", "", false},
	} {
		token, ok := BearerToken(test.header)
		if ok != test.ok || token != test.token {
			t.Errorf("BearerToken(%q) = %q, %v; want %q, %v", test.header, token, ok, test.token, test.ok)
		}
	}
}

func TestRequireBearer(t *testing.T) {
	guarded := RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("reached"))
	}), "secret")

	for _, test := range []struct {
		name   string
		header string
		status int
	}{
		{"right token", "Bearer secret", http.StatusOK},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"no header", "", http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/mcp", nil)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}

			recorder := httptest.NewRecorder()
			guarded.ServeHTTP(recorder, request)

			if recorder.Code != test.status {
				t.Errorf("got %d, want %d", recorder.Code, test.status)
			}
			if test.status == http.StatusUnauthorized &&
				!strings.Contains(recorder.Header().Get("WWW-Authenticate"), "Bearer") {
				t.Error("a refusal should say what kind of credential it wanted")
			}
		})
	}
}

func TestHandlerRoutes(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "0.0.0")

	pages := map[string]http.Handler{
		"/login": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("sign in"))
		}),
	}

	handler := NewHandler(map[string]*server.MCPServer{
		MCPPath:             mcpServer,
		MCPPath + "/slides": server.NewMCPServer("test", "0.0.0"),
	}, nil, "secret", pages)

	// The discovery switch is read before anything else looks at the request, and it has to
	// be readable both ways: some clients let a person set only a URL, some only headers.
	for _, probe := range []struct {
		name   string
		target string
		header string
		want   bool
	}{
		{name: "nothing said", target: MCPPath, want: false},
		{name: "in the address", target: MCPPath + "?discovery=1", want: true},
		{name: "spelled out", target: MCPPath + "?discovery=on", want: true},
		{name: "turned off in the address", target: MCPPath + "?discovery=0", want: false},
		{name: "in a header", target: MCPPath, header: "on", want: true},
		{name: "turned off in a header", target: MCPPath, header: "false", want: false},
	} {
		t.Run(probe.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, probe.target, nil)
			if probe.header != "" {
				request.Header.Set(DiscoveryHeader, probe.header)
			}
			if got := WantsDiscovery(request); got != probe.want {
				t.Errorf("WantsDiscovery = %v, want %v", got, probe.want)
			}
		})
	}

	// Health carries no token: the image is built FROM scratch, and a container
	// healthcheck has no shell to read one with.
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, HealthPath, nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "ok") {
		t.Errorf("health should answer without a token, got %d %s", recorder.Code, recorder.Body)
	}

	// The sign-in page is reached by a browser, which sends no Authorization header; it
	// checks its own key in the query string instead.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusOK {
		t.Errorf("the sign-in page should be mounted, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, MCPPath, strings.NewReader("{}")))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("the MCP endpoint should demand a token, got %d", recorder.Code)
	}

	// A connection that asks for discovery is answered by a different server on the same
	// path — and it is behind the same token, because it is the same server seen another way.
	withDiscovery := NewHandler(
		map[string]*server.MCPServer{MCPPath: mcpServer},
		map[string]*server.MCPServer{MCPPath: server.NewMCPServer("narrow", "0.0.0")},
		"secret", nil)

	recorder = httptest.NewRecorder()
	withDiscovery.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodPost, MCPPath+"?discovery=1", strings.NewReader("{}")))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("the discovery endpoint should demand a token too, got %d", recorder.Code)
	}

	// A family's own path is a second window on the same process, and it is guarded the
	// same way: one token for the server, not one per set of tools.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, MCPPath+"/slides", strings.NewReader("{}")))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("a family endpoint should demand a token too, got %d", recorder.Code)
	}
}
