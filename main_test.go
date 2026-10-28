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
