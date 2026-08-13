package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/oauth2"
)

// CallbackPath is where Google sends the browser back to. It is the same path in both
// sign-in routes, so only the port differs between them.
const CallbackPath = "/oauth2callback"

// Flow is one sign-in attempt: a state to recognise it by and a PKCE verifier to prove
// the code was redeemed by whoever asked for it.
type Flow struct {
	config   *oauth2.Config
	state    string
	verifier string
}

// NewFlow starts a sign-in against one redirect address.
func NewFlow(config *oauth2.Config) (*Flow, error) {
	state, err := randomString(32)
	if err != nil {
		return nil, fmt.Errorf("generating the OAuth state: %w", err)
	}

	return &Flow{config: config, state: state, verifier: oauth2.GenerateVerifier()}, nil
}

// State identifies this attempt among the ones in flight.
func (f *Flow) State() string { return f.state }

// AuthURL is where the person is sent to give consent.
//
// Offline access and a forced consent screen are both deliberate: without them Google
// returns no refresh token on a repeat sign-in, and a server with only an access token
// stops working within the hour.
func (f *Flow) AuthURL() string {
	return f.config.AuthCodeURL(f.state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.S256ChallengeOption(f.verifier),
	)
}

// Exchange turns the code Google sent back into a token, refusing anything whose state
// does not match this attempt.
func (f *Flow) Exchange(ctx context.Context, state, code string) (*oauth2.Token, error) {
	if state == "" || state != f.state {
		return nil, errors.New("the sign-in state does not match: start the sign-in again")
	}
	if code == "" {
		return nil, errors.New("Google sent no authorisation code back")
	}

	token, err := f.config.Exchange(ctx, code, oauth2.VerifierOption(f.verifier))
	if err != nil {
		return nil, fmt.Errorf("exchanging the authorisation code: %w", err)
	}

	if token.RefreshToken == "" {
		return nil, errors.New("Google returned no refresh token: revoke this application's access " +
			"in the account's security settings and sign in again")
	}

	return token, nil
}

func randomString(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
