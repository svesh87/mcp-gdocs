// Package auth signs the server in to Google as one person and keeps that
// authorisation alive.
//
// Two things live in different places on purpose. The OAuth client — the file Google
// Cloud Console hands out — identifies the application and is shared by whoever is
// allowed to run this server. The token is what one person's consent turned into, it
// lives in that person's own directory, and it never travels with the image.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/oauth2"
)

// Google's endpoints, used when the client file names none.
const (
	DefaultAuthURI  = "https://accounts.google.com/o/oauth2/auth"
	DefaultTokenURI = "https://oauth2.googleapis.com/token"
)

// Credentials is the OAuth client of the application: who is asking for consent.
type Credentials struct {
	ClientID     string
	ClientSecret string
	AuthURI      string
	TokenURI     string
}

// clientFile is the shape Google Cloud Console downloads. A desktop client lands under
// "installed"; "web" is accepted too, because a person who picked the wrong client type
// deserves a clear error later rather than a parse failure now.
type clientFile struct {
	Installed *clientSection `json:"installed"`
	Web       *clientSection `json:"web"`
}

type clientSection struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AuthURI      string `json:"auth_uri"`
	TokenURI     string `json:"token_uri"`
}

// LoadCredentials reads the OAuth client file.
//
// Errors never quote the file's contents: the client secret is in there, and an error
// message ends up in logs.
func LoadCredentials(path string) (Credentials, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is the operator's own flag
	if err != nil {
		return Credentials{}, fmt.Errorf("reading the OAuth client file: %w", err)
	}

	var file clientFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return Credentials{}, fmt.Errorf("the OAuth client file %s is not valid JSON", path)
	}

	section := file.Installed
	if section == nil {
		section = file.Web
	}
	if section == nil {
		return Credentials{}, fmt.Errorf("the OAuth client file %s has neither an \"installed\" nor a \"web\" section: "+
			"download it again from Google Cloud Console as a client of type Desktop", path)
	}

	creds := Credentials{
		ClientID:     strings.TrimSpace(section.ClientID),
		ClientSecret: strings.TrimSpace(section.ClientSecret),
		AuthURI:      strings.TrimSpace(section.AuthURI),
		TokenURI:     strings.TrimSpace(section.TokenURI),
	}

	if creds.ClientID == "" || creds.ClientSecret == "" {
		return Credentials{}, fmt.Errorf("the OAuth client file %s is missing client_id or client_secret", path)
	}

	if creds.AuthURI == "" {
		creds.AuthURI = DefaultAuthURI
	}
	if creds.TokenURI == "" {
		creds.TokenURI = DefaultTokenURI
	}

	return creds, nil
}

// OAuthConfig builds the OAuth configuration for one redirect address.
//
// The redirect is decided per sign-in rather than taken from the client file: a desktop
// client accepts any loopback address, and the port depends on where the browser is —
// on this machine or on the host of a container.
func (c Credentials) OAuthConfig(scopes []string, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Scopes:       append([]string(nil), scopes...),
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  c.AuthURI,
			TokenURL: c.TokenURI,
		},
	}
}

// ErrNoToken says nobody has signed in yet. It is a separate error because the answer to
// it is an action by a person, not a retry.
var ErrNoToken = errors.New("no Google token yet: run `mcp-gdocs login` on a machine with a browser, " +
	"or open /login?key=<token> on a server started with --transport=streamable-http")
