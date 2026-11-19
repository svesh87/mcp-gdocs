package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// The tests in this file pin the things the APIs do that their reference pages do not say
// in one place: a chart's range that does not grow, a chart on a slide that does not
// refresh, a table row whose removal takes its neighbours, a picture that has to be public
// to be inserted. Each was found by a deck or a workbook coming out wrong.

// TestUpdateChartChangesOneFieldAndSendsTheRestBack is the partial edit, checked whole.
//
// updateChartSpec has no field mask, so "only the title changes" cannot be asked of Google:
// it has to be true of the specification this server sends. The request is compared with
// what was read, field for field — the title differs and nothing else does, including the
// ranges the data is drawn from and the fields this server has no name for.
func TestUpdateChartChangesOneFieldAndSendsTheRestBack(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetForObjects))

	h.ok(h.registry.sheetsUpdateChart(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "chart_id": 5.0, "title": "Стало",
	})))

	var stored struct {
		Sheets []struct {
			Charts []struct {
				Spec map[string]any `json:"spec"`
			} `json:"charts"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(spreadsheetForObjects), &stored); err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	want := stored.Sheets[0].Charts[0].Spec
	want["title"] = "Стало"

	var sent struct {
		Requests []struct {
			UpdateChartSpec *struct {
				ChartID int            `json:"chartId"`
				Spec    map[string]any `json:"spec"`
				Fields  *string        `json:"fields"`
			} `json:"updateChartSpec"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(h.bodyOf(t, len(h.google.requests)-1), &sent); err != nil {
		t.Fatalf("reading the request: %v", err)
	}
	if len(sent.Requests) != 1 || sent.Requests[0].UpdateChartSpec == nil {
		t.Fatalf("a change of title should be one updateChartSpec, got %+v", sent.Requests)
	}

	update := sent.Requests[0].UpdateChartSpec
	if update.ChartID != 5 {
		t.Errorf("the request should address chart 5, got %d", update.ChartID)
	}
	// The request has no such field, and Google refuses a batch that carries an unknown one.
	if update.Fields != nil {
		t.Errorf("updateChartSpec takes no field mask, and one was sent: %q", *update.Fields)
	}
	if !reflect.DeepEqual(update.Spec, want) {
		t.Errorf("only the title should differ from what was read.\nsent: %v\nwant: %v", update.Spec, want)
	}
}

// TestReferenceSaysWhichBuildAnswered: a transcript in which a tool misbehaved is only
// useful if it says which build that was, and the reference is the one tool every
// connection has.
func TestReferenceSaysWhichBuildAnswered(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))
	h.registry.opts.Version = "v1.2.3"

	for _, args := range []map[string]any{
		{},
		{"topic": "units"},
		{"topic": "shapes", "family": "rectangles"},
		{"topic": "files"},
	} {
		answer := h.ok(h.registry.reference(context.Background(), request(args)))
		if !strings.Contains(answer, `"version": "v1.2.3"`) || !strings.Contains(answer, `"name": "mcp-gdocs"`) {
			t.Errorf("the answer to %v should carry the build, got %s", args, answer)
		}
	}

	// A build nobody stamped says so rather than claiming a number.
	h.registry.opts.Version = ""
	if answer := h.ok(h.registry.reference(context.Background(), request(map[string]any{
		"topic": "units",
	}))); !strings.Contains(answer, `"version": "dev"`) {
		t.Errorf("an unstamped build should call itself dev, got %s", answer)
	}
}

// TestReferenceCarriesTheBehaviourNoPageDocuments walks the topics that are not enums.
func TestReferenceCarriesTheBehaviourNoPageDocuments(t *testing.T) {
	h, dir := filesHarness(t, newFakeGoogle(t))

	for topic, wants := range map[string][]string{
		"charts":         {"gdocs_slides_refresh_sheets_chart", "extend_charts", "same_place_as", "no field mask"},
		"tables":         {"moves every row under it up", "unmerge"},
		"text":           {"gdocs_slides_set_list", "HYPERLINK"},
		"rendering":      {"Speaker notes are not on it", "text_overflow_estimate", "hidden slides included"},
		"export_formats": {"one page per slide", "Hidden slides are in"},
		"files":          {"GDOCS_FILES_DIR", "relative_path", dir},
	} {
		answer := h.ok(h.registry.reference(context.Background(), request(map[string]any{"topic": topic})))
		for _, want := range wants {
			if !strings.Contains(answer, want) {
				t.Errorf("topic %s should say %q, got %s", topic, want, answer)
			}
		}
	}

	// A server with no directory says so in the same place, rather than leaving the topic out.
	bare := newHarness(t, newFakeGoogle(t))
	if answer := bare.ok(bare.registry.reference(context.Background(), request(map[string]any{
		"topic": "files",
	}))); !strings.Contains(answer, "without --files-dir") {
		t.Errorf("a server without a files directory should say so, got %s", answer)
	}
}

// TestAppendRowsNamesTheChartsItDidNotReach is the warning.
//
// The chart in the fixture reads rows 0 to 5 of the tab. Appending a row under them leaves
// the chart exactly as it was, and before this the answer said "rows: 1" and nothing else.
func TestAppendRowsNamesTheChartsItDidNotReach(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetForObjects))

	answer := h.ok(h.registry.sheetsAppendRows(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Данные",
		"rows": []any{[]any{"июль", 42.0}},
	})))

	for _, want := range []string{
		`"chart_id": 5`, `"range_end_row": 6`, "not in them", "gdocs_sheets_update_chart",
		"do not append these rows again", "gdocs_slides_refresh_sheets_chart",
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should carry %q, got %s", want, answer)
		}
	}

	body := string(h.bodyOf(t, len(h.google.requests)-1))
	if strings.Contains(body, "updateChartSpec") {
		t.Errorf("without extend_charts the charts are reported and left alone, got %s", body)
	}
}

// TestAppendRowsSaysNothingAboutChartsOnAnotherTab: the chart reads the first tab, so rows
// appended to the second are no business of its.
func TestAppendRowsSaysNothingAboutChartsOnAnotherTab(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetForObjects))

	answer := h.ok(h.registry.sheetsAppendRows(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Свод",
		"rows": []any{[]any{"итого", 42.0}},
	})))

	if strings.Contains(answer, "charts") || strings.Contains(answer, "warning") {
		t.Errorf("a tab no chart reads should be answered plainly, got %s", answer)
	}
}

// TestAppendRowsExtendsTheChartsThatEndedOnTheLastRow is the fix, and its limit.
//
// A chart whose range ended on the last row of data was drawn from the whole block, and the
// block just grew: its range moves with it, in the same batch as the rows. A chart that
// stopped short was pointed at part of the tab on purpose and stays where it is.
func TestAppendRowsExtendsTheChartsThatEndedOnTheLastRow(t *testing.T) {
	sixRows := `{"range": "'Данные'!A1:B6", "values": [["м","в"],["a",1],["b",2],["c",3],["d",4],["e",5]]}`

	t.Run("the data ends where the chart does", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).
			answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}, {}]}`).
			answer("/values/", sixRows).
			answer("/spreadsheets/book", spreadsheetForObjects))

		answer := h.ok(h.registry.sheetsAppendRows(context.Background(), request(map[string]any{
			"spreadsheet_id": "book", "sheet_title": "Данные", "extend_charts": true,
			"rows": []any{[]any{"f", 6.0}, []any{"g", 7.0}},
		})))

		for _, want := range []string{`"chart_id": 5`, `"range_end_row": 6`, `"extended_to_row": 8`} {
			if !strings.Contains(answer, want) {
				t.Errorf("the answer should carry %s, got %s", want, answer)
			}
		}
		if strings.Contains(answer, "warning") {
			t.Errorf("every chart was extended, so there is nothing to warn about, got %s", answer)
		}

		var sent struct {
			Requests []map[string]json.RawMessage `json:"requests"`
		}
		if err := json.Unmarshal(h.bodyOf(t, len(h.google.requests)-1), &sent); err != nil {
			t.Fatalf("reading the request: %v", err)
		}
		// One batch: the rows and the chart go together or not at all.
		if len(sent.Requests) != 2 || sent.Requests[0]["appendCells"] == nil || sent.Requests[1]["updateChartSpec"] == nil {
			t.Fatalf("the batch should append and then extend, got %s", h.bodyOf(t, len(h.google.requests)-1))
		}

		spec := string(sent.Requests[1]["updateChartSpec"])
		// Both ranges moved — the labels and the numbers — and nothing else was lost.
		if strings.Count(spec, `"endRowIndex": 8`) != 2 || strings.Contains(spec, `"endRowIndex": 6`) {
			t.Errorf("both ranges should now stop before row 8, got %s", spec)
		}
		for _, kept := range []string{`"stackedType": "STACKED"`, `"hiddenDimensionStrategy": "SKIP_HIDDEN_ROWS"`, `"title": "Было"`} {
			if !strings.Contains(spec, kept) {
				t.Errorf("extending a range should keep %s, got %s", kept, spec)
			}
		}
	})

	t.Run("the data ran past the chart already", func(t *testing.T) {
		tenRows := `{"values": [[1],[2],[3],[4],[5],[6],[7],[8],[9],[10]]}`

		h := newHarness(t, newFakeGoogle(t).
			answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
			answer("/values/", tenRows).
			answer("/spreadsheets/book", spreadsheetForObjects))

		answer := h.ok(h.registry.sheetsAppendRows(context.Background(), request(map[string]any{
			"spreadsheet_id": "book", "sheet_title": "Данные", "extend_charts": true,
			"rows": []any{[]any{"f", 6.0}},
		})))

		for _, want := range []string{"left_alone", "stops before row 6", "ran to row 10", "0 of 1"} {
			if !strings.Contains(answer, want) {
				t.Errorf("the answer should carry %q, got %s", want, answer)
			}
		}
		if body := string(h.bodyOf(t, len(h.google.requests)-1)); strings.Contains(body, "updateChartSpec") {
			t.Errorf("a chart pointed at part of the tab should not be moved, got %s", body)
		}
	})
}

// TestChartsReadingTab walks the shapes a specification keeps its ranges in.
func TestChartsReadingTab(t *testing.T) {
	var charts []google.StoredChart
	if err := json.Unmarshal([]byte(`[
	  {"chartId": 1, "spec": {"title": "pie", "pieChart": {
	    "domain": {"sourceRange": {"sources": [{"startRowIndex": 1, "endRowIndex": 9}]}},
	    "series": {"sourceRange": {"sources": [{"startRowIndex": 1, "endRowIndex": 9}]}}}}},
	  {"chartId": 2, "spec": {"basicChart": {"series": [
	    {"series": {"sourceRange": {"sources": [{"sheetId": 7, "startRowIndex": 0, "endRowIndex": 4}]}}}]}}},
	  {"chartId": 3, "spec": {"basicChart": {"series": [
	    {"series": {"sourceRange": {"sources": [{"startRowIndex": 0}]}}}]}}},
	  {"chartId": 4, "spec": {"basicChart": {"domains": [
	    {"domain": {"sourceRange": {"sources": [{"startRowIndex": 0, "endRowIndex": 5}]}}}],
	    "series": [{"series": {"sourceRange": {"sources": [{"startRowIndex": 0, "endRowIndex": 3}]}}}]}}}
	]`), &charts); err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}

	reading := chartsReadingTab(charts, 0)

	// Chart 2 reads another tab; chart 3 has no end row, so it grows by itself.
	if len(reading) != 2 || reading[0].chartID != 1 || reading[1].chartID != 4 {
		t.Fatalf("charts 1 and 4 read tab 0 with a fixed range, got %+v", reading)
	}
	// A range with no sheetId is on tab 0: Sheets leaves a zero out of its answers.
	if reading[0].endRow != 9 || !reading[0].sameEnd || reading[0].title != "pie" {
		t.Errorf("the pie reads rows up to 9 in both its ranges, got %+v", reading[0])
	}
	if reading[1].endRow != 5 || reading[1].sameEnd {
		t.Errorf("chart 4's ranges stop at different rows, got %+v", reading[1])
	}

	if got := chartsReadingTab(charts, 7); len(got) != 1 || got[0].chartID != 2 {
		t.Errorf("only chart 2 reads tab 7, got %+v", got)
	}

	if quotedSheet("Q1 'итог'") != "'Q1 ''итог'''" {
		t.Errorf("a quote in a tab's name has to be doubled, got %s", quotedSheet("Q1 'итог'"))
	}
}

// deckToDuplicateIn holds a slide with a shape, a table and a group of two, and a second
// slide, which is what a duplicate has to tell apart.
const deckToDuplicateIn = `{
  "presentationId": "deck",
  "slides": [
    {"objectId": "metrics", "pageElements": [
      {"objectId": "title"},
      {"objectId": "table"},
      {"objectId": "badge", "elementGroup": {"children": [{"objectId": "badge_bg"}, {"objectId": "badge_text"}]}}
    ]},
    {"objectId": "other", "pageElements": [{"objectId": "stray"}]}
  ]
}`

// TestDuplicateAnswersWithAMapOfIdentifiers: a slide duplicated in order to be filled in
// again arrives with a title and a table under names nobody was told. Naming them in the
// request is what lets the answer say which is which without a second reading.
func TestDuplicateAnswersWithAMapOfIdentifiers(t *testing.T) {
	counter := 0
	numbered := func(prefix string) string {
		counter++
		return prefix + "_" + string(rune('a'+counter-1))
	}

	t.Run("a slide", func(t *testing.T) {
		counter = 0
		h := newHarness(t, newFakeGoogle(t).
			answer("/presentations/deck:batchUpdate",
				`{"presentationId":"deck","replies":[{"duplicateObject":{"objectId":"metrics_2"}}]}`).
			answer("/presentations/deck", deckToDuplicateIn))
		h.registry.opts.NewObjectID = numbered

		answer := h.ok(h.registry.slidesDuplicate(context.Background(), request(map[string]any{
			"presentation_id": "deck", "object_id": "metrics", "new_object_ids": []any{"metrics_2"},
		})))

		var sent struct {
			Requests []struct {
				DuplicateObject struct {
					ObjectID  string            `json:"objectId"`
					ObjectIDs map[string]string `json:"objectIds"`
				} `json:"duplicateObject"`
			} `json:"requests"`
		}
		if err := json.Unmarshal(h.bodyOf(t, 1), &sent); err != nil {
			t.Fatalf("reading the request: %v", err)
		}

		want := map[string]string{
			"metrics": "metrics_2",
			"title":   "copy_a", "table": "copy_b",
			"badge": "copy_c", "badge_bg": "copy_d", "badge_text": "copy_e",
		}
		if got := sent.Requests[0].DuplicateObject.ObjectIDs; !reflect.DeepEqual(got, want) {
			t.Errorf("every element of the slide should be named in the request.\ngot:  %v\nwant: %v", got, want)
		}

		var replied struct {
			IDMaps []map[string]string `json:"id_maps"`
		}
		if err := json.Unmarshal([]byte(answer), &replied); err != nil {
			t.Fatalf("reading the answer: %v", err)
		}
		if len(replied.IDMaps) != 1 || !reflect.DeepEqual(replied.IDMaps[0], want) {
			t.Errorf("the answer should carry the same map, got %v", replied.IDMaps)
		}
		// The other slide's element is none of this copy's business.
		if strings.Contains(answer, "stray") {
			t.Errorf("only what the original holds belongs in the map, got %s", answer)
		}
	})

	t.Run("a group, twice", func(t *testing.T) {
		counter = 0
		h := newHarness(t, newFakeGoogle(t).
			answer("/presentations/deck:batchUpdate",
				`{"presentationId":"deck","replies":[{"duplicateObject":{"objectId":"g1"}},`+
					`{"duplicateObject":{"objectId":"g2"}}]}`).
			answer("/presentations/deck", deckToDuplicateIn))
		h.registry.opts.NewObjectID = numbered

		answer := h.ok(h.registry.slidesDuplicate(context.Background(), request(map[string]any{
			"presentation_id": "deck", "object_id": "badge", "copies": 2.0,
		})))

		var replied struct {
			IDMaps []map[string]string `json:"id_maps"`
		}
		if err := json.Unmarshal([]byte(answer), &replied); err != nil {
			t.Fatalf("reading the answer: %v", err)
		}

		// Each copy has its own members, and the group itself is under the name Google gave.
		want := []map[string]string{
			{"badge": "g1", "badge_bg": "copy_a", "badge_text": "copy_b"},
			{"badge": "g2", "badge_bg": "copy_c", "badge_text": "copy_d"},
		}
		if !reflect.DeepEqual(replied.IDMaps, want) {
			t.Errorf("each copy should have a map of its own.\ngot:  %v\nwant: %v", replied.IDMaps, want)
		}
	})
}

// TestCopySlideInsideOneDeckMapsItsElements: the same map, from the other tool that
// duplicates. The copy between decks answers with one too — see the next test.
func TestCopySlideInsideOneDeckMapsItsElements(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/sample:batchUpdate", `{"replies": [{"duplicateObject": {"objectId": "dup1"}}]}`).
		answer("/presentations/sample", deckToCopyFrom))

	answer := h.ok(h.registry.slidesCopySlide(context.Background(), request(map[string]any{
		"source_presentation_id": "sample", "source_page_object_id": "slide1",
		"target_presentation_id": "sample",
	})))

	var replied struct {
		Method string            `json:"method"`
		IDMap  map[string]string `json:"id_map"`
	}
	if err := json.Unmarshal([]byte(answer), &replied); err != nil {
		t.Fatalf("reading the answer: %v", err)
	}
	if replied.Method != "duplicate" {
		t.Errorf("inside one deck the method is duplicate, got %q", replied.Method)
	}
	if replied.IDMap["slide1"] != "dup1" {
		t.Errorf("the slide should map to the identifier Google answered with, got %v", replied.IDMap)
	}
	for _, element := range []string{"title1", "body1", "table1", "pic1"} {
		if replied.IDMap[element] == "" {
			t.Errorf("%s should be in the map, got %v", element, replied.IDMap)
		}
	}

	body := string(h.bodyOf(t, 1))
	for _, element := range []string{`"title1"`, `"table1"`} {
		if !strings.Contains(body, element) {
			t.Errorf("the request should name %s so its copy's identifier is known, got %s", element, body)
		}
	}
}

// TestCopySlideBetweenDecksSaysItRebuilt: the other value of method, with the map of what
// was built again.
func TestCopySlideBetweenDecksSaysItRebuilt(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/target:batchUpdate", `{"replies": [{"createSlide": {"objectId": "new1"}}]}`).
		answer("/presentations/target", `{"presentationId": "target", "slides": [], "layouts": []}`).
		answer("/presentations/sample", deckToCopyFrom))

	answer := h.ok(h.registry.slidesCopySlide(context.Background(), request(map[string]any{
		"source_presentation_id": "sample", "source_page_object_id": "slide1",
		"target_presentation_id": "target", "copy_speaker_notes": false,
	})))

	var replied struct {
		Method string            `json:"method"`
		IDMap  map[string]string `json:"id_map"`
	}
	if err := json.Unmarshal([]byte(answer), &replied); err != nil {
		t.Fatalf("reading the answer: %v", err)
	}
	if replied.Method != "rebuild" {
		t.Errorf("between decks the method is rebuild, got %q", replied.Method)
	}
	want := map[string]string{"slide1": "new1", "title1": "shape_test", "table1": "table_test", "pic1": "image_test"}
	for source, made := range want {
		if replied.IDMap[source] != made {
			t.Errorf("%s should map to %s, got %v", source, made, replied.IDMap)
		}
	}
}

// TestInspectEstimatesOverflow: the API says nothing about text running out of its box, so
// the estimate is arithmetic and only speaks when it has the numbers.
func TestInspectEstimatesOverflow(t *testing.T) {
	sized := func(text string, points float64) *google.Shape {
		return &google.Shape{Text: &google.TextContent{TextElements: []google.TextElement{
			{TextRun: &google.TextRun{Content: text, Style: &google.TextStyle{
				FontSize: &google.Dimension{Magnitude: points, Unit: "PT"},
			}}},
		}}}
	}

	// A box four inches wide and half an inch tall: one line of 18 pt fits, six do not.
	const width, height = 4 * 914400, 457200

	if estimate := estimateOverflow(sized("Короткий заголовок\n", 18), width, height); estimate != nil {
		t.Errorf("one short line fits, got %+v", estimate)
	}

	long := strings.Repeat("Довольно длинная строка текста для узкой рамки. ", 4) + "\n"
	estimate := estimateOverflow(sized(long, 18), width, height)
	if estimate == nil {
		t.Fatal("four sentences in a half-inch box should be flagged")
	}
	if estimate.LinesNeeded <= estimate.LinesFit || estimate.NeededHeightEMU <= estimate.BoxHeightEMU {
		t.Errorf("the estimate should say more is needed than fits, got %+v", estimate)
	}
	if !strings.Contains(estimate.Note, "not a measurement") {
		t.Errorf("the estimate should call itself one, got %q", estimate.Note)
	}

	// Paragraphs count as lines even when each is short.
	if estimateOverflow(sized("раз\nдва\nтри\nчетыре\nпять\n", 18), width, height) == nil {
		t.Error("five paragraphs in a half-inch box should be flagged")
	}

	// A size inherited from the layout is not in the reading, and guessing one would turn
	// a hint into a rumour.
	inherited := &google.Shape{Text: &google.TextContent{TextElements: []google.TextElement{
		{TextRun: &google.TextRun{Content: long}},
	}}}
	if estimateOverflow(inherited, width, height) != nil {
		t.Error("text with no font size of its own should not be estimated")
	}

	// A box that grows with its text cannot overflow; one that shrinks its text is counted
	// at the size it shrank to.
	growing := sized(long, 18)
	growing.Properties = &google.ShapeProperties{Autofit: &google.Autofit{AutofitType: "SHAPE_AUTOFIT"}}
	if estimateOverflow(growing, width, height) != nil {
		t.Error("a shape that grows with its text should not be flagged")
	}

	shrunk := sized("Заголовок чуть длиннее рамки, в две строки\n", 18)
	if estimateOverflow(shrunk, width, height) == nil {
		t.Fatal("two lines of 18 pt in a half-inch box should be flagged before shrinking")
	}
	shrunk.Properties = &google.ShapeProperties{Autofit: &google.Autofit{AutofitType: "TEXT_AUTOFIT", FontScale: 0.4}}
	if estimateOverflow(shrunk, width, height) != nil {
		t.Error("text shrunk to fit should be counted at the size it shrank to")
	}

	if estimateOverflow(sized("\n", 18), width, height) != nil || estimateOverflow(nil, width, height) != nil {
		t.Error("no text is nothing to overflow")
	}
}

// pictureHarness is a registry with a files directory holding one picture.
func pictureHarness(t *testing.T, fake *fakeGoogle) *harness {
	t.Helper()

	h, dir := filesHarness(t, fake)
	if err := os.MkdirAll(filepath.Join(dir, "charts"), 0o700); err != nil {
		t.Fatalf("making the directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "charts", "q3.png"), []byte("\x89PNG pretend"), 0o600); err != nil {
		t.Fatalf("writing the picture: %v", err)
	}

	previous := imageFetchDelay
	imageFetchDelay = 0
	t.Cleanup(func() { imageFetchDelay = previous })

	return h
}

// sequence lists the requests the fake saw as "METHOD path".
func (h *harness) sequence() []string {
	seen := make([]string, 0, len(h.google.requests))
	for _, sent := range h.google.requests {
		seen = append(seen, sent.Method+" "+sent.Path)
	}

	return seen
}

// TestInsertImageFromAFileOpensAndClosesTheLink is the whole route in order: upload, open,
// insert, close. Slides takes an address and fetches it as nobody in particular, so there is
// no shorter one — and the last step is the one a person doing this by hand forgets.
func TestInsertImageFromAFileOpensAndClosesTheLink(t *testing.T) {
	h := pictureHarness(t, newFakeGoogle(t).
		answer("/upload/drive/v3/files", `{"id": "up1", "name": "q3.png", "mimeType": "image/png"}`).
		answer("/files/up1/permissions", `{"id": "perm1", "type": "anyone", "role": "reader"}`).
		answer("/presentations/deck:batchUpdate", emptyBatchReply))

	answer := h.ok(h.registry.slidesInsertImage(context.Background(), request(map[string]any{
		"presentation_id": "deck", "page_object_id": "slide1", "file": "charts/q3.png",
	})))

	want := []string{
		http.MethodPost + " /upload/drive/v3/files",
		http.MethodPost + " /drive/v3/files/up1/permissions",
		http.MethodPost + " /v1/presentations/deck:batchUpdate",
		http.MethodDelete + " /drive/v3/files/up1/permissions/perm1",
	}
	if got := h.sequence(); !reflect.DeepEqual(got, want) {
		t.Fatalf("the route should be upload, open, insert, close.\ngot:  %v\nwant: %v", got, want)
	}

	if grant := string(h.bodyOf(t, 1)); !strings.Contains(grant, `"type": "anyone"`) || !strings.Contains(grant, `"role": "reader"`) {
		t.Errorf("the link should be opened to anyone as a reader and no more, got %s", grant)
	}
	if insert := string(h.bodyOf(t, 2)); !strings.Contains(insert, "id=up1") || !strings.Contains(insert, "createImage") {
		t.Errorf("the picture should be inserted from the uploaded file's address, got %s", insert)
	}

	for _, wanted := range []string{`"uploaded_file_id": "up1"`, `"link_access": "closed"`, "stays on Drive"} {
		if !strings.Contains(answer, wanted) {
			t.Errorf("the answer should carry %s, got %s", wanted, answer)
		}
	}
}

// TestInsertImageClosesTheLinkWhenTheInsertionFails: a failed insertion must not leave a
// file open behind it. This is the path where forgetting costs something.
func TestInsertImageClosesTheLinkWhenTheInsertionFails(t *testing.T) {
	h := pictureHarness(t, newFakeGoogle(t).
		answer("/upload/drive/v3/files", `{"id": "up1"}`).
		answer("/files/up1/permissions", `{"id": "perm1"}`).
		fail("/presentations/deck:batchUpdate", http.StatusBadRequest,
			`{"error": {"code": 400, "message": "There was a problem retrieving the image."}}`))

	message := h.fail(h.registry.slidesInsertImage(context.Background(), request(map[string]any{
		"presentation_id": "deck", "page_object_id": "slide1", "file": "charts/q3.png",
	})))

	seen := h.sequence()
	if last := seen[len(seen)-1]; last != http.MethodDelete+" /drive/v3/files/up1/permissions/perm1" {
		t.Errorf("the last request should take the link back, got %v", seen)
	}

	// The fetch is retried: Drive answers a grant before all of it has heard.
	inserts := 0
	for _, one := range seen {
		if strings.HasSuffix(one, ":batchUpdate") {
			inserts++
		}
	}
	if inserts != imageFetchAttempts {
		t.Errorf("the insertion should be tried %d times, got %d", imageFetchAttempts, inserts)
	}

	for _, want := range []string{"problem retrieving the image", "uploaded_file_id=up1", "link_access=closed"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal should say %q, got %s", want, message)
		}
	}
}

// TestInsertImageSaysTheLinkWasNeverOpened: an organisation that forbids sharing outside
// itself refuses the grant. Nothing was opened, so nothing is closed, and the answer must
// not claim an opening that did not happen.
func TestInsertImageSaysTheLinkWasNeverOpened(t *testing.T) {
	h := pictureHarness(t, newFakeGoogle(t).
		answer("/upload/drive/v3/files", `{"id": "up1"}`).
		fail("/files/up1/permissions", http.StatusForbidden,
			`{"error": {"code": 403, "message": "Sharing outside the organisation is not allowed"}}`))

	message := h.fail(h.registry.slidesInsertImage(context.Background(), request(map[string]any{
		"presentation_id": "deck", "page_object_id": "slide1", "file": "charts/q3.png",
	})))

	for _, want := range []string{"could not be opened by link", "link_access=never opened", "uploaded_file_id=up1"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal should say %q, got %s", want, message)
		}
	}
	for _, sent := range h.sequence() {
		if strings.HasPrefix(sent, http.MethodDelete) || strings.HasSuffix(sent, ":batchUpdate") {
			t.Errorf("with no link there is nothing to insert from and nothing to close, got %v", h.sequence())
		}
	}
}

// TestInsertImageSaysSoWhenTheLinkStaysOpen: the one failure that must not be quiet.
func TestInsertImageSaysSoWhenTheLinkStaysOpen(t *testing.T) {
	h := pictureHarness(t, newFakeGoogle(t).
		answer("/upload/drive/v3/files", `{"id": "up1"}`).
		fail("/files/up1/permissions/perm1", http.StatusForbidden,
			`{"error": {"code": 403, "message": "Insufficient permissions"}}`).
		answer("/files/up1/permissions", `{"id": "perm1"}`).
		answer("/presentations/deck:batchUpdate", emptyBatchReply))

	answer := h.ok(h.registry.slidesInsertImage(context.Background(), request(map[string]any{
		"presentation_id": "deck", "page_object_id": "slide1", "file": "charts/q3.png",
	})))

	for _, want := range []string{"STILL OPEN", "gdocs_drive_unshare file_id=up1 permission_id=perm1"} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should say %q, got %s", want, answer)
		}
	}
}

// TestInsertImageFromAFileNeedsSharingToBeAllowed: opening a file by link is sharing, however
// briefly, and a server whose operator left drive-share out does not do it for a slides tool
// either. Nothing is uploaded before the refusal.
func TestInsertImageFromAFileNeedsSharingToBeAllowed(t *testing.T) {
	h := pictureHarness(t, newFakeGoogle(t))
	h.registry.opts.Groups = defaultGroups()

	message := h.fail(h.registry.slidesInsertImage(context.Background(), request(map[string]any{
		"presentation_id": "deck", "page_object_id": "slide1", "file": "charts/q3.png",
	})))
	if !strings.Contains(message, "drive-share") {
		t.Errorf("the refusal should name the group an operator would add, got %s", message)
	}
	if len(h.google.requests) != 0 {
		t.Errorf("nothing should reach Google before the refusal, got %v", h.sequence())
	}

	// A window on slides has no drive-share of its own; the operator's decision reaches it
	// beside the groups.
	h.registry.opts.MayShare = true
	if !h.registry.mayPublishPictures() {
		t.Error("a window should publish where the server as a whole may share")
	}
}

// TestInsertImageFromAFileRefusals: every one of these is told apart before anything is
// uploaded.
func TestInsertImageFromAFileRefusals(t *testing.T) {
	h := pictureHarness(t, newFakeGoogle(t))
	if err := os.WriteFile(filepath.Join(h.registry.opts.FilesDir, "logo.svg"), []byte("<svg/>"), 0o600); err != nil {
		t.Fatalf("writing the picture: %v", err)
	}

	for name, probe := range map[string]struct {
		args    map[string]any
		refused string
	}{
		"neither":         {map[string]any{}, "url for an address"},
		"both":            {map[string]any{"url": "https://example.invalid/a.png", "file": "charts/q3.png"}, "alternatives"},
		"not a picture":   {map[string]any{"file": "logo.svg"}, "PNG, JPEG and GIF"},
		"not there":       {map[string]any{"file": "charts/q4.png"}, "reading"},
		"outside":         {map[string]any{"file": "../q3.png"}, "outside the server's files directory"},
		"a path as a url": {map[string]any{"url": "/files/charts/q3.png"}, "goes in file instead"},
	} {
		args := map[string]any{"presentation_id": "deck", "page_object_id": "slide1"}
		for key, value := range probe.args {
			args[key] = value
		}

		message := h.fail(h.registry.slidesInsertImage(context.Background(), request(args)))
		if !strings.Contains(message, probe.refused) {
			t.Errorf("%s: the refusal should say %q, got %s", name, probe.refused, message)
		}
	}

	if len(h.google.requests) != 0 {
		t.Errorf("none of these should reach Google, got %v", h.sequence())
	}

	// Without a files directory there is nowhere to read a picture from.
	bare := newHarness(t, newFakeGoogle(t))
	if message := bare.fail(bare.registry.slidesInsertImage(context.Background(), request(map[string]any{
		"presentation_id": "deck", "page_object_id": "slide1", "file": "q3.png",
	}))); !strings.Contains(message, "--files-dir") {
		t.Errorf("a server without a files directory should say so, got %s", message)
	}
}

// TestCopyAnswersWithWhoCanReachTheCopy: a copy's access is the folder's, not the
// original's, and "is it shared" is the first thing asked about a file about to be sent.
func TestCopyAnswersWithWhoCanReachTheCopy(t *testing.T) {
	permissions := `{"permissions": [
	  {"id": "p1", "type": "user", "role": "owner", "emailAddress": "me@example.invalid"},
	  {"id": "p2", "type": "anyone", "role": "reader"}]}`

	for name, call := range map[string]func(*harness) (*mcp.CallToolResult, error){
		"gdocs_drive_copy": func(h *harness) (*mcp.CallToolResult, error) {
			return h.registry.driveCopy(context.Background(), request(map[string]any{
				"file_id": "template", "name": "Копия",
			}))
		},
		"gdocs_slides_copy_presentation": func(h *harness) (*mcp.CallToolResult, error) {
			return h.registry.slidesCopyPresentation(context.Background(), request(map[string]any{
				"template_id": "template", "name": "Копия",
			}))
		},
	} {
		h := newHarness(t, newFakeGoogle(t).
			answer("/files/copy1/permissions", permissions).
			answer("/files/template/copy", `{"id": "copy1", "name": "Копия"}`))

		answer := h.ok(call(h))
		for _, want := range []string{`"open_to_anyone_with_link": true`, "me@example.invalid", `"role": "owner"`} {
			if !strings.Contains(answer, want) {
				t.Errorf("%s should answer with %s, got %s", name, want, answer)
			}
		}
	}

	// A reading that fails does not fail the call: the copy exists, and an answer that hid
	// it behind an error would invite making another.
	h := newHarness(t, newFakeGoogle(t).
		fail("/files/copy1/permissions", http.StatusForbidden, `{"error": {"code": 403, "message": "no"}}`).
		answer("/files/template/copy", `{"id": "copy1", "name": "Копия"}`))

	answer := h.ok(h.registry.driveCopy(context.Background(), request(map[string]any{
		"file_id": "template", "name": "Копия",
	})))
	if !strings.Contains(answer, `"file_id": "copy1"`) || !strings.Contains(answer, "permissions_unread") {
		t.Errorf("the copy should be reported with its permissions marked unread, got %s", answer)
	}
}

// deckWithAChartAndItsPicture holds a linked chart, a picture of one, and a picture inside
// a group.
const deckWithAChartAndItsPicture = `{
  "presentationId": "deck",
  "slides": [{"objectId": "slide1", "pageElements": [
    {"objectId": "live", "sheetsChart": {"spreadsheetId": "book", "chartId": 5}},
    {"objectId": "stale",
     "size": {"width": {"magnitude": 600, "unit": "EMU"}, "height": {"magnitude": 371, "unit": "EMU"}},
     "transform": {"scaleX": 9525, "scaleY": 9525, "translateX": 400000, "translateY": 900000, "unit": "EMU"},
     "image": {"contentUrl": "https://example.invalid/chart.png"}},
    {"objectId": "group1", "elementGroup": {"children": [{"objectId": "inner"}]}}
  ]}]
}`

// TestRefreshExplainsAPicture: a chart placed unlinked looks like a chart and is a picture.
// Google's refusal to refresh one names an invalid object and stops there.
func TestRefreshExplainsAPicture(t *testing.T) {
	refusal := `{"error": {"code": 400, "message": "Invalid requests[0].refreshSheetsChart: The object (stale) is not a chart."}}`

	h := newHarness(t, newFakeGoogle(t).
		fail("/presentations/deck:batchUpdate", http.StatusBadRequest, refusal).
		answer("/presentations/deck", deckWithAChartAndItsPicture))

	message := h.fail(h.registry.slidesRefreshSheetsChart(context.Background(), request(map[string]any{
		"presentation_id": "deck", "object_id": "stale",
	})))
	for _, want := range []string{"is a picture, not a linked chart", "same_place_as=stale", "gdocs_slides_delete", "not a chart"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal should say %q, got %s", want, message)
		}
	}

	// A real chart that Google refused for its own reasons is passed on as it came.
	message = h.fail(h.registry.slidesRefreshSheetsChart(context.Background(), request(map[string]any{
		"presentation_id": "deck", "object_id": "live",
	})))
	if strings.Contains(message, "picture") {
		t.Errorf("a linked chart is not a picture, got %s", message)
	}

	message = h.fail(h.registry.slidesRefreshSheetsChart(context.Background(), request(map[string]any{
		"presentation_id": "deck", "object_id": "nowhere",
	})))
	if !strings.Contains(message, "no element named nowhere") {
		t.Errorf("a wrong identifier should be called one, got %s", message)
	}
}

// TestRefreshThatWorksReadsNothingExtra: the explanation is looked up only after a refusal.
func TestRefreshThatWorksReadsNothingExtra(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	answer := h.ok(h.registry.slidesRefreshSheetsChart(context.Background(), request(map[string]any{
		"presentation_id": "deck", "object_id": "live",
	})))
	if len(h.google.requests) != 1 {
		t.Errorf("a refresh that works is one request, got %v", h.sequence())
	}
	if !strings.Contains(answer, "will not follow later changes") {
		t.Errorf("the answer should say the chart is a snapshot again, got %s", answer)
	}
}

// TestAddChartInTheSamePlaceAsItsPicture: the way to bring an unlinked chart up to date is
// to put the chart on again where the picture stands, with the picture's own size and
// transform rather than four numbers read off it.
func TestAddChartInTheSamePlaceAsItsPicture(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", deckWithAChartAndItsPicture))

	answer := h.ok(h.registry.slidesAddSheetsChart(context.Background(), request(map[string]any{
		"presentation_id": "deck", "spreadsheet_id": "book", "chart_id": 5.0, "same_place_as": "stale",
	})))

	body := string(h.bodyOf(t, 1))
	for _, want := range []string{
		`"pageObjectId": "slide1"`, `"scaleX": 9525`, `"translateX": 400000`, `"magnitude": 371`,
		`"linkingMode": "LINKED"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the chart should take the picture's place exactly: want %s in %s", want, body)
		}
	}
	// This tool removes nothing, and says what it left behind.
	if strings.Contains(body, "deleteObject") {
		t.Errorf("the picture is not this tool's to remove, got %s", body)
	}
	for _, want := range []string{"stale was not removed", "gdocs_slides_delete", "gdocs_slides_refresh_sheets_chart"} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should say %q, got %s", want, answer)
		}
	}

	for name, probe := range map[string]struct {
		args    map[string]any
		refused string
	}{
		"no place at all":  {map[string]any{}, "page_object_id is required"},
		"both":             {map[string]any{"same_place_as": "stale", "x_emu": 1.0}, "alternatives"},
		"inside a group":   {map[string]any{"same_place_as": "inner"}, "inside a group"},
		"not there":        {map[string]any{"same_place_as": "ghost"}, "no element named ghost"},
		"on another slide": {map[string]any{"same_place_as": "stale", "page_object_id": "slide9"}, "stands on slide slide1"},
		"nothing to take":  {map[string]any{"same_place_as": "live"}, "no size or no transform"},
	} {
		args := map[string]any{"presentation_id": "deck", "spreadsheet_id": "book", "chart_id": 5.0}
		for key, value := range probe.args {
			args[key] = value
		}

		message := h.fail(h.registry.slidesAddSheetsChart(context.Background(), request(args)))
		if !strings.Contains(message, probe.refused) {
			t.Errorf("%s: the refusal should say %q, got %s", name, probe.refused, message)
		}
	}

	// Unlinked, the answer says what was just made.
	answer = h.ok(h.registry.slidesAddSheetsChart(context.Background(), request(map[string]any{
		"presentation_id": "deck", "page_object_id": "slide1", "spreadsheet_id": "book",
		"chart_id": 5.0, "linked": false,
	})))
	if !strings.Contains(answer, "a picture of the chart, not a chart") {
		t.Errorf("an unlinked chart should be called a picture, got %s", answer)
	}
}

// TestFileAnswersCarryBothNamesOfTheFile: a path is true on one side of a mount. The answer
// carries the server's and the name from the directory, which is the half a client can use.
func TestFileAnswersCarryBothNamesOfTheFile(t *testing.T) {
	fake := newFakeGoogle(t).
		answer("/files/deck/export", "%PDF-1.7 pretend").
		answer("/upload/drive/v3/files", `{"id": "new1", "name": "deck"}`)
	h, dir := filesHarness(t, fake)

	answer := h.ok(h.registry.exportFile(context.Background(), request(map[string]any{
		"file_id": "deck", "format": "pdf", "save_as": "reports/deck.pdf",
	})))

	var exported struct {
		Path         string `json:"path"`
		RelativePath string `json:"relative_path"`
	}
	if err := json.Unmarshal([]byte(answer), &exported); err != nil {
		t.Fatalf("reading the answer: %v", err)
	}
	if exported.Path != filepath.Join(dir, "reports", "deck.pdf") || exported.RelativePath != "reports/deck.pdf" {
		t.Errorf("the answer should carry the whole path and the name from the directory, got %+v", exported)
	}

	// And the name from the directory is what goes back in.
	answer = h.ok(h.registry.importFile(context.Background(), request(map[string]any{
		"path": exported.RelativePath, "keep_original_format": true,
	})))
	if !strings.Contains(answer, `"relative_path": "reports/deck.pdf"`) || !strings.Contains(answer, exported.Path) {
		t.Errorf("an import should say which file it read, got %s", answer)
	}
}

// forgetfulSession accepts tools and does not keep them, which is the failure the answer of
// gdocs_find_tools must never report as success.
type forgetfulSession struct{ toolHoldingSession }

func (s *forgetfulSession) SetSessionTools(map[string]server.ServerTool) {}

// TestDiscoveryDoesNotReportWhatTheSessionDoesNotHold: "added" is read back from the
// session, not assumed from the absence of an error.
func TestDiscoveryDoesNotReportWhatTheSessionDoesNotHold(t *testing.T) {
	narrow, _ := discoveryPair(t)
	tool := narrow.ListTools()["gdocs_find_tools"]

	forgetful := &forgetfulSession{toolHoldingSession{id: "forgetful"}}
	if err := narrow.RegisterSession(context.Background(), forgetful); err != nil {
		t.Fatalf("registering a session: %v", err)
	}

	result, err := tool.Handler(narrow.WithContext(context.Background(), forgetful),
		request(map[string]any{"names": []any{"gdocs_sheets_update_chart"}}))
	message := requireError(t, result, err)
	for _, want := range []string{"gdocs_sheets_update_chart could not be added", "gdocs_call_tool"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal should say %q, got %s", want, message)
		}
	}

	// A session that keeps them is told two different things, kept apart: the session holds
	// the tool, and whether the client's own list shows it is not known from here.
	holding := &toolHoldingSession{id: "holding"}
	if err := narrow.RegisterSession(context.Background(), holding); err != nil {
		t.Fatalf("registering a session: %v", err)
	}

	result, err = tool.Handler(narrow.WithContext(context.Background(), holding),
		request(map[string]any{"names": []any{"gdocs_sheets_update_chart"}}))
	answer := requireOK(t, result, err)
	for _, want := range []string{`"held_by_session": true`, "not confirmed", `"call_meanwhile_with": "gdocs_call_tool"`} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should carry %s, got %s", want, answer)
		}
	}
}
