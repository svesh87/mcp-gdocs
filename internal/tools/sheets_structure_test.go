package tools

import (
	"context"
	"strings"
	"testing"
)

// structureHarness is the pair of answers every one of these tools needs: the tab list it
// resolves a name against, and the reply to the batch it then sends.
func structureHarness(t *testing.T) *harness {
	t.Helper()
	return newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetInfo))
}

func TestSheetsSetBorders(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsSetBorders(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(0), "end_row": float64(5),
		"start_column": float64(0), "end_column": float64(3),
		"style": "SOLID_THICK", "color": "#B7B7B7",
		"sides": []any{"top", "inner_horizontal"},
	})))

	checkGolden(t, "sheets_set_borders.json", h.bodyOf(t, 1))
}

// TestSheetsSetBordersTakesHex pins the pair the reading and the writing make: a colour
// comes back as #RRGGBB and has to go in the same way, or every call needs a conversion
// nobody remembers.
func TestSheetsSetBordersTakesHex(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsSetBorders(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(0), "end_row": float64(1),
		"start_column": float64(0), "end_column": float64(1),
		"color": "#3D85C6",
	})))

	body := string(h.bodyOf(t, 1))
	for _, want := range []string{`"red": 0.23921568627450981`, `"green": 0.5215686274509804`} {
		if !strings.Contains(body, want) {
			t.Errorf("the hex colour should have become numbers, want %s in %s", want, body)
		}
	}
}

func TestSheetsSetBordersRefusals(t *testing.T) {
	h := structureHarness(t)

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"a style nobody draws", map[string]any{"style": "WAVY"}, "SOLID, SOLID_MEDIUM"},
		{"a side that is not one", map[string]any{"sides": []any{"middle"}}, "inner_horizontal"},
		{"a colour that is not one", map[string]any{"color": "синий"}, "#RRGGBB"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{"spreadsheet_id": "book", "sheet_title": "Отделы",
				"start_row": 0.0, "end_row": 1.0, "start_column": 0.0, "end_column": 1.0}
			for key, value := range test.args {
				args[key] = value
			}
			if message := h.fail(h.registry.sheetsSetBorders(context.Background(),
				request(args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}

func TestSheetsSetConditionalFormat(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsSetConditionalFormat(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(1), "end_row": float64(100),
		"start_column": float64(10), "end_column": float64(11),
		"condition": "TEXT_EQ", "values": []any{"Застряли"},
		"background_color": "#F4CCCC", "bold": true,
	})))

	checkGolden(t, "sheets_conditional_format.json", h.bodyOf(t, 1))
}

func TestSheetsSetConditionalGradient(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsSetConditionalFormat(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(1), "end_row": float64(20),
		"start_column": float64(11), "end_column": float64(12),
		"gradient": []any{
			map[string]any{"type": "MIN", "color": "#FFFFFF"},
			map[string]any{"type": "PERCENTILE", "color": "#FFD966", "value": "50"},
			map[string]any{"type": "MAX", "color": "#57BB8A"},
		},
	})))

	checkGolden(t, "sheets_conditional_gradient.json", h.bodyOf(t, 1))
}

func TestSheetsSetConditionalFormatRefusals(t *testing.T) {
	h := structureHarness(t)

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"neither a condition nor a gradient", map[string]any{}, "name what the rule does"},
		{"both at once", map[string]any{"condition": "BLANK",
			"gradient": []any{map[string]any{"type": "MIN", "color": "#FFF"}}}, "not both"},
		{"a condition with the wrong count", map[string]any{"condition": "NUMBER_BETWEEN",
			"values": []any{"1"}, "bold": true}, "takes 2 value(s)"},
		{"a rule that changes nothing", map[string]any{"condition": "NOT_BLANK"}, "nothing would change"},
		{"a gradient point with no value", map[string]any{"gradient": []any{
			map[string]any{"type": "MIN", "color": "#FFFFFF"},
			map[string]any{"type": "NUMBER", "color": "#57BB8A"}}}, "needs a value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{"spreadsheet_id": "book", "sheet_title": "Отделы",
				"start_row": 0.0, "end_row": 1.0, "start_column": 0.0, "end_column": 1.0}
			for key, value := range test.args {
				args[key] = value
			}
			if message := h.fail(h.registry.sheetsSetConditionalFormat(context.Background(),
				request(args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}

func TestSheetsSetBanding(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsSetBanding(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(0), "end_row": float64(20),
		"start_column": float64(0), "end_column": float64(5),
		"header_color": "#D9D9D9", "first_band_color": "#FFFFFF", "second_band_color": "#F3F3F3",
	})))

	checkGolden(t, "sheets_banding.json", h.bodyOf(t, 1))

	if message := h.fail(h.registry.sheetsSetBanding(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": 0.0, "end_row": 1.0, "start_column": 0.0, "end_column": 1.0,
		"first_band_color": "#FFFFFF",
	}))); !strings.Contains(message, "second_band_color is required") {
		t.Errorf("a banding needs both stripes, got %q", message)
	}
}

func TestSheetsSetFilter(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsSetFilter(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(0), "end_row": float64(20),
		"start_column": float64(0), "end_column": float64(5),
		"hide": []any{map[string]any{"column": float64(1), "values": []any{"Не начали"}}},
		"sort": []any{map[string]any{"column": float64(2), "order": "DESCENDING"}},
	})))

	checkGolden(t, "sheets_filter.json", h.bodyOf(t, 1))
}

// TestSheetsProtectRangeRefusesALockedOutRange: warning_only off with nobody named leaves a
// range only the file's owner can touch, which is never what was meant.
func TestSheetsProtectRange(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsProtectRange(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(0), "end_row": float64(1),
		"start_column": float64(0), "end_column": float64(5),
		"description": "шапка", "warning_only": true,
	})))

	checkGolden(t, "sheets_protect_range.json", h.bodyOf(t, 1))

	if message := h.fail(h.registry.sheetsProtectRange(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы", "warning_only": false,
	}))); !strings.Contains(message, "no editors are named") {
		t.Errorf("locking everyone out should be refused, got %q", message)
	}
}

func TestSheetsAddNamedRange(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsAddNamedRange(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы", "name": "Кварталы",
		"start_row": float64(1), "end_row": float64(6),
		"start_column": float64(3), "end_column": float64(4),
	})))

	checkGolden(t, "sheets_named_range.json", h.bodyOf(t, 1))
}

func TestSheetsDimensionsGrowAndMove(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsInsertDimensions(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"dimension": "ROWS", "at": float64(5), "count": float64(3),
	})))
	checkGolden(t, "sheets_insert_dimensions.json", h.bodyOf(t, 1))

	h.ok(h.registry.sheetsInsertDimensions(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"dimension": "COLUMNS", "count": float64(2),
	})))
	checkGolden(t, "sheets_append_dimensions.json", h.bodyOf(t, 3))

	h.ok(h.registry.sheetsMoveDimensions(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"dimension": "COLUMNS", "start": float64(4), "end": float64(5), "to": float64(2),
	})))
	checkGolden(t, "sheets_move_dimensions.json", h.bodyOf(t, 5))

	// Rows and columns are only ever added here: a negative count is a deletion asked for
	// in the other direction.
	if message := h.fail(h.registry.sheetsInsertDimensions(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы", "count": float64(-2),
	}))); !strings.Contains(message, "never takes any away") {
		t.Errorf("a negative count should be refused, got %q", message)
	}
}

func TestSheetsGroupAndCollapse(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsGroupDimensions(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"dimension": "ROWS", "start": float64(2), "end": float64(9),
	})))
	checkGolden(t, "sheets_group.json", h.bodyOf(t, 1))

	h.ok(h.registry.sheetsCollapseGroup(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"dimension": "ROWS", "start": float64(2), "end": float64(9), "collapsed": true,
	})))
	checkGolden(t, "sheets_collapse_group.json", h.bodyOf(t, 3))
}

func TestSheetsSortRange(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsSortRange(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(1), "end_row": float64(20),
		"start_column": float64(0), "end_column": float64(5),
		"by": []any{
			map[string]any{"column": float64(1), "order": "ASCENDING"},
			map[string]any{"column": float64(3), "order": "DESCENDING"},
		},
	})))

	checkGolden(t, "sheets_sort_range.json", h.bodyOf(t, 1))

	if message := h.fail(h.registry.sheetsSortRange(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": 1.0, "end_row": 2.0, "start_column": 0.0, "end_column": 1.0,
		"by": []any{map[string]any{"column": float64(0), "order": "ВВЕРХ"}},
	}))); !strings.Contains(message, "ASCENDING or DESCENDING") {
		t.Errorf("an order that is not one should be refused, got %q", message)
	}
}

func TestSheetsDuplicateTabAndProperties(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsDuplicateTab(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы", "new_title": "Отделы (копия)",
		"index": float64(2),
	})))
	checkGolden(t, "sheets_duplicate_tab.json", h.bodyOf(t, 1))

	h.ok(h.registry.sheetsUpdateProperties(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "locale": "ru_RU", "time_zone": "Europe/Moscow",
	})))
	checkGolden(t, "sheets_update_properties.json", h.bodyOf(t, 2))

	if message := h.fail(h.registry.sheetsUpdateProperties(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
	}))); !strings.Contains(message, "nothing to change") {
		t.Errorf("a call that changes nothing should be refused, got %q", message)
	}
}

// TestSheetsSetTextRuns pins the one way a cell can hold two looks at once, and the
// refusals that keep the offsets meaning what they say.
func TestSheetsSetTextRuns(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsSetTextRuns(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"row": float64(4), "column": float64(2),
		"text": "Итог: сходится",
		"runs": []any{
			map[string]any{"start": float64(0), "bold": true},
			map[string]any{"start": float64(5), "bold": false, "text_color": "#666666"},
		},
	})))

	checkGolden(t, "sheets_text_runs.json", h.bodyOf(t, 1))

	for _, test := range []struct {
		name string
		runs []any
		want string
	}{
		{"out of order", []any{
			map[string]any{"start": float64(5)},
			map[string]any{"start": float64(2)}}, "not after the run before it"},
		{"past the end", []any{map[string]any{"start": float64(90)}}, "characters long"},
		{"nothing at all", []any{}, "runs is empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if message := h.fail(h.registry.sheetsSetTextRuns(context.Background(), request(map[string]any{
				"spreadsheet_id": "book", "sheet_title": "Отделы",
				"row": 0.0, "column": 0.0, "text": "Итог: сходится", "runs": test.runs,
			}))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}
