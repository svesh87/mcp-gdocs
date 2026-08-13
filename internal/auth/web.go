package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// LoginPath is where a person starts a sign-in against a running server.
const LoginPath = "/login"

// pendingTTL is how long a started sign-in stays valid. It also bounds how many attempts
// a passer-by could accumulate in memory.
const pendingTTL = 15 * time.Minute

// WebLogin signs in from a browser on the host while the server runs in a container.
//
// The browser reaches the server on a loopback port of the host, and that same address is
// what Google is given as the redirect. A Google client of type Desktop accepts any
// loopback address, so nothing has to be registered in the console beforehand — which is
// the whole reason this works through a published container port at all.
//
// The page is a page rather than a bare redirect: it says whether anybody is signed in
// before it offers to change that, and the trip to Google happens on a form submission
// rather than on opening a link. A link that walks straight into a consent screen is a
// link somebody can send to somebody else.
type WebLogin struct {
	auth   *Authenticator
	bearer string

	mu      sync.Mutex
	pending map[string]*pendingFlow
}

type pendingFlow struct {
	flow    *Flow
	started time.Time
}

// NewWebLogin wires the sign-in pages to the server's own bearer token.
func NewWebLogin(a *Authenticator, bearer string) *WebLogin {
	return &WebLogin{auth: a, bearer: bearer, pending: map[string]*pendingFlow{}}
}

// Handlers are the pages to mount beside the MCP endpoint.
func (l *WebLogin) Handlers() map[string]http.Handler {
	return map[string]http.Handler{
		LoginPath:    http.HandlerFunc(l.login),
		CallbackPath: http.HandlerFunc(l.finish),
	}
}

// login shows the state of the sign-in, and on a submission sends the browser to Google.
//
// The key travels in the query string on the way in and in a hidden field on the way out,
// because a browser sends no Authorization header of its own. Both are why the page must
// not be cached and must not leak a referrer.
func (l *WebLogin) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "the form could not be read", http.StatusBadRequest)
			return
		}
	}

	if subtle.ConstantTimeCompare([]byte(pageKey(r)), []byte(l.bearer)) != 1 {
		// No key, no page: whoever knocked without it gets nothing to look at and
		// nothing to click.
		http.Error(w, "authentication required: open /login?key=<the server's MCP_AUTH_TOKEN>",
			http.StatusUnauthorized)
		return
	}

	noStore(w)

	redirect, err := loopbackRedirect(r)
	if err != nil {
		l.render(w, pageView{Key: l.bearer, Status: l.auth.Store().Status(time.Now()), Error: err.Error()})
		return
	}

	switch r.Method {
	case http.MethodGet:
		l.render(w, pageView{
			Key:      l.bearer,
			Status:   l.auth.Store().Status(time.Now()),
			Scopes:   l.auth.Scopes(),
			Redirect: redirect,
		})

	case http.MethodPost:
		flow, err := NewFlow(l.auth.Config(redirect))
		if err != nil {
			l.render(w, pageView{Key: l.bearer, Status: l.auth.Store().Status(time.Now()), Error: err.Error()})
			return
		}

		l.mu.Lock()
		l.forgetExpiredLocked()
		l.pending[flow.State()] = &pendingFlow{flow: flow, started: time.Now()}
		l.mu.Unlock()

		http.Redirect(w, r, flow.AuthURL(), http.StatusSeeOther)

	default:
		http.Error(w, "only GET and POST", http.StatusMethodNotAllowed)
	}
}

// finish takes the code Google sent back and stores the token.
//
// There is no bearer check here: the browser arrives from Google, which knows nothing
// about this server's token. What guards this path is the state — 32 random bytes handed
// out by the page, single use, and useless without the PKCE verifier held beside it.
func (l *WebLogin) finish(w http.ResponseWriter, r *http.Request) {
	noStore(w)

	query := r.URL.Query()

	if reason := query.Get("error"); reason != "" {
		l.render(w, pageView{
			Key:    l.bearer,
			Status: l.auth.Store().Status(time.Now()),
			Error:  "Google отказал: " + reason,
		})
		return
	}

	state := query.Get("state")

	l.mu.Lock()
	l.forgetExpiredLocked()
	entry, ok := l.pending[state]
	delete(l.pending, state)
	l.mu.Unlock()

	if !ok {
		l.render(w, pageView{
			Key:    l.bearer,
			Status: l.auth.Store().Status(time.Now()),
			Error:  "Этот вход неизвестен или устарел — начните заново",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	token, err := entry.flow.Exchange(ctx, state, query.Get("code"))
	if err != nil {
		l.render(w, pageView{Key: l.bearer, Status: l.auth.Store().Status(time.Now()), Error: err.Error()})
		return
	}

	if err := l.auth.Store().Save(token, l.auth.Scopes()); err != nil {
		l.render(w, pageView{Key: l.bearer, Status: l.auth.Store().Status(time.Now()), Error: err.Error()})
		return
	}

	l.render(w, pageView{
		Key:    l.bearer,
		Status: l.auth.Store().Status(time.Now()),
		Scopes: l.auth.Scopes(),
		Done:   true,
	})
}

// forgetExpiredLocked drops sign-ins nobody finished. Callers hold the mutex.
func (l *WebLogin) forgetExpiredLocked() {
	for state, entry := range l.pending {
		if time.Since(entry.started) > pendingTTL {
			delete(l.pending, state)
		}
	}
}

// pageKey reads the key the page carries: in the query on the way in, in a hidden field
// on the way back.
func pageKey(r *http.Request) string {
	if key := r.URL.Query().Get("key"); key != "" {
		return key
	}
	return r.PostFormValue("key")
}

// noStore keeps the key out of caches and out of the address the browser passes on to
// Google.
func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

// loopbackRedirect builds the redirect address out of the one the browser used.
//
// It has to be loopback: that is the only kind of redirect a Desktop client may use, and
// a redirect to a routable address would hand the authorisation code to whoever answers
// there. On a server across the network the way in is an SSH tunnel, which makes the
// browser's address loopback again.
func loopbackRedirect(r *http.Request) (string, error) {
	host := r.Host
	if host == "" {
		return "", fmt.Errorf("в запросе нет заголовка Host, и браузер некуда возвращать")
	}

	name, port, err := net.SplitHostPort(host)
	if err != nil {
		// No port in the Host header: the browser used the default one, which for a
		// loopback sign-in it never should.
		name, port = host, ""
	}

	switch name {
	case "localhost", "127.0.0.1", "[::1]", "::1":
	default:
		return "", fmt.Errorf("вход работает только через loopback, а открыто по адресу %q. "+
			"Опубликуйте порт сервера на 127.0.0.1 и откройте страницу там; "+
			"если сервер на другой машине — пробросьте порт: "+
			"ssh -L %s:127.0.0.1:%s <хост>", host, portOrDefault(port), portOrDefault(port))
	}

	if port == "" {
		return "", fmt.Errorf("в адресе %q нет порта: откройте сервер на том порту, "+
			"на котором он опубликован", host)
	}

	// 127.0.0.1 rather than the name the browser used: Google documents the literal
	// address for installed clients, and the two are not interchangeable in every
	// account.
	return fmt.Sprintf("http://127.0.0.1:%s%s", port, CallbackPath), nil
}

func portOrDefault(port string) string {
	if port == "" {
		return "8819"
	}
	return port
}
