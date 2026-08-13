package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// TestFilterViewsAndTheirVariants covers the three things one tool does — make, change,
// copy — because which of them happens is decided by two arguments and nothing else.
func TestFilterViewsAndTheirVariants(t *testing.T) {
	t.Run("changing an existing view", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

		h.ok(h.registry.sheetsFilterView(context.Background(), request(map[string]any{
			"spreadsheet_id": "book", "sheet_title": "Данные",
			"filter_view_id": 3.0, "title": "Просроченные",
		})))

		body := string(h.bodyOf(t, 1))
		if !strings.Contains(body, "updateFilterView") || !strings.Contains(body, `"filterViewId": 3`) {
			t.Errorf("an existing view should be changed, not added again: %s", body)
		}
	})

	t.Run("copying one", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

		h.ok(h.registry.sheetsFilterView(context.Background(), request(map[string]any{
			"spreadsheet_id": "book", "sheet_title": "Данные",
			"filter_view_id": 3.0, "duplicate": true,
		})))

		if body := string(h.bodyOf(t, 1)); !strings.Contains(body, "duplicateFilterView") {
			t.Errorf("duplicating should send its own request, got %s", body)
		}
	})

	t.Run("copying nothing", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

		message := h.fail(h.registry.sheetsFilterView(context.Background(), request(map[string]any{
			"spreadsheet_id": "book", "sheet_title": "Данные", "duplicate": true,
		})))
		if !strings.Contains(message, "filter_view_id") {
			t.Errorf("duplicating needs to know what to copy, got %q", message)
		}
	})

	t.Run("a slicer with no range", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

		message := h.fail(h.registry.sheetsSlicer(context.Background(), request(map[string]any{
			"spreadsheet_id": "book", "sheet_title": "Данные",
		})))
		if !strings.Contains(message, "range it filters") {
			t.Errorf("a new slicer needs its range, got %q", message)
		}
	})

	t.Run("changing a slicer", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

		h.ok(h.registry.sheetsSlicer(context.Background(), request(map[string]any{
			"spreadsheet_id": "book", "sheet_title": "Данные", "slicer_id": 4.0, "title": "Статус",
		})))

		if body := string(h.bodyOf(t, 1)); !strings.Contains(body, "updateSlicerSpec") {
			t.Errorf("an existing slicer should be changed, got %s", body)
		}
	})
}

// TestChangingRatherThanAdding covers the three tools that gained an identifier: given
// one, they change what is there instead of laying a second copy over it.
func TestChangingRatherThanAdding(t *testing.T) {
	for _, probe := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "a named range",
			args: map[string]any{"named_range_id": "nr1"},
			want: "updateNamedRange",
		},
		{
			name: "a fresh named range",
			args: map[string]any{},
			want: "addNamedRange",
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

			args := map[string]any{
				"spreadsheet_id": "book", "sheet_title": "Данные", "name": "итоги",
				"start_row": 0.0, "end_row": 10.0, "start_column": 0.0, "end_column": 2.0,
			}
			for key, value := range probe.args {
				args[key] = value
			}

			h.ok(h.registry.sheetsAddNamedRange(context.Background(), request(args)))

			if body := string(h.bodyOf(t, 1)); !strings.Contains(body, probe.want) {
				t.Errorf("the request should be a %s, got %s", probe.want, body)
			}
		})
	}

	t.Run("a protection", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

		h.ok(h.registry.sheetsProtectRange(context.Background(), request(map[string]any{
			"spreadsheet_id": "book", "sheet_title": "Данные", "protected_range_id": 5.0,
			"description": "итоги считает формула",
		})))

		if body := string(h.bodyOf(t, 1)); !strings.Contains(body, "updateProtectedRange") {
			t.Errorf("an existing protection should be changed, got %s", body)
		}
	})

	t.Run("a table", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

		h.ok(h.registry.sheetsAddTable(context.Background(), request(map[string]any{
			"spreadsheet_id": "book", "sheet_title": "Данные", "name": "Реестр",
			"start_row": 0.0, "end_row": 20.0, "start_column": 0.0, "end_column": 5.0,
			"table_id": "t1",
		})))

		if body := string(h.bodyOf(t, 1)); !strings.Contains(body, "updateTable") {
			t.Errorf("an existing table should be changed, got %s", body)
		}
	})
}

// TestCellValuesKeepTheirKind: what a cell holds decides how it behaves, and a formula
// written as a string is a cell showing "=SUM(...)" rather than a total.
func TestCellValuesKeepTheirKind(t *testing.T) {
	for _, probe := range []struct {
		value any
		check func(*google.ExtendedValue) bool
		name  string
	}{
		{"текст", func(v *google.ExtendedValue) bool { return v.StringValue != nil }, "a string"},
		{"=SUM(A1:A9)", func(v *google.ExtendedValue) bool { return v.FormulaValue != nil }, "a formula"},
		{42.0, func(v *google.ExtendedValue) bool { return v.NumberValue != nil }, "a number"},
		{true, func(v *google.ExtendedValue) bool { return v.BoolValue != nil }, "a boolean"},
		{nil, func(v *google.ExtendedValue) bool {
			return v.StringValue == nil && v.NumberValue == nil && v.BoolValue == nil && v.FormulaValue == nil
		}, "nothing"},
	} {
		t.Run(probe.name, func(t *testing.T) {
			if !probe.check(cellValueOf(probe.value)) {
				t.Errorf("%v should land as %s", probe.value, probe.name)
			}
		})
	}
}

// TestDocsEditTableRefusals covers the shape-changing tool's own guard.
func TestDocsEditTableRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.docsEditTable(context.Background(), request(map[string]any{
		"document_id": "doc", "table_start_index": 129.0, "row": 0.0, "column": 0.0,
		"what": "delete_row",
	})))
	if !strings.Contains(message, "insert_row, insert_column or unmerge") {
		t.Errorf("the refusal should list what this tool does, got %q", message)
	}

	h.ok(h.registry.docsEditTable(context.Background(), request(map[string]any{
		"document_id": "doc", "table_start_index": 129.0, "row": 0.0, "column": 1.0,
		"what": "unmerge", "row_span": 1.0, "column_span": 2.0,
	})))
	if body := string(h.bodyOf(t, 0)); !strings.Contains(body, "unmergeTableCells") {
		t.Errorf("unmerging should send its own request, got %s", body)
	}
}

// TestSlidesTableBordersNeedSomethingToDraw, because a request with an empty mask changes
// nothing and reads as if it had worked.
func TestSlidesTableBordersNeedSomethingToDraw(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.slidesSetTableBorders(context.Background(), request(map[string]any{
		"presentation_id": "deck", "table_object_id": "t1",
	})))
	if !strings.Contains(message, "color, weight_emu or dash_style") {
		t.Errorf("the refusal should say what a border is made of, got %q", message)
	}

	// A rectangle narrows the borders to part of the table; without one they apply to all
	// of it, and that difference is worth pinning.
	h.ok(h.registry.slidesSetTableBorders(context.Background(), request(map[string]any{
		"presentation_id": "deck", "table_object_id": "t1", "color": "#000000",
		"row": 0.0, "column": 0.0, "row_span": 1.0, "column_span": 3.0,
	})))
	if body := string(h.bodyOf(t, 0)); !strings.Contains(body, "tableRange") {
		t.Errorf("a rectangle should reach the request, got %s", body)
	}
}

func TestSlidesRouteLineNeedsSomethingToDo(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.slidesRouteLine(context.Background(), request(map[string]any{
		"presentation_id": "deck", "object_id": "line1", "reroute": false,
	})))
	if !strings.Contains(message, "category") {
		t.Errorf("with nothing to do the tool should say so, got %q", message)
	}
}

func TestSheetsUpdateChartNeedsAChange(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

	message := h.fail(h.registry.sheetsUpdateChart(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "chart_id": 5.0,
	})))
	if !strings.Contains(message, "nothing to change") {
		t.Errorf("the refusal should say what can be changed, got %q", message)
	}
}
