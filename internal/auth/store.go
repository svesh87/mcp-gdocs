package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// Store keeps the token of the signed-in account in one file.
//
// The file is the whole of this server's access to somebody's Google account, so it is
// written 0600 and replaced atomically: a token half-written by a process that died is a
// sign-in nobody can repeat without noticing.
type Store struct {
	path string
	mu   sync.Mutex
}

// storedToken is the token as it sits on disk. The field names match what Google's own
// tooling writes, so a file from elsewhere can be dropped in.
type storedToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
}

// NewStore points a store at a file.
func NewStore(path string) *Store { return &Store{path: path} }

// Path is the file the store reads and writes.
func (s *Store) Path() string { return s.path }

// Status is what the sign-in page says about the stored token.
//
// Deliberately no token material, not even truncated: this is rendered in a browser, and
// browsers keep history.
type Status struct {
	// SignedIn is whether a usable token is on disk.
	SignedIn bool
	// AccessValid is whether the access token is still good. False is normal and not a
	// problem: the refresh token makes a new one on the next call.
	AccessValid bool
	// AccessLeft is how long the access token has, when it has any.
	AccessLeft time.Duration
	// Scopes are the scopes the token was granted with, as recorded at sign-in.
	Scopes []string
	// Path is the file the token lives in.
	Path string
	// Problem is why there is no usable token, in words, when there is none.
	Problem string
}

// Status reads the token and describes it without handing any of it out.
func (s *Store) Status(now time.Time) Status {
	status := Status{Path: s.path}

	token, err := s.Load()
	if err != nil {
		status.Problem = err.Error()
		return status
	}

	status.SignedIn = true
	status.Scopes = s.scopes()

	if !token.Expiry.IsZero() {
		status.AccessValid = token.Expiry.After(now)
		status.AccessLeft = token.Expiry.Sub(now)
	} else {
		// No expiry recorded: the token came from elsewhere. It is used until Google
		// says otherwise.
		status.AccessValid = token.AccessToken != ""
	}

	return status
}

// scopes reads the scopes recorded beside the token, which Load does not carry.
func (s *Store) scopes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path) //nolint:gosec // the path comes from the operator's flag
	if err != nil {
		return nil
	}

	var stored storedToken
	if json.Unmarshal(raw, &stored) != nil {
		return nil
	}

	return stored.Scopes
}

// Load reads the token. A missing file is ErrNoToken rather than a path error: the
// difference matters to the person reading it.
func (s *Store) Load() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path) //nolint:gosec // the path comes from the operator's flag
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoToken
		}
		return nil, fmt.Errorf("reading the token file: %w", err)
	}

	var stored storedToken
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("the token file %s is not valid JSON: sign in again to replace it", s.path)
	}

	if stored.RefreshToken == "" {
		return nil, fmt.Errorf("the token file %s has no refresh token: sign in again, "+
			"and make sure the consent screen was actually completed", s.path)
	}

	return &oauth2.Token{
		AccessToken:  stored.AccessToken,
		TokenType:    stored.TokenType,
		RefreshToken: stored.RefreshToken,
		Expiry:       stored.Expiry,
	}, nil
}

// Save writes the token, replacing whatever was there.
func (s *Store) Save(token *oauth2.Token, scopes []string) error {
	if token == nil || token.RefreshToken == "" {
		return errors.New("refusing to save a token without a refresh token: " +
			"without it the server would stop working within the hour")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating the token directory: %w", err)
	}

	payload, err := json.MarshalIndent(storedToken{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
		Scopes:       scopes,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the token: %w", err)
	}

	// Written next to the target and renamed over it: rename within one directory is
	// atomic, so a reader sees either the old token or the new one.
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".token-*.json")
	if err != nil {
		return fmt.Errorf("creating a temporary token file: %w", err)
	}
	tempName := temp.Name()

	defer func() { _ = os.Remove(tempName) }()

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("restricting the token file: %w", err)
	}
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return fmt.Errorf("writing the token: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("flushing the token: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing the token file: %w", err)
	}

	if err := os.Rename(tempName, s.path); err != nil {
		return fmt.Errorf("replacing the token file: %w", err)
	}

	return nil
}
