package tools

import (
	"context"
	"strings"
	"testing"
)

// TestDriveReadingTools covers the answers an agent decides by: what is in a folder, who
// can see a file, and which versions exist.
func TestDriveReadingTools(t *testing.T) {
	t.Run("what is in a folder", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/files", `{"files": [
		  {"id": "d1", "name": "Оффер", "mimeType": "application/vnd.google-apps.document"},
		  {"id": "d2", "name": "Черновик", "mimeType": "application/vnd.google-apps.document"}
		]}`))

		answer := h.ok(h.registry.driveListFolder(context.Background(), request(map[string]any{
			"folder_id": "folder1", "kind": "document",
		})))

		for _, want := range []string{"Оффер", "Черновик", `"folder_id": "folder1"`} {
			if !strings.Contains(answer, want) {
				t.Errorf("the listing should carry %s, got %s", want, answer)
			}
		}

		// The query is what turns "what is in this folder" into something Drive answers,
		// and the folder has to be in it.
		if query := h.google.requests[0].Query; !strings.Contains(query, "folder1") {
			t.Errorf("the search should ask for the folder's children, query was %q", query)
		}
	})

	t.Run("an unknown kind is refused", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t))

		message := h.fail(h.registry.driveListFolder(context.Background(), request(map[string]any{
			"folder_id": "folder1", "kind": "видео",
		})))
		if !strings.Contains(message, "spreadsheet, presentation, document or folder") {
			t.Errorf("the refusal should list the kinds, got %q", message)
		}
	})

	t.Run("who can see it", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/permissions", `{"permissions": [
		  {"id": "owner", "type": "user", "role": "owner", "emailAddress": "someone@example.org"},
		  {"id": "anyoneWithLink", "type": "anyone", "role": "reader"}
		]}`))

		answer := h.ok(h.registry.driveListPermissions(context.Background(), request(map[string]any{
			"file_id": "f1",
		})))

		if !strings.Contains(answer, `"open_to_anyone_with_link": true`) {
			t.Errorf("a file open by link should say so plainly, got %s", answer)
		}
	})

	t.Run("versions", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/revisions", `{"revisions": [
		  {"id": "1", "modifiedTime": "2026-08-13T09:00:00Z", "lastModifyingUser": {"displayName": "Анна Соколова"}},
		  {"id": "42", "modifiedTime": "2026-08-13T18:00:00Z", "keepForever": true}
		]}`))

		answer := h.ok(h.registry.driveListRevisions(context.Background(), request(map[string]any{
			"file_id": "f1",
		})))

		for _, want := range []string{`"id": "42"`, `"kept_forever": true`, "restoring a version"} {
			if !strings.Contains(answer, want) {
				t.Errorf("the reading should carry %s, got %s", want, answer)
			}
		}
	})

	t.Run("taking a grant back", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t))

		answer := h.ok(h.registry.driveUnshare(context.Background(), request(map[string]any{
			"file_id": "f1", "permission_id": "anyoneWithLink",
		})))

		if !strings.Contains(answer, "taken back") {
			t.Errorf("the answer should say the access is gone, got %s", answer)
		}
	})
}

// TestDocsNamedRangesRoundTrip: a name is read back with where it currently is, which is
// the whole reason to use one instead of an index.
func TestDocsNamedRangesRoundTrip(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/documents/doc", `{
	  "documentId": "doc",
	  "namedRanges": {"salary": {"name": "salary", "namedRanges": [
	    {"namedRangeId": "nr1", "name": "salary", "ranges": [{"startIndex": 300, "endIndex": 330}]}
	  ]}}
	}`))

	answer := h.ok(h.registry.docsListNamedRanges(context.Background(), request(map[string]any{
		"document_id": "doc",
	})))

	for _, want := range []string{`"name": "salary"`, `"range_id": "nr1"`, `"start_index": 300`} {
		if !strings.Contains(answer, want) {
			t.Errorf("the reading should carry %s, got %s", want, answer)
		}
	}
}

func TestDocsUpdateTabNeedsSomethingToChange(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.docsUpdateTab(context.Background(), request(map[string]any{
		"document_id": "doc", "tab_id": "t.0",
	})))
	if !strings.Contains(message, "title, index or icon_emoji") {
		t.Errorf("the refusal should say what can be changed, got %q", message)
	}

	h.ok(h.registry.docsUpdateTab(context.Background(), request(map[string]any{
		"document_id": "doc", "tab_id": "t.0", "title": "Приложение", "index": 2.0,
	})))

	body := string(h.bodyOf(t, 0))
	for _, want := range []string{"updateDocumentTabProperties", `"fields": "index,title"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}
}

// TestNarrowKeepsAFamilyWithinWhatWasAllowed is what makes a sub-path a window rather than
// a second configuration: /mcp/docs cannot offer what --tools did not.
func TestNarrowKeepsAFamilyWithinWhatWasAllowed(t *testing.T) {
	allowed, err := ParseGroups("docs-read,slides")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	docs := Narrow(allowed, "docs")
	if !docs[DocsRead] {
		t.Error("the docs window should keep reading documents")
	}
	if docs[DocsWrite] {
		t.Error("the docs window must not gain writing that --tools never allowed")
	}
	if !docs[Common] {
		t.Error("the reference belongs to every window")
	}

	slides := Narrow(allowed, "slides")
	if !slides[SlidesWrite] {
		t.Error("the slides window should keep what the family allowed")
	}
	if slides[DocsRead] {
		t.Error("a family window should hold nothing of another family")
	}

	if len(Families()) != 4 {
		t.Errorf("there should be four families to narrow to, got %v", Families())
	}
	if len(GroupNames()) != len(allGroups) {
		t.Error("every group should be nameable to an operator")
	}
}
