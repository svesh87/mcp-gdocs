package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDriveManagementCallsTheRightEndpoints pins the addresses and the methods, because
// these are the calls whose consequences are outside a document: a file moved, a link
// opened, a version kept. A PATCH sent as a POST creates rather than changes, and the
// difference is a second file on somebody's Drive.
func TestDriveManagementCallsTheRightEndpoints(t *testing.T) {
	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/permissions") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"permissions": [{"id": "p1", "type": "anyone", "role": "reader"}]}`))
		case strings.HasSuffix(r.URL.Path, "/permissions"):
			_, _ = w.Write([]byte(`{"id": "p1", "type": "anyone", "role": "reader"}`))
		case strings.Contains(r.URL.Path, "/comments") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"comments": [{"id": "c1", "content": "правка"}]}`))
		case strings.Contains(r.URL.Path, "/revisions") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"revisions": [{"id": "1", "modifiedTime": "2026-08-13T10:00:00Z"}]}`))
		default:
			_, _ = w.Write([]byte(`{"id": "f1", "name": "Отчёт"}`))
		}
	}))
	defer server.Close()

	client := New(server.Client(), WithBaseURL(server.URL))
	ctx := context.Background()

	if _, err := client.CreateFolder(ctx, "Офферы", "parent"); err != nil {
		t.Fatalf("creating a folder: %v", err)
	}
	if _, err := client.RenameFile(ctx, "f1", "Отчёт за август"); err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if _, err := client.SetTrashed(ctx, "f1", true); err != nil {
		t.Fatalf("trashing: %v", err)
	}
	if _, err := client.Permissions(ctx, "f1"); err != nil {
		t.Fatalf("reading permissions: %v", err)
	}
	granted, err := client.Share(ctx, "f1", SharePermission{Type: "anyone", Role: "reader"}, false)
	if err != nil {
		t.Fatalf("sharing: %v", err)
	}
	if err := client.Unshare(ctx, "f1", granted.ID); err != nil {
		t.Fatalf("unsharing: %v", err)
	}
	if _, err := client.Comments(ctx, "f1", false, ""); err != nil {
		t.Fatalf("reading comments: %v", err)
	}
	if _, err := client.AddComment(ctx, "f1", "правка"); err != nil {
		t.Fatalf("commenting: %v", err)
	}
	if _, err := client.ReplyToComment(ctx, "f1", "c1", "поправил", "resolve"); err != nil {
		t.Fatalf("replying: %v", err)
	}
	if _, err := client.Revisions(ctx, "f1", ""); err != nil {
		t.Fatalf("reading revisions: %v", err)
	}
	if _, err := client.KeepRevision(ctx, "f1", "1", true); err != nil {
		t.Fatalf("keeping a revision: %v", err)
	}

	want := []string{
		"POST /drive/v3/files",
		"PATCH /drive/v3/files/f1",
		"PATCH /drive/v3/files/f1",
		"GET /drive/v3/files/f1/permissions",
		"POST /drive/v3/files/f1/permissions",
		"DELETE /drive/v3/files/f1/permissions/p1",
		"GET /drive/v3/files/f1/comments",
		"POST /drive/v3/files/f1/comments",
		"POST /drive/v3/files/f1/comments/c1/replies",
		"GET /drive/v3/files/f1/revisions",
		"PATCH /drive/v3/files/f1/revisions/1",
	}

	if len(seen) != len(want) {
		t.Fatalf("expected %d calls, saw %d: %v", len(want), len(seen), seen)
	}
	for index, expected := range want {
		if seen[index] != expected {
			t.Errorf("call %d should be %q, was %q", index, expected, seen[index])
		}
	}
}

// TestSharingSendsTheGrantItWasGiven, because the body of this one decides who can see a
// file afterwards.
func TestSharingSendsTheGrantItWasGiven(t *testing.T) {
	var body string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buffer := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buffer)
		body = string(buffer)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": "p2", "type": "user", "role": "writer"}`))
	}))
	defer server.Close()

	client := New(server.Client(), WithBaseURL(server.URL))

	if _, err := client.Share(context.Background(), "f1",
		SharePermission{Type: "user", Role: "writer", EmailAddress: "someone@example.org"}, false); err != nil {
		t.Fatalf("sharing: %v", err)
	}

	for _, want := range []string{`"type":"user"`, `"role":"writer"`, "someone@example.org"} {
		if !strings.Contains(body, want) {
			t.Errorf("the grant should carry %s, got %s", want, body)
		}
	}
}
