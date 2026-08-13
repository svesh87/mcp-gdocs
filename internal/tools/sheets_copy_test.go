package tools

import (
	"context"
	"strings"
	"testing"
)

// spreadsheetLikeASample is a tab the way a real one comes back: a heading row styled
// wholesale, data rows carrying only their own font, a rectangle of dropdowns, a link,
// a note, and a white background somebody entered on purpose.
const spreadsheetLikeASample = `{
  "spreadsheetId": "book",
  "properties": {"title": "Цели"},
  "sheets": [
    {
      "properties": {"sheetId": 0, "title": "Цели", "index": 0,
        "gridProperties": {"rowCount": 993, "columnCount": 31, "frozenRowCount": 1}},
      "merges": [{"sheetId": 0, "startRowIndex": 0, "endRowIndex": 1,
        "startColumnIndex": 0, "endColumnIndex": 2}],
      "data": [
        {
          "startRow": 4,
          "startColumn": 2,
          "columnMetadata": [{"pixelSize": 262}, {"pixelSize": 102, "hiddenByUser": true}],
          "rowMetadata": [{"pixelSize": 68}, {"pixelSize": 50}],
          "rowData": [
            {"values": [
              {"formattedValue": "Статус",
               "userEnteredFormat": {
                 "horizontalAlignment": "LEFT", "verticalAlignment": "MIDDLE",
                 "wrapStrategy": "WRAP",
                 "backgroundColor": {"red": 1, "green": 1, "blue": 1},
                 "textFormat": {"bold": true, "italic": true, "underline": true,
                   "strikethrough": true,
                   "foregroundColor": {"red": 1, "green": 1, "blue": 1}}},
               "dataValidation": {"condition": {"type": "ONE_OF_LIST",
                 "values": [{"userEnteredValue": "Всё ок"}, {"userEnteredValue": "Пауза"}]},
                 "strict": true, "showCustomUi": true}},
              {"formattedValue": "Эпик",
               "note": "поле для номера задачи",
               "hyperlink": "https://example.invalid/board",
               "userEnteredFormat": {"hyperlinkDisplayType": "LINKED",
                 "numberFormat": {"type": "PERCENT", "pattern": "0%"},
                 "textFormat": {"fontFamily": "Arial",
                   "link": {"uri": "https://example.invalid/board"}}},
               "dataValidation": {"condition": {"type": "ONE_OF_LIST",
                 "values": [{"userEnteredValue": "Всё ок"}, {"userEnteredValue": "Пауза"}]},
                 "strict": true, "showCustomUi": true}}
            ]},
            {"values": [
              {"formattedValue": "Всё ок"},
              {"formattedValue": "Пауза"}
            ]}
          ]
        }
      ]
    }
  ]
}`

// TestReadFormatReportsTheSheetsOwnIndexes: a reading of a rectangle that starts partway
// down has to say where it starts, because the writing tools take the sheet's own numbers
// and a cell reported as row 0 would be written over the first row.
func TestReadFormatReportsTheSheetsOwnIndexes(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetLikeASample))

	answer := h.ok(h.registry.sheetsReadFormat(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"range":          "'Цели'!C5:D6",
	})))

	for _, want := range []string{
		`"first_row": 4`,
		`"first_column": 2`,
		`"row": 4`,
		`"column": 2`,
		`"column": 3`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the reading should carry %s, got %s", want, answer)
		}
	}
	if strings.Contains(answer, `"row": 0`) {
		t.Errorf("no cell of C5:D6 sits on row 0, got %s", answer)
	}
}

// TestReadFormatCarriesWhatARebuildNeeds pins the properties that were missing while a
// copy of a real workbook came out looking wrong for reasons nothing reported.
func TestReadFormatCarriesWhatARebuildNeeds(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetLikeASample))

	answer := h.ok(h.registry.sheetsReadFormat(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"range":          "'Цели'!C5:D6",
	})))

	for _, want := range []string{
		`"vertical_alignment": "MIDDLE"`,
		`"italic": true`,
		`"underline": true`,
		`"strikethrough": true`,
		`"link": "https://example.invalid/board"`,
		`"link_display": "LINKED"`,
		`"note": "поле для номера задачи"`,
		`"number_type": "PERCENT"`,
		`"row_heights_pixels"`,
		`68`,
		`"hidden_columns"`,
		`"merges"`,
		// White was entered on that heading on purpose. Dropping it as noise is what
		// paints a coloured block through a cell meant to cover it, and turns white
		// letters on a dark heading black.
		`"background": "#FFFFFF"`,
		`"text_color": "#FFFFFF"`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the reading should carry %s, got %s", want, answer)
		}
	}
}

// TestReadFormatGathersDropdownsIntoRectangles: the same rule on two hundred cells is one
// dropdown, and reporting it per cell gives nothing that can be written back in one call.
func TestReadFormatGathersDropdownsIntoRectangles(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetLikeASample))

	answer := h.ok(h.registry.sheetsReadFormat(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"range":          "'Цели'!C5:D6",
	})))

	for _, want := range []string{
		`"validations"`,
		`"type": "ONE_OF_LIST"`,
		`"start_row": 4`,
		`"end_row": 5`,
		`"start_column": 2`,
		`"end_column": 4`,
		`"strict": true`,
		`"show_dropdown": true`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the dropdowns should come back as rectangles carrying %s, got %s", want, answer)
		}
	}
	// Two columns of the same rule are one rectangle, not two.
	if count := strings.Count(answer, `"type": "ONE_OF_LIST"`); count != 1 {
		t.Errorf("the two columns share a rule and should be one rectangle, got %d", count)
	}
}

// spreadsheetWithEverything is a tab carrying the things that are not a cell's own look:
// rules that colour by content, banding, protection, a filter, groups — plus the cell
// properties a plain format reading leaves out.
const spreadsheetWithEverything = `{
  "spreadsheetId": "book",
  "properties": {"title": "Цели"},
  "sheets": [
    {
      "properties": {"sheetId": 0, "title": "Цели", "index": 0,
        "gridProperties": {"rowCount": 100, "columnCount": 10, "frozenRowCount": 1}},
      "conditionalFormats": [
        {"ranges": [{"sheetId": 0, "startRowIndex": 1, "endRowIndex": 20,
           "startColumnIndex": 0, "endColumnIndex": 1}],
         "booleanRule": {"condition": {"type": "TEXT_EQ",
           "values": [{"userEnteredValue": "Застряли"}]},
           "format": {"backgroundColor": {"red": 0.95, "green": 0.8, "blue": 0.8},
             "textFormat": {"bold": true}}}},
        {"ranges": [{"sheetId": 0, "startRowIndex": 1, "endRowIndex": 20,
           "startColumnIndex": 2, "endColumnIndex": 3}],
         "gradientRule": {
           "minpoint": {"type": "MIN", "color": {"red": 1, "green": 1, "blue": 1}},
           "maxpoint": {"type": "MAX", "color": {"red": 0.34, "green": 0.73, "blue": 0.54}}}}
      ],
      "bandedRanges": [
        {"bandedRangeId": 5, "range": {"sheetId": 0, "startRowIndex": 0, "endRowIndex": 20,
          "startColumnIndex": 0, "endColumnIndex": 5},
         "rowProperties": {"headerColor": {"red": 0.85, "green": 0.85, "blue": 0.85},
           "firstBandColor": {"red": 1, "green": 1, "blue": 1},
           "secondBandColor": {"red": 0.95, "green": 0.95, "blue": 0.95}}}
      ],
      "protectedRanges": [
        {"protectedRangeId": 9, "description": "шапка", "warningOnly": true,
         "range": {"sheetId": 0, "startRowIndex": 0, "endRowIndex": 1,
           "startColumnIndex": 0, "endColumnIndex": 5}}
      ],
      "basicFilter": {"range": {"sheetId": 0, "startRowIndex": 0, "endRowIndex": 20,
         "startColumnIndex": 0, "endColumnIndex": 5},
        "sortSpecs": [{"dimensionIndex": 2, "sortOrder": "DESCENDING"}],
        "filterSpecs": [{"columnIndex": 1,
          "filterCriteria": {"hiddenValues": ["Не начали"]}}]},
      "filterViews": [
        {"filterViewId": 3, "title": "Только риски",
         "range": {"sheetId": 0, "startRowIndex": 0, "endRowIndex": 20,
           "startColumnIndex": 0, "endColumnIndex": 5}}
      ],
      "rowGroups": [
        {"range": {"sheetId": 0, "dimension": "ROWS", "startIndex": 2, "endIndex": 9},
         "depth": 1, "collapsed": true}
      ],
      "data": [
        {
          "columnMetadata": [{"pixelSize": 200}],
          "rowMetadata": [{"pixelSize": 40}, {"pixelSize": 21, "hiddenByUser": true}],
          "rowData": [
            {"values": [
              {"formattedValue": "Итог: сходится",
               "userEnteredFormat": {
                 "textRotation": {"angle": 45},
                 "padding": {"top": 6, "right": 8, "bottom": 6, "left": 8},
                 "borders": {
                   "bottom": {"style": "SOLID_THICK", "width": 3,
                     "color": {"red": 0.26, "green": 0.26, "blue": 0.26}},
                   "top": {"style": "SOLID", "width": 1}}},
               "textFormatRuns": [
                 {"format": {"bold": true}},
                 {"startIndex": 5, "format": {"bold": false,
                   "foregroundColor": {"red": 0.4, "green": 0.4, "blue": 0.4},
                   "link": {"uri": "https://example.invalid/итог"}}}
               ]}
            ]}
          ]
        }
      ]
    }
  ]
}`

// TestReadFormatDescribesWhatIsNotACellsOwnLook: a copy built from a reading that stops at
// the cells comes out with no rules, no banding, no filter and no groups, and looks right
// exactly until the data changes.
func TestReadFormatDescribesWhatIsNotACellsOwnLook(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetWithEverything))

	answer := h.ok(h.registry.sheetsReadFormat(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"range":          "'Цели'!A1:E20",
	})))

	for _, want := range []string{
		`"conditional_formats"`,
		`"condition": "TEXT_EQ"`,
		`"background_color": "#F2CCCC"`,
		`"gradient"`,
		`"type": "MAX"`,
		`"bandings"`,
		`"first_band_color": "#FFFFFF"`,
		`"protected_ranges"`,
		`"description": "шапка"`,
		`"basic_filter"`,
		`"values": [`,
		`"Не начали"`,
		`"order": "DESCENDING"`,
		`"filter_views"`,
		`"title": "Только риски"`,
		`"row_groups"`,
		`"collapsed": true`,
		`"hidden_rows"`,
		// The cell's own half that a plain reading leaves out.
		`"rotation_angle": 45`,
		`"padding"`,
		`"borders"`,
		`"style": "SOLID_THICK"`,
		`"runs"`,
		`"start": 5`,
		`"link": "https://example.invalid/итог"`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the reading should carry %s, got %s", want, answer)
		}
	}
}

func TestSheetsInfoReportsWhatACopyCannotCarry(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", `{
	  "spreadsheetId": "book", "properties": {"title": "Цели"},
	  "sheets": [{"properties": {"sheetId": 0, "title": "Цели",
	    "gridProperties": {"rowCount": 993, "columnCount": 31, "frozenRowCount": 1},
	    "tabColor": {"red": 1, "green": 0, "blue": 0}},
	    "merges": [{"sheetId": 0}],
	    "conditionalFormats": [{"ranges": []}],
	    "bandedRanges": [{"bandedRangeId": 1}],
	    "basicFilter": {"range": {}}}]}`))

	answer := h.ok(h.registry.sheetsInfo(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
	})))

	for _, want := range []string{
		`"merges": 1`,
		`"conditional_formats": 1`,
		`"banded_ranges": 1`,
		`"basic_filter": true`,
		`"tab_color": "#FF0000"`,
		`"rows": 993`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the description should carry %s, got %s", want, answer)
		}
	}
}

func TestSheetsFormatCellsCarriesEveryProperty(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetInfo))

	h.ok(h.registry.sheetsFormatCells(context.Background(), request(map[string]any{
		"spreadsheet_id":       "book",
		"sheet_title":          "Отделы",
		"start_row":            float64(1),
		"end_row":              float64(19),
		"start_column":         float64(0),
		"end_column":           float64(1),
		"bold":                 false,
		"italic":               true,
		"underline":            true,
		"strikethrough":        true,
		"font_family":          "Arial",
		"horizontal_alignment": "LEFT",
		"vertical_alignment":   "MIDDLE",
		"wrap":                 "WRAP",
		"number_format":        "0%",
		"number_type":          "PERCENT",
		"link":                 "https://example.invalid/board",
		"link_display":         "LINKED",
		"note":                 "откуда взялось",
	})))

	checkGolden(t, "sheets_format_cells_everything.json", h.bodyOf(t, 1))
}

func TestSheetsFormatCellsRefusesUnknownWords(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetInfo))

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"vertical alignment", map[string]any{"vertical_alignment": "CENTRE"}, "TOP, MIDDLE or BOTTOM"},
		{"wrap", map[string]any{"wrap": "FOLD"}, "WRAP, OVERFLOW_CELL or CLIP"},
		{"link display", map[string]any{"link_display": "BLUE"}, "LINKED or PLAIN_TEXT"},
		{"number type", map[string]any{"number_format": "0%", "number_type": "PERCENTAGE"}, "NUMBER, PERCENT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{"spreadsheet_id": "book", "sheet_title": "Отделы",
				"start_row": 0.0, "end_row": 1.0, "start_column": 0.0, "end_column": 1.0}
			for key, value := range test.args {
				args[key] = value
			}
			if message := h.fail(h.registry.sheetsFormatCells(context.Background(),
				request(args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}

func TestSheetsSetValidation(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetInfo))

	h.ok(h.registry.sheetsSetValidation(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"sheet_title":    "Отделы",
		"start_row":      float64(1),
		"end_row":        float64(19),
		"start_column":   float64(10),
		"end_column":     float64(11),
		"values":         []any{"Всё ок", "Есть риск", "Пауза"},
		"strict":         true,
		"show_dropdown":  true,
	})))

	checkGolden(t, "sheets_set_validation.json", h.bodyOf(t, 1))
}

func TestSheetsSetValidationRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetInfo))

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"no values", map[string]any{}, "values is empty"},
		{"a range given as a list", map[string]any{"type": "ONE_OF_RANGE",
			"values": []any{"=Лист1!A2:A9", "=Лист1!B2:B9"}}, "exactly one value"},
		{"a kind nobody has", map[string]any{"type": "ONE_OF_ANYTHING",
			"values": []any{"да"}}, "ONE_OF_LIST or ONE_OF_RANGE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{"spreadsheet_id": "book", "sheet_title": "Отделы",
				"start_row": 0.0, "end_row": 1.0, "start_column": 0.0, "end_column": 1.0}
			for key, value := range test.args {
				args[key] = value
			}
			if message := h.fail(h.registry.sheetsSetValidation(context.Background(),
				request(args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}

// spreadsheetSized is a tab with a size, which is what a refusal to shrink one needs.
const spreadsheetSized = `{
  "spreadsheetId": "book",
  "properties": {"title": "Цели"},
  "sheets": [{"properties": {"sheetId": 0, "title": "Цели",
    "gridProperties": {"rowCount": 1000, "columnCount": 26}}}]}`

func TestSheetsSetLayoutSetsRowHeightsAndGrowsTheGrid(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/spreadsheets/book:batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetSized))

	h.ok(h.registry.sheetsSetLayout(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"sheet_title":    "Цели",
		"row_heights": []any{
			map[string]any{"row": float64(0), "pixels": float64(68)},
			map[string]any{"row": float64(1), "through_row": float64(19), "pixels": float64(50)},
		},
		"columns": float64(31),
	})))

	checkGolden(t, "sheets_set_layout_rows.json", h.bodyOf(t, 1))
}

// TestSheetsSetLayoutRefusesToShrinkTheGrid: a smaller grid is a deleted row, and this
// server deletes nothing. The refusal has to say where the size can be set instead.
func TestSheetsSetLayoutRefusesToShrinkTheGrid(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetSized))

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"fewer rows", map[string]any{"rows": float64(993)}, "when the tab is created"},
		{"fewer columns", map[string]any{"columns": float64(10)}, "when the tab is created"},
		{"a run that ends before it starts", map[string]any{
			"row_heights": []any{map[string]any{"row": float64(9), "through_row": float64(2),
				"pixels": float64(50)}}}, "before row 9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{"spreadsheet_id": "book", "sheet_title": "Цели"}
			for key, value := range test.args {
				args[key] = value
			}
			if message := h.fail(h.registry.sheetsSetLayout(context.Background(),
				request(args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}

// TestSheetsSetLayoutHidesShowsAndPaints covers the arms of set_layout that are not sizes:
// hiding is not deleting, and it comes back with hidden false.
func TestSheetsSetLayoutHidesShowsAndPaints(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/spreadsheets/book:batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetSized))

	h.ok(h.registry.sheetsSetLayout(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"sheet_title":    "Цели",
		"hide_columns": []any{
			map[string]any{"column": float64(9), "through_column": float64(25), "hidden": true},
		},
		"hide_rows": []any{
			map[string]any{"row": float64(4), "hidden": false},
		},
		"auto_resize_columns": []any{float64(2), float64(3)},
		"tab_color":           "#3D85C6",
		"unmerge": []any{
			map[string]any{"start_row": float64(0), "end_row": float64(1),
				"start_column": float64(0), "end_column": float64(3)},
		},
	})))

	checkGolden(t, "sheets_set_layout_hiding.json", h.bodyOf(t, 1))
}

func TestSheetsCreateWithTheSampleSize(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets",
		`{"spreadsheetId": "new_book", "properties": {"title": "Цели"},
		  "sheets": [{"properties": {"sheetId": 0, "title": "Цели"}}]}`))

	h.ok(h.registry.sheetsCreate(context.Background(), request(map[string]any{
		"title":     "Цели",
		"locale":    "ru_RU",
		"time_zone": "Europe/Moscow",
		"sheets": []any{
			map[string]any{"title": "Цели", "rows": float64(993), "columns": float64(31),
				"frozen_rows": float64(1)},
			map[string]any{"title": "заметки"},
		},
	})))

	checkGolden(t, "sheets_create_sized.json", h.bodyOf(t, 0))
}

func TestSheetsCreateRefusesTwoWaysOfNamingTabs(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.sheetsCreate(context.Background(), request(map[string]any{
		"title":        "Цели",
		"sheet_titles": []any{"Данные"},
		"sheets":       []any{map[string]any{"title": "Цели"}},
	})))

	if !strings.Contains(message, "not both") {
		t.Errorf("the refusal should say to name the tabs once, got %q", message)
	}
}

func TestSheetsAddTabWithASize(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer(":batchUpdate",
		`{"spreadsheetId": "book", "replies": [{"addSheet": {"properties": {"sheetId": 91, "title": "Новый"}}}]}`))

	h.ok(h.registry.sheetsAddTab(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"title":          "Новый",
		"rows":           float64(994),
		"columns":        float64(29),
		"frozen_rows":    float64(1),
	})))

	checkGolden(t, "sheets_add_tab_sized.json", h.bodyOf(t, 0))
}
