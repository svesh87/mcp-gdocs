package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// spreadsheetForObjects is a workbook with the pieces the newer tools address: two tabs,
// a chart, and a label attached to the workbook itself.
const spreadsheetForObjects = `{
  "spreadsheetId": "book",
  "properties": {"title": "Отчёт"},
  "developerMetadata": [
    {"metadataId": 7, "metadataKey": "total_row", "metadataValue": "12", "visibility": "DOCUMENT"}
  ],
  "sheets": [
    {"properties": {"sheetId": 0, "title": "Данные", "index": 0,
      "gridProperties": {"rowCount": 100, "columnCount": 10}},
     "developerMetadata": [{"metadataId": 8, "metadataKey": "source", "metadataValue": "crm"}],
     "charts": [
       {"chartId": 5,
        "spec": {
          "title": "Было",
          "backgroundColor": {"red": 1, "green": 1, "blue": 1},
          "hiddenDimensionStrategy": "SKIP_HIDDEN_ROWS",
          "basicChart": {
            "chartType": "COLUMN", "stackedType": "STACKED", "headerCount": 1,
            "domains": [{"domain": {"sourceRange": {"sources": [{"sheetId": 0,
              "startRowIndex": 0, "endRowIndex": 6,
              "startColumnIndex": 0, "endColumnIndex": 1}]}}}],
            "series": [
              {"series": {"sourceRange": {"sources": [{"sheetId": 0,
                "startRowIndex": 0, "endRowIndex": 6,
                "startColumnIndex": 1, "endColumnIndex": 2}]}},
               "targetAxis": "LEFT_AXIS", "color": {"red": 0.2, "green": 0.4, "blue": 0.9}}
            ]
          }
        }}
     ]},
    {"properties": {"sheetId": 7, "title": "Свод", "index": 1}}
  ]
}`

// TestSheetsMovingKeepsWhatARewriteWouldLose pins the requests behind copying and moving:
// the paste type is what decides whether the formatting travels, and a move is a different
// request from a copy rather than a flag on the same one.
func TestSheetsMovingKeepsWhatARewriteWouldLose(t *testing.T) {
	for _, probe := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{
			name: "copy with formatting",
			args: map[string]any{"what": "FORMAT", "to_row": 10.0, "to_column": 0.0},
			want: []string{"copyPaste", `"pasteType": "PASTE_FORMAT"`, `"startRowIndex": 10`},
		},
		{
			name: "copy repeating into a bigger rectangle",
			args: map[string]any{"to_row": 10.0, "to_column": 0.0, "to_end_row": 40.0, "to_end_column": 4.0},
			want: []string{"copyPaste", `"endRowIndex": 40`},
		},
		{
			name: "move leaves nothing behind",
			args: map[string]any{"move": true, "to_row": 10.0, "to_column": 2.0},
			want: []string{"cutPaste", `"rowIndex": 10`, `"columnIndex": 2`},
		},
		{
			name: "transposed copy",
			args: map[string]any{"orientation": "TRANSPOSE", "to_row": 0.0, "to_column": 5.0},
			want: []string{`"pasteOrientation": "TRANSPOSE"`},
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

			args := map[string]any{
				"spreadsheet_id": "book", "sheet_title": "Данные",
				"start_row": 0.0, "end_row": 5.0, "start_column": 0.0, "end_column": 3.0,
			}
			for key, value := range probe.args {
				args[key] = value
			}

			h.ok(h.registry.sheetsMoveRange(context.Background(), request(args)))

			body := string(h.bodyOf(t, 1))
			for _, want := range probe.want {
				if !strings.Contains(body, want) {
					t.Errorf("the request should carry %s, got %s", want, body)
				}
			}
		})
	}
}

func TestSheetsPasteTextSplitsOnGooglesSide(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

	h.ok(h.registry.sheetsPasteText(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "sheet_title": "Данные",
		"data": "имя;значение\nстрока;42", "delimiter": ";", "row": 3.0, "column": 1.0,
	})))

	body := string(h.bodyOf(t, 1))
	for _, want := range []string{"pasteData", `"delimiter": ";"`, `"rowIndex": 3`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}
}

func TestSheetsShapeAndAppend(t *testing.T) {
	for _, probe := range []struct {
		name string
		call func(*harness) (*mcp.CallToolResult, error)
		want string
	}{
		{
			name: "insert cells",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsShapeRange(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "sheet_title": "Данные",
					"start_row": 0.0, "end_row": 3.0, "start_column": 0.0, "end_column": 1.0,
					"what": "insert_cells", "shift": "COLUMNS",
				}))
			},
			want: `"shiftDimension": "COLUMNS"`,
		},
		{
			name: "randomize",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsShapeRange(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "sheet_title": "Данные",
					"start_row": 1.0, "end_row": 30.0, "start_column": 0.0, "end_column": 4.0,
					"what": "randomize",
				}))
			},
			want: "randomizeRange",
		},
		{
			name: "append rows with a formula",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsAppendRows(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "sheet_title": "Данные",
					"rows": []any{[]any{"итого", "=SUM(B2:B10)", 42.0, true}},
				}))
			},
			want: `"formulaValue": "=SUM(B2:B10)"`,
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))
			h.ok(probe.call(h))

			if body := string(h.bodyOf(t, 1)); !strings.Contains(body, probe.want) {
				t.Errorf("the request should carry %s, got %s", probe.want, body)
			}
		})
	}
}

// TestSheetsDeleteNamesOneThing covers the removal tool's refusals, which are the point of
// it: a call that does not say exactly what goes is a call that must not go through.
func TestSheetsDeleteNamesOneThing(t *testing.T) {
	for _, probe := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"nothing named", map[string]any{"what": "banding"}, "object_id"},
		{"a range with no bounds", map[string]any{"what": "rows", "sheet_title": "Данные"}, "start and end"},
		{"a tab with no name", map[string]any{"what": "tab"}, "name the tab"},
		{"an unknown target", map[string]any{"what": "everything"}, "what is rows"},
		{"cells with no rectangle", map[string]any{"what": "cells", "sheet_title": "Данные"}, "name the rectangle"},
		{"a label with neither id nor key", map[string]any{"what": "metadata"}, "metadata_id or its key"},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

			args := map[string]any{"spreadsheet_id": "book"}
			for key, value := range probe.args {
				args[key] = value
			}

			if message := h.fail(h.registry.sheetsDelete(context.Background(), request(args))); !strings.Contains(message, probe.want) {
				t.Errorf("the refusal should carry %q, got %q", probe.want, message)
			}
		})
	}
}

func TestSheetsDeleteSendsTheRightRequest(t *testing.T) {
	for _, probe := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"rows", map[string]any{"what": "rows", "sheet_title": "Данные", "start": 2.0, "end": 5.0}, "deleteDimension"},
		{"a tab", map[string]any{"what": "tab", "sheet_title": "Свод"}, `"sheetId": 7`},
		{"cells", map[string]any{"what": "cells", "sheet_title": "Данные",
			"start_row": 0.0, "end_row": 2.0, "start_column": 0.0, "end_column": 2.0}, "deleteRange"},
		{"a banding", map[string]any{"what": "banding", "object_id": 3.0}, "deleteBanding"},
		{"a protection", map[string]any{"what": "protection", "object_id": 9.0}, "deleteProtectedRange"},
		{"a named range", map[string]any{"what": "named_range", "named_range_id": "nr1"}, "deleteNamedRange"},
		{"a chart", map[string]any{"what": "chart", "object_id": 11.0}, "deleteEmbeddedObject"},
		{"a table", map[string]any{"what": "table", "table_id": "t1"}, "deleteTable"},
		{"a label", map[string]any{"what": "metadata", "key": "source"}, "deleteDeveloperMetadata"},
		{"duplicates", map[string]any{"what": "duplicates", "sheet_title": "Данные",
			"start_row": 0.0, "end_row": 50.0, "start_column": 0.0, "end_column": 3.0}, "deleteDuplicates"},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

			args := map[string]any{"spreadsheet_id": "book"}
			for key, value := range probe.args {
				args[key] = value
			}

			h.ok(h.registry.sheetsDelete(context.Background(), request(args)))

			body := string(h.bodyOf(t, len(h.google.requests)-1))
			if !strings.Contains(body, probe.want) {
				t.Errorf("the request should carry %s, got %s", probe.want, body)
			}
		})
	}
}

func TestSheetsObjectsAndLabels(t *testing.T) {
	for _, probe := range []struct {
		name string
		call func(*harness) (*mcp.CallToolResult, error)
		want []string
	}{
		{
			name: "moving a chart",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsUpdateChart(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "chart_id": 5.0, "sheet_title": "Свод",
					"anchor_row": 2.0, "anchor_column": 1.0, "width": 600.0, "height": 300.0,
				}))
			},
			want: []string{"updateEmbeddedObjectPosition", `"widthPixels": 600`},
		},
		{
			name: "retitling a chart",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsUpdateChart(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "chart_id": 5.0, "title": "Выручка по месяцам",
				}))
			},
			want: []string{"updateChartSpec", "Выручка по месяцам"},
		},
		{
			name: "a filter view",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsFilterView(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "sheet_title": "Данные", "title": "Только просроченные",
					"start_row": 0.0, "end_row": 100.0, "start_column": 0.0, "end_column": 6.0,
					"sort_column": 2.0, "sort_order": "DESCENDING",
				}))
			},
			want: []string{"addFilterView", "Только просроченные", `"sortOrder": "DESCENDING"`},
		},
		{
			name: "a slicer",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsSlicer(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "sheet_title": "Данные", "title": "Статус",
					"start_row": 0.0, "end_row": 100.0, "start_column": 0.0, "end_column": 6.0,
					"column_index": 3.0, "anchor_row": 1.0, "anchor_column": 8.0,
				}))
			},
			want: []string{"addSlicer", `"columnIndex": 3`},
		},
		{
			name: "a label on a row",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsSetMetadata(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "key": "total_row", "value": "12",
					"sheet_title": "Данные", "dimension": "ROWS", "start": 11.0, "end": 12.0,
				}))
			},
			want: []string{"createDeveloperMetadata", `"dimension": "ROWS"`},
		},
		{
			name: "changing a label",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsSetMetadata(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "key": "total_row", "value": "18", "metadata_id": 7.0,
				}))
			},
			want: []string{"updateDeveloperMetadata", `"metadataId": 7`},
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))
			h.ok(probe.call(h))

			body := string(h.bodyOf(t, len(h.google.requests)-1))
			for _, want := range probe.want {
				if !strings.Contains(body, want) {
					t.Errorf("the request should carry %s, got %s", want, body)
				}
			}
		})
	}
}

// TestUpdateChartKeepsWhatItWasNotAskedAbout is the reason the specification is read before
// it is written. updateChartSpec has no field mask: whatever is sent becomes the chart. A
// title change built from the caller's arguments alone would replace a chart that draws six
// months of data with a chart that draws nothing and has a nice title.
func TestUpdateChartKeepsWhatItWasNotAskedAbout(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetForObjects))

	h.ok(h.registry.sheetsUpdateChart(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "chart_id": 5.0, "title": "Стало",
	})))

	body := string(h.bodyOf(t, len(h.google.requests)-1))
	for _, want := range []string{
		`"title": "Стало"`,
		// The data survives, and so does everything this server does not model at all.
		`"chartType": "COLUMN"`,
		`"stackedType": "STACKED"`,
		`"hiddenDimensionStrategy": "SKIP_HIDDEN_ROWS"`,
		`"backgroundColor"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}
}

// TestUpdateChartPrintsTheNumbers: a chart read off a screen during a meeting is read by
// its printed values, not by eye against an axis. On a stacked column the per-segment
// numbers stop fitting, and the total over the column is a field of its own.
func TestUpdateChartPrintsTheNumbers(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetForObjects))

	h.ok(h.registry.sheetsUpdateChart(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "chart_id": 5.0,
		"data_labels": true, "total_data_labels": true,
	})))

	body := string(h.bodyOf(t, len(h.google.requests)-1))
	for _, want := range []string{`"totalDataLabel"`, `"dataLabel"`, `"type": "DATA"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}
}

// TestUpdateChartPaintsItsBackground: a chart standing inside a panel on a slide arrives with
// a white rectangle of its own and paints over the panel. On a dark seasonal variant of a
// deck that is not "slightly off" — it is an unreadable slide, and the chart inherits nothing
// from the deck's palette because it lives in the workbook.
func TestUpdateChartPaintsItsBackground(t *testing.T) {
	for _, probe := range []struct {
		name  string
		args  map[string]any
		want  []string
		avoid string
	}{
		{
			name: "matched to the panel",
			args: map[string]any{"background_color": "#EEF2F7"},
			want: []string{`"backgroundColor"`, `"backgroundColorStyle"`, "0.93"},
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t).
				answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
				answer("/spreadsheets/book", spreadsheetForObjects))

			args := map[string]any{"spreadsheet_id": "book", "chart_id": 5.0}
			for key, value := range probe.args {
				args[key] = value
			}
			h.ok(h.registry.sheetsUpdateChart(context.Background(), request(args)))

			body := string(h.bodyOf(t, len(h.google.requests)-1))
			for _, want := range probe.want {
				if !strings.Contains(body, want) {
					t.Errorf("the request should carry %s, got %s", want, body)
				}
			}
			if probe.avoid != "" && strings.Contains(body, probe.avoid) {
				t.Errorf("the request should not carry %q, got %s", probe.avoid, body)
			}
		})
	}
}

// TestUpdateChartRefusesTransparency records what a live chart answered:
// "chart.backgroundColorStyle.alpha not supported". The parameter exists so that the refusal
// says so and names the way round it, rather than the caller learning it from Google's own
// message, which says nothing about what to do instead.
func TestUpdateChartRefusesTransparency(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetForObjects))

	message := h.fail(h.registry.sheetsUpdateChart(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "chart_id": 5.0, "transparent_background": true,
	})))

	for _, want := range []string{"alpha", "background_color", "inspect_page"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal should mention %s, got %s", want, message)
		}
	}
}

// TestUpdateChartRepointsWithoutRebuilding: changing the range keeps the chart's number, and
// the number is what a slide holds on to. Deleting the chart and drawing it again leaves
// every slide showing it pointing at nothing.
func TestUpdateChartRepointsWithoutRebuilding(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
		answer("/spreadsheets/book", spreadsheetForObjects))

	h.ok(h.registry.sheetsUpdateChart(context.Background(), request(map[string]any{
		"spreadsheet_id": "book", "chart_id": 5.0,
		"data_sheet_title": "Данные", "labels_column": 0.0,
		"value_columns": []any{1.0, 2.0, 3.0},
		"start_row":     0.0, "end_row": 92.0,
	})))

	body := string(h.bodyOf(t, len(h.google.requests)-1))
	for _, want := range []string{
		`"endRowIndex": 92`,
		`"startColumnIndex": 3`,
		// The first series keeps the colour it had: a range that grew is not a reason for
		// the chart to change appearance.
		`"color"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}
}

func TestSheetsListMetadataReadsBothLevels(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/spreadsheets/book", spreadsheetForObjects))

	answer := h.ok(h.registry.sheetsListMetadata(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
	})))

	for _, want := range []string{"total_row", "source", `"where": "workbook"`, `"where": "Данные"`} {
		if !strings.Contains(answer, want) {
			t.Errorf("the reading should carry %s, got %s", want, answer)
		}
	}
}

// TestSlidesExtraSendsWhatTheApiTakes covers the requests added for decks.
func TestSlidesExtraSendsWhatTheApiTakes(t *testing.T) {
	for _, probe := range []struct {
		name string
		call func(*harness) (*mcp.CallToolResult, error)
		want []string
	}{
		{
			name: "growing a table",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesEditTable(context.Background(), request(map[string]any{
					"presentation_id": "deck", "table_object_id": "t1", "what": "insert_rows",
					"row": 2.0, "column": 0.0, "count": 3.0,
				}))
			},
			want: []string{"insertTableRows", `"number": 3`, `"insertBelow": true`},
		},
		{
			name: "unmerging",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesEditTable(context.Background(), request(map[string]any{
					"presentation_id": "deck", "table_object_id": "t1", "what": "unmerge",
					"row": 0.0, "column": 0.0, "row_span": 2.0, "column_span": 2.0,
				}))
			},
			want: []string{"unmergeTableCells", `"rowSpan": 2`},
		},
		{
			name: "table borders",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesSetTableBorders(context.Background(), request(map[string]any{
					"presentation_id": "deck", "table_object_id": "t1", "position": "OUTER",
					"color": "#B7B7B7", "weight_emu": 9525.0, "dash_style": "SOLID",
				}))
			},
			want: []string{"updateTableBorderProperties", `"borderPosition": "OUTER"`, "tableBorderFill"},
		},
		{
			name: "a theme colour for the borders",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesSetTableBorders(context.Background(), request(map[string]any{
					"presentation_id": "deck", "table_object_id": "t1", "color": "accent1",
				}))
			},
			want: []string{`"themeColor": "ACCENT1"`},
		},
		{
			name: "replacing a picture",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesReplaceImage(context.Background(), request(map[string]any{
					"presentation_id": "deck", "image_object_id": "img1",
					"url": "https://example.org/new.png",
				}))
			},
			want: []string{"replaceImage", "CENTER_CROP"},
		},
		{
			// One emoji swapped across a whole deck of panels: the words around it keep
			// their styling, which is the only reason this is not gdocs_slides_set_text.
			name: "text replaced across the deck",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesReplaceText(context.Background(), request(map[string]any{
					"presentation_id": "deck", "find": "✅", "replace": "🍬",
				}))
			},
			want: []string{"replaceAllText", `"text": "✅"`, `"replaceText": "🍬"`},
		},
		{
			name: "shapes into pictures",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesReplaceShapesWithImage(context.Background(), request(map[string]any{
					"presentation_id": "deck", "contains_text": "{{photo}}",
					"url": "https://example.org/p.png",
				}))
			},
			want: []string{"replaceAllShapesWithImage", "{{photo}}"},
		},
		{
			name: "shapes into charts",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesReplaceShapesWithChart(context.Background(), request(map[string]any{
					"presentation_id": "deck", "contains_text": "{{chart}}",
					"spreadsheet_id": "book", "chart_id": 4.0,
				}))
			},
			want: []string{"replaceAllShapesWithSheetsChart", `"linkingMode": "LINKED"`},
		},
		{
			name: "alt text",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesSetAltText(context.Background(), request(map[string]any{
					"presentation_id": "deck", "object_id": "img1",
					"description": "График выручки по кварталам",
				}))
			},
			want: []string{"updatePageElementAltText", "График выручки"},
		},
		{
			name: "routing a connector",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesRouteLine(context.Background(), request(map[string]any{
					"presentation_id": "deck", "object_id": "line1", "category": "bent",
				}))
			},
			want: []string{"updateLineCategory", `"lineCategory": "BENT"`, "rerouteLine"},
		},
		{
			name: "a chart from a workbook",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesAddSheetsChart(context.Background(), request(map[string]any{
					"presentation_id": "deck", "page_object_id": "slide1",
					"spreadsheet_id": "book", "chart_id": 4.0,
					"x_emu": 100000.0, "y_emu": 200000.0, "width_emu": 3000000.0, "height_emu": 2000000.0,
				}))
			},
			want: []string{"createSheetsChart", `"chartId": 4`, `"translateX": 100000`},
		},
		{
			name: "refreshing it",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesRefreshSheetsChart(context.Background(), request(map[string]any{
					"presentation_id": "deck", "object_id": "chart1",
				}))
			},
			want: []string{"refreshSheetsChart"},
		},
		{
			name: "a video that starts muted",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesAddVideo(context.Background(), request(map[string]any{
					"presentation_id": "deck", "page_object_id": "slide1", "video_id": "abc123",
					"mute": true, "start_seconds": 30.0,
				}))
			},
			want: []string{"createVideo", "updateVideoProperties", `"mute": true`, `"start": 30`},
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t))
			h.ok(probe.call(h))

			body := string(h.bodyOf(t, 0))
			for _, want := range probe.want {
				if !strings.Contains(body, want) {
					t.Errorf("the request should carry %s, got %s", want, body)
				}
			}
		})
	}
}

// TestDocsExtraSendsWhatTheApiTakes covers the requests added for documents.
func TestDocsExtraSendsWhatTheApiTakes(t *testing.T) {
	for _, probe := range []struct {
		name string
		call func(*harness) (*mcp.CallToolResult, error)
		want []string
	}{
		{
			name: "naming a range",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.docsAddNamedRange(context.Background(), request(map[string]any{
					"document_id": "doc", "name": "salary", "start_index": 300.0, "end_index": 330.0,
				}))
			},
			want: []string{"createNamedRange", `"name": "salary"`},
		},
		{
			name: "filling it",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.docsFillNamedRange(context.Background(), request(map[string]any{
					"document_id": "doc", "name": "salary", "text": "420 000 руб.",
				}))
			},
			want: []string{"replaceNamedRangeContent", "420 000"},
		},
		{
			name: "a tab",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.docsAddTab(context.Background(), request(map[string]any{
					"document_id": "doc", "title": "Приложение", "icon_emoji": "📎",
				}))
			},
			want: []string{"addDocumentTab", "Приложение"},
		},
		{
			name: "a person chip",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.docsInsertChip(context.Background(), request(map[string]any{
					"document_id": "doc", "kind": "person", "email": "someone@example.org",
					"index": 42.0,
				}))
			},
			want: []string{"insertPerson", "someone@example.org"},
		},
		{
			name: "a date chip",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.docsInsertChip(context.Background(), request(map[string]any{
					"document_id": "doc", "kind": "date", "timestamp": "2026-08-13T00:00:00Z",
					"date_format": "dd.MM.yyyy",
				}))
			},
			want: []string{"insertDate", "2026-08-13"},
		},
		{
			name: "replacing a picture",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.docsReplaceImage(context.Background(), request(map[string]any{
					"document_id": "doc", "image_object_id": "kix.pic",
					"uri": "https://example.org/new.png",
				}))
			},
			want: []string{"replaceImage", "CENTER_CROP"},
		},
		{
			name: "growing a table",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.docsEditTable(context.Background(), request(map[string]any{
					"document_id": "doc", "table_start_index": 129.0, "row": 1.0, "column": 0.0,
					"what": "insert_row",
				}))
			},
			want: []string{"insertTableRow", `"insertBelow": true`},
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t))
			h.ok(probe.call(h))

			body := string(h.bodyOf(t, 0))
			for _, want := range probe.want {
				if !strings.Contains(body, want) {
					t.Errorf("the request should carry %s, got %s", want, body)
				}
			}
		})
	}
}

// TestDriveManagementTools covers the file-level half of Drive.
func TestDriveManagementTools(t *testing.T) {
	const fileAnswer = `{"id": "f1", "name": "Отчёт", "parents": ["folder1"], "mimeType": "application/pdf"}`

	for _, probe := range []struct {
		name    string
		call    func(*harness) (*mcp.CallToolResult, error)
		answers string
		want    []string
	}{
		{
			name: "a folder",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.driveCreateFolder(context.Background(), request(map[string]any{
					"name": "Офферы", "parent_folder_id": "folder1",
				}))
			},
			answers: `{"id": "new", "name": "Офферы"}`,
			want:    []string{`"folder_id": "new"`},
		},
		{
			name: "renaming",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.driveRename(context.Background(), request(map[string]any{
					"file_id": "f1", "name": "Отчёт за август",
				}))
			},
			answers: fileAnswer,
			want:    []string{`"file_id": "f1"`},
		},
		{
			name: "moving",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.driveMove(context.Background(), request(map[string]any{
					"file_id": "f1", "to_folder_id": "folder2", "from_folder_id": "folder1",
				}))
			},
			answers: fileAnswer,
			want:    []string{`"folders"`},
		},
		{
			name: "into the bin",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.driveTrash(context.Background(), request(map[string]any{
					"file_id": "f1",
				}))
			},
			answers: fileAnswer,
			want:    []string{`"state": "in the bin"`},
		},
		{
			name: "back out of the bin",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.driveTrash(context.Background(), request(map[string]any{
					"file_id": "f1", "restore": true,
				}))
			},
			answers: fileAnswer,
			want:    []string{`"state": "restored"`},
		},
		{
			name: "a comment",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.driveAddComment(context.Background(), request(map[string]any{
					"file_id": "f1", "content": "Поправить сумму",
				}))
			},
			answers: `{"id": "c1", "content": "Поправить сумму"}`,
			want:    []string{`"comment_id": "c1"`},
		},
		{
			name: "resolving one",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.driveReplyComment(context.Background(), request(map[string]any{
					"file_id": "f1", "comment_id": "c1", "content": "Поправил", "action": "resolve",
				}))
			},
			answers: `{"id": "r1", "action": "resolve"}`,
			want:    []string{`"reply_id": "r1"`},
		},
		{
			name: "keeping a revision",
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.driveKeepRevision(context.Background(), request(map[string]any{
					"file_id": "f1", "revision_id": "42",
				}))
			},
			answers: `{"id": "42", "keepForever": true}`,
			want:    []string{`"kept_forever": true`},
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t).answer("/", probe.answers))
			answer := h.ok(probe.call(h))

			for _, want := range probe.want {
				if !strings.Contains(answer, want) {
					t.Errorf("the answer should carry %s, got %s", want, answer)
				}
			}
		})
	}
}

// TestDriveSharingSaysWhatItOpened, because a grant to anyone with the link is the one
// action here whose consequence is outside the account.
func TestDriveSharingSaysWhatItOpened(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/permissions", `{"id": "anyoneWithLink", "type": "anyone", "role": "reader"}`))

	answer := h.ok(h.registry.driveShare(context.Background(), request(map[string]any{
		"file_id": "f1", "type": "anyone", "role": "reader",
	})))

	if !strings.Contains(answer, "readable by everybody who has the link") {
		t.Errorf("opening a file to anyone should say so, got %s", answer)
	}

	message := h.fail(h.registry.driveShare(context.Background(), request(map[string]any{
		"file_id": "f1", "type": "user",
	})))
	if !strings.Contains(message, "email address") {
		t.Errorf("sharing with a person needs an address, got %q", message)
	}

	message = h.fail(h.registry.driveShare(context.Background(), request(map[string]any{
		"file_id": "f1", "type": "everyone",
	})))
	if !strings.Contains(message, "user, group, domain or anyone") {
		t.Errorf("an unknown kind should be refused, got %q", message)
	}
}

func TestDriveReadingComments(t *testing.T) {
	const comments = `{"comments": [
	  {"id": "c1", "content": "Сумма не та", "resolved": false,
	   "author": {"displayName": "Анна Соколова"},
	   "quotedFileContent": {"value": "416 000"},
	   "replies": [{"id": "r1", "content": "Поправил", "action": "resolve",
	                "author": {"displayName": "Пётр Иванов"}}]},
	  {"id": "c2", "content": "Готово", "resolved": true}
	]}`

	h := newHarness(t, newFakeGoogle(t).answer("/comments", comments))

	answer := h.ok(h.registry.driveListComments(context.Background(), request(map[string]any{
		"file_id": "f1",
	})))

	for _, want := range []string{"Сумма не та", `"open": 1`, "Поправил", `"about": "416 000"`} {
		if !strings.Contains(answer, want) {
			t.Errorf("the reading should carry %s, got %s", want, answer)
		}
	}
}
