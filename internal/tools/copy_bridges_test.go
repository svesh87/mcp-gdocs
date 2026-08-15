package tools

import (
	"context"
	"strings"
	"testing"
)

// bookRectangle is a heading row and two data rows, formatted the way a real one is: the
// heading bold on a fill, the numbers right-aligned, one cell holding a formula.
const bookRectangle = `{
  "spreadsheetId": "book",
  "properties": {"title": "Цели"},
  "sheets": [
    {
      "properties": {"sheetId": 0, "title": "Цели"},
      "merges": [{"sheetId": 0, "startRowIndex": 0, "endRowIndex": 1,
        "startColumnIndex": 0, "endColumnIndex": 2}],
      "conditionalFormats": [{"ranges": [{"sheetId": 0, "startRowIndex": 0, "endRowIndex": 3}]}],
      "data": [
        {
          "rowData": [
            {"values": [
              {"formattedValue": "Команда",
               "userEnteredFormat": {"horizontalAlignment": "LEFT",
                 "backgroundColor": {"red": 0.9, "green": 0.9, "blue": 0.9},
                 "textFormat": {"bold": true, "fontSize": 11, "fontFamily": "Rubik"}}},
              {"formattedValue": "Закрыто",
               "userEnteredFormat": {"horizontalAlignment": "RIGHT",
                 "textFormat": {"bold": true, "fontSize": 11}}}]},
            {"values": [
              {"formattedValue": "SRE"},
              {"formattedValue": "12",
               "userEnteredFormat": {"horizontalAlignment": "RIGHT",
                 "textFormat": {"foregroundColor": {"red": 0.1, "green": 0.5, "blue": 0.2}}}}]},
            {"values": [
              {"formattedValue": "QA"},
              {"formattedValue": "18"}]}
          ]
        }
      ]
    }
  ]
}`

// TestTableFromSheetsOntoASlide pins the batch. The values go in as they are shown rather than
// as they were typed, which is the difference between this and gdocs_sheets_copy_range: a
// formula on a slide is a formula nobody can evaluate.
func TestTableFromSheetsOntoASlide(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/spreadsheets/book", bookRectangle))

	answer := h.ok(h.registry.slidesCopyTableFromSheets(context.Background(), request(map[string]any{
		"source_spreadsheet_id": "book", "source_sheet_title": "Цели",
		"start_row": float64(0), "end_row": float64(3),
		"start_column": float64(0), "end_column": float64(2),
		"target_presentation_id": "deck", "target_page_object_id": "slide1",
		"x_emu": float64(500000), "y_emu": float64(1000000),
		"width_emu": float64(4000000), "height_emu": float64(1500000),
	})))

	checkGolden(t, "copy_table_to_slide.json", h.bodyOf(t, 1))

	// Everything the rectangle had and a slide's table has not is named, because a table that
	// stopped reacting to its numbers looks exactly like one that never did.
	for _, want := range []string{"not_carried", "rules that colour by content", "formulas", "merged cells"} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should carry %s, got %s", want, answer)
		}
	}
}

// TestTableFromSheetsIntoADocument covers the two passes. A table's cells have no indices
// until the table exists, so the first batch makes it, the document is read back, and the
// second batch fills it — from the last cell backwards, because every insertion moves
// everything after it.
func TestTableFromSheetsIntoADocument(t *testing.T) {
	fake := newFakeGoogle(t).
		answer("/documents/doc:batchUpdate", `{"documentId": "doc", "replies": [{}]}`).
		answer("/spreadsheets/book", bookRectangle).
		answer("/documents/doc", `{"documentId": "doc", "body": {"content": [
          {"startIndex": 0, "endIndex": 1, "sectionBreak": {}},
          {"startIndex": 1, "endIndex": 40, "table": {"rows": 3, "columns": 2, "tableRows": [
            {"tableCells": [{"startIndex": 2, "endIndex": 4}, {"startIndex": 4, "endIndex": 6}]},
            {"tableCells": [{"startIndex": 6, "endIndex": 8}, {"startIndex": 8, "endIndex": 10}]},
            {"tableCells": [{"startIndex": 10, "endIndex": 12}, {"startIndex": 12, "endIndex": 14}]}]}}]}}`)
	h := newHarness(t, fake)

	h.ok(h.registry.docsCopyTableFromSheets(context.Background(), request(map[string]any{
		"source_spreadsheet_id": "book", "source_sheet_title": "Цели",
		"start_row": float64(0), "end_row": float64(3),
		"start_column": float64(0), "end_column": float64(2),
		"target_document_id": "doc", "target_index": float64(1),
	})))

	checkGolden(t, "copy_table_to_doc.json", h.bodyOf(t, len(h.google.requests)-1))

	// The first batch makes the table and nothing else: filling it in the same batch would
	// address cells that do not exist yet.
	first := ""
	for index, sent := range h.google.requests {
		if strings.Contains(sent.Path, ":batchUpdate") {
			first = string(h.bodyOf(t, index))
			break
		}
	}
	if !strings.Contains(first, "insertTable") || strings.Contains(first, "insertText") {
		t.Errorf("the first batch should only make the table, got %s", first)
	}
}

// documentWithProse is a heading, a paragraph and two list items — what a section of an offer
// looks like, and what a slide's body has to end up holding.
const documentWithProse = `{
  "documentId": "doc",
  "title": "Оффер",
  "body": {"content": [
    {"startIndex": 0, "endIndex": 1, "sectionBreak": {}},
    {"startIndex": 1, "endIndex": 20, "paragraph": {
      "paragraphStyle": {"namedStyleType": "HEADING_1", "alignment": "CENTER",
        "indentStart": {"magnitude": 36, "unit": "PT"}},
      "elements": [{"startIndex": 1, "endIndex": 20, "textRun": {"content": "Условия работы\n",
        "textStyle": {"bold": true, "fontSize": {"magnitude": 18, "unit": "PT"},
          "weightedFontFamily": {"fontFamily": "Rubik", "weight": 500},
          "foregroundColor": {"color": {"rgbColor": {"red": 0.6}}}}}}]}},
    {"startIndex": 20, "endIndex": 44, "paragraph": {
      "bullet": {"listId": "l1", "nestingLevel": 0},
      "paragraphStyle": {"indentStart": {"magnitude": 36, "unit": "PT"}},
      "elements": [{"startIndex": 20, "endIndex": 44, "textRun": {"content": "Удалённо\n"}}]}},
    {"startIndex": 44, "endIndex": 70, "paragraph": {
      "bullet": {"listId": "l1", "nestingLevel": 1},
      "elements": [{"startIndex": 44, "endIndex": 70, "textRun": {"content": "Гибкий график\n"}}]}},
    {"startIndex": 70, "endIndex": 100, "table": {"rows": 2, "columns": 2}}
  ]}
}`

// TestTextFromDocsOntoASlide: the depth of a list is tabs in the text and the indents are left
// behind, which is the lesson a copied slide taught — Slides derives the indents from the
// depth, and sending both counts the depth twice.
func TestTextFromDocsOntoASlide(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/documents/doc", documentWithProse))

	answer := h.ok(h.registry.slidesCopyTextFromDocs(context.Background(), request(map[string]any{
		"source_document_id": "doc", "start_index": float64(1), "end_index": float64(100),
		"target_presentation_id": "deck", "target_page_object_id": "slide1",
		"x_emu": float64(311700), "y_emu": float64(900000),
		"width_emu": float64(5000000), "height_emu": float64(2500000),
	})))

	checkGolden(t, "copy_text_to_slide.json", h.bodyOf(t, 1))

	body := string(h.bodyOf(t, 1))
	if strings.Contains(body, "indentStart") {
		t.Errorf("a list item's indents come from its depth and must not be sent, got %s", body)
	}
	if !strings.Contains(body, `\tГибкий график`) {
		t.Errorf("the second level should be a tab in the text, got %s", body)
	}

	// A table in the range has no place in a text box and is named rather than flattened.
	if !strings.Contains(answer, "a table, which is not text") {
		t.Errorf("the answer should name the table, got %s", answer)
	}
}

// TestSlideImageIntoADocument: a slide has no equivalent in a document, so what crosses is its
// rendering — and the answer says plainly that a snapshot stops following its slide.
func TestSlideImageIntoADocument(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/thumbnail", `{"contentUrl": "https://example.invalid/slide.png", "width": 800, "height": 450}`).
		answer("/documents/doc:batchUpdate", `{"documentId": "doc", "replies": [{}]}`).
		answer("/documents/doc", `{"documentId": "doc", "body": {"content": [
          {"startIndex": 0, "endIndex": 1, "sectionBreak": {}},
          {"startIndex": 1, "endIndex": 2, "paragraph": {"elements": [
            {"startIndex": 1, "endIndex": 2, "textRun": {"content": "\n"}}]}}]}}`))

	answer := h.ok(h.registry.docsCopySlideImage(context.Background(), request(map[string]any{
		"source_presentation_id": "deck", "source_page_object_id": "slide1",
		"target_document_id": "doc", "width_pt": float64(400),
	})))

	body := string(h.bodyOf(t, len(h.google.requests)-1))
	for _, want := range []string{"insertInlineImage", "example.invalid/slide.png"} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}

	// The height follows the slide's proportions: 400 pt wide on a 800×450 rendering is 225
	// pt tall, and a squashed picture of a slide reads as a mistake.
	if !strings.Contains(body, `"magnitude": 225`) {
		t.Errorf("the height should follow the slide's proportions, got %s", body)
	}
	if !strings.Contains(answer, "snapshot") {
		t.Errorf("the answer should say the picture stops following the slide, got %s", answer)
	}
}

// TestTableFromADocumentIntoAWorkbook: the values land as values, so a column of figures can
// be summed. Written raw, every figure out of a document arrives as text and the first sum
// over it is zero.
func TestTableFromADocumentIntoAWorkbook(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/values/", `{"updatedRange": "'Лист1'!A1:B3", "updatedCells": 6}`).
		answer("/documents/doc", `{"documentId": "doc", "body": {"content": [
          {"startIndex": 1, "endIndex": 60, "table": {"rows": 2, "columns": 2, "tableRows": [
            {"tableCells": [
              {"startIndex": 2, "tableCellStyle": {"columnSpan": 2},
               "content": [{"paragraph": {"elements": [{"textRun": {"content": "Команда\n"}}]}}]},
              {"startIndex": 12,
               "content": [{"paragraph": {"elements": [{"textRun": {"content": "Закрыто\n"}}]}}]}]},
            {"tableCells": [
              {"startIndex": 22,
               "content": [{"paragraph": {"elements": [{"textRun": {"content": "SRE\n"}}]}}]},
              {"startIndex": 32,
               "content": [{"paragraph": {"elements": [{"textRun": {"content": "12\n"}}]}}]}]}]}}]}}`))

	answer := h.ok(h.registry.sheetsCopyTableFromDocs(context.Background(), request(map[string]any{
		"source_document_id": "doc", "table_start_index": float64(1),
		"target_spreadsheet_id": "book", "target_sheet_title": "Лист1",
	})))

	last := h.google.requests[len(h.google.requests)-1]
	if !strings.Contains(last.Query, "USER_ENTERED") {
		t.Errorf("the values should be parsed as if typed, got query %s", last.Query)
	}
	if body := string(h.bodyOf(t, len(h.google.requests)-1)); !strings.Contains(body, "SRE") {
		t.Errorf("the cells should be written, got %s", body)
	}

	for _, want := range []string{"the look of each cell", "merged cells"} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should name %s, got %s", want, answer)
		}
	}
}

// TestTableFromDocsNeedsOneSource: naming both a document and a presentation is a caller who
// has not decided, and guessing which they meant is how a table comes out of the wrong file.
func TestTableFromDocsNeedsOneSource(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	for _, probe := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "both named",
			args: map[string]any{"source_document_id": "doc", "source_presentation_id": "deck"},
			want: "not both",
		},
		{
			name: "neither named",
			args: map[string]any{},
			want: "name where the table is",
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			args := map[string]any{"target_spreadsheet_id": "book", "target_sheet_title": "Лист1"}
			for key, value := range probe.args {
				args[key] = value
			}

			message := h.fail(h.registry.sheetsCopyTableFromDocs(context.Background(), request(args)))
			if !strings.Contains(message, probe.want) {
				t.Errorf("the refusal should say %q, got %s", probe.want, message)
			}
		})
	}
}
