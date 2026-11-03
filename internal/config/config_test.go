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

// Without --public-url nothing changes: the field stays empty and the loopback sign-in is
// what the server serves.
func TestPublicURLIsOptIn(t *testing.T) {
	flags := validFlags()
	flags.Transport = TransportHTTP
	flags.Address = "127.0.0.1:8819"

	cfg, err := Load(flags, env(map[string]string{EnvAuthToken: "t"}))
	if err != nil || cfg.PublicURL != "" {
		t.Fatalf("no flag means no public address, got %q %v", cfg.PublicURL, err)
	}

	flags.PublicURL = " https://mcp.example.com/gdocs/ "
	cfg, err = Load(flags, env(map[string]string{EnvAuthToken: "t"}))
	if err != nil || cfg.PublicURL != "https://mcp.example.com/gdocs" {
		t.Fatalf("the public address should lose its trailing slash, got %q %v", cfg.PublicURL, err)
	}

	// The address belongs to the HTTP sign-in page; on stdio there is no page.
	stdio := validFlags()
	stdio.PublicURL = "https://mcp.example.com/gdocs"
	if _, err := Load(stdio, env(nil)); err == nil || !strings.Contains(err.Error(), "--public-url") {
		t.Errorf("--public-url on stdio should be refused, got %v", err)
	}
}

func TestParsePublicURL(t *testing.T) {
	for _, bad := range []string{
		"/gdocs",
		"mcp.example.com/gdocs",
		"ftp://mcp.example.com/gdocs",
		"https://mcp.example.com/gdocs?key=x",
		"https://mcp.example.com/gdocs#top",
		"https://user:pass@mcp.example.com/gdocs",
	} {
		if _, err := ParsePublicURL(bad); err == nil {
			t.Errorf("ParsePublicURL(%q) should be refused", bad)
		}
	}

	for raw, want := range map[string]string{
		"":                               "",
		"https://mcp.example.com/gdocs":  "https://mcp.example.com/gdocs",
		"https://mcp.example.com/gdocs/": "https://mcp.example.com/gdocs",
		"http://127.0.0.1:8080":          "http://127.0.0.1:8080",
		// A host of its own, with no path: the pages are then <host>/login and
		// <host>/callback.
		"https://gdocs.example.com":  "https://gdocs.example.com",
		"https://gdocs.example.com/": "https://gdocs.example.com",
	} {
		got, err := ParsePublicURL(raw)
		if err != nil || got != want {
			t.Errorf("ParsePublicURL(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

// publicFlags are a server behind a gateway, the only setup the Access check applies to.
func publicFlags() Flags {
	flags := validFlags()
	flags.Transport = TransportHTTP
	flags.Address = "127.0.0.1:8819"
	flags.PublicURL = "https://gdocs.example.com"
	return flags
}

// Without the Access flags nothing changes: no check, and the configuration is what it was.
func TestAccessIsOptIn(t *testing.T) {
	for _, flags := range []Flags{validFlags(), publicFlags()} {
		cfg, err := Load(flags, env(map[string]string{EnvAuthToken: "t"}))
		if err != nil {
			t.Fatalf("no Access flags should start as before: %v", err)
		}
		if cfg.AccessTeamDomain != "" || cfg.AccessAUD != "" || cfg.AccessEmails != nil {
			t.Errorf("no flags means no check, got %q %q %v", cfg.AccessTeamDomain, cfg.AccessAUD, cfg.AccessEmails)
		}
	}
}

func TestAccessFlags(t *testing.T) {
	flags := publicFlags()
	flags.AccessTeamDomain = " https://team.cloudflareaccess.com/ "
	flags.AccessAUD = " aud-tag "
	flags.AccessEmail = "owner@example.com, ,Second@example.com,owner@EXAMPLE.com"

	cfg, err := Load(flags, env(map[string]string{EnvAuthToken: "t"}))
	if err != nil {
		t.Fatalf("all three flags with a public address should start: %v", err)
	}
	if cfg.AccessTeamDomain != "team.cloudflareaccess.com" {
		t.Errorf("the team domain should come back as a bare host, got %q", cfg.AccessTeamDomain)
	}
	if cfg.AccessAUD != "aud-tag" {
		t.Errorf("the AUD tag came back as %q", cfg.AccessAUD)
	}
	if strings.Join(cfg.AccessEmails, ",") != "owner@example.com,Second@example.com" {
		t.Errorf("the e-mails should lose blanks and repeats, got %v", cfg.AccessEmails)
	}
}

// Part of the check is refused rather than ignored: a server started with half of it would
// look locked and be open.
func TestAccessFlagsAreAllOrNothing(t *testing.T) {
	const (
		team  = "team.cloudflareaccess.com"
		aud   = "aud-tag"
		email = "owner@example.com"
	)

	for _, partial := range [][3]string{
		{team, "", ""},
		{"", aud, ""},
		{"", "", email},
		{team, aud, ""},
		{team, "", email},
		{"", aud, email},
		{team, aud, " , "},
	} {
		flags := publicFlags()
		flags.AccessTeamDomain, flags.AccessAUD, flags.AccessEmail = partial[0], partial[1], partial[2]

		_, err := Load(flags, env(map[string]string{EnvAuthToken: "t"}))
		if err == nil || !strings.Contains(err.Error(), "all three or none") {
			t.Errorf("%q: a partial Access check should be refused, got %v", partial, err)
		}
	}
}

func TestAccessNeedsThePublicRoute(t *testing.T) {
	loopback := publicFlags()
	loopback.PublicURL = ""
	stdio := validFlags()

	for name, flags := range map[string]Flags{"loopback HTTP": loopback, "stdio": stdio} {
		flags.AccessTeamDomain = "team.cloudflareaccess.com"
		flags.AccessAUD = "aud-tag"
		flags.AccessEmail = "owner@example.com"

		_, err := Load(flags, env(map[string]string{EnvAuthToken: "t"}))
		if err == nil || !strings.Contains(err.Error(), "--public-url") {
			t.Errorf("%s: the Access check without a public address should be refused, got %v", name, err)
		}
	}
}

func TestAccessRefusesBadValues(t *testing.T) {
	for _, test := range []struct{ team, email, want string }{
		{"https://team.cloudflareaccess.com/path", "owner@example.com", "--access-team-domain"},
		{"http://team.cloudflareaccess.com", "owner@example.com", "--access-team-domain"},
		{"team.cloudflareaccess.com?x=1", "owner@example.com", "--access-team-domain"},
		{"team.cloudflareaccess.com", "owner", "--access-email"},
	} {
		flags := publicFlags()
		flags.AccessTeamDomain = test.team
		flags.AccessAUD = "aud-tag"
		flags.AccessEmail = test.email

		_, err := Load(flags, env(map[string]string{EnvAuthToken: "t"}))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%q / %q: want a refusal naming %s, got %v", test.team, test.email, test.want, err)
		}
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
