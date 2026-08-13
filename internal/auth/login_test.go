package auth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// authURLPattern picks the address out of what the login command prints.
var authURLPattern = regexp.MustCompile(`https?://\S+`)

// syncBuffer is a buffer the test reads while the command writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestLoginCLI walks the whole sign-in a person does on a machine with a browser: the
// command prints an address, the browser comes back to the loopback port it opened, and
// the token lands in the file the server will read.
func TestLoginCLI(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "token.json"))

	google := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("code") != "auth-code" {
			t.Errorf("the code from the browser should be exchanged, got %q", form.Get("code"))
		}
		// PKCE: the verifier has to come back with the code, or a code stolen in
		// transit would be enough to get a token.
		if form.Get("code_verifier") == "" {
			t.Error("the PKCE verifier should be sent with the code")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w,
			`{"access_token": "a", "refresh_token": "r", "token_type": "Bearer", "expires_in": 3600}`)
	})

	authenticator := New(Credentials{
		ClientID: "id", ClientSecret: "secret",
		AuthURI: DefaultAuthURI, TokenURI: google.URL,
	}, store, []string{"drive"})

	out := &syncBuffer{}
	done := make(chan error, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() { done <- LoginCLI(ctx, authenticator, out) }()

	address := waitForAuthURL(t, out)

	redirect := address.Query().Get("redirect_uri")
	if !strings.HasPrefix(redirect, "http://127.0.0.1:") {
		t.Fatalf("the sign-in should come back to loopback, got %q", redirect)
	}

	callback := redirect + "?state=" + url.QueryEscape(address.Query().Get("state")) + "&code=auth-code"
	resp, err := http.Get(callback) //nolint:gosec,noctx // the address is this test's own loopback listener
	if err != nil {
		t.Fatalf("calling the callback: %v", err)
	}
	_ = resp.Body.Close()

	if err := <-done; err != nil {
		t.Fatalf("the sign-in failed: %v", err)
	}

	saved, err := store.Load()
	if err != nil {
		t.Fatalf("the token should have been saved: %v", err)
	}
	if saved.RefreshToken != "r" {
		t.Errorf("the saved token is %+v", saved)
	}
}

func TestLoginCLIStopsWhenAsked(t *testing.T) {
	authenticator := New(Credentials{ClientID: "id", ClientSecret: "secret", AuthURI: DefaultAuthURI},
		NewStore(filepath.Join(t.TempDir(), "token.json")), nil)

	ctx, cancel := context.WithCancel(context.Background())
	out := &syncBuffer{}
	done := make(chan error, 1)

	go func() { done <- LoginCLI(ctx, authenticator, out) }()

	waitForAuthURL(t, out)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("a cancelled sign-in should report that it did not finish")
		}
	case <-time.After(5 * time.Second):
		t.Error("the sign-in should stop when the context is cancelled")
	}
}

// waitForAuthURL reads the address the login command printed.
func waitForAuthURL(t *testing.T, out *syncBuffer) *url.URL {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if match := authURLPattern.FindString(out.String()); match != "" {
			address, err := url.Parse(match)
			if err != nil {
				t.Fatalf("the printed address is not a URL: %v", err)
			}
			return address
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("the login command printed no address: %s", out.String())
	return nil
}
