package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// loginTimeout bounds how long the temporary listener waits for a person to finish at
// Google. Long enough to read the consent screen, short enough that a forgotten sign-in
// does not leave a port open all day.
const loginTimeout = 10 * time.Minute

// LoginCLI signs in from a machine that has a browser.
//
// The listener is on loopback and on a port the operating system picks, which is exactly
// what a Google client of type Desktop accepts as a redirect: any 127.0.0.1 port, with
// no registration of the address beforehand.
func LoginCLI(ctx context.Context, a *Authenticator, out io.Writer) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("opening the loopback listener for the sign-in: %w", err)
	}
	defer func() { _ = listener.Close() }()

	redirect := fmt.Sprintf("http://127.0.0.1:%d%s", listener.Addr().(*net.TCPAddr).Port, CallbackPath)

	flow, err := NewFlow(a.Config(redirect))
	if err != nil {
		return err
	}

	type callback struct {
		state string
		code  string
		err   error
	}
	results := make(chan callback, 1)

	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != CallbackPath {
				http.NotFound(w, r)
				return
			}

			query := r.URL.Query()
			result := callback{state: query.Get("state"), code: query.Get("code")}
			if reason := query.Get("error"); reason != "" {
				result.err = fmt.Errorf("Google refused the sign-in: %s", reason)
			}

			select {
			case results <- result:
			default:
			}

			writePage(w, result.err == nil)
		}),
	}

	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	fmt.Fprintln(out, "Open this address in a browser, sign in as the account this server should act as,")
	fmt.Fprintln(out, "and give consent:")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+flow.AuthURL())
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Waiting for the browser to come back…")

	timeout := time.NewTimer(loginTimeout)
	defer timeout.Stop()

	var result callback
	select {
	case result = <-results:
	case <-ctx.Done():
		return ctx.Err()
	case <-timeout.C:
		return errors.New("nobody finished the sign-in in time")
	}

	if result.err != nil {
		return result.err
	}

	token, err := flow.Exchange(ctx, result.state, result.code)
	if err != nil {
		return err
	}

	if err := a.Store().Save(token, a.Scopes()); err != nil {
		return err
	}

	fmt.Fprintln(out, "Signed in. The token is in "+a.Store().Path())

	return nil
}

// writePage is what the browser shows after the redirect. It is deliberately plain: this
// page is seen once, and anything fancier is one more thing that can be wrong.
func writePage(w http.ResponseWriter, ok bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "<!doctype html><meta charset=utf-8><title>mcp-gdocs</title>"+
			"<p>Sign-in failed. Look at the terminal that started it.")
		return
	}

	_, _ = io.WriteString(w, "<!doctype html><meta charset=utf-8><title>mcp-gdocs</title>"+
		"<p>Signed in. You can close this tab.")
}
