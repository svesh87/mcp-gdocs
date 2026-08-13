package auth

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/oauth2"
)

// Authenticator turns the stored token into an HTTP client that keeps working.
type Authenticator struct {
	creds  Credentials
	store  *Store
	scopes []string
}

// New builds an authenticator over one credentials file and one token file.
func New(creds Credentials, store *Store, scopes []string) *Authenticator {
	return &Authenticator{creds: creds, store: store, scopes: append([]string(nil), scopes...)}
}

// Scopes are the scopes this server signs in with.
func (a *Authenticator) Scopes() []string { return append([]string(nil), a.scopes...) }

// Config builds the OAuth configuration for one redirect address.
func (a *Authenticator) Config(redirectURL string) *oauth2.Config {
	return a.creds.OAuthConfig(a.scopes, redirectURL)
}

// Store is where the token lives.
func (a *Authenticator) Store() *Store { return a.store }

// HTTPClient returns a client that carries the access token and refreshes it when it
// runs out. The refreshed token is written back, so the next process starts from the
// current one instead of refreshing again.
//
// The redirect address is irrelevant here: refreshing never sends the browser anywhere.
//
// The cancellation of ctx is deliberately dropped. This client is built once, on whichever
// request needed it first, and then kept for the life of the process — while the token
// source it carries refreshes an hour later, long after that request is over. Tied to the
// request's context, every refresh from then on fails with "context canceled": the server
// works for an hour and then stops, and only a restart brings it back. Values are kept, so
// an HTTP client put into the context for a test is still honoured.
func (a *Authenticator) HTTPClient(ctx context.Context) (*http.Client, error) {
	token, err := a.store.Load()
	if err != nil {
		return nil, err
	}

	lifetime := context.WithoutCancel(ctx)

	config := a.Config("")
	source := &savingSource{
		source: config.TokenSource(lifetime, token),
		store:  a.store,
		scopes: a.scopes,
		last:   token.AccessToken,
	}

	return oauth2.NewClient(lifetime, source), nil
}

// savingSource writes a refreshed token back to the store.
type savingSource struct {
	source oauth2.TokenSource
	store  *Store
	scopes []string

	mu   sync.Mutex
	last string
}

// Token refreshes when needed and persists what came back.
//
// A failure to save is not a failure to authenticate: the request in hand still has a
// valid token, and refusing it because a file could not be written would turn a
// permissions problem on a mounted directory into an outage.
func (s *savingSource) Token() (*oauth2.Token, error) {
	token, err := s.source.Token()
	if err != nil {
		return nil, fmt.Errorf("refreshing the Google token: %w "+
			"(if this says invalid_grant, the consent was revoked or the token expired: sign in again)", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if token.AccessToken != s.last {
		s.last = token.AccessToken
		_ = s.store.Save(token, s.scopes)
	}

	return token, nil
}
