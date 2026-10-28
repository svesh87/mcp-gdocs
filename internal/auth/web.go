package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
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
//
// With a public address (--public-url) the same pages serve a different route: the server
// sits behind a gateway that terminates TLS, checks who the person is and adds the bearer
// header itself, and Google returns the browser to <public-url>/callback — an address a
// client of type Web application has registered in advance. Without one, nothing about the
// loopback route changes.
type WebLogin struct {
	auth   *Authenticator
	bearer string

	// publicURL is the external base address without a trailing slash, and publicOrigin
	// its scheme and host. Both empty in the loopback route.
	publicURL    string
	publicOrigin string
	// crossOrigin guards form submissions in the public route; nil in the loopback route.
	crossOrigin *http.CrossOriginProtection

	mu      sync.Mutex
	pending map[string]*pendingFlow
}

type pendingFlow struct {
	flow    *Flow
	started time.Time
}

// PublicCallbackPath is where Google returns the browser in the public route. It is not
// CallbackPath on purpose: a Web application client has its redirect registered in the
// console by hand, and the address written there is <public-url>/callback.
const PublicCallbackPath = "/callback"

// RootPattern is the exact root of the public route, in http.ServeMux syntax.
const RootPattern = "/{$}"

// NewWebLogin wires the sign-in pages to the server's own bearer token.
func NewWebLogin(a *Authenticator, bearer string) *WebLogin {
	return &WebLogin{auth: a, bearer: bearer, pending: map[string]*pendingFlow{}}
}

// NewPublicWebLogin wires the sign-in pages for a server reached at publicURL through a
// gateway. publicURL is absolute, e.g. https://mcp.example.com/gdocs.
func NewPublicWebLogin(a *Authenticator, bearer, publicURL string) (*WebLogin, error) {
	base := strings.TrimRight(publicURL, "/")

	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("the public URL %q is not an absolute address", publicURL)
	}

	l := NewWebLogin(a, bearer)
	l.publicURL = base
	l.publicOrigin = parsed.Scheme + "://" + parsed.Host
	// Behind the gateway the Host header is not the public address, so that one is trusted
	// explicitly.
	l.crossOrigin = http.NewCrossOriginProtection()
	if err := l.crossOrigin.AddTrustedOrigin(l.publicOrigin); err != nil {
		return nil, fmt.Errorf("the public URL %q has no usable origin: %w", publicURL, err)
	}

	return l, nil
}

// LoginURL is the address a person opens to sign in, when the server has a public one.
// It never carries the key: behind the gateway the bearer travels in a header the gateway
// adds. Empty in the loopback route, where the address depends on the published port.
func (l *WebLogin) LoginURL() string {
	if l.publicURL == "" {
		return ""
	}
	return l.publicURL + LoginPath
}

// Handlers are the pages to mount beside the MCP endpoint.
func (l *WebLogin) Handlers() map[string]http.Handler {
	if l.publicURL == "" {
		return map[string]http.Handler{
			LoginPath:    http.HandlerFunc(l.login),
			CallbackPath: http.HandlerFunc(l.finish),
		}
	}

	return map[string]http.Handler{
		LoginPath:          http.HandlerFunc(l.login),
		PublicCallbackPath: http.HandlerFunc(l.finish),
		// The bare address of the route is where a person lands by typing it; the page
		// they want is the sign-in.
		RootPattern: http.HandlerFunc(l.root),
	}
}

// root sends the bare public address to the sign-in page.
func (l *WebLogin) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, l.LoginURL(), http.StatusFound)
}

// authenticate says whether the request carries the server's token, and whether it came
// in the Authorization header — in which case the page must not repeat it anywhere.
//
// The header is accepted only in the public route, where the gateway adds it. The loopback
// route keeps answering exactly as it always did: the key in the query or in the form.
func (l *WebLogin) authenticate(r *http.Request) (viaHeader, ok bool) {
	if l.publicURL != "" {
		if presented, found := bearerFromHeader(r.Header.Get("Authorization")); found &&
			subtle.ConstantTimeCompare([]byte(presented), []byte(l.bearer)) == 1 {
			return true, true
		}
	}

	return false, subtle.ConstantTimeCompare([]byte(pageKey(r)), []byte(l.bearer)) == 1
}

// redirectFor is where Google sends the browser back to for this request.
func (l *WebLogin) redirectFor(r *http.Request) (string, error) {
	if l.publicURL != "" {
		return l.publicURL + PublicCallbackPath, nil
	}
	return loopbackRedirect(r)
}

// view starts a page with what every page of this route carries. key is what the form
// carries back, and is empty when the request was authenticated by its header.
func (l *WebLogin) view(key string) pageView {
	v := pageView{Key: key, Status: l.auth.Store().Status(time.Now())}
	if l.publicURL != "" {
		v.Public = true
		v.Action = l.LoginURL()
		v.LoginURL = l.LoginURL()
	}
	return v
}

// crossSite says whether a form submission came from a page other than this server's own.
// Only the public route asks: there the gateway adds the bearer to whatever the browser
// sends, so a form on someone else's site would otherwise be enough to start a sign-in.
//
// Origin alone cannot tell: the page is served with no-referrer, and then browsers send
// "Origin: null" even from the page itself. Sec-Fetch-Site does tell, and
// CrossOriginProtection reads it first, falling back to Origin for browsers without it.
func (l *WebLogin) crossSite(r *http.Request) bool {
	if l.crossOrigin == nil {
		return false
	}
	return l.crossOrigin.Check(r) != nil
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

	viaHeader, ok := l.authenticate(r)
	if !ok {
		// No key, no page: whoever knocked without it gets nothing to look at and
		// nothing to click.
		message := "authentication required: open /login?key=<the server's MCP_AUTH_TOKEN>"
		if l.publicURL != "" {
			message = "authentication required: open " + l.LoginURL() +
				" through the gateway, which adds the server's bearer token"
		}
		http.Error(w, message, http.StatusUnauthorized)
		return
	}

	noStore(w)

	// A request the gateway authenticated carries the token in a header, and the page
	// gives it nothing to repeat: no hidden field, no address with the key in it.
	key := l.bearer
	if viaHeader {
		key = ""
	}

	if r.Method == http.MethodPost && l.crossSite(r) {
		http.Error(w, "the sign-in form was submitted from another site", http.StatusForbidden)
		return
	}

	redirect, err := l.redirectFor(r)
	if err != nil {
		view := l.view(key)
		view.Error = err.Error()
		l.render(w, view)
		return
	}

	switch r.Method {
	case http.MethodGet:
		view := l.view(key)
		view.Scopes = l.auth.Scopes()
		view.Redirect = redirect
		l.render(w, view)

	case http.MethodPost:
		flow, err := NewFlow(l.auth.Config(redirect))
		if err != nil {
			view := l.view(key)
			view.Error = err.Error()
			l.render(w, view)
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
//
// In the public route the browser comes back through the gateway, which adds the bearer
// header as on any other request. The check stays the state all the same: the callback is
// guarded by the same thing in both routes, and the pages it renders carry no form, so
// there is nothing for a key to be repeated in.
func (l *WebLogin) finish(w http.ResponseWriter, r *http.Request) {
	noStore(w)

	query := r.URL.Query()

	// The key goes into the view only in the loopback route, where it always did; none of
	// the pages below renders it.
	key := l.bearer
	if l.publicURL != "" {
		key = ""
	}

	if reason := query.Get("error"); reason != "" {
		view := l.view(key)
		view.Error = "Google отказал: " + reason
		l.render(w, view)
		return
	}

	state := query.Get("state")

	l.mu.Lock()
	l.forgetExpiredLocked()
	entry, ok := l.pending[state]
	delete(l.pending, state)
	l.mu.Unlock()

	if !ok {
		view := l.view(key)
		view.Error = "Этот вход неизвестен или устарел — начните заново"
		l.render(w, view)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	token, err := entry.flow.Exchange(ctx, state, query.Get("code"))
	if err != nil {
		view := l.view(key)
		view.Error = err.Error()
		l.render(w, view)
		return
	}

	if err := l.auth.Store().Save(token, l.auth.Scopes()); err != nil {
		view := l.view(key)
		view.Error = err.Error()
		l.render(w, view)
		return
	}

	view := l.view(key)
	view.Scopes = l.auth.Scopes()
	view.Done = true
	l.render(w, view)
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

// bearerFromHeader pulls the token out of an Authorization header value, matching the
// scheme case-insensitively. The transport package has the same rule for /mcp; it is
// repeated here rather than imported so the sign-in does not depend on the MCP transport.
func bearerFromHeader(header string) (string, bool) {
	const scheme = "bearer "

	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}

	token := strings.TrimSpace(header[len(scheme):])

	return token, token != ""
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
