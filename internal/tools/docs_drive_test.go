package tools

import (
	"context"
	"strings"
	"testing"
)

const documentBody = `{
  "documentId": "doc",
  "title": "Регламент",
  "body": {"content": [
    {"paragraph": {"elements": [{"textRun": {"content": "Первый абзац\n"}}]}},
    {"table": {"rows": 1, "columns": 2, "tableRows": [
      {"tableCells": [
        {"content": [{"paragraph": {"elements": [{"textRun": {"content": "Ключ\n"}}]}}]},
        {"content": [{"paragraph": {"elements": [{"textRun": {"content": "Значение\n"}}]}}]}
      ]}
    ]}}
  ]}
}`

func TestDocsRead(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/documents/doc", documentBody))

	answer := h.ok(h.registry.docsRead(context.Background(), request(map[string]any{
		"document_id": "doc",
	})))

	// A table comes back row by row with tabs between cells, so a reader gets the shape
	// of it rather than a wall of words.
	if !strings.Contains(answer, `Ключ\tЗначение`) {
		t.Errorf("the table should come out tab-separated, got %s", answer)
	}
	if !strings.Contains(answer, "Первый абзац") {
		t.Errorf("the paragraph should be there, got %s", answer)
	}
}

func TestDocsAppend(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer(":batchUpdate", `{"documentId": "doc"}`))

	h.ok(h.registry.docsAppend(context.Background(), request(map[string]any{
		"document_id": "doc",
		"text":        "Ещё абзац\n",
	})))

	checkGolden(t, "docs_append.json", h.bodyOf(t, 0))
}

func TestDocsInsertTextRefusesIndexZero(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	if message := h.fail(h.registry.docsInsertText(context.Background(), request(map[string]any{
		"document_id": "doc",
		"index":       float64(0),
		"text":        "x",
	}))); !strings.Contains(message, "starts at 1") {
		t.Errorf("expected a refusal explaining where a document starts, got %q", message)
	}
}

func TestDocsInsertText(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer(":batchUpdate", `{"documentId": "doc"}`))

	h.ok(h.registry.docsInsertText(context.Background(), request(map[string]any{
		"document_id": "doc",
		"index":       float64(14),
		"text":        "вставка",
	})))

	checkGolden(t, "docs_insert_text.json", h.bodyOf(t, 0))
}

func TestDocsReplaceText(t *testing.T) {
	fake := newFakeGoogle(t).answer(":batchUpdate",
		`{"documentId": "doc", "replies": [{"replaceAllText": {"occurrencesChanged": 3}}]}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.docsReplaceText(context.Background(), request(map[string]any{
		"document_id": "doc",
		"find":        "{{имя}}",
		"replace":     "Аня",
	})))

	if !strings.Contains(answer, `"occurrences": 3`) {
		t.Errorf("the count of replacements should come back, got %s", answer)
	}

	checkGolden(t, "docs_replace_text.json", h.bodyOf(t, 0))
}

func TestDocsReplaceTextRefusesEmptyNeedle(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	if message := h.fail(h.registry.docsReplaceText(context.Background(), request(map[string]any{
		"document_id": "doc",
		"find":        "   ",
		"replace":     "x",
	}))); !strings.Contains(message, "find is empty") {
		t.Errorf("expected a refusal about the empty needle, got %q", message)
	}
}

func TestDocsCreate(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/documents", `{"documentId": "new_doc", "title": "Черновик"}`))

	answer := h.ok(h.registry.docsCreate(context.Background(), request(map[string]any{
		"title": "Черновик",
	})))

	if !strings.Contains(answer, "new_doc") {
		t.Errorf("the new document's identifier should come back, got %s", answer)
	}
}

const driveListing = `{
  "files": [
    {"id": "deck1", "name": "Шаблон колоды", "mimeType": "application/vnd.google-apps.presentation",
     "modifiedTime": "2026-08-01T10:00:00Z", "webViewLink": "https://example.invalid/d/deck1",
     "owners": [{"displayName": "Отдел"}], "parents": ["folder1"]}
  ],
  "nextPageToken": "next"
}`

func TestDriveSearchByKind(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/files", driveListing))

	answer := h.ok(h.registry.driveSearch(context.Background(), request(map[string]any{
		"query": "name contains 'Шаблон'",
		"kind":  "presentation",
	})))

	if !strings.Contains(answer, "deck1") || !strings.Contains(answer, `"next_page_token": "next"`) {
		t.Errorf("the listing and its page token should come back, got %s", answer)
	}

	query := h.google.requests[0].Query
	// Shared drives have to be asked for explicitly, and a template deck almost always
	// lives on one.
	for _, want := range []string{"corpora=allDrives", "includeItemsFromAllDrives=true", "supportsAllDrives=true"} {
		if !strings.Contains(query, want) {
			t.Errorf("the search should carry %s, got %s", want, query)
		}
	}
	if !strings.Contains(query, "vnd.google-apps.presentation") {
		t.Errorf("the kind should have become a mimeType clause, got %s", query)
	}
}

func TestDriveSearchRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	if message := h.fail(h.registry.driveSearch(context.Background(), request(map[string]any{
		"kind": "picture",
	}))); !strings.Contains(message, "spreadsheet, presentation, document or folder") {
		t.Errorf("expected a refusal naming the kinds, got %q", message)
	}

	if message := h.fail(h.registry.driveSearch(context.Background(), request(map[string]any{
		"limit": float64(500),
	}))); !strings.Contains(message, "outside 1..100") {
		t.Errorf("expected a refusal about the limit, got %q", message)
	}
}

func TestDriveFileInfo(t *testing.T) {
	fake := newFakeGoogle(t).answer("/files/deck1",
		`{"id": "deck1", "name": "Шаблон колоды", "mimeType": "application/vnd.google-apps.presentation",
		  "owners": [{"displayName": "Отдел", "emailAddress": "team@example.invalid"}]}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.driveFileInfo(context.Background(), request(map[string]any{
		"file_id": "deck1",
	})))

	if !strings.Contains(answer, "Шаблон колоды") {
		t.Errorf("the file's name should come back, got %s", answer)
	}
	// Owners are reported by name only: an address is personal data that nobody asked
	// this tool for.
	if strings.Contains(answer, "team@example.invalid") {
		t.Errorf("an owner's address should not be handed out, got %s", answer)
	}
}

func TestDriveExportText(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/export", "Имя,Отдел\nАня,SRE\n"))

	answer := h.ok(h.registry.driveExport(context.Background(), request(map[string]any{
		"file_id":   "book",
		"mime_type": "text/csv",
	})))

	if !strings.Contains(answer, "Аня,SRE") {
		t.Errorf("text should come back as text, got %s", answer)
	}
	if strings.Contains(answer, "content_base64") {
		t.Errorf("text should not be encoded, got %s", answer)
	}
}

func TestDriveExportBinary(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/export", "\xff\xfe\x00binary"))

	answer := h.ok(h.registry.driveExport(context.Background(), request(map[string]any{
		"file_id":   "doc",
		"mime_type": "application/pdf",
	})))

	if !strings.Contains(answer, "content_base64") {
		t.Errorf("bytes that are not text should come back encoded, got %s", answer)
	}
}

func TestDriveCopy(t *testing.T) {
	fake := newFakeGoogle(t).answer("/files/deck1/copy",
		`{"id": "deck2", "name": "Колода на квартал", "webViewLink": "https://example.invalid/d/deck2"}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.driveCopy(context.Background(), request(map[string]any{
		"file_id": "deck1",
		"name":    "Колода на квартал",
	})))

	if !strings.Contains(answer, "deck2") {
		t.Errorf("the copy's identifier should come back, got %s", answer)
	}
}
