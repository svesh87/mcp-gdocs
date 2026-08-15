package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCopyAndRestoreCallTheRightEndpoints pins the addresses and the methods of the calls
// whose consequences reach outside one document.
//
// The method matters more here than anywhere else in this client. copyTo is the one request
// Google offers that carries content between documents, and restoring is a PATCH over an
// existing file: sent as a POST it would create a second file instead, with none of the
// sharing and none of the links pointing at it.
func TestCopyAndRestoreCallTheRightEndpoints(t *testing.T) {
	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, ":copyTo"):
			_, _ = w.Write([]byte(`{"sheetId": 42, "title": "Копия Цели"}`))
		case strings.Contains(r.URL.Path, "/revisions/"):
			_, _ = w.Write([]byte(`{"id": "7", "mimeType": "application/pdf"}`))
		case strings.Contains(r.URL.Path, "/spreadsheets/"):
			_, _ = w.Write([]byte(`{"spreadsheetId": "book",
              "sheets": [{"properties": {"sheetId": 0, "title": "Цели"},
                "charts": [{"chartId": 5, "spec": {"title": "Было"}}]}]}`))
		default:
			_, _ = w.Write([]byte(`{"id": "f1", "name": "Отчёт"}`))
		}
	}))
	defer server.Close()

	client := New(server.Client(), WithBaseURL(server.URL))
	ctx := context.Background()

	copied, err := client.CopySheetTo(ctx, "source", 3, "target")
	if err != nil {
		t.Fatalf("copying a tab: %v", err)
	}
	if copied.Title != "Копия Цели" {
		t.Errorf("the copy's own name should come back, got %q", copied.Title)
	}

	if _, err := client.SpreadsheetCopyGrid(ctx, "book", "'Цели'!A1:C5"); err != nil {
		t.Fatalf("reading a rectangle for a copy: %v", err)
	}

	spec, err := client.ChartSpecOf(ctx, "book", 5)
	if err != nil {
		t.Fatalf("reading a chart's specification: %v", err)
	}
	if spec["title"] != "Было" {
		t.Errorf("the specification should come back as Google stores it, got %v", spec)
	}

	// A chart that is not there is named as such, because the number is the only handle a
	// caller has and a wrong one is the commonest mistake.
	if _, err := client.ChartSpecOf(ctx, "book", 99); err == nil {
		t.Error("a chart that does not exist should be refused")
	}

	content, format, err := client.RevisionContent(ctx, "report", "7", nil)
	if err != nil {
		t.Fatalf("fetching a version: %v", err)
	}
	if len(content) == 0 || format != "application/pdf" {
		t.Errorf("an ordinary file's bytes come back as they are, got %d bytes as %q", len(content), format)
	}

	if _, err := client.ReplaceFileContent(ctx, "report", "application/pdf", []byte("bytes")); err != nil {
		t.Fatalf("writing a version back: %v", err)
	}

	for _, want := range []string{
		"POST /v4/spreadsheets/source/sheets/3:copyTo",
		"PATCH /upload/drive/v3/files/report",
	} {
		if !contains(seen, want) {
			t.Errorf("expected a call to %s, saw %v", want, seen)
		}
	}
}

// TestRevisionContentNamesWhatItCannotUse: a version offered only in formats a restore cannot
// write back is a dead end, and the way out is to say which formats those were rather than
// leaving a caller to guess at a parameter that does not exist.
func TestRevisionContentNamesWhatItCannotUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": "7", "exportLinks": {"application/pdf": "https://example.invalid/x",
          "text/plain": "https://example.invalid/y"}}`))
	}))
	defer server.Close()

	client := New(server.Client(), WithBaseURL(server.URL))

	_, _, err := client.RevisionContent(context.Background(), "deck", "7",
		[]string{"application/vnd.openxmlformats-officedocument.presentationml.presentation"})
	if err == nil {
		t.Fatal("a version in no usable format should be refused")
	}
	for _, want := range []string{"application/pdf", "text/plain"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should list %s, got %v", want, err)
		}
	}
}

func contains(list []string, want string) bool {
	for _, one := range list {
		if one == want {
			return true
		}
	}

	return false
}
