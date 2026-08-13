package tools

import (
	"context"
	"strings"
	"testing"
)

func TestSheetsAddChart(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsAddChart(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы", "type": "COLUMN",
		"title": "Задачи по командам", "subtitle": "за квартал",
		"labels_column": float64(0), "value_columns": []any{float64(2), float64(3)},
		"start_row": float64(0), "end_row": float64(20), "header_rows": float64(1),
		"axis_title": "Команда", "value_axis_title": "Задач", "stacked": "STACKED",
		"anchor_row": float64(22), "anchor_column": float64(0),
		"width_pixels": float64(600), "height_pixels": float64(371),
	})))

	checkGolden(t, "sheets_add_chart.json", h.bodyOf(t, 1))
}

func TestSheetsAddPieChartOnItsOwnTab(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsAddChart(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы", "type": "PIE",
		"title": "Доли", "labels_column": float64(0), "value_columns": []any{float64(2)},
		"start_row": float64(1), "end_row": float64(6), "pie_hole": 0.4,
		"own_tab": true, "legend": "LABELED_LEGEND",
	})))

	checkGolden(t, "sheets_add_pie.json", h.bodyOf(t, 1))
}

func TestSheetsAddChartRefusals(t *testing.T) {
	h := structureHarness(t)

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"a kind nobody draws", map[string]any{"type": "RADAR"}, "COLUMN, BAR, LINE"},
		{"a pie of two series", map[string]any{"type": "PIE",
			"value_columns": []any{float64(2), float64(3)}}, "a pie draws one column"},
		{"no numbers at all", map[string]any{"value_columns": []any{}}, "value_columns is empty"},
		{"a stacking nobody has", map[string]any{"stacked": "PILED"}, "STACKED or PERCENT_STACKED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{"spreadsheet_id": "book", "sheet_title": "Отделы",
				"labels_column": 0.0, "value_columns": []any{2.0},
				"start_row": 0.0, "end_row": 5.0}
			for key, value := range test.args {
				args[key] = value
			}
			if message := h.fail(h.registry.sheetsAddChart(context.Background(),
				request(args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}

// TestSheetsAddTable pins the one path to a chip-style dropdown and a banding that follows
// rows added to it: neither is reachable by formatting cells.
func TestSheetsAddTable(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsAddTable(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы", "name": "Статусы",
		"start_row": float64(0), "end_row": float64(20),
		"start_column": float64(0), "end_column": float64(3),
		"columns": []any{
			map[string]any{"column": float64(0), "name": "Задача", "type": "TEXT"},
			map[string]any{"column": float64(1), "name": "Статус", "type": "DROPDOWN",
				"values": []any{"Всё ок", "Есть риск"}},
			map[string]any{"column": float64(2), "name": "Готово", "type": "PERCENT"},
		},
		"header_color": "#D9EAD3", "first_band_color": "#FFFFFF", "second_band_color": "#F6F6F6",
	})))

	checkGolden(t, "sheets_add_table.json", h.bodyOf(t, 1))

	if message := h.fail(h.registry.sheetsAddTable(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы", "name": "Статусы",
		"start_row": 0.0, "end_row": 2.0, "start_column": 0.0, "end_column": 1.0,
		"columns": []any{map[string]any{"column": float64(0), "type": "TEXT",
			"values": []any{"да", "нет"}}},
	}))); !strings.Contains(message, "belongs to a DROPDOWN") {
		t.Errorf("a list on a text column should be refused, got %q", message)
	}
}

func TestSheetsFindReplace(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsFindReplace(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"find": "Q1", "replacement": "Q2", "match_entire_cell": true,
		"start_row": float64(1), "end_row": float64(20),
		"start_column": float64(3), "end_column": float64(4),
	})))

	checkGolden(t, "sheets_find_replace.json", h.bodyOf(t, 1))

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"nowhere to run", map[string]any{}, "name a sheet_title"},
		{"both at once", map[string]any{"sheet_title": "Отделы", "all_sheets": true}, "alternatives"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{"spreadsheet_id": "book", "find": "а", "replacement": "б"}
			for key, value := range test.args {
				args[key] = value
			}
			if message := h.fail(h.registry.sheetsFindReplace(context.Background(),
				request(args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}

func TestSheetsTrimSplitAndFill(t *testing.T) {
	h := structureHarness(t)

	h.ok(h.registry.sheetsTrimWhitespace(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(1), "end_row": float64(20),
		"start_column": float64(4), "end_column": float64(5),
	})))
	checkGolden(t, "sheets_trim.json", h.bodyOf(t, 1))

	h.ok(h.registry.sheetsSplitColumn(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"column": float64(4), "start_row": float64(1), "end_row": float64(20),
		"separator": "CUSTOM", "custom_separator": " — ",
	})))
	checkGolden(t, "sheets_split_column.json", h.bodyOf(t, 3))

	h.ok(h.registry.sheetsAutoFill(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"start_row": float64(1), "end_row": float64(3),
		"start_column": float64(0), "end_column": float64(1),
		"direction": "ROWS", "length": float64(10),
	})))
	checkGolden(t, "sheets_auto_fill.json", h.bodyOf(t, 5))

	if message := h.fail(h.registry.sheetsSplitColumn(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Отделы",
		"column": 0.0, "start_row": 0.0, "end_row": 1.0, "separator": "CUSTOM",
	}))); !strings.Contains(message, "custom_separator is empty") {
		t.Errorf("a custom split with no separator should be refused, got %q", message)
	}
}
