package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}

	return path
}

func TestLoadCredentials(t *testing.T) {
	dir := t.TempDir()

	path := writeFile(t, dir, "gcp-oauth.keys.json", `{"installed": {
		"client_id": "client.apps.googleusercontent.com",
		"client_secret": "secret",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"}}`)

	creds, err := LoadCredentials(path)
	if err != nil {
		t.Fatalf("a desktop client file should load: %v", err)
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		t.Error("both halves of the client should be read")
	}
}

func TestLoadCredentialsFailures(t *testing.T) {
	dir := t.TempDir()

	if _, err := LoadCredentials(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("a missing client file should be an error")
	}

	broken := writeFile(t, dir, "broken.json", `{"installed": {"client_id": "id"`)
	_, err := LoadCredentials(broken)
	if err == nil {
		t.Fatal("a truncated client file should be an error")
	}
	// The file holds a client secret, so an error about it must not quote its contents.
	if strings.Contains(err.Error(), "client_id") {
		t.Errorf("the error should not echo the file: %v", err)
	}

	empty := writeFile(t, dir, "empty.json", `{}`)
	if _, err := LoadCredentials(empty); err == nil || !strings.Contains(err.Error(), "Desktop") {
		t.Errorf("a file with no client section should say what to download instead, got %v", err)
	}

	half := writeFile(t, dir, "half.json", `{"installed": {"client_id": "id"}}`)
	if _, err := LoadCredentials(half); err == nil || !strings.Contains(err.Error(), "client_secret") {
		t.Errorf("a client with no secret should be refused, got %v", err)
	}

	// A web client is accepted: the person who downloaded the wrong type deserves a
	// clear failure at sign-in rather than a parse error here.
	web := writeFile(t, dir, "web.json", `{"web": {"client_id": "id", "client_secret": "secret"}}`)
	creds, err := LoadCredentials(web)
	if err != nil {
		t.Fatalf("a web client file should load: %v", err)
	}
	if creds.AuthURI != DefaultAuthURI || creds.TokenURI != DefaultTokenURI {
		t.Error("a client file naming no endpoints should fall back to Google's own")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sub", "token.json"))

	if _, err := store.Load(); !errors.Is(err, ErrNoToken) {
		t.Errorf("a missing token should be ErrNoToken, got %v", err)
	}

	token := &oauth2.Token{
		AccessToken:  "access",
		TokenType:    "Bearer",
		RefreshToken: "refresh",
		Expiry:       time.Now().Add(time.Hour).Truncate(time.Second),
	}

	if err := store.Save(token, []string{"drive"}); err != nil {
		t.Fatalf("saving the token: %v", err)
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("the token file should exist: %v", err)
	}
	// The file is the whole of this server's access to somebody's account.
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the token file is %v, want 0600", mode)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("loading the token: %v", err)
	}
	if loaded.RefreshToken != "refresh" || loaded.AccessToken != "access" {
		t.Errorf("the token came back as %+v", loaded)
	}
}

func TestStoreRefusesTokenWithoutRefresh(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "token.json"))

	// Without a refresh token the server would stop working within the hour, and the
	// failure would look like an outage rather than a sign-in that went wrong.
	if err := store.Save(&oauth2.Token{AccessToken: "access"}, nil); err == nil {
		t.Error("a token with no refresh token should not be saved")
	}
	if err := store.Save(nil, nil); err == nil {
		t.Error("saving nothing should be an error")
	}
}

func TestStoreRejectsUnusableFiles(t *testing.T) {
	dir := t.TempDir()

	broken := NewStore(writeFile(t, dir, "broken.json", `not json`))
	if _, err := broken.Load(); err == nil || !strings.Contains(err.Error(), "sign in again") {
		t.Errorf("a broken token file should say what to do, got %v", err)
	}

	partial := NewStore(writeFile(t, dir, "partial.json", `{"access_token": "a"}`))
	if _, err := partial.Load(); err == nil || !strings.Contains(err.Error(), "no refresh token") {
		t.Errorf("a token file with no refresh token should say so, got %v", err)
	}
}

// tokenServer stands in for Google's token endpoint.
func tokenServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)

	return server
}

func TestHTTPClientRefreshesAndSaves(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "token.json"))

	// An expired access token with a refresh token: exactly the state a server restarts
	// into after a night of doing nothing.
	if err := store.Save(&oauth2.Token{
		AccessToken:  "old",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	}, nil); err != nil {
		t.Fatalf("preparing the token: %v", err)
	}

	google := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("refresh_token") != "refresh" {
			t.Errorf("the refresh token should be sent, got %q", form.Get("refresh_token"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token": "fresh", "token_type": "Bearer", "expires_in": 3600}`)
	})

	authenticator := New(Credentials{
		ClientID:     "id",
		ClientSecret: "secret",
		AuthURI:      DefaultAuthURI,
		TokenURI:     google.URL,
	}, store, []string{"drive"})

	client, err := authenticator.HTTPClient(context.Background())
	if err != nil {
		t.Fatalf("building the client: %v", err)
	}

	api := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fresh" {
			t.Errorf("the refreshed token should be used, got %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{}`)
	})

	resp, err := client.Get(api.URL)
	if err != nil {
		t.Fatalf("calling the API: %v", err)
	}
	_ = resp.Body.Close()

	// The refreshed token is written back, so the next process starts from it rather
	// than refreshing again.
	saved, err := store.Load()
	if err != nil {
		t.Fatalf("reading the saved token: %v", err)
	}
	if saved.AccessToken != "fresh" {
		t.Errorf("the refreshed token should have been saved, got %q", saved.AccessToken)
	}
	if saved.RefreshToken != "refresh" {
		t.Errorf("the refresh token should survive a refresh, got %q", saved.RefreshToken)
	}
}

// TestHTTPClientOutlivesTheRequestThatBuiltIt is the outage this cured. The client is
// built once, on whichever tool call needed Google first, and kept for the life of the
// process; the refresh comes an hour later, when that call is long over. Tied to the
// call's context, every refresh from then on failed with "context canceled" — the server
// worked for an hour and then answered nothing until it was restarted.
func TestHTTPClientOutlivesTheRequestThatBuiltIt(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "token.json"))

	if err := store.Save(&oauth2.Token{
		AccessToken:  "old",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	}, nil); err != nil {
		t.Fatalf("preparing the token: %v", err)
	}

	google := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token": "fresh", "token_type": "Bearer", "expires_in": 3600}`)
	})

	authenticator := New(Credentials{
		ClientID:     "id",
		ClientSecret: "secret",
		AuthURI:      DefaultAuthURI,
		TokenURI:     google.URL,
	}, store, []string{"drive"})

	// The context of the request that first needed a client, cancelled the moment that
	// request is answered.
	requestCtx, done := context.WithCancel(context.Background())
	client, err := authenticator.HTTPClient(requestCtx)
	if err != nil {
		t.Fatalf("building the client: %v", err)
	}
	done()

	api := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fresh" {
			t.Errorf("the refreshed token should be used, got %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{}`)
	})

	resp, err := client.Get(api.URL)
	if err != nil {
		t.Fatalf("the client should still refresh after the request that built it is over: %v", err)
	}
	_ = resp.Body.Close()
}

func TestHTTPClientWithoutTokenSaysSo(t *testing.T) {
	authenticator := New(Credentials{ClientID: "id", ClientSecret: "secret"},
		NewStore(filepath.Join(t.TempDir(), "token.json")), nil)

	_, err := authenticator.HTTPClient(context.Background())
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("a server nobody signed in to should say exactly that, got %v", err)
	}
	if !strings.Contains(err.Error(), "login") {
		t.Errorf("the error should name the way out of it: %v", err)
	}
}

func TestFlowRefusesMismatchedState(t *testing.T) {
	flow, err := NewFlow(&oauth2.Config{ClientID: "id", Endpoint: oauth2.Endpoint{AuthURL: DefaultAuthURI}})
	if err != nil {
		t.Fatalf("starting a flow: %v", err)
	}

	if _, err := flow.Exchange(context.Background(), "somebody else's state", "code"); err == nil {
		t.Error("a callback carrying the wrong state should be refused")
	}
	if _, err := flow.Exchange(context.Background(), flow.State(), ""); err == nil {
		t.Error("a callback with no code should be refused")
	}
}

func TestAuthURLAsksForOfflineAccess(t *testing.T) {
	flow, err := NewFlow(&oauth2.Config{
		ClientID:    "id",
		RedirectURL: "http://127.0.0.1:9999" + CallbackPath,
		Endpoint:    oauth2.Endpoint{AuthURL: DefaultAuthURI},
	})
	if err != nil {
		t.Fatalf("starting a flow: %v", err)
	}

	address, err := url.Parse(flow.AuthURL())
	if err != nil {
		t.Fatalf("the authorisation address is not a URL: %v", err)
	}

	query := address.Query()
	// Without offline access and a forced consent screen Google returns no refresh
	// token on a repeat sign-in, and the server stops working within the hour.
	if query.Get("access_type") != "offline" || query.Get("prompt") != "consent" {
		t.Errorf("the consent request should ask for offline access: %s", address)
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Errorf("the sign-in should be PKCE-protected: %s", address)
	}
}

func TestExchangeRefusesTokenWithoutRefresh(t *testing.T) {
	google := tokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token": "a", "token_type": "Bearer", "expires_in": 3600}`)
	})

	flow, err := NewFlow(&oauth2.Config{
		ClientID: "id",
		Endpoint: oauth2.Endpoint{AuthURL: DefaultAuthURI, TokenURL: google.URL},
	})
	if err != nil {
		t.Fatalf("starting a flow: %v", err)
	}

	_, err = flow.Exchange(context.Background(), flow.State(), "code")
	if err == nil || !strings.Contains(err.Error(), "revoke") {
		t.Errorf("a consent that returned no refresh token should say how to fix it, got %v", err)
	}
}

func TestLoopbackRedirect(t *testing.T) {
	for _, test := range []struct {
		host string
		want string
	}{
		{"127.0.0.1:8819", "http://127.0.0.1:8819" + CallbackPath},
		{"localhost:8819", "http://127.0.0.1:8819" + CallbackPath},
	} {
		got, err := loopbackRedirect(&http.Request{Host: test.host})
		if err != nil {
			t.Fatalf("%s: %v", test.host, err)
		}
		if got != test.want {
			t.Errorf("loopbackRedirect(%q) = %q, want %q", test.host, got, test.want)
		}
	}

	// A redirect to anything routable would hand the authorisation code to whoever
	// answers there.
	if _, err := loopbackRedirect(&http.Request{Host: "gdocs.example.invalid:8819"}); err == nil {
		t.Error("a sign-in against a routable address should be refused")
	}
	if _, err := loopbackRedirect(&http.Request{Host: ""}); err == nil {
		t.Error("a request with no Host has nowhere to redirect to")
	}
	if _, err := loopbackRedirect(&http.Request{Host: "localhost"}); err == nil {
		t.Error("a Host with no port should be refused")
	}
}

func TestWebLoginNeedsTheServersToken(t *testing.T) {
	// The scopes are full URLs by the time they reach the authenticator: config resolves
	// the aliases, and the page shows what will actually be asked for.
	authenticator := New(Credentials{ClientID: "id", ClientSecret: "secret", AuthURI: DefaultAuthURI},
		NewStore(filepath.Join(t.TempDir(), "token.json")),
		[]string{"https://www.googleapis.com/auth/drive"})
	login := NewWebLogin(authenticator, "bearer-token")

	handlers := login.Handlers()

	recorder := httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8819/login?key=wrong", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("a wrong key should be refused, got %d", recorder.Code)
	}
	// Nothing to look at and nothing to click without the key.
	if strings.Contains(recorder.Body.String(), "<form") {
		t.Error("the page should not be rendered to whoever knocked without the key")
	}

	recorder = httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8819/login?key=bearer-token", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("the right key should show the page, got %d", recorder.Code)
	}

	body := recorder.Body.String()
	// The page says what state the sign-in is in and what will be asked for, and only
	// then offers to go to Google.
	for _, want := range []string{"входа ещё не было", "auth/drive", "<form", `method="post"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page should carry %q, got %s", want, body)
		}
	}

	// The key is in the address, so the page must not be cached and must not travel in a
	// referrer to Google.
	if recorder.Header().Get("Cache-Control") != "no-store" ||
		recorder.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("the page leaks the key: %v", recorder.Header())
	}
}

func TestWebLoginGoesToGoogleOnSubmit(t *testing.T) {
	authenticator := New(Credentials{ClientID: "id", ClientSecret: "secret", AuthURI: DefaultAuthURI},
		NewStore(filepath.Join(t.TempDir(), "token.json")), []string{"drive"})
	handlers := NewWebLogin(authenticator, "bearer-token").Handlers()

	recorder := httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, postForm(t, "http://127.0.0.1:8819/login", url.Values{
		"key": {"bearer-token"},
	}))

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("submitting the form should send the browser to Google, got %d", recorder.Code)
	}

	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatalf("the redirect is not a URL: %v", err)
	}
	if location.Query().Get("redirect_uri") != "http://127.0.0.1:8819"+CallbackPath {
		t.Errorf("the redirect should come back to this server: %s", location)
	}

	// Submitting without the key is a refusal, not a trip to Google: the form is the only
	// thing that carries it back.
	recorder = httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, postForm(t, "http://127.0.0.1:8819/login", url.Values{}))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("a submission without the key should be refused, got %d", recorder.Code)
	}
}

func TestWebLoginRefusesNonLoopback(t *testing.T) {
	authenticator := New(Credentials{ClientID: "id", ClientSecret: "secret", AuthURI: DefaultAuthURI},
		NewStore(filepath.Join(t.TempDir(), "token.json")), []string{"drive"})
	handlers := NewWebLogin(authenticator, "bearer-token").Handlers()

	request := httptest.NewRequest(http.MethodGet, "http://gdocs.example.invalid:8819/login?key=bearer-token", nil)
	request.Host = "gdocs.example.invalid:8819"

	recorder := httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, request)

	// A sign-in opened on a routable address cannot work — Google only redirects a
	// desktop client to loopback — so the page says how to get one instead of failing at
	// Google.
	if !strings.Contains(recorder.Body.String(), "ssh -L") {
		t.Errorf("the page should explain the tunnel, got %s", recorder.Body)
	}
}

// postForm builds a form submission the way a browser sends one.
func postForm(t *testing.T, address string, values url.Values) *http.Request {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, address, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return request
}

func TestWebLoginCallbackChecksState(t *testing.T) {
	authenticator := New(Credentials{ClientID: "id", ClientSecret: "secret", AuthURI: DefaultAuthURI},
		NewStore(filepath.Join(t.TempDir(), "token.json")), nil)
	handlers := NewWebLogin(authenticator, "bearer-token").Handlers()

	// Nothing started this sign-in, so the state is unknown: this is what an
	// authorisation code arriving from anywhere else looks like.
	recorder := httptest.NewRecorder()
	handlers[CallbackPath].ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8819"+CallbackPath+"?state=made-up&code=x", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("an unknown state should be refused, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handlers[CallbackPath].ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8819"+CallbackPath+"?error=access_denied", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("a refused consent should be reported, got %d", recorder.Code)
	}
}

func TestWebLoginStoresTheToken(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "token.json"))

	google := tokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token": "a", "refresh_token": "r", "token_type": "Bearer", "expires_in": 3600}`)
	})

	authenticator := New(Credentials{
		ClientID: "id", ClientSecret: "secret",
		AuthURI: DefaultAuthURI, TokenURI: google.URL,
	}, store, []string{"drive"})

	login := NewWebLogin(authenticator, "bearer-token")
	handlers := login.Handlers()

	recorder := httptest.NewRecorder()
	handlers[LoginPath].ServeHTTP(recorder, postForm(t, "http://127.0.0.1:8819/login", url.Values{
		"key": {"bearer-token"},
	}))

	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatalf("the redirect is not a URL: %v", err)
	}
	state := location.Query().Get("state")

	recorder = httptest.NewRecorder()
	handlers[CallbackPath].ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"http://127.0.0.1:8819"+CallbackPath+"?state="+url.QueryEscape(state)+"&code=auth-code", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("the callback should have finished the sign-in, got %d: %s", recorder.Code, recorder.Body)
	}

	saved, err := store.Load()
	if err != nil {
		t.Fatalf("the token should have been saved: %v", err)
	}
	if saved.RefreshToken != "r" {
		t.Errorf("the saved token is %+v", saved)
	}

	// The state is single use: replaying the same callback is not a second sign-in.
	recorder = httptest.NewRecorder()
	handlers[CallbackPath].ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"http://127.0.0.1:8819"+CallbackPath+"?state="+url.QueryEscape(state)+"&code=auth-code", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("a replayed callback should be refused, got %d", recorder.Code)
	}
}

func TestSavedTokenIsReadableJSON(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "token.json"))

	if err := store.Save(&oauth2.Token{AccessToken: "a", RefreshToken: "r"}, []string{"drive"}); err != nil {
		t.Fatalf("saving: %v", err)
	}

	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the token file should be readable JSON: %v", err)
	}
	if decoded["refresh_token"] != "r" {
		t.Errorf("the file is %s", raw)
	}
}
