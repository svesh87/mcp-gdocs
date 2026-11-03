// Package access checks the token Cloudflare Access puts on every request it lets through,
// so that the sign-in pages answer one person's browser and nothing else.
//
// It is a second lock, not the first. Access already decides who reaches the pages; this
// package makes the server refuse whatever Access let through by mistake. The case it
// exists for is a policy that grows wider than meant: the MCP endpoints are reached by
// agents with service tokens, and a service token that one day also opens the sign-in
// would let an agent start a consent screen in the owner's name. A service token's JWT
// carries a common_name and no e-mail, so a page that demands an allowed e-mail refuses it
// whatever the policies say.
//
// Only the standard library is used: the token is RS256 over a JSON body, and the keys are
// a JSON document at a fixed address of the team.
package access

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Header is where Cloudflare Access puts the token on a request it forwards.
const Header = "Cf-Access-Jwt-Assertion"

// certsPath is where a team publishes the keys its tokens are signed with.
const certsPath = "/cdn-cgi/access/certs"

const (
	// leeway absorbs the clock difference between Cloudflare and this host.
	leeway = 30 * time.Second
	// refetchInterval bounds how often a token with an unknown key can make this server
	// ask Cloudflare for the keys again: anybody can invent a kid, and a request with one
	// must not turn into a request to Cloudflare every time.
	refetchInterval = time.Minute
	// keysMaxAge is how long a key set is trusted without asking again, so a key
	// Cloudflare withdrew stops opening the pages within the hour.
	keysMaxAge = time.Hour
	// maxCertsSize bounds the key document; the real one is a few kilobytes.
	maxCertsSize = 1 << 20
)

// Verifier checks Access tokens for one application of one team.
type Verifier struct {
	issuer   string
	certsURL string
	aud      string
	emails   map[string]bool

	// client fetches the keys; logger receives the reason of every refusal, never the
	// token; now is the clock. Tests replace all three.
	client *http.Client
	logger *log.Logger
	now    func() time.Time

	mu sync.Mutex
	// keys is the last key set fetched, by kid. fetched is when it was fetched, and
	// attempted when a fetch was last tried, successful or not.
	keys      map[string]*rsa.PublicKey
	fetched   time.Time
	attempted time.Time
}

// New builds a verifier for the team at teamDomain (e.g. example.cloudflareaccess.com),
// the application with the AUD tag aud, and the e-mails allowed through.
func New(teamDomain, aud string, emails []string) (*Verifier, error) {
	teamDomain = strings.TrimSpace(teamDomain)
	aud = strings.TrimSpace(aud)
	if teamDomain == "" || aud == "" {
		return nil, errors.New("the Access check needs a team domain and an application AUD tag")
	}

	allowed := map[string]bool{}
	for _, email := range emails {
		if email = normalizeEmail(email); email != "" {
			allowed[email] = true
		}
	}
	if len(allowed) == 0 {
		return nil, errors.New("the Access check needs at least one allowed e-mail")
	}

	issuer := "https://" + teamDomain

	return &Verifier{
		issuer:   issuer,
		certsURL: issuer + certsPath,
		aud:      aud,
		emails:   allowed,
		client:   &http.Client{Timeout: 10 * time.Second},
		logger:   log.New(os.Stderr, "mcp-gdocs: ", log.LstdFlags),
		now:      time.Now,
	}, nil
}

// Require refuses every request whose Access token does not verify, and hands the rest to
// next.
//
// The answer to a refusal is a bare 403 that says nothing about why: the reason goes to
// the log, where the operator reads it, and not to whoever is probing.
func (v *Verifier) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := v.Verify(r.Context(), r.Header.Get(Header)); err != nil {
			v.logger.Printf("access: refused %s %s: %v", r.Method, r.URL.Path, err)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// claims are the parts of an Access token this check reads.
type claims struct {
	Audience  audience `json:"aud"`
	Issuer    string   `json:"iss"`
	Expires   *float64 `json:"exp"`
	NotBefore *float64 `json:"nbf"`
	Email     string   `json:"email"`
	// CommonName is what a service token's JWT carries instead of an e-mail. It is read
	// only to say so in the log.
	CommonName string `json:"common_name"`
}

// audience is the aud claim, which the JWT spec allows as one string or a list, and which
// Access sends as a list.
type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*a = audience{one}
		return nil
	}

	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return errors.New("aud is neither a string nor a list of strings")
	}
	*a = many

	return nil
}

func (a audience) contains(want string) bool {
	for _, value := range a {
		if value == want {
			return true
		}
	}
	return false
}

// Verify checks a token and returns the e-mail it was issued to.
//
// Errors name what failed and never quote the token: they end up in the log.
func (v *Verifier) Verify(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", errors.New("no " + Header + " header: the request did not come through Cloudflare Access")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("the token is not a JWT")
	}

	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &header); err != nil {
		return "", fmt.Errorf("the token header: %w", err)
	}
	// The algorithm is fixed here rather than taken from the token: a token that names
	// its own algorithm is how "none" and HMAC-with-the-public-key get in.
	if header.Algorithm != "RS256" {
		return "", fmt.Errorf("the token is signed with %q, not RS256", header.Algorithm)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errors.New("the token signature is not base64url")
	}

	key, err := v.key(ctx, header.KeyID)
	if err != nil {
		return "", err
	}

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return "", errors.New("the token signature does not verify")
	}

	var body claims
	if err := decodeSegment(parts[1], &body); err != nil {
		return "", fmt.Errorf("the token claims: %w", err)
	}

	return v.check(body)
}

// check applies the claims rules to a token whose signature has verified.
func (v *Verifier) check(body claims) (string, error) {
	if body.Issuer != v.issuer {
		return "", fmt.Errorf("the token was issued by %q, not %q", body.Issuer, v.issuer)
	}
	if !body.Audience.contains(v.aud) {
		return "", errors.New("the token is for another Access application")
	}

	now := v.now()
	if body.Expires == nil {
		return "", errors.New("the token has no expiry")
	}
	if now.After(unixTime(*body.Expires).Add(leeway)) {
		return "", errors.New("the token has expired")
	}
	if body.NotBefore != nil && now.Add(leeway).Before(unixTime(*body.NotBefore)) {
		return "", errors.New("the token is not valid yet")
	}

	if body.Email == "" {
		if body.CommonName != "" {
			return "", errors.New("the token belongs to a service token, not a person")
		}
		return "", errors.New("the token carries no e-mail")
	}
	if !v.emails[normalizeEmail(body.Email)] {
		return "", fmt.Errorf("%s is not among the allowed e-mails", body.Email)
	}

	return body.Email, nil
}

// key finds the public key a token names, asking Cloudflare again when the key is unknown
// or the set is old — but not more often than refetchInterval.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, errors.New("the token names no key")
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	now := v.now()
	key, known := v.keys[kid]
	stale := now.Sub(v.fetched) >= keysMaxAge
	if known && !stale {
		return key, nil
	}

	if !v.attempted.IsZero() && now.Sub(v.attempted) < refetchInterval {
		if known {
			return key, nil
		}
		return nil, fmt.Errorf("the token is signed with an unknown key %q", kid)
	}

	v.attempted = now
	keys, err := v.fetch(ctx)
	if err != nil {
		// A set that is merely old still beats no set: Cloudflare being unreachable for a
		// minute should not lock the owner out of a page they are already on.
		if known {
			v.logger.Printf("access: refreshing the keys: %v; keeping the ones fetched before", err)
			return key, nil
		}
		return nil, fmt.Errorf("fetching the Access keys: %w", err)
	}

	v.keys = keys
	v.fetched = now

	if key, ok := keys[kid]; ok {
		return key, nil
	}

	return nil, fmt.Errorf("the token is signed with an unknown key %q", kid)
}

// fetch reads the team's key set.
func (v *Verifier) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return nil, err
	}

	response, err := v.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", v.certsURL, response.Status)
	}

	var document struct {
		Keys []struct {
			KeyType string `json:"kty"`
			KeyID   string `json:"kid"`
			Modulus string `json:"n"`
			Exp     string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxCertsSize)).Decode(&document); err != nil {
		return nil, fmt.Errorf("%s is not a key set: %w", v.certsURL, err)
	}

	keys := map[string]*rsa.PublicKey{}
	for _, entry := range document.Keys {
		if entry.KeyType != "RSA" || entry.KeyID == "" {
			continue
		}
		key, err := rsaKey(entry.Modulus, entry.Exp)
		if err != nil {
			return nil, fmt.Errorf("key %q of %s: %w", entry.KeyID, v.certsURL, err)
		}
		keys[entry.KeyID] = key
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%s holds no RSA keys", v.certsURL)
	}

	return keys, nil
}

// rsaKey builds a public key out of the base64url modulus and exponent of a JWK.
func rsaKey(modulus, exponent string) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil || len(n) == 0 {
		return nil, errors.New("the modulus is not base64url")
	}
	e, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil || len(e) == 0 || len(e) > 4 {
		return nil, errors.New("the exponent is not a small base64url number")
	}

	exp := 0
	for _, b := range e {
		exp = exp<<8 | int(b)
	}

	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}, nil
}

// decodeSegment reads one base64url JSON part of a token.
func decodeSegment(segment string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return errors.New("not base64url")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return errors.New("not JSON of the expected shape")
	}
	return nil
}

func unixTime(seconds float64) time.Time {
	return time.Unix(int64(seconds), 0)
}

// normalizeEmail is the form e-mails are compared in: the domain part is case-insensitive
// everywhere, and no mail system anyone signs in to Access with treats the local part
// otherwise.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
