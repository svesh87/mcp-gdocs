package tools

import (
	"context"
	"strings"
	"testing"
)

// TestDocsStyleParagraphSendsWhatWasNamed pins the request body, because what a page ends
// up looking like is decided by these fields and by the mask beside them: a field left out
// of the mask keeps whatever the paragraph had.
func TestDocsStyleParagraphSendsWhatWasNamed(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	h.ok(h.registry.docsStyleParagraph(context.Background(), request(map[string]any{
		"document_id": "doc",
		"start_index": 14,
		"end_index":   30,
		"style": map[string]any{
			"named_style":       "NORMAL_TEXT",
			"alignment":         "CENTER",
			"space_above_pt":    18.0,
			"indent_start_pt":   36.0,
			"line_spacing":      115.0,
			"keep_with_next":    true,
			"page_break_before": false,
			"shading_color":     "#F3F3F3",
			"border_top": map[string]any{
				"color": "#B7B7B7", "width_pt": 0.0, "padding_pt": 1.0, "dash_style": "DOT",
			},
			"border_bottom": nil,
		},
	})))

	checkGolden(t, "docs_style_paragraph.json", h.bodyOf(t, 0))
}

// TestDocsStyleTextCarriesTheFontWeightWithTheFamily, because sending a family on its own
// resets the weight to 400 and a bold heading quietly turns regular.
func TestDocsStyleTextCarriesTheFontWeightWithTheFamily(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	h.ok(h.registry.docsStyleText(context.Background(), request(map[string]any{
		"document_id": "doc",
		"start_index": 1,
		"end_index":   14,
		"style": map[string]any{
			"bold": true, "font_size_pt": 26.0, "font_family": "Roboto", "font_weight": 700.0,
			"color": "#FFFFFF", "background_color": "none", "link": "https://example.org",
		},
	})))

	checkGolden(t, "docs_style_text.json", h.bodyOf(t, 0))
}

// TestDocsStyleTableSendsCellsWidthsAndHeights in one batch, in the order that survives:
// the merges last, because a merge changes which cells exist.
func TestDocsStyleTableSendsCellsWidthsAndHeights(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	h.ok(h.registry.docsStyleTable(context.Background(), request(map[string]any{
		"document_id":       "doc",
		"table_start_index": 129,
		"cells": []any{map[string]any{
			"row": 0.0, "column": 0.0, "column_span": 2.0,
			"style": map[string]any{
				"background_color":  "#F3F3F3",
				"content_alignment": "MIDDLE",
				"padding_left_pt":   2.8,
				"border_top":        map[string]any{"color": "#B7B7B7", "width_pt": 0.0, "dash_style": "DOT"},
			},
		}},
		"column_widths":   []any{map[string]any{"columns": []any{0.0}, "width_pt": 233.5}},
		"row_heights":     []any{map[string]any{"rows": []any{0.0}, "min_height_pt": 116.2}},
		"merge":           []any{map[string]any{"row": 0.0, "column": 0.0, "column_span": 2.0}},
		"pin_header_rows": 1.0,
	})))

	checkGolden(t, "docs_style_table.json", h.bodyOf(t, 0))
}

// TestDocsStyleNamedPutsTheTypeInTheMask. Left out of it, Google does not read the field
// at all and answers "Named style type is required" about a request that carries one.
func TestDocsStyleNamedPutsTheTypeInTheMask(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	h.ok(h.registry.docsStyleNamed(context.Background(), request(map[string]any{
		"document_id": "doc",
		"named_style": "NORMAL_TEXT",
		"text_style":  map[string]any{"font_family": "Roboto", "font_size_pt": 12.0},
	})))

	body := string(h.bodyOf(t, 0))
	if !strings.Contains(body, `"fields": "namedStyleType,textStyle.fontSize,textStyle.weightedFontFamily"`) {
		t.Errorf("the mask should name the type and each style field, got %s", body)
	}

	checkGolden(t, "docs_style_named.json", h.bodyOf(t, 0))
}

func TestDocsInsertImageTakesAnAddressAndASize(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	h.ok(h.registry.docsInsertImage(context.Background(), request(map[string]any{
		"document_id": "doc",
		"uri":         "https://lh7-rt.example/pic",
		"index":       31.0,
		"width_pt":    45.4,
		"height_pt":   45.4,
	})))

	checkGolden(t, "docs_insert_image.json", h.bodyOf(t, 0))
}

// TestDocsDeleteNamesOneThing is the shape of the rule: one call removes one thing, and a
// call that names two is refused rather than guessed at.
func TestDocsDeleteNamesOneThing(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.docsDelete(context.Background(), request(map[string]any{
		"document_id": "doc",
		"start_index": 10.0,
		"end_index":   20.0,
		"header_id":   "kix.head",
	})))
	if !strings.Contains(message, "more than one thing") {
		t.Errorf("the refusal should say why, got %q", message)
	}

	message = h.fail(h.registry.docsDelete(context.Background(), request(map[string]any{
		"document_id": "doc",
	})))
	if !strings.Contains(message, "name one thing to remove") {
		t.Errorf("a call naming nothing should say what to name, got %q", message)
	}
}

func TestDocsDeleteRemovesARowOfATable(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	h.ok(h.registry.docsDelete(context.Background(), request(map[string]any{
		"document_id":       "doc",
		"table_start_index": 129.0,
		"row":               2.0,
		"what":              "row",
	})))

	checkGolden(t, "docs_delete_table_row.json", h.bodyOf(t, 0))
}

func TestDocsDeleteTakesBulletsOffWithoutTheText(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	answer := h.ok(h.registry.docsDelete(context.Background(), request(map[string]any{
		"document_id": "doc",
		"start_index": 565.0,
		"end_index":   901.0,
		"what":        "bullets",
	})))

	if !strings.Contains(answer, `"removed": "bullets"`) {
		t.Errorf("the answer should say what went, got %s", answer)
	}
	if !strings.Contains(string(h.bodyOf(t, 0)), "deleteParagraphBullets") {
		t.Error("taking bullets off is its own request, not a deletion of the text")
	}
}

// TestDocsRefusalsExplainTheApiRatherThanRepeatIt collects the four fields a reading
// reports and the writing side will not take. Each was checked against a live document,
// and each refusal says what to do instead — the alternative is a batch that fails with
// everything else in it unwritten.
func TestDocsRefusalsExplainTheApiRatherThanRepeatIt(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	for _, probe := range []struct {
		name string
		call func() string
		want string
	}{
		{
			name: "heading_id",
			call: func() string {
				return h.fail(h.registry.docsStyleParagraph(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 1.0, "end_index": 5.0,
					"style": map[string]any{"heading_id": "h.abc", "alignment": "CENTER"},
				})))
			},
			want: "Docs assigns it",
		},
		{
			name: "transparent page",
			call: func() string {
				return h.fail(h.registry.docsStyleDocument(context.Background(), request(map[string]any{
					"document_id": "doc", "style": map[string]any{"background_color": "none"},
				})))
			},
			want: "transparent background",
		},
		{
			name: "derived margins flag",
			call: func() string {
				return h.fail(h.registry.docsStyleDocument(context.Background(), request(map[string]any{
					"document_id": "doc",
					"style":       map[string]any{"use_custom_header_footer_margins": true},
				})))
			},
			want: "Unallowed field",
		},
		{
			name: "a row marked as a header",
			call: func() string {
				return h.fail(h.registry.docsStyleTable(context.Background(), request(map[string]any{
					"document_id": "doc", "table_start_index": 129.0,
					"row_heights": []any{map[string]any{"rows": []any{0.0}, "table_header": true}},
				})))
			},
			want: "pin_header_rows",
		},
		{
			name: "first page header",
			call: func() string {
				return h.fail(h.registry.docsAddHeaderFooter(context.Background(), request(map[string]any{
					"document_id": "doc", "kind": "header", "type": "FIRST_PAGE",
				})))
			},
			want: "section break after the first page",
		},
		{
			name: "named style inside a named style",
			call: func() string {
				return h.fail(h.registry.docsStyleNamed(context.Background(), request(map[string]any{
					"document_id": "doc", "named_style": "TITLE",
					"paragraph_style": map[string]any{"named_style": "TITLE", "alignment": "CENTER"},
				})))
			},
			want: "named in the named_style argument",
		},
	} {
		if message := probe.call(); !strings.Contains(message, probe.want) {
			t.Errorf("%s: the refusal should carry %q, got %q", probe.name, probe.want, message)
		}
	}
}

// TestDocsSegmentsStartAtZero: the body's own text starts at 1, and a header's at 0. A
// server that applies the body's rule everywhere refuses to write the first character of
// every header there is.
func TestDocsSegmentsStartAtZero(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	h.ok(h.registry.docsStyleText(context.Background(), request(map[string]any{
		"document_id": "doc", "segment_id": "kix.head",
		"start_index": 0.0, "end_index": 5.0,
		"style": map[string]any{"italic": true},
	})))

	message := h.fail(h.registry.docsStyleText(context.Background(), request(map[string]any{
		"document_id": "doc",
		"start_index": 0.0, "end_index": 5.0,
		"style": map[string]any{"italic": true},
	})))
	if !strings.Contains(message, "starts at 1") {
		t.Errorf("the body should still refuse index 0, got %q", message)
	}
}
