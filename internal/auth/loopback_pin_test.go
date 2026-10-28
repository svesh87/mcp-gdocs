package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The loopback sign-in is what colleagues run today: a Desktop client, the page at
// http://127.0.0.1:8819/login?key=…, and Google coming back to /oauth2callback on the same
// port. --public-url was added beside it, and nothing about it may change for a server
// started without that flag. These pages were recorded from the code as it stood before
// --public-url existed; a diff here is a change to what those people see.

const loopbackGoldenDir = "testdata"

// loopbackPages renders every page the loopback sign-in can show without a token, with the
// token path replaced by a placeholder so the recording does not depend on the test's
// temporary directory.
func loopbackPages(t *testing.T) map[string]string {
	t.Helper()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")
	authenticator := New(Credentials{ClientID: "id", ClientSecret: "secret", AuthURI: DefaultAuthURI},
		NewStore(tokenPath), []string{"https://www.googleapis.com/auth/drive"})
	handlers := NewWebLogin(authenticator, "bearer-token").Handlers()

	pages := map[string]string{}
	record := func(name string, handler http.Handler, request *http.Request) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		var b strings.Builder
		b.WriteString("status: " + http.StatusText(recorder.Code) + "\n")
		for _, header := range []string{"Cache-Control", "Referrer-Policy", "Content-Type"} {
			b.WriteString(header + ": " + recorder.Header().Get(header) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(strings.ReplaceAll(recorder.Body.String(), tokenPath, "TOKEN_PATH"))
		pages[name] = b.String()
	}

	record("login_page", handlers[LoginPath],
		httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8819/login?key=bearer-token", nil))
	record("login_refused", handlers[LoginPath],
		httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8819/login?key=wrong", nil))

	routable := httptest.NewRequest(http.MethodGet, "http://gdocs.example.invalid:8819/login?key=bearer-token", nil)
	routable.Host = "gdocs.example.invalid:8819"
	record("login_routable", handlers[LoginPath], routable)

	record("callback_unknown_state", handlers[CallbackPath],
		httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8819"+CallbackPath+"?state=made-up&code=x", nil))
	record("callback_denied", handlers[CallbackPath],
		httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8819"+CallbackPath+"?error=access_denied", nil))

	// The trip to Google: the address changes with every state, so what is recorded is its
	// shape — where it goes and which parameters it carries.
	recorder := httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, postForm(t, "http://127.0.0.1:8819/login",
		url.Values{"key": {"bearer-token"}}))
	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatalf("the redirect is not a URL: %v", err)
	}
	query := location.Query()
	var b strings.Builder
	b.WriteString("status: " + http.StatusText(recorder.Code) + "\n")
	b.WriteString("to: " + location.Scheme + "://" + location.Host + location.Path + "\n")
	for _, name := range []string{"access_type", "client_id", "code_challenge_method", "prompt",
		"redirect_uri", "response_type", "scope"} {
		b.WriteString(name + "=" + query.Get(name) + "\n")
	}
	pages["login_submit"] = b.String()

	return pages
}

// The recordings are deliberately not rewritten by -update: they are the old behaviour, and
// there is no change to it that this test should ever accept on its own.
func TestLoopbackSignInIsUnchanged(t *testing.T) {
	for name, got := range loopbackPages(t) {
		want, err := os.ReadFile(filepath.Join(loopbackGoldenDir, "loopback_"+name+".txt"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != string(want) {
			t.Errorf("%s changed for a server without --public-url:\n--- want\n%s\n--- got\n%s", name, want, got)
		}
	}
}

// A tool called before anybody signed in says this, and the people running the loopback
// sign-in follow it to the page.
func TestNoTokenMessageIsUnchanged(t *testing.T) {
	const want = "no Google token yet: run `mcp-gdocs login` on a machine with a browser, " +
		"or open /login?key=<token> on a server started with --transport=streamable-http"
	if ErrNoToken.Error() != want {
		t.Errorf("ErrNoToken = %q", ErrNoToken)
	}
}

// Without --public-url the server mounts exactly the two paths it always had: nothing at the
// root, nothing at /callback.
func TestLoopbackMountsOnlyItsOwnPaths(t *testing.T) {
	authenticator := New(Credentials{ClientID: "id", ClientSecret: "secret", AuthURI: DefaultAuthURI},
		NewStore(filepath.Join(t.TempDir(), "token.json")), nil)
	handlers := NewWebLogin(authenticator, "bearer-token").Handlers()

	if len(handlers) != 2 || handlers[LoginPath] == nil || handlers[CallbackPath] == nil {
		t.Errorf("the loopback sign-in mounts %d paths, want /login and %s", len(handlers), CallbackPath)
	}
}
