package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recorder answers every path with a canned body and keeps what it was asked, so one
// test can walk several calls in a row.
type recorder struct {
	responses map[string]string
	calls     []string
	bodies    []string
}

func (rec *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec.calls = append(rec.calls, r.Method+" "+r.URL.Path)

	body := make([]byte, r.ContentLength)
	if r.ContentLength > 0 {
		_, _ = r.Body.Read(body)
	}
	rec.bodies = append(rec.bodies, string(body))

	for fragment, response := range rec.responses {
		if strings.Contains(r.URL.Path, fragment) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(response))
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{}`))
}

func newRecorder(t *testing.T, responses map[string]string) (*Client, *recorder) {
	t.Helper()

	rec := &recorder{responses: responses}
	server := httptest.NewServer(rec)
	t.Cleanup(server.Close)

	return New(server.Client(), WithBaseURL(server.URL)), rec
}

func TestSpreadsheetCalls(t *testing.T) {
	client, rec := newRecorder(t, map[string]string{
		"/spreadsheets": `{"spreadsheetId": "book", "properties": {"title": "Отчёт"},
		                   "sheets": [{"properties": {"sheetId": 0, "title": "Данные"}}]}`,
	})

	ctx := context.Background()

	spreadsheet, err := client.Spreadsheet(ctx, "book")
	if err != nil || spreadsheet.Properties.Title != "Отчёт" {
		t.Fatalf("reading a workbook: %v %+v", err, spreadsheet)
	}

	if _, err := client.CreateSpreadsheet(ctx, CreateSpreadsheetRequest{
		Properties: SpreadsheetProperties{Title: "Новый", Locale: "ru_RU"},
		Sheets: []Sheet{
			{Properties: SheetProperties{Title: "Данные"}},
			{Properties: SheetProperties{Title: "Сводка", GridProps: &GridProps{RowCount: 993, ColumnCount: 31}}},
		},
	}); err != nil {
		t.Fatalf("creating a workbook: %v", err)
	}
	if !strings.Contains(rec.bodies[1], `"title":"Сводка"`) {
		t.Errorf("the tabs should be named in the request, got %s", rec.bodies[1])
	}
	// The size travels with the tab: it is the only moment it can be set without deleting
	// rows later.
	if !strings.Contains(rec.bodies[1], `"rowCount":993`) || !strings.Contains(rec.bodies[1], `"locale":"ru_RU"`) {
		t.Errorf("the size and the locale should be in the request, got %s", rec.bodies[1])
	}

	if _, err := client.AppendValues(ctx, "book", "A:B", [][]any{{"a", "b"}}, "USER_ENTERED"); err != nil {
		t.Fatalf("appending: %v", err)
	}

	if _, err := client.SheetsBatchUpdate(ctx, "book", []SheetsRequest{{
		AddSheet: &AddSheetRequest{Properties: SheetProperties{Title: "Ещё"}},
	}}); err != nil {
		t.Fatalf("batch update: %v", err)
	}
	if !strings.Contains(rec.calls[3], ":batchUpdate") {
		t.Errorf("the batch should go to :batchUpdate, got %s", rec.calls[3])
	}
}

func TestDocumentCalls(t *testing.T) {
	client, rec := newRecorder(t, map[string]string{
		"/documents": `{"documentId": "doc", "title": "Регламент"}`,
	})

	ctx := context.Background()

	document, err := client.Document(ctx, "doc")
	if err != nil || document.Title != "Регламент" {
		t.Fatalf("reading a document: %v %+v", err, document)
	}

	if _, err := client.CreateDocument(ctx, "Черновик"); err != nil {
		t.Fatalf("creating a document: %v", err)
	}
	if !strings.Contains(rec.bodies[1], "Черновик") {
		t.Errorf("the title should be in the request, got %s", rec.bodies[1])
	}

	if _, err := client.DocsBatchUpdate(ctx, "doc", []DocsRequest{{
		InsertText: &DocsInsertText{EndOfDoc: &DocsSegmentEnd{}, Text: "конец"},
	}}); err != nil {
		t.Fatalf("batch update: %v", err)
	}
	if !strings.Contains(rec.bodies[2], "endOfSegmentLocation") {
		t.Errorf("appending should aim at the end of the body, got %s", rec.bodies[2])
	}
}

func TestDriveCalls(t *testing.T) {
	client, rec := newRecorder(t, map[string]string{
		"/files": `{"id": "deck1", "name": "Шаблон", "mimeType": "application/vnd.google-apps.presentation"}`,
	})

	ctx := context.Background()

	file, err := client.FileMetadata(ctx, "deck1")
	if err != nil || file.Name != "Шаблон" {
		t.Fatalf("reading a file: %v %+v", err, file)
	}

	if _, err := client.CopyFile(ctx, "deck1", "Копия", []string{"folder1"}); err != nil {
		t.Fatalf("copying a file: %v", err)
	}
	if !strings.Contains(rec.bodies[1], `"parents":["folder1"]`) {
		t.Errorf("the destination folder should be named, got %s", rec.bodies[1])
	}
	if !strings.Contains(rec.calls[1], "/copy") {
		t.Errorf("the copy should go to /copy, got %s", rec.calls[1])
	}
}

func TestSlidesBatchUpdateSendsWhatItWasGiven(t *testing.T) {
	client, rec := newRecorder(t, map[string]string{
		":batchUpdate": `{"presentationId": "deck", "replies": [{"createTable": {"objectId": "t1"}}]}`,
	})

	response, err := client.SlidesBatchUpdate(context.Background(), "deck", []Request{{
		InsertText: &InsertTextRequest{ObjectID: "body1", Text: "Текст", InsertionIndex: 0},
	}})
	if err != nil {
		t.Fatalf("batch update: %v", err)
	}
	if len(response.Replies) != 1 || response.Replies[0].CreateTable.ObjectID != "t1" {
		t.Errorf("the replies came back as %+v", response.Replies)
	}

	// The body is what this server is worth: it goes out exactly as it was built.
	if !strings.Contains(rec.bodies[0], `"insertText":{"objectId":"body1","text":"Текст","insertionIndex":0}`) {
		t.Errorf("the request body is %s", rec.bodies[0])
	}
}

func TestDefaultBaseURLs(t *testing.T) {
	client := New(http.DefaultClient)

	// Without the test option the client talks to Google itself.
	for name, got := range map[string]string{
		"drive":  client.driveBase,
		"sheets": client.sheetsBase,
		"slides": client.slidesBase,
		"docs":   client.docsBase,
	} {
		if !strings.HasPrefix(got, "https://") {
			t.Errorf("the %s address is %q", name, got)
		}
	}
}
