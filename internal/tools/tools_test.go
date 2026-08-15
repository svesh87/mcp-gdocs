package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// update rewrites the golden files instead of comparing against them.
//
// The golden files are the request bodies this server sends to Google, and they are the
// point of these tests: what a deck ends up looking like is decided by the exact
// requests, not by which methods were called. A change here is a change in what the
// slides will look like, so it gets rewritten deliberately and read in the diff.
var update = flag.Bool("update", false, "rewrite the golden request bodies")

// captured is one request the fake Google saw.
type captured struct {
	Method string
	Path   string
	Query  string
	Body   json.RawMessage
}

// route is one canned answer and the piece of a path it answers for.
type route struct {
	fragment string
	body     string
	status   int
}

// fakeGoogle answers canned responses and keeps every request body.
type fakeGoogle struct {
	t        *testing.T
	requests []captured
	// routes are matched in the order a test declared them, so a test can answer one
	// endpoint specially and let the rest fall through to an empty object. A slice rather
	// than a map because the order has to be the test's, not the runtime's.
	routes []route
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	return &fakeGoogle{t: t}
}

// answer registers a canned response for every path containing the fragment.
func (f *fakeGoogle) answer(fragment, body string) *fakeGoogle {
	f.routes = append(f.routes, route{fragment: fragment, body: body})
	return f
}

// fail registers a status for every path containing the fragment.
func (f *fakeGoogle) fail(fragment string, status int, body string) *fakeGoogle {
	f.routes = append(f.routes, route{fragment: fragment, body: body, status: status})
	return f
}

func (f *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		f.t.Fatalf("reading the request body: %v", err)
	}

	f.requests = append(f.requests, captured{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   body,
	})

	// The first route registered that matches, in the order a test declared them. Paths
	// overlap — a thumbnail lives at /presentations/deck/pages/slide1/thumbnail and so
	// matches "/presentations/deck" as well — and picking by map order made the answer
	// depend on the run rather than on the request: Go randomises it deliberately.
	for _, route := range f.routes {
		if !strings.Contains(r.URL.Path, route.fragment) {
			continue
		}
		if route.status != 0 {
			w.WriteHeader(route.status)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(route.body))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{}`))
}

func readBody(r *http.Request) (json.RawMessage, error) {
	if r.Body == nil {
		return nil, nil
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r.Body); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, nil
	}

	return json.RawMessage(buf.Bytes()), nil
}

// harness is a registry wired to a fake Google.
type harness struct {
	t        *testing.T
	registry *registry
	google   *fakeGoogle
	server   *httptest.Server
}

// ok insists a tool call succeeded and hands back what it answered. It takes the result
// and the error as one pair so a call reads as h.ok(h.registry.something(…)).
func (h *harness) ok(result *mcp.CallToolResult, err error) string {
	h.t.Helper()
	return requireOK(h.t, result, err)
}

// fail insists a tool call was refused and hands back the refusal.
func (h *harness) fail(result *mcp.CallToolResult, err error) string {
	h.t.Helper()
	return requireError(h.t, result, err)
}

func newHarness(t *testing.T, fake *fakeGoogle) *harness {
	t.Helper()

	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	client := google.New(server.Client(), google.WithBaseURL(server.URL))

	return &harness{
		t: t,
		registry: &registry{opts: Options{
			Clients:    ClientFunc(func(context.Context) (*google.Client, error) { return client, nil }),
			AllowWrite: true,
			// Every group, because these tests call the handlers directly and so skip the
			// registration that would have taken an unwanted tool back out. A handler
			// reached at all is one whose group was allowed; a test that wants a narrower
			// configuration says so, as the removal ones do.
			Groups: everyGroup(),
			// Fixed identifiers so a whole request body can be compared with a golden
			// file; the real one is random.
			NewObjectID: func(prefix string) string { return prefix + "_test" },
		}},
		google: fake,
		server: server,
	}
}

// request builds a tool call with the given arguments.
func request(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

// resultText is the text a tool answered with.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	if result == nil {
		t.Fatal("the tool returned no result")
	}

	var out strings.Builder
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			out.WriteString(text.Text)
		}
	}

	return out.String()
}

// requireError insists the tool refused, and hands back what it said.
func requireError(t *testing.T, result *mcp.CallToolResult, err error) string {
	t.Helper()

	if err != nil {
		t.Fatalf("the tool returned a protocol error rather than a refusal: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("expected a refusal, got %s", resultText(t, result))
	}

	return resultText(t, result)
}

// requireOK insists the tool succeeded, and hands back what it said.
func requireOK(t *testing.T, result *mcp.CallToolResult, err error) string {
	t.Helper()

	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if result == nil {
		t.Fatal("the tool returned no result")
	}
	if result.IsError {
		t.Fatalf("the tool refused: %s", resultText(t, result))
	}

	return resultText(t, result)
}

// bodyOf is the body of the nth request the fake saw, indented so a golden file reads
// as something a person can check against the Slides documentation.
func (h *harness) bodyOf(t *testing.T, index int) []byte {
	t.Helper()

	if index >= len(h.google.requests) {
		t.Fatalf("only %d requests were sent, wanted number %d", len(h.google.requests), index)
	}

	body := h.google.requests[index].Body
	if len(body) == 0 {
		t.Fatalf("request %d (%s %s) had no body", index, h.google.requests[index].Method, h.google.requests[index].Path)
	}

	var indented bytes.Buffer
	if err := json.Indent(&indented, body, "", "  "); err != nil {
		t.Fatalf("the request body is not JSON: %v", err)
	}

	return append(indented.Bytes(), '\n')
}

// checkGolden compares a request body with the file that records what it should be.
func checkGolden(t *testing.T, name string, actual []byte) {
	t.Helper()

	path := filepath.Join("testdata", name)

	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("creating testdata: %v", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatalf("writing the golden file: %v", err)
		}
		return
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the golden file (run the tests with -update to create it): %v", err)
	}

	if !bytes.Equal(expected, actual) {
		t.Errorf("the request body differs from %s.\n--- want ---\n%s\n--- got ---\n%s\n"+
			"If this change is intended, say what changes in the deck and rerun with -update.",
			path, expected, actual)
	}
}

func TestUTF16Length(t *testing.T) {
	// The Google APIs index text in UTF-16 code units. Bytes and runes both give the
	// wrong answer for anything outside ASCII, and a wrong index means a refused batch.
	for _, test := range []struct {
		text string
		want int64
	}{
		{"", 0},
		{"abc", 3},
		{"Что сделали?", 12},
		{"👍", 2},
	} {
		if got := utf16Length(test.text); got != test.want {
			t.Errorf("utf16Length(%q) = %d, want %d", test.text, got, test.want)
		}
	}
}

func TestFirstLineEnd(t *testing.T) {
	for _, test := range []struct {
		text string
		want int64
	}{
		{"Заголовок\nтело", 9},
		{"one line", 8},
		{"\nbody", 0},
	} {
		if got := firstLineEnd(test.text); got != test.want {
			t.Errorf("firstLineEnd(%q) = %d, want %d", test.text, got, test.want)
		}
	}
}

func TestCellText(t *testing.T) {
	for _, test := range []struct {
		cell any
		want string
	}{
		{nil, ""},
		{"text", "text"},
		{float64(1), "1"},
		{float64(1.5), "1.5"},
		{true, "true"},
	} {
		got, err := cellText(test.cell)
		if err != nil {
			t.Fatalf("cellText(%v): %v", test.cell, err)
		}
		if got != test.want {
			t.Errorf("cellText(%v) = %q, want %q", test.cell, got, test.want)
		}
	}

	if _, err := cellText(map[string]any{}); err == nil {
		t.Error("a nested object should not be accepted as a cell")
	}
}

func TestRegisterNeedsClients(t *testing.T) {
	if err := Register(nil, Options{}); err == nil {
		t.Error("registering without a client provider should fail")
	}
}

// errNoClient stands in for the real "nobody has signed in" error.
var errNoClient = errors.New("not signed in yet")

func TestToolsRefuseWithoutClient(t *testing.T) {
	failing := &registry{opts: Options{Clients: ClientFunc(func(context.Context) (*google.Client, error) {
		return nil, errNoClient
	})}}

	result, err := failing.slidesList(context.Background(), request(map[string]any{
		"presentation_id": "deck",
	}))

	if message := requireError(t, result, err); !strings.Contains(message, "not signed in") {
		t.Errorf("the refusal should say what to do about it, got %q", message)
	}
}
