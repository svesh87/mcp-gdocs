package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svesh87/mcp-gdocs/internal/auth"
)

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
