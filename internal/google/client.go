// Package google is a thin client over the Drive, Sheets, Slides and Docs REST APIs.
//
// Thin on purpose. What this server is worth is the exact sequence of requests it sends
// to Slides and Sheets, so those request bodies are built here from plain structures and
// go out unchanged — no generated layer in between deciding what a field is called. The
// tests read the bodies off the wire and compare them byte for byte with golden files.
package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Base addresses of the four APIs. They are fields rather than constants so a test can
// point the client at an httptest server.
const (
	DefaultDriveBase  = "https://www.googleapis.com/drive/v3"
	DefaultSheetsBase = "https://sheets.googleapis.com/v4"
	DefaultSlidesBase = "https://slides.googleapis.com/v1"
	DefaultDocsBase   = "https://docs.googleapis.com/v1"
	// DefaultUploadBase is where Drive takes file content rather than metadata.
	DefaultUploadBase = "https://www.googleapis.com/upload/drive/v3"
	// DefaultEditorsBase is the editors' own address. What it serves is a rendering
	// rather than an API answer, and it is asked only for what no API reports.
	DefaultEditorsBase = "https://docs.google.com"
)

// Client talks to the four APIs over one authenticated HTTP client.
type Client struct {
	http *http.Client

	driveBase  string
	sheetsBase string
	slidesBase string
	docsBase   string
	uploadBase string

	// editorsBase is where the editors serve their own exports, which is not an API
	// address and answers things the APIs do not — see ExportFile and ExportSheetHTML.
	editorsBase string

	// retryDelay is how long to wait before another attempt. It is a field so tests do
	// not sit through a real quota window.
	retryDelay func(attempt int) time.Duration
}

// Option adjusts a client. Only the addresses are adjustable, and only tests do it.
type Option func(*Client)

// WithBaseURL points every API at one address. A test server answers all four paths.
func WithBaseURL(base string) Option {
	return func(c *Client) {
		base = strings.TrimSuffix(base, "/")
		c.driveBase = base + "/drive/v3"
		c.sheetsBase = base + "/v4"
		c.slidesBase = base + "/v1"
		c.docsBase = base + "/v1"
		c.uploadBase = base + "/upload/drive/v3"
		c.editorsBase = base + "/editors"
	}
}

// WithRetryDelay replaces the wait between attempts. Tests use it to avoid sitting
// through a real quota window.
func WithRetryDelay(delay func(attempt int) time.Duration) Option {
	return func(c *Client) { c.retryDelay = delay }
}

// New builds a client over an authenticated HTTP client.
func New(httpClient *http.Client, opts ...Option) *Client {
	client := &Client{
		http:        httpClient,
		driveBase:   DefaultDriveBase,
		sheetsBase:  DefaultSheetsBase,
		slidesBase:  DefaultSlidesBase,
		docsBase:    DefaultDocsBase,
		uploadBase:  DefaultUploadBase,
		editorsBase: DefaultEditorsBase,
		retryDelay:  backoff,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// Error is what an API said when it refused.
type Error struct {
	Status  int
	Message string
	Reason  string
	URL     string
}

func (e *Error) Error() string {
	message := fmt.Sprintf("Google returned %d", e.Status)
	if e.Reason != "" {
		message += " (" + e.Reason + ")"
	}
	if e.Message != "" {
		message += ": " + e.Message
	}
	return message
}

// apiError is the error envelope Google returns.
type apiError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Errors  []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}

// call sends one request and decodes the answer into out, which may be nil.
func (c *Client) call(ctx context.Context, method, endpoint string, body, out any) error {
	var reader io.Reader

	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding the request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("building the request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	raw, err := c.send(req)
	if err != nil {
		return err
	}

	if out == nil || len(raw) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decoding the answer from %s: %w", endpoint, err)
	}

	return nil
}

// send performs the request, retrying the failures that pass on their own, and turns a
// refusal into an *Error.
//
// Google meters writes per minute per user, and building a deck is a burst of them: a
// slide, its text, its styles, its picture, and again for the next slide. Without retries
// the twentieth slide of a deck fails on quota while the first nineteen went through,
// which leaves the document half-built and the caller with nothing sensible to do.
func (c *Client) send(req *http.Request) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := c.retryDelay(attempt)
			select {
			case <-time.After(delay):
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}

			// A request body is read once, so a retry needs its own copy.
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("preparing a retry of %s: %w", req.URL.Path, err)
				}
				req.Body = body
			}
		}

		raw, retryable, err := c.attempt(req)
		if err == nil {
			return raw, nil
		}
		lastErr = err

		if !retryable {
			return nil, err
		}
	}

	return nil, fmt.Errorf("%w (gave up after %d attempts)", lastErr, maxAttempts)
}

// maxAttempts bounds the retrying. Quota windows are measured in a minute, so a handful
// of attempts with a growing pause covers them without hanging a caller for long.
const maxAttempts = 5

// backoff is how long to wait before attempt n, growing quickly enough to outlast a
// per-minute quota window.
func backoff(attempt int) time.Duration {
	return time.Duration(1<<attempt) * 2 * time.Second
}

// attempt sends the request once and says whether the failure is worth repeating.
func (c *Client) attempt(req *http.Request) ([]byte, bool, error) {
	raw, err := c.once(req)
	if err == nil {
		return raw, false, nil
	}

	var failure *Error
	if errors.As(err, &failure) {
		// Quota and the transient server-side failures pass by themselves; everything
		// else is a decision Google has made and will make again.
		switch {
		case failure.Status == http.StatusTooManyRequests,
			failure.Status == http.StatusInternalServerError,
			failure.Status == http.StatusBadGateway,
			failure.Status == http.StatusServiceUnavailable,
			failure.Status == http.StatusGatewayTimeout:
			return nil, true, err
		}
	}

	return nil, false, err
}

// once performs the request and turns a refusal into an *Error.
func (c *Client) once(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling %s: %w", req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading the answer from %s: %w", req.URL.Path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		failure := &Error{Status: resp.StatusCode, URL: req.URL.Path}

		var envelope apiError
		if json.Unmarshal(raw, &envelope) == nil && envelope.Error.Message != "" {
			failure.Message = envelope.Error.Message
			failure.Reason = envelope.Error.Status
			if failure.Reason == "" && len(envelope.Error.Errors) > 0 {
				failure.Reason = envelope.Error.Errors[0].Reason
			}
		} else {
			// Not the usual envelope. The body is shown as-is, trimmed, because the
			// alternative is an error that says only "400".
			failure.Message = strings.TrimSpace(truncate(string(raw), 500))
		}

		return nil, failure
	}

	return raw, nil
}

// download fetches bytes rather than JSON: exports and thumbnails.
func (c *Client) download(ctx context.Context, endpoint string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("building the request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("calling %s: %w", req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading the answer from %s: %w", req.URL.Path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		failure := &Error{Status: resp.StatusCode, URL: req.URL.Path}

		var envelope apiError
		if json.Unmarshal(raw, &envelope) == nil && envelope.Error.Message != "" {
			failure.Message = envelope.Error.Message
			failure.Reason = envelope.Error.Status
		} else {
			failure.Message = strings.TrimSpace(truncate(string(raw), 500))
		}

		return nil, "", failure
	}

	return raw, resp.Header.Get("Content-Type"), nil
}

// endpoint joins a base, a path and a query.
func endpoint(base, path string, query url.Values) string {
	address := base + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	return address
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}
