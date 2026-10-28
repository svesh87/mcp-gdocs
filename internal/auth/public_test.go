package auth

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

// The public route: the server behind a gateway at https://mcp.example.com/gdocs, which
// strips the /gdocs prefix, checks who the person is and adds the bearer header. Requests
// reach this server at /login and /callback; every address the browser is shown is built
// from the public one.

const testPublicURL = "https://mcp.example.com/gdocs"

func publicLogin(t *testing.T, store *Store, tokenURL string) *WebLogin {
	t.Helper()

	authenticator := New(Credentials{
		ClientID: "web-id", ClientSecret: "web-secret",
		AuthURI: DefaultAuthURI, TokenURI: tokenURL,
	}, store, []string{"https://www.googleapis.com/auth/drive"})

	login, err := NewPublicWebLogin(authenticator, "bearer-token", testPublicURL+"/")
	if err != nil {
		t.Fatalf("building the public sign-in: %v", err)
	}

	return login
}

// throughGateway is a request as the gateway forwards it: prefix stripped, bearer added.
func throughGateway(method, path string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, "http://gdocs:8819"+path, body)
	request.Host = "gdocs:8819"
	request.Header.Set("Authorization", "Bearer bearer-token")
	return request
}

func TestPublicLoginURL(t *testing.T) {
	login := publicLogin(t, NewStore(filepath.Join(t.TempDir(), "token.json")), "")

	// The trailing slash of the flag does not double up, and the address has no key.
	if got := login.LoginURL(); got != testPublicURL+"/login" {
		t.Errorf("LoginURL = %q", got)
	}

	if NewWebLogin(nil, "x").LoginURL() != "" {
		t.Error("the loopback route has no single sign-in address to print")
	}

	if _, err := NewPublicWebLogin(nil, "x", "/gdocs"); err == nil {
		t.Error("a relative public address should be refused")
	}
}

func TestPublicRootGoesToTheLoginPage(t *testing.T) {
	login := publicLogin(t, NewStore(filepath.Join(t.TempDir(), "token.json")), "")
	handlers := login.Handlers()

	for _, path := range []string{LoginPath, PublicCallbackPath, RootPattern} {
		if handlers[path] == nil {
			t.Errorf("the public route should mount %s", path)
		}
	}
	if handlers[CallbackPath] != nil {
		t.Errorf("the public route returns to %s, not %s", PublicCallbackPath, CallbackPath)
	}

	// Through a real mux, as the transport mounts it: only the exact root redirects.
	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.Handle(path, handler)
	}

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, throughGateway(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != testPublicURL+"/login" {
		t.Errorf("GET / should redirect to the public sign-in page, got %d %q",
			recorder.Code, recorder.Header().Get("Location"))
	}

	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, throughGateway(http.MethodGet, "/elsewhere", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("anything but the exact root should be unknown, got %d", recorder.Code)
	}

	// The handler guards itself too, for a mux that would hand it more than the root.
	recorder = httptest.NewRecorder()
	handlers[RootPattern].ServeHTTP(recorder, throughGateway(http.MethodGet, "/elsewhere", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("the root handler should answer only the root, got %d", recorder.Code)
	}
}

func TestPublicPageKeepsTheTokenOutOfTheHTML(t *testing.T) {
	login := publicLogin(t, NewStore(filepath.Join(t.TempDir(), "token.json")), "")
	handlers := login.Handlers()

	recorder := httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, throughGateway(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("the gateway's header should open the page, got %d: %s", recorder.Code, recorder.Body)
	}

	body := recorder.Body.String()
	if strings.Contains(body, "bearer-token") || strings.Contains(body, `name="key"`) {
		t.Errorf("a page opened with the header must not repeat the token: %s", body)
	}
	for _, want := range []string{
		`action="` + testPublicURL + `/login"`,
		testPublicURL + "/callback",
		"auth/drive",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page should carry %q, got %s", want, body)
		}
	}

	if recorder.Header().Get("Cache-Control") != "no-store" ||
		recorder.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("the page should keep its headers: %v", recorder.Header())
	}

	// Without the header and without the key there is nothing to see, and the refusal
	// points at the public address with no key in it.
	request := throughGateway(http.MethodGet, "/login", nil)
	request.Header.Del("Authorization")
	recorder = httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || strings.Contains(recorder.Body.String(), "<form") {
		t.Errorf("no credential should be refused, got %d %s", recorder.Code, recorder.Body)
	}
	if !strings.Contains(recorder.Body.String(), testPublicURL+"/login") ||
		strings.Contains(recorder.Body.String(), "key=") {
		t.Errorf("the refusal should name the public page without a key, got %s", recorder.Body)
	}

	// A wrong header is no better than none.
	request = throughGateway(http.MethodGet, "/login", nil)
	request.Header.Set("Authorization", "Bearer wrong")
	recorder = httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("a wrong bearer should be refused, got %d", recorder.Code)
	}
}

// A person reaching the server directly with ?key= still gets in, and then the form
// carries the key back as it always has — it came from the address, not from a header.
func TestPublicPageStillTakesTheKey(t *testing.T) {
	login := publicLogin(t, NewStore(filepath.Join(t.TempDir(), "token.json")), "")

	request := throughGateway(http.MethodGet, "/login?key=bearer-token", nil)
	request.Header.Del("Authorization")

	recorder := httptest.NewRecorder()
	login.Handlers()[LoginPath].ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `name="key"`) {
		t.Errorf("the key should still open the page, got %d %s", recorder.Code, recorder.Body)
	}
}

func TestPublicSignInStoresTheToken(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "token.json"))

	google := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		// The code is redeemed against the same redirect it was issued for, with the
		// PKCE verifier beside it.
		if form.Get("redirect_uri") != testPublicURL+"/callback" {
			t.Errorf("the exchange should name the public callback, got %q", form.Get("redirect_uri"))
		}
		if form.Get("code_verifier") == "" {
			t.Error("the PKCE verifier should be sent with the code")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token": "a", "refresh_token": "r", "token_type": "Bearer", "expires_in": 3600}`)
	})

	handlers := publicLogin(t, store, google.URL).Handlers()

	// The form, submitted from the page itself: header from the gateway, origin of the
	// public address, no key anywhere.
	submit := throughGateway(http.MethodPost, "/login", strings.NewReader(""))
	submit.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	submit.Header.Set("Origin", "https://mcp.example.com")

	recorder := httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, submit)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("submitting should send the browser to Google, got %d: %s", recorder.Code, recorder.Body)
	}

	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatalf("the redirect is not a URL: %v", err)
	}
	query := location.Query()
	for name, want := range map[string]string{
		"redirect_uri":          testPublicURL + "/callback",
		"client_id":             "web-id",
		"access_type":           "offline",
		"prompt":                "consent",
		"code_challenge_method": "S256",
	} {
		if query.Get(name) != want {
			t.Errorf("%s = %q, want %q", name, query.Get(name), want)
		}
	}
	if strings.Contains(location.String(), "bearer-token") {
		t.Error("the trip to Google must not carry the token")
	}
	state := query.Get("state")

	// Google returns the browser through the gateway, which adds the header again.
	recorder = httptest.NewRecorder()
	handlers[PublicCallbackPath].ServeHTTP(recorder, throughGateway(http.MethodGet,
		"/callback?state="+url.QueryEscape(state)+"&code=auth-code", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("the callback should finish the sign-in, got %d: %s", recorder.Code, recorder.Body)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "Вход выполнен") || strings.Contains(body, "bearer-token") {
		t.Errorf("the success page should say so without the token: %s", body)
	}
	if !strings.Contains(body, `href="`+testPublicURL+`/login"`) {
		t.Errorf("the success page should link back to the public sign-in: %s", body)
	}

	// The same token file the loopback route writes: one sign-in for every window.
	saved, err := store.Load()
	if err != nil || saved.RefreshToken != "r" {
		t.Fatalf("the token should have been saved, got %+v %v", saved, err)
	}

	// Single use, as in the loopback route.
	recorder = httptest.NewRecorder()
	handlers[PublicCallbackPath].ServeHTTP(recorder, throughGateway(http.MethodGet,
		"/callback?state="+url.QueryEscape(state)+"&code=auth-code", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("a replayed callback should be refused, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handlers[PublicCallbackPath].ServeHTTP(recorder, throughGateway(http.MethodGet, "/callback?error=access_denied", nil))
	if recorder.Code != http.StatusBadRequest || strings.Contains(recorder.Body.String(), "bearer-token") {
		t.Errorf("a refused consent should be reported without the token, got %d", recorder.Code)
	}
}

// The gateway adds the bearer to whatever the browser sends, so a form on another site
// would otherwise be enough to start a sign-in in the owner's browser.
func TestPublicFormRefusesAnotherSite(t *testing.T) {
	handlers := publicLogin(t, NewStore(filepath.Join(t.TempDir(), "token.json")), "").Handlers()

	submit := throughGateway(http.MethodPost, "/login", strings.NewReader(""))
	submit.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	submit.Header.Set("Origin", "https://evil.example")

	recorder := httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, submit)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("a cross-site submission should be refused, got %d", recorder.Code)
	}
}

// The loopback route answers the header exactly as before --public-url existed: it never
// looked at it, and a page that suddenly opened for it would be a change nobody asked for.
func TestLoopbackIgnoresTheHeader(t *testing.T) {
	authenticator := New(Credentials{ClientID: "id", ClientSecret: "secret", AuthURI: DefaultAuthURI},
		NewStore(filepath.Join(t.TempDir(), "token.json")), nil)
	handlers := NewWebLogin(authenticator, "bearer-token").Handlers()

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8819/login", nil)
	request.Header.Set("Authorization", "Bearer bearer-token")

	recorder := httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("the loopback page takes only the key, got %d", recorder.Code)
	}
}

func TestNoTokenAt(t *testing.T) {
	err := NoTokenAt(testPublicURL + "/login")
	if !errors.Is(err, ErrNoToken) {
		t.Error("the public form of the error should still be ErrNoToken")
	}
	if !strings.Contains(err.Error(), testPublicURL+"/login") || strings.Contains(err.Error(), "key=") {
		t.Errorf("the error should name the public page without a key: %v", err)
	}
}

func TestBearerFromHeader(t *testing.T) {
	for header, want := range map[string]string{
		"Bearer abc":   "abc",
		"bearer  abc ": "abc",
		"Basic abc":    "",
		"Bearer ":      "",
		"":             "",
	} {
		got, ok := bearerFromHeader(header)
		if got != want || ok != (want != "") {
			t.Errorf("bearerFromHeader(%q) = %q, %v", header, got, ok)
		}
	}
}
