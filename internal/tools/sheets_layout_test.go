package tools

import (
	"context"
	"strings"
	"testing"
)

// spreadsheetWithFormat is a tab the way one comes back when the grid is asked for:
// values with their formatting, column widths, and the frozen header.
const spreadsheetWithFormat = `{
  "spreadsheetId": "book",
  "properties": {"title": "Метрики"},
  "sheets": [
    {
      "properties": {"sheetId": 0, "title": "Лист 1", "index": 0,
        "gridProperties": {"rowCount": 100, "columnCount": 10, "frozenRowCount": 1}},
      "data": [
        {
          "columnMetadata": [{"pixelSize": 220}, {"pixelSize": 120}],
          "rowMetadata": [{"pixelSize": 32}, {"pixelSize": 21}],
          "rowData": [
            {"values": [
              {"formattedValue": "Показатель",
               "userEnteredFormat": {
                 "horizontalAlignment": "CENTER",
                 "backgroundColor": {"red": 0.85, "green": 0.85, "blue": 0.85},
                 "textFormat": {"bold": true, "fontSize": 11,
                   "foregroundColor": {"red": 0.2, "green": 0.2, "blue": 0.2}}}},
              {"formattedValue": "Значение",
               "userEnteredFormat": {"textFormat": {"bold": true}}}
            ]},
            {"values": [
              {"formattedValue": "Время сборки"},
              {"formattedValue": "12 мин",
               "userEnteredFormat": {"numberFormat": {"type": "NUMBER", "pattern": "0.0"}}}
            ]}
          ]
        }
      ]
    }
  ]
}`

const spreadsheetTabs = `{
  "spreadsheetId": "book",
  "properties": {"title": "Метрики"},
  "sheets": [
    {"properties": {"sheetId": 0, "title": "Лист 1", "index": 0}},
    {"properties": {"sheetId": 7, "title": "Свод", "index": 1}}
  ]
}`

// TestSheetsReadFormat pins what a sample tab hands back, because a tab is reproduced
// from exactly these numbers: the widths, the frozen rows and the cell formatting.
func TestSheetsReadFormat(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetWithFormat))

	answer := h.ok(h.registry.sheetsReadFormat(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"range":          "'Лист 1'!A1:B2",
	})))

	for _, want := range []string{
		`"Показатель"`,
		`"bold": true`,
		`"alignment": "CENTER"`,
		`220`,
		`"frozen_rows": 1`,
		`"0.0"`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the format reading should carry %s, got %s", want, answer)
		}
	}
}

func TestSheetsReadFormatWithNothingBack(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", `{"spreadsheetId": "book", "sheets": []}`))

	message := h.fail(h.registry.sheetsReadFormat(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"range":          "'Нет такого'!A1:B2",
	})))

	if !strings.Contains(message, "check the tab name") {
		t.Errorf("the refusal should say where to look, got %q", message)
	}
}

func TestSheetsSetLayout(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/spreadsheets/book:batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetTabs))

	h.ok(h.registry.sheetsSetLayout(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"sheet_title":    "Свод",
		"column_widths": []any{
			map[string]any{"column": float64(0), "pixels": float64(220)},
			map[string]any{"column": float64(1), "pixels": float64(120)},
		},
		"frozen_rows": float64(1),
		"merge": []any{
			map[string]any{"start_row": float64(0), "end_row": float64(1),
				"start_column": float64(0), "end_column": float64(2)},
		},
	})))

	checkGolden(t, "sheets_set_layout.json", h.bodyOf(t, 1))
}

func TestSheetsSetLayoutRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/spreadsheets/book:batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetTabs))

	for _, test := range []struct {
		title string
		args  map[string]any
		want  string
	}{
		{
			"a tab that is not there",
			map[string]any{"spreadsheet_id": "book", "sheet_title": "Нет", "frozen_rows": float64(1)},
			"Лист 1, Свод",
		},
		{
			"a width without pixels",
			map[string]any{"spreadsheet_id": "book", "sheet_title": "Свод",
				"column_widths": []any{map[string]any{"column": float64(0)}}},
			"pixels",
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			if message := h.fail(h.registry.sheetsSetLayout(context.Background(),
				request(test.args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}
