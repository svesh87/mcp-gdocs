package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// filesHarness is a registry that may touch one directory, wired to a fake Google.
func filesHarness(t *testing.T, fake *fakeGoogle) (*harness, string) {
	t.Helper()

	h := newHarness(t, fake)
	dir := t.TempDir()
	h.registry.opts.FilesDir = dir

	return h, dir
}

// TestFilePathStaysInsideTheDirectory is the whole reason the file tools take a name
// rather than a path: a server that writes where it is told writes over its own token.
func TestFilePathStaysInsideTheDirectory(t *testing.T) {
	h, dir := filesHarness(t, newFakeGoogle(t))

	inside, err := h.registry.filePath("decks/sample.pdf")
	if err != nil {
		t.Fatalf("a plain name should be allowed: %v", err)
	}
	if !strings.HasPrefix(inside, dir) {
		t.Errorf("the path should be inside %s, got %s", dir, inside)
	}

	for _, name := range []string{
		"../outside.pdf",
		"decks/../../outside.pdf",
		"/etc/passwd",
		"",
	} {
		if _, err := h.registry.filePath(name); err == nil {
			t.Errorf("%q should be refused", name)
		}
	}
}

func TestFileToolsAreAbsentWithoutADirectory(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	if _, err := h.registry.filePath("anything.pdf"); err == nil ||
		!strings.Contains(err.Error(), "--files-dir") {
		t.Errorf("without a directory the file tools should say so, got %v", err)
	}
}

func TestExportFileWritesToDisk(t *testing.T) {
	fake := newFakeGoogle(t).answer("/files/deck/export", "%PDF-1.7 pretend")
	h, dir := filesHarness(t, fake)

	answer := h.ok(h.registry.exportFile(context.Background(), request(map[string]any{
		"file_id": "deck",
		"format":  "pdf",
		"save_as": "reports/deck.pdf",
	})))

	path := filepath.Join(dir, "reports", "deck.pdf")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the export should be on disk: %v", err)
	}
	if !strings.HasPrefix(string(content), "%PDF") {
		t.Errorf("the file holds %q", content)
	}
	if !strings.Contains(answer, "reports/deck.pdf") {
		t.Errorf("the answer should name the path, got %s", answer)
	}

	// The format is named rather than spelled as a MIME type, and the MIME type is what
	// reaches Drive.
	if query := h.google.requests[0].Query; !strings.Contains(query, "mimeType=application%2Fpdf") {
		t.Errorf("the export should ask Drive for a PDF, got %s", query)
	}
}

func TestExportFileRefusesAFormatItDoesNotKnow(t *testing.T) {
	h, _ := filesHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.exportFile(context.Background(), request(map[string]any{
		"file_id": "deck",
		"format":  "keynote",
		"save_as": "deck.key",
	})))

	if !strings.Contains(message, "pdf") || !strings.Contains(message, "pptx") {
		t.Errorf("the refusal should list the formats, got %q", message)
	}
}

// TestDownloadFileSavesTheBytesAsTheyAre covers the half of the job export cannot do: a
// PDF that was uploaded rather than authored has no conversion to ask for, and alt=media is
// what tells Drive to answer with content instead of metadata.
func TestDownloadFileSavesTheBytesAsTheyAre(t *testing.T) {
	fake := newFakeGoogle(t).answer("/files/brandbook", "%PDF-1.7 pretend brandbook")
	h, dir := filesHarness(t, fake)

	answer := h.ok(h.registry.downloadFile(context.Background(), request(map[string]any{
		"file_id": "brandbook",
		"save_as": "brand/brandbook.pdf",
	})))

	content, err := os.ReadFile(filepath.Join(dir, "brand", "brandbook.pdf"))
	if err != nil {
		t.Fatalf("the file should be on disk: %v", err)
	}
	if !strings.HasPrefix(string(content), "%PDF") {
		t.Errorf("the file holds %q", content)
	}
	if !strings.Contains(answer, "brand/brandbook.pdf") {
		t.Errorf("the answer should name the path, got %s", answer)
	}

	if query := h.google.requests[0].Query; !strings.Contains(query, "alt=media") {
		t.Errorf("the download should ask for the content, got %s", query)
	}
}

// TestDownloadFileKeepsTheDriveName is why save_as is optional: a caller that found a file
// by searching knows its name and should not have to repeat it, and a name with a slash in
// it must not decide which directory the file lands in.
func TestDownloadFileKeepsTheDriveName(t *testing.T) {
	fake := newFakeGoogle(t).answer("/files/kit", `{"id": "kit", "name": "decks/Media kit.pdf"}`)
	h, dir := filesHarness(t, fake)

	answer := h.ok(h.registry.downloadFile(context.Background(), request(map[string]any{
		"file_id": "kit",
	})))

	if _, err := os.Stat(filepath.Join(dir, "Media kit.pdf")); err != nil {
		t.Errorf("the file should keep its own name, without the directory in it: %v", err)
	}
	if strings.Contains(answer, "decks/Media kit.pdf") {
		t.Errorf("the name's directory should have been dropped, got %s", answer)
	}
}

// TestDownloadFileRefusesAnEditorFile pins the error, not the failure: Drive says only that
// the content is not binary, and the caller needs to be told which tool does this instead.
func TestDownloadFileRefusesAnEditorFile(t *testing.T) {
	fake := newFakeGoogle(t).fail("/files/doc", 403,
		`{"error": {"code": 403, "status": "PERMISSION_DENIED", "message":
		   "Only files with binary content can be downloaded. Use Export with Docs Editors files."}}`)
	h, _ := filesHarness(t, fake)

	message := h.fail(h.registry.downloadFile(context.Background(), request(map[string]any{
		"file_id": "doc",
		"save_as": "doc.bin",
	})))

	if !strings.Contains(message, "gdocs_drive_export_file") {
		t.Errorf("the refusal should name the tool that does convert, got %q", message)
	}
}

// TestExportSlideImagesRendersTheWholeDeck pins the loop a visual check depends on: every
// slide, in order, named so a listing sorts them the way the deck runs.
func TestExportSlideImagesRendersTheWholeDeck(t *testing.T) {
	pictures := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n pretend"))
	}))
	t.Cleanup(pictures.Close)

	fake := newFakeGoogle(t).
		answer("/thumbnail", `{"contentUrl": "`+pictures.URL+`/render.png", "width": 800, "height": 450}`).
		answer("/presentations/deck", `{"presentationId": "deck", "slides": [
		   {"objectId": "slide1"}, {"objectId": "slide2"}]}`)
	h, dir := filesHarness(t, fake)

	answer := h.ok(h.registry.exportSlideImages(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"name_prefix":     "check/ours",
	})))

	for _, name := range []string{"check/ours_00.png", "check/ours_01.png"} {
		if _, err := os.ReadFile(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should be on disk: %v", name, err)
		}
	}
	if !strings.Contains(answer, `"slides": 2`) {
		t.Errorf("the answer should count the slides, got %s", answer)
	}
}

func TestExportSlideImagesRefusesASizeItDoesNotKnow(t *testing.T) {
	h, _ := filesHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.exportSlideImages(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"name_prefix":     "x",
		"size":            "HUGE",
	})))

	if !strings.Contains(message, "SMALL, MEDIUM or LARGE") {
		t.Errorf("the refusal should list the sizes, got %q", message)
	}
}

// TestImportFileConvertsByExtension pins the point of the tool: a .pptx uploaded without
// a target type stays an attachment nobody can edit, and every other tool here would
// refuse it.
func TestImportFileConvertsByExtension(t *testing.T) {
	fake := newFakeGoogle(t).answer("/upload/drive/v3/files",
		`{"id": "new1", "name": "Отчёт", "mimeType": "application/vnd.google-apps.presentation"}`)
	h, dir := filesHarness(t, fake)

	source := filepath.Join(dir, "Отчёт.pptx")
	if err := os.WriteFile(source, []byte("PK pretend pptx"), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}

	answer := h.ok(h.registry.importFile(context.Background(), request(map[string]any{
		"path": "Отчёт.pptx",
	})))

	if !strings.Contains(answer, `"converted": true`) {
		t.Errorf("a .pptx should be converted, got %s", answer)
	}

	body := string(h.google.requests[0].Body)
	if !strings.Contains(body, google.MimePresentation) {
		t.Errorf("the upload should ask for a Google presentation, got %s", body)
	}
	if !strings.Contains(body, "PK pretend pptx") {
		t.Errorf("the upload should carry the file's bytes, got %s", body)
	}
}

func TestImportFileKeepsTheOriginalWhenAsked(t *testing.T) {
	fake := newFakeGoogle(t).answer("/upload/drive/v3/files",
		`{"id": "new2", "name": "deck.pptx", "mimeType": "application/vnd.openxmlformats-officedocument.presentationml.presentation"}`)
	h, dir := filesHarness(t, fake)

	if err := os.WriteFile(filepath.Join(dir, "deck.pptx"), []byte("PK"), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}

	answer := h.ok(h.registry.importFile(context.Background(), request(map[string]any{
		"path":                 "deck.pptx",
		"keep_original_format": true,
	})))

	if !strings.Contains(answer, `"converted": false`) {
		t.Errorf("the file should stay as it is, got %s", answer)
	}
}

func TestImportFileRefusesWhatIsNotThere(t *testing.T) {
	h, _ := filesHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.importFile(context.Background(), request(map[string]any{
		"path": "missing.pptx",
	})))

	if !strings.Contains(message, "missing.pptx") {
		t.Errorf("the refusal should name the file, got %q", message)
	}
}

// TestExportedFilesAreNotReadableByEveryone keeps somebody's document from landing in a
// shared directory world-readable.
func TestExportedFilesAreNotReadableByEveryone(t *testing.T) {
	fake := newFakeGoogle(t).answer("/files/deck/export", "text")
	h, dir := filesHarness(t, fake)

	h.ok(h.registry.exportFile(context.Background(), request(map[string]any{
		"file_id": "deck",
		"format":  "txt",
		"save_as": "notes.txt",
	})))

	info, err := os.Stat(filepath.Join(dir, "notes.txt"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("an exported document should be 0600, it is %o", mode)
	}
}

// TestFileToolsRegisterOnlyWithADirectory keeps a server with no place to write from
// offering tools that would fail on every call, and keeps importing behind --allow-write:
// reading a deck out is not the same act as putting one in.
func TestFileToolsRegisterOnlyWithADirectory(t *testing.T) {
	without := registeredTools(t, true)
	for _, name := range []string{"gdocs_drive_export_file", "gdocs_slides_export_images", "gdocs_drive_import_file"} {
		if contains(without, name) {
			t.Errorf("%s should not be offered without --files-dir", name)
		}
	}

	writing := registeredFileTools(t, t.TempDir(), true)
	for _, name := range []string{"gdocs_drive_export_file", "gdocs_slides_export_images", "gdocs_drive_import_file"} {
		if !contains(writing, name) {
			t.Errorf("%s should be offered with a files directory, the server offers %v", name, writing)
		}
	}

	readOnly := registeredFileTools(t, t.TempDir(), false)
	if !contains(readOnly, "gdocs_drive_export_file") {
		t.Error("exporting is reading and should be offered without --allow-write")
	}
	if contains(readOnly, "gdocs_drive_import_file") {
		t.Error("importing creates a file on Drive and should need --allow-write")
	}
}

// registeredFileTools lists what a server with a files directory offers.
func registeredFileTools(t *testing.T, dir string, allowWrite bool) []string {
	t.Helper()

	mcpServer := server.NewMCPServer("mcp-gdocs", "test", server.WithToolCapabilities(true))

	err := Register(mcpServer, Options{
		Clients:    ClientFunc(func(context.Context) (*google.Client, error) { return nil, errNoClient }),
		AllowWrite: allowWrite,
		FilesDir:   dir,
	})
	if err != nil {
		t.Fatalf("registering the tools: %v", err)
	}

	answer := mcpServer.HandleMessage(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))

	response, ok := answer.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("listing the tools failed: %+v", answer)
	}

	encoded, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatalf("encoding the listing: %v", err)
	}

	var listing struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(encoded, &listing); err != nil {
		t.Fatalf("decoding the listing: %v", err)
	}

	names := make([]string, 0, len(listing.Tools))
	for _, tool := range listing.Tools {
		names = append(names, tool.Name)
	}

	return names
}

// TestUploadIsMultipart checks the shape of the request Drive needs: metadata and content
// in one body, which is the part that is easy to get subtly wrong.
func TestUploadIsMultipart(t *testing.T) {
	fake := newFakeGoogle(t).answer("/upload/drive/v3/files", `{"id": "new3", "name": "x"}`)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	client := google.New(server.Client(), google.WithBaseURL(server.URL))

	if _, err := client.UploadFile(context.Background(), "Отчёт", "text/csv",
		google.MimeSpreadsheet, "folder1", []byte("a,b\n")); err != nil {
		t.Fatalf("uploading: %v", err)
	}

	body := string(fake.requests[0].Body)
	for _, want := range []string{"application/json", "text/csv", "a,b", "folder1", "Отчёт"} {
		if !strings.Contains(body, want) {
			t.Errorf("the upload should carry %q, got %s", want, body)
		}
	}

	var metadata map[string]any
	start := strings.Index(body, "{")
	end := strings.Index(body[start:], "}") + start + 1
	if err := json.Unmarshal([]byte(body[start:end]), &metadata); err != nil {
		t.Fatalf("the metadata part is not JSON: %v", err)
	}
	if metadata["mimeType"] != google.MimeSpreadsheet {
		t.Errorf("the metadata should ask for a spreadsheet, got %v", metadata)
	}
}
