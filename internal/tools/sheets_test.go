package tools

import (
	"context"
	"strings"
	"testing"
)

const spreadsheetInfo = `{
  "spreadsheetId": "book",
  "spreadsheetUrl": "https://example.invalid/d/book",
  "properties": {"title": "Люди", "locale": "ru_RU", "timeZone": "Europe/Belgrade"},
  "sheets": [
    {"properties": {"sheetId": 0, "title": "Сотрудники", "index": 0,
      "gridProperties": {"rowCount": 500, "columnCount": 12, "frozenRowCount": 1}}},
    {"properties": {"sheetId": 77, "title": "Отделы", "index": 1}}
  ]
}`

func TestSheetsInfo(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetInfo))

	answer := h.ok(h.registry.sheetsInfo(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
	})))

	for _, want := range []string{"Сотрудники", `"sheet_id": 77`, `"frozen_rows": 1`} {
		if !strings.Contains(answer, want) {
			t.Errorf("the description should carry %s, got %s", want, answer)
		}
	}
}

// spreadsheetWithCharts is a tab carrying three charts, which is what a workbook built to
// feed a deck looks like.
const spreadsheetWithCharts = `{
  "spreadsheetId": "book",
  "properties": {"title": "Данные графиков"},
  "sheets": [
    {"properties": {"sheetId": 0, "title": "Категории", "index": 0},
     "charts": [
       {"chartId": 1147694029, "spec": {"title": "Закрытые задачи по категориям"},
        "position": {"overlayPosition": {"anchorCell": {"sheetId": 0, "rowIndex": 12, "columnIndex": 1}}}},
       {"chartId": 883012, "spec": {"title": "Попугаи по категориям"},
        "position": {"newSheet": true}}
     ]}
  ]
}`

// TestSheetsInfoNamesTheCharts: a chart is addressed by a number, and until this was
// reported there was nowhere to learn it. A count said a chart existed and left it
// unusable — gdocs_slides_replace_shapes_with_chart takes the number and nothing else.
func TestSheetsInfoNamesTheCharts(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetWithCharts))

	answer := h.ok(h.registry.sheetsInfo(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
	})))

	for _, want := range []string{
		`"chart_id": 1147694029`,
		"Закрытые задачи по категориям",
		`"anchor_row": 12`,
		`"chart_id": 883012`,
		`"own_tab": true`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the description should carry %s, got %s", want, answer)
		}
	}
}

// TestSheetsAddChartHandsBackTheIdentifier: the number Google assigns is the only handle
// the chart will ever have, and dropping the reply meant a chart could be made and then
// never pointed at.
func TestSheetsAddChartHandsBackTheIdentifier(t *testing.T) {
	fake := newFakeGoogle(t).
		answer(":batchUpdate", `{"replies": [{"addChart": {"chart": {"chartId": 42}}}]}`).
		answer("/spreadsheets/book", spreadsheetInfo)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.sheetsAddChart(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Сотрудники", "type": "COLUMN",
		"title": "Закрытые задачи", "labels_column": float64(0),
		"value_columns": []any{float64(1)},
		"start_row":     float64(0), "end_row": float64(6),
	})))

	if !strings.Contains(answer, `"chart_id": 42`) {
		t.Errorf("the answer should carry the new chart's number, got %s", answer)
	}
}

func TestSheetsReadWholeTab(t *testing.T) {
	fake := newFakeGoogle(t).answer("/values/",
		`{"range": "'Сотрудники'!A1:C2", "majorDimension": "ROWS", "values": [["Имя", "Отдел"], ["Аня", "SRE"]]}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.sheetsRead(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"sheet_title":    "Сотрудники",
	})))

	if !strings.Contains(answer, `"rows": 2`) {
		t.Errorf("both rows should come back, got %s", answer)
	}

	// A tab named in Cyrillic with no range is read whole, and its name is quoted the
	// way Sheets wants before it goes into the path.
	if path := h.google.requests[0].Path; !strings.Contains(path, "'Сотрудники'") {
		t.Errorf("the tab name should be quoted in the range, got %s", path)
	}
}

func TestSheetsReadNeedsATarget(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	if message := h.fail(h.registry.sheetsRead(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
	}))); !strings.Contains(message, "name what to read") {
		t.Errorf("expected a refusal asking what to read, got %q", message)
	}
}

func TestSheetsReadRefusesUnknownRender(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	if message := h.fail(h.registry.sheetsRead(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"range":          "A1:B2",
		"value_render":   "PRETTY",
	}))); !strings.Contains(message, "FORMATTED_VALUE") {
		t.Errorf("expected a refusal naming the options, got %q", message)
	}
}

func TestSheetsWrite(t *testing.T) {
	fake := newFakeGoogle(t).answer("/values/",
		`{"spreadsheetId": "book", "updatedRange": "'Сотрудники'!A1:B2", "updatedRows": 2, "updatedCells": 4}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.sheetsWrite(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"range":          "'Сотрудники'!A1",
		"values":         []any{[]any{"Имя", "Отдел"}, []any{"Аня", "SRE"}},
	})))

	if !strings.Contains(answer, `"updated_cells": 4`) {
		t.Errorf("the answer should report what changed, got %s", answer)
	}

	if method := h.google.requests[0].Method; method != "PUT" {
		t.Errorf("a write over a range is a PUT, got %s", method)
	}
	if query := h.google.requests[0].Query; !strings.Contains(query, "valueInputOption=USER_ENTERED") {
		t.Errorf("the default should read values the way typing them would, got %s", query)
	}

	checkGolden(t, "sheets_write.json", h.bodyOf(t, 0))
}

func TestSheetsAppendInsertsRows(t *testing.T) {
	fake := newFakeGoogle(t).answer("/values/",
		`{"spreadsheetId": "book", "tableRange": "'Сотрудники'!A1:B2",
		  "updates": {"updatedRange": "'Сотрудники'!A3:B3", "updatedRows": 1, "updatedCells": 2}}`)
	h := newHarness(t, fake)

	h.ok(h.registry.sheetsAppend(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"range":          "'Сотрудники'!A:B",
		"values":         []any{[]any{"Боря", "DBA"}},
		"value_input":    "RAW",
	})))

	query := h.google.requests[0].Query
	// INSERT_ROWS is what keeps an append from writing over whatever sits below the
	// table it was aimed at.
	for _, want := range []string{"insertDataOption=INSERT_ROWS", "valueInputOption=RAW"} {
		if !strings.Contains(query, want) {
			t.Errorf("the request should carry %s, got %s", want, query)
		}
	}
}

func TestSheetsWriteRefusesBadValues(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "values not a list",
			args: map[string]any{"spreadsheet_id": "book", "range": "A1", "values": "A,B"},
			want: "must be a list of lists",
		},
		{
			name: "row not a list",
			args: map[string]any{"spreadsheet_id": "book", "range": "A1", "values": []any{"A"}},
			want: "values[0] must be a list",
		},
		{
			name: "unknown input option",
			args: map[string]any{"spreadsheet_id": "book", "range": "A1",
				"values": []any{[]any{"A"}}, "value_input": "MAGIC"},
			want: "USER_ENTERED or RAW",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if message := h.fail(h.registry.sheetsWrite(context.Background(), request(test.args))); !strings.Contains(message, test.want) {
				t.Errorf("expected a refusal mentioning %q, got %q", test.want, message)
			}
		})
	}
}

func TestSheetsCreate(t *testing.T) {
	fake := newFakeGoogle(t).answer("/spreadsheets",
		`{"spreadsheetId": "new_book", "spreadsheetUrl": "https://example.invalid/d/new_book",
		  "properties": {"title": "Отчёт"}, "sheets": [{"properties": {"sheetId": 0, "title": "Данные"}}]}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.sheetsCreate(context.Background(), request(map[string]any{
		"title":        "Отчёт",
		"sheet_titles": []any{"Данные", "Сводка"},
	})))

	if !strings.Contains(answer, "new_book") {
		t.Errorf("the new spreadsheet's identifier should come back, got %s", answer)
	}

	checkGolden(t, "sheets_create.json", h.bodyOf(t, 0))
}

func TestSheetsAddTab(t *testing.T) {
	fake := newFakeGoogle(t).answer(":batchUpdate",
		`{"spreadsheetId": "book", "replies": [{"addSheet": {"properties": {"sheetId": 91, "title": "Новый", "index": 2}}}]}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.sheetsAddTab(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"title":          "Новый",
	})))

	if !strings.Contains(answer, `"sheet_id": 91`) {
		t.Errorf("the new tab's identifier should come back, got %s", answer)
	}
}

func TestSheetsFormatCells(t *testing.T) {
	fake := newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetInfo)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.sheetsFormatCells(context.Background(), request(map[string]any{
		"spreadsheet_id":       "book",
		"sheet_title":          "Отделы",
		"start_row":            float64(0),
		"end_row":              float64(1),
		"start_column":         float64(0),
		"end_column":           float64(3),
		"bold":                 true,
		"font_size":            float64(11),
		"horizontal_alignment": "CENTER",
		"background_color":     map[string]any{"red": 0.9, "green": 0.9, "blue": 0.9},
	})))

	// The tab was named, not numbered: the numeric identifier is looked up first, which
	// is why the request order is read-then-write.
	if !strings.Contains(answer, `"sheet_id": 77`) {
		t.Errorf("the tab should have been resolved to its identifier, got %s", answer)
	}

	checkGolden(t, "sheets_format_cells.json", h.bodyOf(t, 1))
}

// TestAnEmptyStringIsADecision covers the class of defect the deck of 2026.08 ran into: an
// argument named with an empty value was dropped as if it had not been named at all, so the
// old value stayed and the answer still said the call had worked. Nothing that can be set
// here should be impossible to unset.
func TestAnEmptyStringIsADecision(t *testing.T) {
	t.Run("a note comes off a cell", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).
			answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
			answer("/spreadsheets/book", spreadsheetInfo))

		h.ok(h.registry.sheetsFormatCells(context.Background(), request(map[string]any{
			"spreadsheet_id": "book",
			"sheet_title":    "Отделы",
			"start_row":      float64(0),
			"end_row":        float64(1),
			"start_column":   float64(0),
			"end_column":     float64(3),
			"note":           "",
			"link":           "",
		})))

		// Both are in the mask and neither carries a value: that pair is what clears them.
		body := string(h.bodyOf(t, 1))
		if !strings.Contains(body, `"fields": "userEnteredFormat.textFormat(link),note"`) {
			t.Errorf("the mask should name the note and the link so both are written as empty, got %s", body)
		}
		if strings.Contains(body, `"note":`) || strings.Contains(body, `"link":`) {
			t.Errorf("neither should carry a value, got %s", body)
		}
	})

	t.Run("alt text can be taken off", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

		h.ok(h.registry.slidesSetAltText(context.Background(), request(map[string]any{
			"presentation_id": "deck",
			"object_id":       "image1",
			"description":     "",
		})))

		// The description is sent as an empty string; the title, which was not named, is
		// not sent at all and stays whatever it was.
		body := string(h.bodyOf(t, 0))
		if !strings.Contains(body, `"description": ""`) {
			t.Errorf("an empty description should go out as one, got %s", body)
		}
		if strings.Contains(body, `"title"`) {
			t.Errorf("what was not named should not be written, got %s", body)
		}

		if message := h.fail(h.registry.slidesSetAltText(context.Background(), request(map[string]any{
			"presentation_id": "deck",
			"object_id":       "image1",
		}))); !strings.Contains(message, "title, a description, or both") {
			t.Errorf("naming neither is still nothing to do, got %q", message)
		}
	})

	t.Run("a workbook cannot be left unnamed", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetInfo))

		message := h.fail(h.registry.sheetsUpdateProperties(context.Background(), request(map[string]any{
			"spreadsheet_id": "book",
			"title":          "",
		})))
		if !strings.Contains(message, "has to have a name") {
			t.Errorf("an empty name should be refused rather than ignored, got %q", message)
		}
	})
}

func TestSheetsFormatCellsRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetInfo))

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "empty range",
			args: map[string]any{"spreadsheet_id": "book", "sheet_title": "Отделы",
				"start_row": 0.0, "end_row": 0.0, "start_column": 0.0, "end_column": 1.0, "bold": true},
			want: "the range is empty",
		},
		{
			name: "unknown tab",
			args: map[string]any{"spreadsheet_id": "book", "sheet_title": "Нет такого",
				"start_row": 0.0, "end_row": 1.0, "start_column": 0.0, "end_column": 1.0, "bold": true},
			want: "no tab called",
		},
		{
			name: "nothing to change",
			args: map[string]any{"spreadsheet_id": "book", "sheet_title": "Отделы",
				"start_row": 0.0, "end_row": 1.0, "start_column": 0.0, "end_column": 1.0},
			want: "nothing to change",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if message := h.fail(h.registry.sheetsFormatCells(context.Background(), request(test.args))); !strings.Contains(message, test.want) {
				t.Errorf("expected a refusal mentioning %q, got %q", test.want, message)
			}
		})
	}
}

func TestSheetsErrorFromGoogleIsReadable(t *testing.T) {
	fake := newFakeGoogle(t).fail("/spreadsheets/book", 403,
		`{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "The caller does not have permission"}}`)
	h := newHarness(t, fake)

	message := h.fail(h.registry.sheetsInfo(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
	})))

	for _, want := range []string{"403", "PERMISSION_DENIED", "does not have permission"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal should carry %s, got %q", want, message)
		}
	}
}
