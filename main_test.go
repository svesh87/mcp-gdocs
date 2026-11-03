package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/access"
	"github.com/svesh87/mcp-gdocs/internal/auth"
	"github.com/svesh87/mcp-gdocs/internal/transport"
)

// gatewayStack builds the HTTP handler exactly as serve mounts it, from a command line.
func gatewayStack(t *testing.T, extra ...string) http.Handler {
	t.Helper()

	credentials, tokenDir := writeCredentials(t)
	t.Setenv("MCP_AUTH_TOKEN", "bearer-token")

	args := append([]string{"--transport", "streamable-http", "--credentials", credentials,
		"--token-dir", tokenDir}, extra...)

	flags, done, err := parseFlags("test", args)
	if err != nil || done {
		t.Fatalf("parsing flags: %v %v", err, done)
	}
	cfg, err := loadConfig(flags)
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}
	authenticator, err := newAuthenticator(cfg)
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	pages, err := newWebLogin(cfg, authenticator)
	if err != nil {
		t.Fatalf("building the sign-in: %v", err)
	}
	handlers, err := pageHandlers(cfg, pages)
	if err != nil {
		t.Fatalf("building the pages: %v", err)
	}

	servers := map[string]*server.MCPServer{transport.MCPPath: server.NewMCPServer("test", "0")}

	return transport.NewHandler(servers, nil, cfg.AuthToken, handlers)
}

// throughGateway is a request as the gateway forwards it, with the bearer it adds.
func throughGateway(method, path string, header http.Header) *http.Request {
	request := httptest.NewRequest(method, "http://gdocs:8819"+path, nil)
	request.Header.Set("Authorization", "Bearer bearer-token")
	for name, values := range header {
		request.Header[name] = values
	}
	return request
}

// A public address that is a host of its own, with no path: the pages are <host>/login and
// <host>/callback, the root redirects to the first, and Google is sent back to the second.
func TestPublicURLOnABareHost(t *testing.T) {
	stack := gatewayStack(t, "--public-url", "https://gdocs.example.com/")

	recorder := httptest.NewRecorder()
	stack.ServeHTTP(recorder, throughGateway(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "https://gdocs.example.com/login" {
		t.Errorf("GET / should redirect to the sign-in, got %d %q", recorder.Code, recorder.Header().Get("Location"))
	}

	recorder = httptest.NewRecorder()
	stack.ServeHTTP(recorder, throughGateway(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "https://gdocs.example.com/callback") {
		t.Errorf("the sign-in page should name <host>/callback, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	stack.ServeHTTP(recorder, throughGateway(http.MethodPost, "/login",
		http.Header{"Sec-Fetch-Site": {"same-origin"}, "Origin": {"https://gdocs.example.com"}}))
	location, err := url.Parse(recorder.Header().Get("Location"))
	if recorder.Code != http.StatusSeeOther || err != nil {
		t.Fatalf("the form should go to Google, got %d %v", recorder.Code, err)
	}
	if got := location.Query().Get("redirect_uri"); got != "https://gdocs.example.com/callback" {
		t.Errorf("Google is told to return to %q", got)
	}

	// A form from another site is refused on the bare host too.
	recorder = httptest.NewRecorder()
	stack.ServeHTTP(recorder, throughGateway(http.MethodPost, "/login",
		http.Header{"Sec-Fetch-Site": {"cross-site"}, "Origin": {"https://evil.example.com"}}))
	if recorder.Code != http.StatusForbidden {
		t.Errorf("a form from another site should be refused, got %d", recorder.Code)
	}
}

// With the Access flags every page demands the Access token — the sign-in, the root and
// the callback alike — and the MCP endpoint and the health check do not.
func TestAccessGuardsEveryPageAndNothingElse(t *testing.T) {
	stack := gatewayStack(t, "--public-url", "https://gdocs.example.com",
		"--access-team-domain", "team.cloudflareaccess.com",
		"--access-aud", "aud-tag", "--access-email", "owner@example.com")

	for _, path := range []string{"/", auth.LoginPath, auth.PublicCallbackPath + "?state=s&code=c"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			recorder := httptest.NewRecorder()
			stack.ServeHTTP(recorder, throughGateway(method, path, nil))
			if recorder.Code != http.StatusForbidden {
				t.Errorf("%s %s without an Access token: status %d, want 403", method, path, recorder.Code)
			}
		}
	}

	recorder := httptest.NewRecorder()
	stack.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, transport.HealthPath, nil))
	if recorder.Code != http.StatusOK {
		t.Errorf("the health check needs no Access token, got %d", recorder.Code)
	}

	// /mcp answers its bearer as before: a POST with no body is the transport's own
	// refusal, not the Access check's. (A GET would open the listening stream and stay.)
	recorder = httptest.NewRecorder()
	stack.ServeHTTP(recorder, throughGateway(http.MethodPost, transport.MCPPath, nil))
	if recorder.Code == http.StatusForbidden || recorder.Code == http.StatusUnauthorized {
		t.Errorf("/mcp is not behind the Access check, got %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	stack.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, transport.MCPPath, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("/mcp still demands its bearer, got %d", recorder.Code)
	}
}

// Without the Access flags the pages are mounted exactly as they come from the sign-in.
func TestNoAccessFlagsLeavesThePagesAlone(t *testing.T) {
	stack := gatewayStack(t)

	recorder := httptest.NewRecorder()
	stack.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"http://127.0.0.1:8819"+auth.LoginPath+"?key=bearer-token", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "http://127.0.0.1:8819"+auth.CallbackPath) {
		t.Errorf("the loopback /login?key= should open as before, got %d", recorder.Code)
	}

	if _, err := access.New("", "", nil); err == nil {
		t.Error("an empty Access check should not build")
	}
}

func TestAccessFlagsAreRefusedHalfway(t *testing.T) {
	credentials, tokenDir := writeCredentials(t)
	t.Setenv("MCP_AUTH_TOKEN", "bearer-token")

	err := run([]string{"--transport", "streamable-http", "--credentials", credentials, "--token-dir", tokenDir,
		"--public-url", "https://gdocs.example.com", "--access-team-domain", "team.cloudflareaccess.com"})
	if err == nil || !strings.Contains(err.Error(), "all three or none") {
		t.Errorf("a partial Access check should stop the server before it starts, got %v", err)
	}
}

func writeCredentials(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "gcp-oauth.keys.json")

	if err := os.WriteFile(path, []byte(
		`{"installed": {"client_id": "id", "client_secret": "secret"}}`), 0o600); err != nil {
		t.Fatalf("writing the client file: %v", err)
	}

	return path, filepath.Join(dir, "tokens")
}

func TestVersionFlag(t *testing.T) {
	if err := run([]string{"--version"}); err != nil {
		t.Errorf("--version should print and stop, got %v", err)
	}
}

func TestUnknownFlag(t *testing.T) {
	if err := run([]string{"--nonsense"}); err == nil {
		t.Error("an unknown flag should be an error")
	}
}

func TestServerRefusesWithoutCredentials(t *testing.T) {
	err := run([]string{"--token-dir", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "--credentials") {
		t.Errorf("expected a refusal naming the missing flag, got %v", err)
	}
}

func TestLoginRefusesWithoutAClientFile(t *testing.T) {
	err := run([]string{"login", "--credentials", filepath.Join(t.TempDir(), "missing.json"),
		"--token-dir", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "OAuth client file") {
		t.Errorf("expected a refusal about the client file, got %v", err)
	}
}

// The login command signs in through loopback, which a Web application client does not
// accept; a server with a public address signs in on its own page.
func TestLoginRefusesAPublicURL(t *testing.T) {
	credentials, tokenDir := writeCredentials(t)

	err := run([]string{"login", "--credentials", credentials, "--token-dir", tokenDir,
		"--public-url", "https://mcp.example.com/gdocs"})
	if err == nil || !strings.Contains(err.Error(), "/login") {
		t.Errorf("expected a refusal pointing at the server's page, got %v", err)
	}
}

func TestPublicURLChoosesTheSignIn(t *testing.T) {
	credentials, tokenDir := writeCredentials(t)
	t.Setenv("MCP_AUTH_TOKEN", "bearer-token")

	for _, test := range []struct {
		args      []string
		loginURL  string
		mountsOld bool
	}{
		// No flag: the loopback route, mounted at the paths it always had.
		{nil, "", true},
		{[]string{"--public-url", "https://mcp.example.com/gdocs/"}, "https://mcp.example.com/gdocs/login", false},
	} {
		args := append([]string{"--transport", "streamable-http", "--credentials", credentials,
			"--token-dir", tokenDir}, test.args...)

		flags, done, err := parseFlags("test", args)
		if err != nil || done {
			t.Fatalf("parsing flags: %v %v", err, done)
		}
		cfg, err := loadConfig(flags)
		if err != nil {
			t.Fatalf("loading the configuration: %v", err)
		}
		authenticator, err := newAuthenticator(cfg)
		if err != nil {
			t.Fatalf("building the authenticator: %v", err)
		}

		pages, err := newWebLogin(cfg, authenticator)
		if err != nil {
			t.Fatalf("building the sign-in: %v", err)
		}
		if pages.LoginURL() != test.loginURL {
			t.Errorf("%v: LoginURL = %q, want %q", test.args, pages.LoginURL(), test.loginURL)
		}
		if _, ok := pages.Handlers()[auth.CallbackPath]; ok != test.mountsOld {
			t.Errorf("%v: %s mounted = %v, want %v", test.args, auth.CallbackPath, ok, test.mountsOld)
		}

		// A tool called before anybody signed in: the loopback advice without the flag,
		// the public page with it — and ErrNoToken either way.
		provider := &lazyClients{authenticator: authenticator, loginURL: pages.LoginURL()}
		_, err = provider.Google(context.Background())
		if !errors.Is(err, auth.ErrNoToken) {
			t.Fatalf("%v: a server with no token should say so, got %v", test.args, err)
		}
		if test.loginURL == "" && err.Error() != auth.ErrNoToken.Error() {
			t.Errorf("without --public-url the message is the old one, got %v", err)
		}
		if test.loginURL != "" && !strings.Contains(err.Error(), test.loginURL) {
			t.Errorf("with --public-url the message names the page, got %v", err)
		}
	}
}

func TestNewAuthenticatorCreatesTheTokenDirectory(t *testing.T) {
	credentials, tokenDir := writeCredentials(t)

	flags, done, err := parseFlags("test", []string{"--credentials", credentials, "--token-dir", tokenDir})
	if err != nil || done {
		t.Fatalf("parsing flags: %v %v", err, done)
	}

	cfg, err := loadConfig(flags)
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}

	authenticator, err := newAuthenticator(cfg)
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}

	info, err := os.Stat(tokenDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("the token directory should have been created: %v", err)
	}
	// The directory holds somebody's access to their Google account.
	if mode := info.Mode().Perm(); mode != 0o700 {
		t.Errorf("the token directory is %v, want 0700", mode)
	}

	// Nobody has signed in yet, so the tools have to refuse with something a person can
	// act on rather than crash the server at startup.
	provider := &lazyClients{authenticator: authenticator}
	if _, err := provider.Google(context.Background()); !errors.Is(err, auth.ErrNoToken) {
		t.Errorf("a server with no token should say so, got %v", err)
	}
}
