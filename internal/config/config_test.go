package config

import (
	"strings"
	"testing"
)

// env builds a fake environment out of a map.
func env(values map[string]string) Env {
	return func(key string) string { return values[key] }
}

func validFlags() Flags {
	return Flags{
		Transport:   TransportStdio,
		Credentials: "/config/gcp-oauth.keys.json",
		TokenDir:    "/config/tokens",
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(validFlags(), env(nil))
	if err != nil {
		t.Fatalf("a stdio server with credentials and a token directory should start: %v", err)
	}

	if len(cfg.Scopes) != len(DefaultScopes) {
		t.Errorf("without --scopes the default four are asked for, got %v", cfg.Scopes)
	}
	if cfg.TokenPath() != "/config/tokens/token.json" {
		t.Errorf("the token path is %s", cfg.TokenPath())
	}
	if cfg.AllowWrite {
		t.Error("writing is off unless it was asked for")
	}
}

func TestLoadRequiresAClientAndATokenDir(t *testing.T) {
	flags := validFlags()
	flags.Credentials = ""

	_, err := Load(flags, env(nil))
	if err == nil || !strings.Contains(err.Error(), "--credentials") || !strings.Contains(err.Error(), EnvClientID) {
		t.Errorf("with no client at all the error should name both ways of giving one, got %v", err)
	}

	flags = validFlags()
	flags.TokenDir = ""

	if _, err := Load(flags, env(nil)); err == nil || !strings.Contains(err.Error(), "--token-dir") {
		t.Errorf("a server with nowhere to keep the token should refuse to start, got %v", err)
	}
}

// TestClientFromEnvironment covers the setup where the client comes out of a password
// store: two variables, no file, and nothing about the application on disk.
func TestClientFromEnvironment(t *testing.T) {
	flags := validFlags()
	flags.Credentials = ""

	cfg, err := Load(flags, env(map[string]string{
		EnvClientID:     "client.apps.googleusercontent.com",
		EnvClientSecret: "secret",
	}))
	if err != nil {
		t.Fatalf("a client from the environment should be enough: %v", err)
	}

	if cfg.ClientID != "client.apps.googleusercontent.com" || cfg.ClientSecret != "secret" {
		t.Errorf("the client came back as %q / %q", cfg.ClientID, cfg.ClientSecret)
	}
	if cfg.Credentials != "" {
		t.Errorf("with a client in the environment there is no file to read, got %q", cfg.Credentials)
	}
}

func TestClientFromEnvironmentOutranksTheFile(t *testing.T) {
	// A file left behind from an earlier setup should not quietly outrank what was just
	// injected.
	cfg, err := Load(validFlags(), env(map[string]string{
		EnvClientID:     "client.apps.googleusercontent.com",
		EnvClientSecret: "secret",
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	if cfg.Credentials != "" || cfg.ClientID == "" {
		t.Errorf("the environment should win, got file %q and id %q", cfg.Credentials, cfg.ClientID)
	}
}

func TestHalfAClientIsRefused(t *testing.T) {
	flags := validFlags()
	flags.Credentials = ""

	for _, half := range []struct {
		name    string
		values  map[string]string
		missing string
	}{
		{"only the id", map[string]string{EnvClientID: "id"}, EnvClientSecret},
		{"only the secret", map[string]string{EnvClientSecret: "secret"}, EnvClientID},
	} {
		t.Run(half.name, func(t *testing.T) {
			// Half a client fails at Google as invalid_client, which says nothing about
			// which half is missing. Better to refuse here and name it.
			_, err := Load(flags, env(half.values))
			if err == nil || !strings.Contains(err.Error(), half.missing) {
				t.Errorf("expected a refusal naming %s, got %v", half.missing, err)
			}
		})
	}
}

func TestHTTPTransportDemandsAToken(t *testing.T) {
	flags := validFlags()
	flags.Transport = TransportHTTP
	flags.Address = "0.0.0.0:8819"

	// The port reaches Google as the person who signed in. Serving it without a token
	// would hand that reach to anything that can knock.
	if _, err := Load(flags, env(nil)); err == nil || !strings.Contains(err.Error(), EnvAuthToken) {
		t.Errorf("HTTP without a bearer token should be refused, got %v", err)
	}

	cfg, err := Load(flags, env(map[string]string{EnvAuthToken: "secret"}))
	if err != nil {
		t.Fatalf("HTTP with a token should start: %v", err)
	}
	if cfg.AuthToken != "secret" {
		t.Errorf("the token should come from the environment, got %q", cfg.AuthToken)
	}

	flags.Address = ""
	if _, err := Load(flags, env(map[string]string{EnvAuthToken: "secret"})); err == nil {
		t.Error("HTTP with no address should be refused")
	}
}

func TestUnknownTransport(t *testing.T) {
	flags := validFlags()
	flags.Transport = "grpc"

	if _, err := Load(flags, env(nil)); err == nil || !strings.Contains(err.Error(), "unknown transport") {
		t.Errorf("expected a refusal naming the transports, got %v", err)
	}
}

func TestParseScopes(t *testing.T) {
	scopes, err := ParseScopes("presentations, drive , presentations")
	if err != nil {
		t.Fatalf("aliases should be accepted: %v", err)
	}

	if len(scopes) != 2 || scopes[0] != ScopePresentations || scopes[1] != ScopeDrive {
		t.Errorf("scopes should be resolved once each and in order, got %v", scopes)
	}

	if _, err := ParseScopes("mail"); err == nil || !strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("an unknown alias should be refused before Google sees it, got %v", err)
	}

	custom, err := ParseScopes("https://www.googleapis.com/auth/calendar")
	if err != nil || len(custom) != 1 {
		t.Errorf("a full URL should pass through, got %v %v", custom, err)
	}

	empty, err := ParseScopes("")
	if err != nil || len(empty) != len(DefaultScopes) {
		t.Errorf("an empty list means the defaults, got %v %v", empty, err)
	}
}
