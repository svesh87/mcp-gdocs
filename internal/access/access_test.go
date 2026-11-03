package access

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testAUD   = "aud-tag-of-the-pages"
	testEmail = "owner@example.com"
)

// team is a Cloudflare Access team as this package sees it: a key set at
// /cdn-cgi/access/certs on an https address, and tokens signed with its keys.
type team struct {
	t       *testing.T
	server  *httptest.Server
	domain  string
	keys    map[string]*rsa.PrivateKey
	served  atomic.Value // []string: the kids the key set currently lists
	fetches atomic.Int32
}

func newTeam(t *testing.T, kids ...string) *team {
	t.Helper()

	tm := &team{t: t, keys: map[string]*rsa.PrivateKey{}}
	for _, kid := range kids {
		tm.addKey(kid)
	}
	tm.served.Store(kids)

	tm.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != certsPath {
			http.NotFound(w, r)
			return
		}
		tm.fetches.Add(1)

		type jwk struct {
			KeyType string `json:"kty"`
			KeyID   string `json:"kid"`
			Alg     string `json:"alg"`
			N       string `json:"n"`
			E       string `json:"e"`
		}
		var document struct {
			Keys       []jwk  `json:"keys"`
			PublicCert string `json:"public_cert"`
		}
		for _, kid := range tm.served.Load().([]string) {
			public := tm.keys[kid].PublicKey
			document.Keys = append(document.Keys, jwk{
				KeyType: "RSA", KeyID: kid, Alg: "RS256",
				N: base64.RawURLEncoding.EncodeToString(public.N.Bytes()),
				E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(public.E)).Bytes()),
			})
		}
		// Cloudflare lists an EC key too in some teams; it must be skipped, not fail.
		document.Keys = append(document.Keys, jwk{KeyType: "EC", KeyID: "ec-key"})
		document.PublicCert = "ignored"

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(document)
	}))
	t.Cleanup(tm.server.Close)

	tm.domain = strings.TrimPrefix(tm.server.URL, "https://")

	return tm
}

func (tm *team) addKey(kid string) {
	tm.t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tm.t.Fatalf("generating a key: %v", err)
	}
	tm.keys[kid] = key
}

func (tm *team) issuer() string { return "https://" + tm.domain }

// sign makes a token with the given claims, signed by the key kid.
func (tm *team) sign(kid string, body map[string]any) string {
	tm.t.Helper()

	return signWith(tm.t, tm.keys[kid], map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"}, body)
}

func signWith(t *testing.T, key *rsa.PrivateKey, header, body map[string]any) string {
	t.Helper()

	encode := func(value map[string]any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("encoding: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}

	signed := encode(header) + "." + encode(body)
	digest := sha256.Sum256([]byte(signed))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	return signed + "." + base64.RawURLEncoding.EncodeToString(signature)
}

// personClaims are what Access puts in the token of a person who signed in.
func (tm *team) personClaims(now time.Time) map[string]any {
	return map[string]any{
		"aud":   []string{testAUD},
		"iss":   tm.issuer(),
		"email": testEmail,
		"sub":   "person-id",
		"type":  "app",
		"iat":   now.Unix(),
		"nbf":   now.Unix(),
		"exp":   now.Add(24 * time.Hour).Unix(),
	}
}

// verifier builds a verifier pointed at the team, with a clock the test moves and a log
// the test reads.
func (tm *team) verifier(now *time.Time) (*Verifier, *bytes.Buffer) {
	tm.t.Helper()

	v, err := New(tm.domain, testAUD, []string{" Owner@Example.com ", "", "second@example.com"})
	if err != nil {
		tm.t.Fatalf("building the verifier: %v", err)
	}

	logs := &bytes.Buffer{}
	v.client = tm.server.Client()
	v.logger = log.New(logs, "", 0)
	v.now = func() time.Time { return *now }

	return v, logs
}

func TestVerify(t *testing.T) {
	tm := newTeam(t, "k1")
	now := time.Unix(1_800_000_000, 0)

	for _, test := range []struct {
		name   string
		change func(map[string]any)
		want   string // a fragment of the error; empty means the token passes
	}{
		{"a person who is allowed", func(map[string]any) {}, ""},
		{"aud as a single string", func(c map[string]any) { c["aud"] = testAUD }, ""},
		{"aud among several", func(c map[string]any) { c["aud"] = []string{"other", testAUD} }, ""},
		{"the e-mail in another case", func(c map[string]any) { c["email"] = "OWNER@example.COM" }, ""},
		{"the second allowed e-mail", func(c map[string]any) { c["email"] = "second@example.com" }, ""},
		{"no nbf at all", func(c map[string]any) { delete(c, "nbf") }, ""},
		{"expired within the leeway", func(c map[string]any) { c["exp"] = now.Add(-10 * time.Second).Unix() }, ""},

		{"another application", func(c map[string]any) { c["aud"] = []string{"someone-else"} }, "another Access application"},
		{"aud of the wrong type", func(c map[string]any) { c["aud"] = 42 }, "claims"},
		{"another team", func(c map[string]any) { c["iss"] = "https://evil.cloudflareaccess.com" }, "issued by"},
		{"expired", func(c map[string]any) { c["exp"] = now.Add(-time.Hour).Unix() }, "expired"},
		{"no expiry", func(c map[string]any) { delete(c, "exp") }, "no expiry"},
		{"not valid yet", func(c map[string]any) { c["nbf"] = now.Add(time.Hour).Unix() }, "not valid yet"},
		{"a service token", func(c map[string]any) {
			delete(c, "email")
			c["common_name"] = "agent.access"
			c["sub"] = ""
		}, "service token"},
		{"no e-mail and nothing else", func(c map[string]any) { delete(c, "email") }, "no e-mail"},
		{"an e-mail not allowed", func(c map[string]any) { c["email"] = "stranger@example.com" }, "not among the allowed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			v, _ := tm.verifier(&now)

			body := tm.personClaims(now)
			test.change(body)

			email, err := v.Verify(context.Background(), tm.sign("k1", body))
			switch {
			case test.want == "" && err != nil:
				t.Fatalf("the token should pass, got %v", err)
			case test.want == "" && !strings.EqualFold(email, testEmail) && email != "second@example.com":
				t.Errorf("the e-mail came back as %q", email)
			case test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)):
				t.Errorf("want an error with %q, got %v", test.want, err)
			}
		})
	}
}

func TestVerifyRefusesMalformedTokens(t *testing.T) {
	tm := newTeam(t, "k1")
	now := time.Unix(1_800_000_000, 0)
	v, _ := tm.verifier(&now)

	good := tm.sign("k1", tm.personClaims(now))
	parts := strings.Split(good, ".")

	// A token signed by a key the team never had, but naming the team's kid.
	stranger, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	forged := signWith(t, stranger, map[string]any{"alg": "RS256", "kid": "k1"}, tm.personClaims(now))

	// The body changed after signing.
	otherBody := tm.personClaims(now)
	otherBody["email"] = "second@example.com"
	tampered := parts[0] + "." + strings.Split(tm.sign("k1", otherBody), ".")[1] + "." + parts[2]

	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"k1"}`))
	hmac := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","kid":"k1"}`))
	noKid := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))

	for name, token := range map[string]string{
		"missing":             "",
		"not a JWT":           "abc",
		"header not base64":   "!!!." + parts[1] + "." + parts[2],
		"header not JSON":     base64.RawURLEncoding.EncodeToString([]byte("nope")) + "." + parts[1] + "." + parts[2],
		"alg none":            none + "." + parts[1] + ".",
		"alg HS256":           hmac + "." + parts[1] + "." + parts[2],
		"no kid":              noKid + "." + parts[1] + "." + parts[2],
		"signature not b64":   parts[0] + "." + parts[1] + ".!!!",
		"bad signature":       forged,
		"tampered body":       tampered,
		"body not JSON":       tm.signRaw("k1", "nope"),
		"empty signature":     parts[0] + "." + parts[1] + ".",
		"body not base64":     parts[0] + ".!!!." + parts[2],
		"four parts":          good + ".x",
		"signature truncated": parts[0] + "." + parts[1] + "." + parts[2][:20],
	} {
		if _, err := v.Verify(context.Background(), token); err == nil {
			t.Errorf("%s: the token should be refused", name)
		} else if token != "" && strings.Contains(err.Error(), token) {
			t.Errorf("%s: the error quotes the token: %v", name, err)
		}
	}
}

// signRaw signs a body that is not a JSON object.
func (tm *team) signRaw(kid, body string) string {
	tm.t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"` + kid + `"}`))
	signed := header + "." + base64.RawURLEncoding.EncodeToString([]byte(body))
	digest := sha256.Sum256([]byte(signed))
	signature, err := rsa.SignPKCS1v15(rand.Reader, tm.keys[kid], crypto.SHA256, digest[:])
	if err != nil {
		tm.t.Fatalf("signing: %v", err)
	}

	return signed + "." + base64.RawURLEncoding.EncodeToString(signature)
}

// Cloudflare rotates its keys: a token signed by a key the cached set does not have makes
// the verifier ask again — once, and not again until the interval has passed.
func TestUnknownKeyIsFetchedOnceThenRateLimited(t *testing.T) {
	tm := newTeam(t, "old")
	now := time.Unix(1_800_000_000, 0)
	v, _ := tm.verifier(&now)

	if _, err := v.Verify(context.Background(), tm.sign("old", tm.personClaims(now))); err != nil {
		t.Fatalf("a token with the current key should pass: %v", err)
	}
	if got := tm.fetches.Load(); got != 1 {
		t.Fatalf("the first token fetches the keys once, got %d fetches", got)
	}

	// Cached: the next token costs no fetch.
	if _, err := v.Verify(context.Background(), tm.sign("old", tm.personClaims(now))); err != nil {
		t.Fatalf("a second token should pass from the cache: %v", err)
	}
	if got := tm.fetches.Load(); got != 1 {
		t.Fatalf("a cached key costs no fetch, got %d fetches", got)
	}

	// The team rotates, and the next minute a token with the new key arrives.
	tm.addKey("new")
	tm.served.Store([]string{"old", "new"})
	now = now.Add(2 * time.Minute)

	if _, err := v.Verify(context.Background(), tm.sign("new", tm.personClaims(now))); err != nil {
		t.Fatalf("a token with a rotated-in key should pass after one refetch: %v", err)
	}
	if got := tm.fetches.Load(); got != 2 {
		t.Fatalf("an unknown kid refetches once, got %d fetches", got)
	}

	// An invented kid right after: refused, and Cloudflare is not asked again however many
	// times it is tried within the interval.
	tm.addKey("invented")
	for range 5 {
		_, err := v.Verify(context.Background(), tm.sign("invented", tm.personClaims(now)))
		if err == nil || !strings.Contains(err.Error(), "unknown key") {
			t.Fatalf("a kid the team does not publish should be refused, got %v", err)
		}
	}
	if got := tm.fetches.Load(); got != 2 {
		t.Fatalf("no fetch within the interval, got %d fetches", got)
	}

	// Once the interval has passed, one fetch is allowed again, and it does not help.
	now = now.Add(refetchInterval)
	for range 3 {
		if _, err := v.Verify(context.Background(), tm.sign("invented", tm.personClaims(now))); err == nil {
			t.Fatal("a kid the team does not publish stays refused after a refetch")
		}
	}
	if got := tm.fetches.Load(); got != 3 {
		t.Fatalf("after the interval the keys are fetched once more, got %d fetches", got)
	}

	// The keys already known keep working while the unknown kid is rate limited.
	if _, err := v.Verify(context.Background(), tm.sign("old", tm.personClaims(now))); err != nil {
		t.Fatalf("a known key is unaffected by the rate limit: %v", err)
	}
}

// A key set older than keysMaxAge is fetched again, so a key Cloudflare withdrew stops
// working; and when Cloudflare cannot be reached the old set keeps serving.
func TestKeySetExpires(t *testing.T) {
	tm := newTeam(t, "k1", "k2")
	now := time.Unix(1_800_000_000, 0)
	v, logs := tm.verifier(&now)

	if _, err := v.Verify(context.Background(), tm.sign("k1", tm.personClaims(now))); err != nil {
		t.Fatalf("the first token should pass: %v", err)
	}

	// k1 is withdrawn; within the hour it still opens the pages.
	tm.served.Store([]string{"k2"})
	now = now.Add(30 * time.Minute)
	if _, err := v.Verify(context.Background(), tm.sign("k1", tm.personClaims(now))); err != nil {
		t.Fatalf("a cached key is trusted within its age: %v", err)
	}

	// After the hour the set is fetched again, and k1 is gone from it.
	now = now.Add(keysMaxAge)
	if _, err := v.Verify(context.Background(), tm.sign("k1", tm.personClaims(now))); err == nil {
		t.Fatal("a withdrawn key should stop verifying once the set is refetched")
	}
	if _, err := v.Verify(context.Background(), tm.sign("k2", tm.personClaims(now))); err != nil {
		t.Fatalf("a key still published keeps verifying: %v", err)
	}

	// Cloudflare unreachable when the set is old again: the old set keeps serving.
	tm.server.Close()
	now = now.Add(keysMaxAge)
	if _, err := v.Verify(context.Background(), tm.sign("k2", tm.personClaims(now))); err != nil {
		t.Fatalf("an old set beats no set when Cloudflare is unreachable: %v", err)
	}
	if !strings.Contains(logs.String(), "keeping the ones fetched before") {
		t.Errorf("the failed refresh should be logged, got %q", logs.String())
	}
}

func TestKeysUnavailable(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)

	for name, handler := range map[string]http.HandlerFunc{
		"an error status": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "down", http.StatusBadGateway)
		},
		"not JSON": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html>"))
		},
		"no RSA keys": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"keys":[{"kty":"EC","kid":"e"}]}`))
		},
		"a broken modulus": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"k1","n":"!!","e":"AQAB"}]}`))
		},
		"a broken exponent": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"k1","n":"AQAB","e":"AQABAQAB"}]}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			tm := newTeam(t, "k1")
			token := tm.sign("k1", tm.personClaims(now))

			// The keys come from a server that answers wrongly instead of the team's.
			broken := httptest.NewTLSServer(handler)
			t.Cleanup(broken.Close)

			v, _ := tm.verifier(&now)
			v.client = broken.Client()
			v.certsURL = broken.URL + certsPath
			if _, err := v.Verify(context.Background(), token); err == nil ||
				!strings.Contains(err.Error(), "fetching the Access keys") {
				t.Errorf("without keys every token is refused, got %v", err)
			}
		})
	}
}

func TestNewRefusesAnIncompleteCheck(t *testing.T) {
	for _, test := range []struct {
		team, aud string
		emails    []string
	}{
		{"", testAUD, []string{testEmail}},
		{"team.cloudflareaccess.com", " ", []string{testEmail}},
		{"team.cloudflareaccess.com", testAUD, nil},
		{"team.cloudflareaccess.com", testAUD, []string{" ", ""}},
	} {
		if _, err := New(test.team, test.aud, test.emails); err == nil {
			t.Errorf("New(%q, %q, %q) should be refused", test.team, test.aud, test.emails)
		}
	}

	v, err := New("team.cloudflareaccess.com", testAUD, []string{testEmail})
	if err != nil {
		t.Fatalf("a complete check should build: %v", err)
	}
	if v.issuer != "https://team.cloudflareaccess.com" ||
		v.certsURL != "https://team.cloudflareaccess.com/cdn-cgi/access/certs" {
		t.Errorf("issuer %q, keys at %q", v.issuer, v.certsURL)
	}
}

// Require is what the pages are wrapped in: a refusal is a bare 403, the reason is in the
// log, and the token is in neither.
func TestRequire(t *testing.T) {
	tm := newTeam(t, "k1")
	now := time.Unix(1_800_000_000, 0)
	v, logs := tm.verifier(&now)

	reached := 0
	page := v.Require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte("the page"))
	}))

	serviceClaims := tm.personClaims(now)
	delete(serviceClaims, "email")
	serviceClaims["common_name"] = "agent.access"
	serviceToken := tm.sign("k1", serviceClaims)

	for _, test := range []struct {
		name   string
		token  string
		status int
	}{
		{"the owner", tm.sign("k1", tm.personClaims(now)), http.StatusOK},
		{"no header", "", http.StatusForbidden},
		{"a service token", serviceToken, http.StatusForbidden},
	} {
		logs.Reset()
		request := httptest.NewRequest(http.MethodGet, "/callback?state=s&code=c", nil)
		if test.token != "" {
			request.Header.Set(Header, test.token)
		}
		recorder := httptest.NewRecorder()
		page.ServeHTTP(recorder, request)

		if recorder.Code != test.status {
			t.Errorf("%s: status %d, want %d", test.name, recorder.Code, test.status)
		}
		if test.status != http.StatusForbidden {
			continue
		}

		body := recorder.Body.String()
		if strings.TrimSpace(body) != "forbidden" {
			t.Errorf("%s: the refusal says %q; it should say nothing about why", test.name, body)
		}
		logged := logs.String()
		if !strings.Contains(logged, "refused GET /callback") {
			t.Errorf("%s: the refusal should be logged with the path, got %q", test.name, logged)
		}
		if strings.Contains(logged, "code=c") {
			t.Errorf("%s: the log carries the query: %q", test.name, logged)
		}
		if test.token != "" && strings.Contains(logged, test.token) {
			t.Errorf("%s: the log carries the token", test.name)
		}
	}

	if reached != 1 {
		t.Errorf("only the owner should reach the page, it was reached %d times", reached)
	}
}
