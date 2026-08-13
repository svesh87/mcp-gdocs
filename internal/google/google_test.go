package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fake answers a canned body for every request and records what it was asked.
type fake struct {
	body    string
	status  int
	path    string
	query   string
	method  string
	request json.RawMessage
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.path, f.query, f.method = r.URL.Path, r.URL.RawQuery, r.Method

	if r.Body != nil {
		buf := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(buf)
			f.request = json.RawMessage(buf)
		}
	}

	if f.status != 0 {
		w.WriteHeader(f.status)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(f.body))
}

func newClient(t *testing.T, f *fake) *Client {
	t.Helper()

	server := httptest.NewServer(f)
	t.Cleanup(server.Close)

	// No real waiting here: retrying is checked in retry_test.go, and every other test
	// that provokes a failure would otherwise sit through the whole backoff.
	return New(server.Client(), WithBaseURL(server.URL), WithRetryDelay(noDelay))
}

// noDelay retries at once.
func noDelay(int) time.Duration { return 0 }

func TestPresentationAsksForOnlyWhatItNeeds(t *testing.T) {
	f := &fake{body: `{"presentationId": "deck", "title": "Отчёт"}`}
	client := newClient(t, f)

	presentation, err := client.Presentation(context.Background(), "deck", "slides(objectId)")
	if err != nil {
		t.Fatalf("reading the presentation: %v", err)
	}
	if presentation.Title != "Отчёт" {
		t.Errorf("the title came back as %q", presentation.Title)
	}

	// A whole deck is megabytes of JSON; the mask is what keeps a read small.
	if !strings.Contains(f.query, "fields=slides") {
		t.Errorf("the fields mask should be sent, got %s", f.query)
	}
}

func TestErrorsCarryWhatGoogleSaid(t *testing.T) {
	f := &fake{
		status: http.StatusNotFound,
		body: `{"error": {"code": 404, "status": "NOT_FOUND",
		         "message": "Requested entity was not found."}}`,
	}
	client := newClient(t, f)

	_, err := client.Presentation(context.Background(), "missing", "")
	if err == nil {
		t.Fatal("a 404 should be an error")
	}

	var apiErr *Error
	if !asError(err, &apiErr) {
		t.Fatalf("the error should be a *google.Error, got %T", err)
	}
	if apiErr.Status != 404 || apiErr.Reason != "NOT_FOUND" {
		t.Errorf("the error is %+v", apiErr)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("the message from Google should survive: %v", err)
	}
}

func TestErrorWithoutTheUsualEnvelope(t *testing.T) {
	client := newClient(t, &fake{status: http.StatusBadGateway, body: "<html>bad gateway</html>"})

	_, err := client.Presentation(context.Background(), "deck", "")
	if err == nil || !strings.Contains(err.Error(), "bad gateway") {
		t.Errorf("a body that is not the usual envelope should still be shown, got %v", err)
	}
}

func TestThumbnailPassesItsOptions(t *testing.T) {
	f := &fake{body: `{"contentUrl": "https://example.invalid/t.png", "width": 800, "height": 450}`}
	client := newClient(t, f)

	thumbnail, err := client.Thumbnail(context.Background(), "deck", "slide1", "PNG", "MEDIUM")
	if err != nil {
		t.Fatalf("rendering the slide: %v", err)
	}
	if thumbnail.Width != 800 {
		t.Errorf("the size came back as %dx%d", thumbnail.Width, thumbnail.Height)
	}
	if !strings.Contains(f.path, "/pages/slide1/thumbnail") {
		t.Errorf("the address is %s", f.path)
	}
}

func TestExportReturnsBytesAndType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n"))
	}))
	t.Cleanup(server.Close)

	client := New(server.Client(), WithBaseURL(server.URL))

	content, contentType, err := client.ExportFile(context.Background(), "book", "text/csv")
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if string(content) != "a,b\n" || contentType != "text/csv" {
		t.Errorf("got %q of type %q", content, contentType)
	}
}

func TestExportReportsRefusals(t *testing.T) {
	client := newClient(t, &fake{
		status: http.StatusForbidden,
		body:   `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "no"}}`,
	})

	if _, _, err := client.ExportFile(context.Background(), "book", "text/csv"); err == nil {
		t.Error("a refused export should be an error")
	}
}

func TestValuesAndUpdates(t *testing.T) {
	f := &fake{body: `{"range": "A1:B2", "values": [["a", "b"]]}`}
	client := newClient(t, f)

	values, err := client.Values(context.Background(), "book", "'Лист 1'!A1:B2", "ROWS", "FORMATTED_VALUE")
	if err != nil {
		t.Fatalf("reading values: %v", err)
	}
	if len(values.Values) != 1 {
		t.Errorf("the values came back as %v", values.Values)
	}

	// A tab named in Cyrillic with a space survives the trip into the path.
	if !strings.Contains(f.path, "Лист 1") {
		t.Errorf("the range should reach the API intact, got %s", f.path)
	}

	if _, err := client.UpdateValues(context.Background(), "book", "A1", [][]any{{"a"}}, "RAW"); err != nil {
		t.Fatalf("writing values: %v", err)
	}
	if f.method != http.MethodPut {
		t.Errorf("a write over a range is a PUT, got %s", f.method)
	}
}

func TestA1Range(t *testing.T) {
	for _, test := range []struct {
		title string
		cells string
		want  string
	}{
		{"Sheet1", "A1:B2", "'Sheet1'!A1:B2"},
		{"Sheet1", "", "'Sheet1'"},
		{"", "A1:B2", "A1:B2"},
		// A quote inside a tab name is doubled, or the range would end early.
		{"Пете'с", "A1", "'Пете''с'!A1"},
	} {
		if got := A1Range(test.title, test.cells); got != test.want {
			t.Errorf("A1Range(%q, %q) = %q, want %q", test.title, test.cells, got, test.want)
		}
	}
}

func TestColumnLetters(t *testing.T) {
	for index, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 51: "AZ", 52: "BA"} {
		if got := ColumnLetters(index); got != want {
			t.Errorf("ColumnLetters(%d) = %q, want %q", index, got, want)
		}
	}
	if ColumnLetters(-1) != "" {
		t.Error("a negative column has no letters")
	}
}

func TestSheetLookup(t *testing.T) {
	spreadsheet := &Spreadsheet{Sheets: []Sheet{
		{Properties: SheetProperties{SheetID: SheetIDOf(0), Title: "Первый"}},
		{Properties: SheetProperties{SheetID: SheetIDOf(42), Title: "Второй"}},
	}}

	if id, ok := spreadsheet.SheetIDByTitle("Второй"); !ok || id != 42 {
		t.Errorf("looking up a tab gave %d, %v", id, ok)
	}
	if _, ok := spreadsheet.SheetIDByTitle("Нет"); ok {
		t.Error("an absent tab should not be found")
	}
	if titles := spreadsheet.SheetTitles(); len(titles) != 2 || titles[1] != "Второй" {
		t.Errorf("the titles came back as %v", titles)
	}
}

func TestTextStyleFieldsAreOrdered(t *testing.T) {
	bold := true
	style := TextStyle{
		Bold:       &bold,
		FontFamily: "Roboto",
		FontSize:   PT(28),
	}

	// The order is the declaration order, so the same style always produces the same
	// mask and a golden file stays stable.
	got := strings.Join(style.Fields(), ",")
	if got != "bold,fontFamily,fontSize" {
		t.Errorf("the mask is %q", got)
	}

	if (TextStyle{}).IsEmpty() != true {
		t.Error("a style with nothing set is empty")
	}
	if style.IsEmpty() {
		t.Error("a style with fields set is not empty")
	}
}

func TestDocumentPlainText(t *testing.T) {
	document := &Document{Body: &DocsBody{Content: []StructuralElement{
		{Paragraph: &DocsParagraph{Elements: []DocsParagraphElement{
			{TextRun: &DocsTextRun{Content: "Абзац\n"}},
		}}},
		{Table: &DocsTable{Content: []DocsTableRow{
			{Cells: []DocsTableCell{
				{Content: []StructuralElement{{Paragraph: &DocsParagraph{Elements: []DocsParagraphElement{
					{TextRun: &DocsTextRun{Content: "Ключ\n"}},
				}}}}},
				{Content: []StructuralElement{{Paragraph: &DocsParagraph{Elements: []DocsParagraphElement{
					{TextRun: &DocsTextRun{Content: "Значение\n"}},
				}}}}},
			}},
		}}},
	}}}

	if got := document.PlainText(); got != "Абзац\nКлюч\tЗначение\n" {
		t.Errorf("the document reads as %q", got)
	}

	if (&Document{}).PlainText() != "" {
		t.Error("a document with no body reads as nothing")
	}
}

func TestSearchFilesReachesSharedDrives(t *testing.T) {
	f := &fake{body: `{"files": [{"id": "1", "name": "Шаблон"}]}`}
	client := newClient(t, f)

	if _, err := client.SearchFiles(context.Background(), SearchOptions{
		Query: "name contains 'Шаблон'", PageSize: 10, PageToken: "tok", OrderBy: "modifiedTime desc",
	}); err != nil {
		t.Fatalf("searching: %v", err)
	}

	for _, want := range []string{"corpora=allDrives", "includeItemsFromAllDrives=true", "pageSize=10", "pageToken=tok"} {
		if !strings.Contains(f.query, want) {
			t.Errorf("the search should carry %s, got %s", want, f.query)
		}
	}
}

func TestExportSheetHTMLAsksTheEditors(t *testing.T) {
	var asked string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path + "?" + r.URL.RawQuery
		_, _ = w.Write([]byte("PK\x03\x04zip"))
	}))
	t.Cleanup(server.Close)

	client := New(server.Client(), WithBaseURL(server.URL))

	content, err := client.ExportSheetHTML(context.Background(), "book")
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if string(content) != "PK\x03\x04zip" {
		t.Errorf("the export came back as %q", content)
	}
	// The zipped HTML is served by the editors, not by Drive: Drive answers "The requested
	// conversion is not supported" for it.
	if asked != "/editors/spreadsheets/d/book/export?format=zip" {
		t.Errorf("the export was asked for at %s", asked)
	}
}

func TestExportSheetHTMLReportsRefusals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("no"))
	}))
	t.Cleanup(server.Close)

	client := New(server.Client(), WithBaseURL(server.URL))

	if _, err := client.ExportSheetHTML(context.Background(), "book"); err == nil {
		t.Error("a refused export should be an error")
	}
}

// asError is errors.As without the import, kept local so the test reads in one place.
func asError(err error, target **Error) bool {
	for err != nil {
		if typed, ok := err.(*Error); ok {
			*target = typed
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}
