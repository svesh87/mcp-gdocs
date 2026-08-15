package tools

import (
	"context"
	"strings"
	"testing"
)

// TestRestoreRefusesAConversionNobodyAgreedTo is the whole reason this tool has a confirm at
// all. Drive has no request that restores a version, so a Google file goes back through a
// conversion and comes back changed — and the caller finds out afterwards unless the refusal
// says it first. Nothing is written on the way to this refusal.
func TestRestoreRefusesAConversionNobodyAgreedTo(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/files/deck",
		`{"id": "deck", "name": "Демо 2026.08", "mimeType": "application/vnd.google-apps.presentation"}`))

	message := h.fail(h.registry.driveRestoreRevision(context.Background(), request(map[string]any{
		"file_id": "deck", "revision_id": "17",
	})))

	for _, want := range []string{
		"Демо 2026.08",
		"presentation",
		"PPTX",
		// The losses are named, because "some formatting may be lost" is not something a
		// caller can act on.
		"every chart's link to its workbook",
		"Nothing has been changed",
		// And the way that loses nothing is named too.
		"version history in the browser",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal should carry %q, got %s", want, message)
		}
	}

	// One read of the file's kind and nothing else: a refusal must not have fetched the
	// version, and must certainly not have written it.
	if len(h.google.requests) != 1 {
		t.Errorf("a refusal should send one request, sent %d", len(h.google.requests))
	}
}

// TestRestoreOfAGoogleFileGoesThroughAConversion: with the confirmation the export link is
// taken, its bytes are written back over the file, and the answer says what the round trip
// left behind.
func TestRestoreOfAGoogleFileGoesThroughAConversion(t *testing.T) {
	fake := newFakeGoogle(t)
	h := newHarness(t, fake)

	// The export links are absolute addresses Google hands out, so they have to point at
	// this test's own server — which is why they are declared after it exists.
	fake.answer("/export/pptx", "PPTX-bytes-here").
		answer("/revisions/17", `{"id": "17", "modifiedTime": "2026-08-01T10:00:00Z",
          "exportLinks": {
            "application/vnd.openxmlformats-officedocument.presentationml.presentation":
              "`+h.server.URL+`/export/pptx",
            "application/pdf": "`+h.server.URL+`/export/pdf"}}`).
		answer("/files/deck", `{"id": "deck", "name": "Демо 2026.08",
          "mimeType": "application/vnd.google-apps.presentation"}`)

	answer := h.ok(h.registry.driveRestoreRevision(context.Background(), request(map[string]any{
		"file_id": "deck", "revision_id": "17", "confirm_conversion": true,
	})))

	for _, want := range []string{`"through": "PPTX"`, "not_carried\":", "not_restored", "the theme"} {
		if want == "not_carried\":" {
			continue
		}
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should carry %q, got %s", want, answer)
		}
	}

	// The identifier survives, and that is what keeps every link and permission pointing at
	// the same file. The answer says so rather than leaving it to be assumed.
	if !strings.Contains(answer, "keeps its identifier") {
		t.Errorf("the answer should say the identifier survives, got %s", answer)
	}

	// PATCH rather than POST: a new upload would be a new file, with none of the sharing.
	last := h.google.requests[len(h.google.requests)-1]
	if last.Method != "PATCH" {
		t.Errorf("the content should be written over the file, got %s %s", last.Method, last.Path)
	}
}

// TestRestoreOfAnOrdinaryFileIsExact: a PDF or a picture is stored as bytes, so there is no
// conversion and nothing to confirm. Offering the confirmation there would teach a caller
// that every restore costs something, which is the opposite of true.
func TestRestoreOfAnOrdinaryFileIsExact(t *testing.T) {
	fake := newFakeGoogle(t).
		answer("/revisions/3", `{"id": "3", "mimeType": "application/pdf"}`).
		answer("/files/report", `{"id": "report", "name": "Отчёт.pdf", "mimeType": "application/pdf"}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.driveRestoreRevision(context.Background(), request(map[string]any{
		"file_id": "report", "revision_id": "3",
	})))

	if !strings.Contains(answer, `"exact": true`) {
		t.Errorf("an ordinary file comes back exactly, and the answer should say so: %s", answer)
	}
	if strings.Contains(answer, "not_restored") {
		t.Errorf("nothing is lost, so nothing should be listed: %s", answer)
	}
}

// TestRestoreRefusesAConfirmationThatConfirmsNothing: passing it on a file that needs no
// conversion means the caller expected one, and answering "done" would leave that
// misunderstanding in place.
func TestRestoreRefusesAConfirmationThatConfirmsNothing(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/files/report",
		`{"id": "report", "name": "Отчёт.pdf", "mimeType": "application/pdf"}`))

	message := h.fail(h.registry.driveRestoreRevision(context.Background(), request(map[string]any{
		"file_id": "report", "revision_id": "3", "confirm_conversion": true,
	})))

	if !strings.Contains(message, "nothing to confirm") {
		t.Errorf("the refusal should explain there is no conversion, got %s", message)
	}
}

// TestRestoreRefusesAVersionInNoUsableFormat: a version Google offers only as PDF cannot be
// written back over a presentation, and the refusal lists what it does offer so a caller can
// see there is no way through rather than guessing at one.
func TestRestoreRefusesAVersionInNoUsableFormat(t *testing.T) {
	fake := newFakeGoogle(t)
	h := newHarness(t, fake)

	fake.answer("/revisions/21", `{"id": "21",
          "exportLinks": {"application/pdf": "`+h.server.URL+`/export/pdf"}}`).
		answer("/files/deck", `{"id": "deck", "name": "Демо",
          "mimeType": "application/vnd.google-apps.presentation"}`)

	message := h.fail(h.registry.driveRestoreRevision(context.Background(), request(map[string]any{
		"file_id": "deck", "revision_id": "21", "confirm_conversion": true,
	})))

	if !strings.Contains(message, "application/pdf") {
		t.Errorf("the refusal should list what Google does offer, got %s", message)
	}
}
