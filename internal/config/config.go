// Package config turns the command line and the environment into a validated
// configuration, and refuses to start on anything ambiguous.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Transport names accepted by the --transport flag.
const (
	TransportStdio = "stdio"
	TransportHTTP  = "streamable-http"
)

// Environment variables. Secrets live here rather than in flags, so they do not show up
// in a process list, and so they can come out of a password store instead of a file on
// disk.
const (
	// EnvAuthToken carries the bearer token the HTTP transport demands.
	EnvAuthToken = "MCP_AUTH_TOKEN"

	// EnvClientID and EnvClientSecret are the OAuth client, as an alternative to the
	// file Google Cloud Console downloads. The two halves are the whole of what that
	// file carries that matters.
	EnvClientID     = "GOOGLE_OAUTH_CLIENT_ID"
	EnvClientSecret = "GOOGLE_OAUTH_CLIENT_SECRET"

	// EnvTools carries the set of tool groups, as an alternative to --tools: in a compose
	// file the set is written in the environment, not in a command line.
	EnvTools = "GDOCS_TOOLS"
)

// Scope aliases. The full URLs are what Google wants; the short names are what a person
// types into --scopes.
const (
	ScopeDrive         = "https://www.googleapis.com/auth/drive"
	ScopeDriveFile     = "https://www.googleapis.com/auth/drive.file"
	ScopeDriveReadonly = "https://www.googleapis.com/auth/drive.readonly"
	ScopeSpreadsheets  = "https://www.googleapis.com/auth/spreadsheets"
	ScopePresentations = "https://www.googleapis.com/auth/presentations"
	ScopeDocuments     = "https://www.googleapis.com/auth/documents"
)

// scopeAliases maps what a person types to what Google is asked for.
var scopeAliases = map[string]string{
	"drive":          ScopeDrive,
	"drive.file":     ScopeDriveFile,
	"drive.readonly": ScopeDriveReadonly,
	"spreadsheets":   ScopeSpreadsheets,
	"presentations":  ScopePresentations,
	"documents":      ScopeDocuments,
}

// DefaultScopes is what the server asks for unless told otherwise.
//
// The full drive scope rather than drive.file: a deck is made by copying a template, and
// drive.file only ever sees files this application created itself, so it cannot open the
// template at all.
var DefaultScopes = []string{ScopeDrive, ScopeSpreadsheets, ScopePresentations, ScopeDocuments}

// Config is the validated configuration of one server process.
type Config struct {
	Transport string
	Address   string
	AuthToken string
	// Credentials is the path to the OAuth client file, when the client comes from a
	// file. Empty when it comes from the environment.
	Credentials string
	// ClientID and ClientSecret are the OAuth client when it comes from the environment.
	// Empty when it comes from a file.
	ClientID     string
	ClientSecret string
	TokenDir     string
	Scopes       []string
	AllowWrite   bool
	// FilesDir is the one directory this server may read files from and write them to.
	// Empty means it may not touch the filesystem at all beyond its token, which is the
	// default: a server that can write anywhere is a server that can overwrite anything
	// the account it runs as owns.
	FilesDir string
	// Tools is the value of --tools, as written: the groups of tools this server offers.
	// Empty means the default set. It is kept as text here and turned into groups by the
	// tools package, which owns the names.
	Tools string
}

// Flags are the command line values Load needs. Kept apart from parsing so a test can
// build a configuration without touching the global flag set.
type Flags struct {
	Transport   string
	Address     string
	Credentials string
	TokenDir    string
	Scopes      string
	AllowWrite  bool
	FilesDir    string
	Tools       string
}

// Env reads a variable. Tests substitute a map instead of the process environment.
type Env func(key string) string

// firstNonEmpty is the flag-beats-environment rule, in one place.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

// OSEnv reads the process environment.
func OSEnv(key string) string { return os.Getenv(key) }

// Load validates flags and environment together. Every failure comes back as an error
// rather than a log line, so the caller decides how loudly to die.
func Load(flags Flags, env Env) (*Config, error) {
	scopes, err := ParseScopes(flags.Scopes)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Transport:    flags.Transport,
		Address:      flags.Address,
		AuthToken:    env(EnvAuthToken),
		Credentials:  strings.TrimSpace(flags.Credentials),
		ClientID:     strings.TrimSpace(env(EnvClientID)),
		ClientSecret: strings.TrimSpace(env(EnvClientSecret)),
		TokenDir:     strings.TrimSpace(flags.TokenDir),
		Scopes:       scopes,
		AllowWrite:   flags.AllowWrite,
		FilesDir:     strings.TrimSpace(flags.FilesDir),
		// The flag wins over the environment, and the environment exists because in a
		// compose file the set is written there rather than in a command line.
		Tools: firstNonEmpty(strings.TrimSpace(flags.Tools), strings.TrimSpace(env(EnvTools))),
	}

	if cfg.FilesDir != "" {
		cfg.FilesDir = filepath.Clean(cfg.FilesDir)
	}

	if err := loadClient(cfg); err != nil {
		return nil, err
	}

	// The token is what a person's consent turns into, and it has to outlive the
	// container it was created in. A server that keeps it in the image would ask for
	// consent again on every restart, and an image with a token baked in would carry
	// someone's Google access to whoever pulls it.
	if cfg.TokenDir == "" {
		return nil, errors.New("--token-dir is required: the OAuth token has to outlive the process")
	}

	cfg.TokenDir = filepath.Clean(cfg.TokenDir)

	if err := validateTransport(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadClient decides where the OAuth client comes from.
//
// Two ways in, because two habits are legitimate: the file Google Cloud Console
// downloads, mounted read-only; or the two halves as environment variables, which is what
// a password store hands over — then nothing about the application is on disk at all.
//
// The environment wins when both are present. A file left behind from an earlier setup
// should not quietly outrank what the operator just injected.
func loadClient(cfg *Config) error {
	switch {
	case cfg.ClientID != "" && cfg.ClientSecret != "":
		cfg.Credentials = ""
		return nil

	case cfg.ClientID != "" || cfg.ClientSecret != "":
		// Half a client is always a mistake, and the failure it causes at Google is
		// unrecognisable — an invalid_client with nothing to say which half is missing.
		missing := EnvClientSecret
		if cfg.ClientID == "" {
			missing = EnvClientID
		}
		return fmt.Errorf("%s is set but %s is not: the OAuth client is both halves or neither",
			present(cfg), missing)

	case cfg.Credentials != "":
		cfg.Credentials = filepath.Clean(cfg.Credentials)
		return nil

	default:
		return fmt.Errorf("the OAuth client is missing: either pass --credentials with the file "+
			"downloaded from Google Cloud Console, or set %s and %s", EnvClientID, EnvClientSecret)
	}
}

// present names the half of the client that was given.
func present(cfg *Config) string {
	if cfg.ClientID != "" {
		return EnvClientID
	}
	return EnvClientSecret
}

// ParseScopes accepts short aliases and full URLs, comma separated. An empty list means
// DefaultScopes: narrowing is a deliberate act, not something that happens by omission.
func ParseScopes(raw string) ([]string, error) {
	fields := strings.Split(raw, ",")
	seen := map[string]bool{}
	var scopes []string

	for _, field := range fields {
		name := strings.TrimSpace(field)
		switch {
		case name == "":
			continue
		case scopeAliases[name] != "":
			name = scopeAliases[name]
		case strings.HasPrefix(name, "https://"):
			// A full URL passes through: Google gains scopes faster than this list does.
		default:
			return nil, fmt.Errorf("unknown scope %q: use a full https:// URL or one of %s",
				name, strings.Join(knownAliases(), ", "))
		}

		if !seen[name] {
			seen[name] = true
			scopes = append(scopes, name)
		}
	}

	if len(scopes) == 0 {
		return append([]string(nil), DefaultScopes...), nil
	}

	return scopes, nil
}

// knownAliases lists the aliases in the order a person cares about them, which a map
// iteration would not do.
func knownAliases() []string {
	return []string{"drive", "drive.file", "drive.readonly", "spreadsheets", "presentations", "documents"}
}

func validateTransport(cfg *Config) error {
	switch cfg.Transport {
	case TransportStdio:
		return nil
	case TransportHTTP:
		// Whatever reaches this port acts as the person who signed in, and on loopback
		// that is any process running as the same account. Refusing to start beats
		// serving quietly.
		if cfg.AuthToken == "" {
			return fmt.Errorf("%s is required with --transport=%s: refusing to serve without authentication",
				EnvAuthToken, TransportHTTP)
		}
		if cfg.Address == "" {
			return errors.New("--address is required with --transport=" + TransportHTTP)
		}
		return nil
	default:
		return fmt.Errorf("unknown transport %q: use %s or %s", cfg.Transport, TransportStdio, TransportHTTP)
	}
}

// TokenPath is where the OAuth token of the signed-in account lives.
func (c *Config) TokenPath() string { return filepath.Join(c.TokenDir, "token.json") }
