package tools

import (
	"context"
	"strings"
	"testing"
)

// presentationWithFilledTable is a deck holding a table the way a real one comes back:
// column widths, row heights, merged header cells and styled text inside.
const presentationWithFilledTable = `{
  "presentationId": "deck",
  "title": "Отчёт",
  "pageSize": {"width": {"magnitude": 9144000, "unit": "EMU"}, "height": {"magnitude": 5143500, "unit": "EMU"}},
  "slides": [
    {
      "objectId": "slide1",
      "pageElements": [
        {
          "objectId": "table1",
          "size": {"width": {"magnitude": 3000000, "unit": "EMU"}, "height": {"magnitude": 3000000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 500000, "translateY": 1000000, "unit": "EMU"},
          "table": {
            "rows": 3,
            "columns": 2,
            "tableColumns": [
              {"columnWidth": {"magnitude": 2000000, "unit": "EMU"}},
              {"columnWidth": {"magnitude": 1500000, "unit": "EMU"}}
            ],
            "tableRows": [
              {
                "rowHeight": {"magnitude": 600000, "unit": "EMU"},
                "tableCells": [
                  {
                    "location": {"rowIndex": 0, "columnIndex": 0},
                    "rowSpan": 1, "columnSpan": 2,
                    "tableCellProperties": {
                      "contentAlignment": "MIDDLE",
                      "tableCellBackgroundFill": {"solidFill": {
                        "color": {"rgbColor": {"red": 1, "green": 1, "blue": 1}}, "alpha": 1}}
                    },
                    "text": {"textElements": [
                      {"paragraphMarker": {"style": {"alignment": "CENTER"}}},
                      {"textRun": {"content": "Показатель\n", "style": {
                        "fontFamily": "Rubik", "fontSize": {"magnitude": 14, "unit": "PT"}, "bold": true}}}
                    ]}
                  }
                ]
              },
              {
                "rowHeight": {"magnitude": 400000, "unit": "EMU"},
                "tableCells": [
                  {"location": {"rowIndex": 1, "columnIndex": 0}, "rowSpan": 2, "columnSpan": 1,
                   "text": {"textElements": [{"textRun": {"content": "Время сборки\n"}}]}},
                  {"location": {"rowIndex": 1, "columnIndex": 1}, "rowSpan": 1, "columnSpan": 1,
                   "text": {"textElements": [{"textRun": {"content": "12 мин\n"}}]}}
                ]
              },
              {
                "rowHeight": {"magnitude": 400000, "unit": "EMU"},
                "tableCells": [
                  {"location": {"rowIndex": 2, "columnIndex": 1}, "rowSpan": 1, "columnSpan": 1}
                ]
              }
            ]
          }
        }
      ]
    }
  ],
  "layouts": []
}`

// TestReadTable pins what a table hands back, because reproducing one in another deck is
// done from exactly these numbers.
func TestReadTable(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithFilledTable))

	answer := h.ok(h.registry.slidesReadTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
	})))

	for _, want := range []string{
		`"Показатель"`,
		`"Время сборки"`,
		// The widths are the table's real width: Slides reports 3000000×3000000 for the
		// table itself whatever it actually measures.
		`2000000`,
		`"column_span": 2`,
		// White is a decision on a slide, not the default it is in a spreadsheet.
		`#FFFFFF`,
		// The body cells set no colour of their own. Said out loud, because a blank there
		// reads as "this table has no text colour" and sends the next row's colour to a
		// guess — it comes from the deck's theme, and gdocs_slides_read_theme has it.
		`"text_color_inherited": true`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the table reading should carry %s, got %s", want, answer)
		}
	}
}

// TestReadTableTellsAThemeColourFromALiteralOne: the two are different answers, and writing
// a themed one back as a literal is how a table stops following the deck's palette — the
// recolouring that fixes every other slide leaves that table in the old season's colours.
func TestReadTableTellsAThemeColourFromALiteralOne(t *testing.T) {
	const deck = `{
	  "presentationId": "deck",
	  "slides": [{"objectId": "slide1", "pageElements": [{
	    "objectId": "table1",
	    "table": {"rows": 1, "columns": 2, "tableRows": [{"tableCells": [
	      {"location": {"rowIndex": 0, "columnIndex": 0}, "text": {"textElements": [
	        {"textRun": {"content": "Из палитры\n", "style": {"foregroundColor": {
	          "opaqueColor": {"themeColor": "ACCENT1"}}}}}]}},
	      {"location": {"rowIndex": 0, "columnIndex": 1}, "text": {"textElements": [
	        {"textRun": {"content": "Своим цветом\n", "style": {"foregroundColor": {
	          "opaqueColor": {"rgbColor": {"red": 0.2, "green": 0.2, "blue": 0.2}}}}}}]}}
	    ]}]}
	  }]}]
	}`

	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", deck))

	answer := h.ok(h.registry.slidesReadTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
	})))

	if !strings.Contains(answer, `"theme_color": "ACCENT1"`) {
		t.Errorf("a colour taken from the palette should be reported by its name, got %s", answer)
	}
	if !strings.Contains(answer, `"text_color": "#333333"`) {
		t.Errorf("a colour chosen outright should be reported by its value, got %s", answer)
	}
	if strings.Contains(answer, `"text_color_inherited"`) {
		t.Errorf("neither cell inherits, so nothing should say it does, got %s", answer)
	}
}

func TestReadTableRefusesSomethingThatIsNotOne(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	message := h.fail(h.registry.slidesReadTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "title1",
	})))

	if !strings.Contains(message, "not a table") {
		t.Errorf("the refusal should say what it is instead, got %q", message)
	}
}

func TestUpdateTableCells(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesUpdateTableCells(context.Background(), request(map[string]any{
		"presentation_id":   "deck",
		"object_id":         "table1",
		"column_widths_emu": []any{float64(2500000), float64(1000000)},
		"cells": []any{
			map[string]any{"row": 1, "column": 1, "text": "9 мин"},
		},
	})))

	checkGolden(t, "update_table_cells.json", h.bodyOf(t, 1))
}

// TestUpdateTableCellsSkipsDeletingNothing pins the rule an empty cell taught: deleting
// text from a cell that has none is an error, not a no-op.
//
// The cell it uses is also the one a merge shifts: row 2 holds a single cell, and that
// cell is column 1. Reading it by its position in the row would find nothing at column 1
// and a stale "12 мин" at column 0.
func TestUpdateTableCellsSkipsDeletingNothing(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesUpdateTableCells(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"cells": []any{
			map[string]any{"row": 2, "column": 1, "text": "Значение"},
		},
	})))

	body := string(h.bodyOf(t, 1))
	if strings.Contains(body, "deleteText") {
		t.Errorf("an empty cell should not be cleared before it is filled: %s", body)
	}
}

// TestUpdateTableCellsFindsCellsUnderAMerge is the defect a real deck taught: under a
// first column merged down five rows, the rows below hold two cells, and counting
// positions reads column 2 as column 1. The old text is then looked for where there is
// none, so nothing is cleared and the new text lands in front of it — "Новый
// Jenkins30% -> 30%".
func TestUpdateTableCellsFindsCellsUnderAMerge(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesUpdateTableCells(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"cells": []any{
			map[string]any{"row": 1, "column": 1, "text": "9 мин"},
		},
	})))

	body := string(h.bodyOf(t, 1))
	if !strings.Contains(body, "deleteText") {
		t.Errorf("the cell holds 12 мин and has to be cleared before the new text: %s", body)
	}
}

// TestUpdateTableCellsRefusesASwallowedCell keeps a merge from quietly collecting other
// cells' text. Slides accepts the write and puts it into the merged cell, so a merged
// heading ends up holding every row at once and the columns beside it look shifted.
func TestUpdateTableCellsRefusesASwallowedCell(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	for _, test := range []struct {
		title       string
		row, column int
	}{
		{"beside a column merged across", 0, 1},
		{"below a column merged down", 2, 0},
	} {
		t.Run(test.title, func(t *testing.T) {
			message := h.fail(h.registry.slidesUpdateTableCells(context.Background(),
				request(map[string]any{
					"presentation_id": "deck",
					"object_id":       "table1",
					"cells": []any{map[string]any{
						"row": float64(test.row), "column": float64(test.column), "text": "x",
					}},
				})))

			if !strings.Contains(message, "merge") {
				t.Errorf("the refusal should say the cell was merged away, got %q", message)
			}
		})
	}
}

func TestUpdateTableCellsRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	for _, test := range []struct {
		title string
		args  map[string]any
		want  string
	}{
		{
			"a cell outside the table",
			map[string]any{"presentation_id": "deck", "object_id": "table1",
				"cells": []any{map[string]any{"row": 9, "column": 0, "text": "x"}}},
			"row",
		},
		{
			"widths that do not match the columns",
			map[string]any{"presentation_id": "deck", "object_id": "table1",
				"column_widths_emu": []any{float64(1000000)}},
			"column",
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			if message := h.fail(h.registry.slidesUpdateTableCells(context.Background(),
				request(test.args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should mention %q, got %q", test.want, message)
			}
		})
	}
}

func TestStyleTable(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"merge": []any{
			map[string]any{"row": 0, "column": 0, "row_span": 1, "column_span": 2},
		},
		"fill": []any{
			map[string]any{"row": 0, "column": 0, "column_span": 2,
				"color": map[string]any{"red": 0.8, "green": 0.0, "blue": 0.0}},
		},
		"content_alignment":     "MIDDLE",
		"header_row_height_emu": float64(600000),
	})))

	checkGolden(t, "style_table.json", h.bodyOf(t, 1))
}

// TestStyleTableStylesCells pins the pair with gdocs_slides_read_table: a header that is
// centred and bold while the body is neither cannot be described by one style per column,
// and a table built that way reads as a different table.
func TestStyleTableStylesCells(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"cell_styles": []any{
			map[string]any{"row": float64(0), "column": float64(0), "bold": true,
				"font_size": float64(14), "alignment": "CENTER",
				"text_color": map[string]any{"red": 0.26, "green": 0.26, "blue": 0.26}},
			map[string]any{"row": float64(1), "column": float64(0), "bold": true},
			// The empty half of the merged header: styling text that is not there is an
			// error, so it is skipped rather than sent.
			map[string]any{"row": float64(0), "column": float64(1), "bold": true},
		},
	})))

	checkGolden(t, "style_table_cells.json", h.bodyOf(t, 1))
}

// TestStyleTableStylesABand is the entry that keeps a table even: the body of a table is
// named once instead of cell by cell, so the cell somebody forgot cannot stay a size larger
// than its neighbours. The empty half of a merge is still skipped, which is what makes a
// band safe to ask for at all.
func TestStyleTableStylesABand(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"cell_styles": []any{
			// Everything under the header row, every column: two cells hold text, the
			// third row's missing cell and the merged-away one do not.
			map[string]any{"rows": []any{float64(1)}, "font_size": float64(11.5)},
		},
	})))

	checkGolden(t, "style_table_band.json", h.bodyOf(t, 1))
}

// TestStyleTableStylesARectangle: the same spelling fill and gdocs_docs_style_table use, so
// one entry means the same thing wherever it is written.
func TestStyleTableStylesARectangle(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"cell_styles": []any{
			map[string]any{"row": float64(1), "column": float64(0),
				"row_span": float64(2), "column_span": float64(2), "bold": true},
		},
	})))

	checkGolden(t, "style_table_rectangle.json", h.bodyOf(t, 1))
}

func TestStyleTableCellRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	if message := h.fail(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"cell_styles":     []any{map[string]any{"row": float64(9), "column": float64(0), "bold": true}},
	}))); !strings.Contains(message, "(9,0)") {
		t.Errorf("a cell outside the table should be refused by its coordinates, got %q", message)
	}

	// A band is checked the same way, and says which boundary is the problem.
	if message := h.fail(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"cell_styles":     []any{map[string]any{"rows": []any{float64(1), float64(9)}, "bold": true}},
	}))); !strings.Contains(message, "rows reaches 9, and the table has 3") {
		t.Errorf("a band past the last row should say so, got %q", message)
	}

	if message := h.fail(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"cell_styles":     []any{map[string]any{"rows": []any{float64(2), float64(1)}, "bold": true}},
	}))); !strings.Contains(message, "backwards") {
		t.Errorf("a band that runs backwards should say so, got %q", message)
	}

	if message := h.fail(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"cell_styles": []any{map[string]any{"row": float64(0), "column": float64(0),
			"alignment": "MIDDLE"}},
	}))); !strings.Contains(message, "START, CENTER, END, JUSTIFIED") {
		t.Errorf("vertical alignment in a horizontal field should be refused, got %q", message)
	}
}

// TestStyleTablePaintsFromThePalette covers the table's half of the same idea as the
// shapes': a themed deck's table has to follow the palette, or recolouring the deck leaves
// its tables behind in the old season's colours.
func TestStyleTablePaintsFromThePalette(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"fill": []any{
			map[string]any{"row": 0, "column": 0, "theme_color": "accent2"},
		},
		"cell_styles": []any{
			map[string]any{"row": float64(0), "column": float64(0), "theme_color": "LIGHT1"},
		},
	})))

	body := string(h.bodyOf(t, 1))
	for _, want := range []string{`"themeColor": "ACCENT2"`, `"themeColor": "LIGHT1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}

	if message := h.fail(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"fill":            []any{map[string]any{"row": 0, "column": 0, "theme_color": "PUMPKIN"}},
	}))); !strings.Contains(message, "PUMPKIN") {
		t.Errorf("a colour outside the palette should be refused by name, got %q", message)
	}
}

func TestStyleTableRefusesAFillWithoutAColour(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	message := h.fail(h.registry.slidesStyleTable(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
		"fill":            []any{map[string]any{"row": 0, "column": 0}},
	})))

	if !strings.Contains(message, "color") {
		t.Errorf("the refusal should name the missing colour, got %q", message)
	}
}

func TestInsertImage(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesInsertImage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"url":             "https://example.invalid/chart.png",
		"x":               float64(4000000),
		"y":               float64(1000000),
		"width":           float64(2000000),
		"height":          float64(1500000),
	})))

	checkGolden(t, "insert_image.json", h.bodyOf(t, 0))
}

func TestInsertImageRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	if message := h.fail(h.registry.slidesInsertImage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"url":             "gs://bucket/chart.png",
	}))); !strings.Contains(message, "http or https") {
		t.Errorf("an address Google cannot fetch should be refused, got %q", message)
	}

	if message := h.fail(h.registry.slidesInsertImage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"url":             "https://example.invalid/chart.png",
		"width":           float64(2000000),
	}))); !strings.Contains(message, "width and height go together") {
		t.Errorf("half a size should be refused, got %q", message)
	}
}

// TestDelete is the test the no-deletion rule leans on: the tool reaches slides and their
// elements and nothing else, and it refuses an identifier that is not in the deck rather
// than passing a guess on to Google.
func TestDelete(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesDelete(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{"table1"},
	})))

	if message := h.fail(h.registry.slidesDelete(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{"whatever"},
	}))); !strings.Contains(message, "whatever") {
		t.Errorf("an identifier that is not in the deck should be refused by name, got %q", message)
	}

	// Request 0 reads the deck to check the identifier, request 1 is the deletion itself.
	checkGolden(t, "delete_objects.json", h.bodyOf(t, 1))

	if message := h.fail(h.registry.slidesDelete(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{},
	}))); !strings.Contains(message, "object_ids") {
		t.Errorf("a call naming nothing should be refused, got %q", message)
	}
}

func TestReorder(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	h.ok(h.registry.slidesReorder(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"order":           []any{"slide1"},
	})))

	checkGolden(t, "reorder.json", h.bodyOf(t, 1))
}

// TestTextToolsReachIntoACell is the rule this whole set of arguments exists for: a summary
// table is where the task numbers people want to click on live, and before this the text
// tools stopped at the edge of a table. The only way to change one number was to rebuild the
// table, which loses the template's own styling.
func TestTextToolsReachIntoACell(t *testing.T) {
	t.Run("a link on a number in a cell", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).
			answer("/presentations/deck:batchUpdate", emptyBatchReply).
			answer("/presentations/deck", presentationWithFilledTable))

		h.ok(h.registry.slidesLinkText(context.Background(), request(map[string]any{
			"presentation_id": "deck",
			"object_id":       "table1",
			"row":             float64(1),
			"column":          float64(1),
			"links": []any{
				map[string]any{"text": "12 мин", "url": "https://example.invalid/build"},
			},
		})))

		checkGolden(t, "link_text_in_cell.json", h.bodyOf(t, 1))
	})

	t.Run("text of a cell", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).
			answer("/presentations/deck:batchUpdate", emptyBatchReply).
			answer("/presentations/deck", presentationWithFilledTable))

		h.ok(h.registry.slidesSetText(context.Background(), request(map[string]any{
			"presentation_id": "deck",
			"object_id":       "table1",
			"row":             float64(1),
			"column":          float64(1),
			"text":            "9 мин",
		})))

		checkGolden(t, "set_text_in_cell.json", h.bodyOf(t, 1))
	})

	t.Run("style of a cell's text", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).
			answer("/presentations/deck:batchUpdate", emptyBatchReply).
			answer("/presentations/deck", presentationWithFilledTable))

		h.ok(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
			"presentation_id": "deck",
			"object_id":       "table1",
			"row":             float64(1),
			"column":          float64(1),
			"bold":            true,
		})))

		// The whole of a cell needs no reading first, so the batch is the only request.
		checkGolden(t, "set_text_style_in_cell.json", h.bodyOf(t, 0))
	})

	t.Run("reading a cell's paragraphs", func(t *testing.T) {
		h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithFilledTable))

		answer := h.ok(h.registry.slidesInspectTextStructure(context.Background(), request(map[string]any{
			"presentation_id": "deck",
			"object_id":       "table1",
			"row":             float64(0),
			"column":          float64(0),
		})))

		if !strings.Contains(answer, "Показатель") {
			t.Errorf("reading a cell should report its text, got %s", answer)
		}
	})
}

// TestCellAddressingRefusals: every way of naming a cell wrongly has to say what was wrong,
// because the alternative is a write that lands somewhere else in the table.
func TestCellAddressingRefusals(t *testing.T) {
	for _, probe := range []struct {
		name      string
		arguments map[string]any
		want      string
	}{
		{
			name:      "half a coordinate",
			arguments: map[string]any{"object_id": "table1", "row": float64(1), "text": "x"},
			want:      "row and column go together",
		},
		{
			name: "outside the table",
			arguments: map[string]any{"object_id": "table1", "row": float64(9),
				"column": float64(0), "text": "x"},
			want: "outside a table of 3×2",
		},
		{
			// Row 2 column 0 was swallowed by the merge that starts at row 1: writing there
			// would go into that cell, on top of what it already holds.
			name: "inside a merge",
			arguments: map[string]any{"object_id": "table1", "row": float64(2),
				"column": float64(0), "text": "x"},
			want: "inside the merge that starts at row 1 column 0",
		},
		{
			name: "not a table at all",
			arguments: map[string]any{"object_id": "title1", "row": float64(0),
				"column": float64(0), "text": "x"},
			want: "row and column only mean something on a table",
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			deck := presentationWithFilledTable
			if probe.arguments["object_id"] == "title1" {
				deck = presentationWithBody
			}
			h := newHarness(t, newFakeGoogle(t).
				answer("/presentations/deck:batchUpdate", emptyBatchReply).
				answer("/presentations/deck", deck))

			arguments := map[string]any{"presentation_id": "deck"}
			for key, value := range probe.arguments {
				arguments[key] = value
			}

			message := h.fail(h.registry.slidesSetText(context.Background(), request(arguments)))
			if !strings.Contains(message, probe.want) {
				t.Errorf("the refusal should say %q, got %q", probe.want, message)
			}
		})
	}
}

// TestReorderInsistsOnEverySlide keeps a caller from reordering a deck by naming half of
// it: the slides not named would end up wherever the shuffle left them.
func TestReorderInsistsOnEverySlide(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithFilledTable))

	message := h.fail(h.registry.slidesReorder(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"order":           []any{"slide1", "slide404"},
	})))

	if !strings.Contains(message, "slide404") {
		t.Errorf("the refusal should name the slide that is not there, got %q", message)
	}
}
