package tools

import (
	"context"
	"strings"
	"testing"
)

// TestDocsBuildingBlocksSendTheRightRequests covers the pieces a document is assembled
// from. Each is one request, and the golden file for the pair of them is where the shape
// is checked; here it is that the right request goes out at all, and with the place the
// caller named.
func TestDocsBuildingBlocksSendTheRightRequests(t *testing.T) {
	for _, probe := range []struct {
		name string
		send func(*harness) string
		want []string
	}{
		{
			name: "bullets over a range",
			send: func(h *harness) string {
				return h.ok(h.registry.docsMakeBullets(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 565.0, "end_index": 901.0,
					"preset": "BULLET_DISC_CIRCLE_SQUARE",
				})))
			},
			want: []string{"createParagraphBullets", "BULLET_DISC_CIRCLE_SQUARE", `"startIndex": 565`},
		},
		{
			name: "bullets with the preset left out",
			send: func(h *harness) string {
				return h.ok(h.registry.docsMakeBullets(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 1.0, "end_index": 5.0,
				})))
			},
			want: []string{"BULLET_DISC_CIRCLE_SQUARE"},
		},
		{
			name: "a table at the end of the body",
			send: func(h *harness) string {
				return h.ok(h.registry.docsInsertTable(context.Background(), request(map[string]any{
					"document_id": "doc", "rows": 4.0, "columns": 2.0,
				})))
			},
			want: []string{"insertTable", `"rows": 4`, "endOfSegmentLocation"},
		},
		{
			name: "a section break at a place",
			send: func(h *harness) string {
				return h.ok(h.registry.docsInsertSectionBreak(context.Background(), request(map[string]any{
					"document_id": "doc", "section_type": "NEXT_PAGE", "index": 113.0,
				})))
			},
			want: []string{"insertSectionBreak", "NEXT_PAGE", `"index": 113`},
		},
		{
			name: "a page break",
			send: func(h *harness) string {
				return h.ok(h.registry.docsInsertPageBreak(context.Background(), request(map[string]any{
					"document_id": "doc", "index": 42.0,
				})))
			},
			want: []string{"insertPageBreak", `"index": 42`},
		},
		{
			name: "a footnote",
			send: func(h *harness) string {
				return h.ok(h.registry.docsInsertFootnote(context.Background(), request(map[string]any{
					"document_id": "doc",
				})))
			},
			want: []string{"createFootnote", "endOfSegmentLocation"},
		},
		{
			name: "a section's page setup",
			send: func(h *harness) string {
				return h.ok(h.registry.docsStyleSection(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 540.0, "end_index": 541.0,
					"style": map[string]any{
						"margin_footer_pt": 34.0, "default_footer_id": "kix.foot",
						"column_separator": "NONE", "use_first_page_header_footer": false,
						"page_number_start": 1.0, "flip_page_orientation": false,
					},
				})))
			},
			want: []string{"updateSectionStyle", "marginFooter", "defaultFooterId", "pageNumberStart"},
		},
		{
			name: "the whole document's page setup",
			send: func(h *harness) string {
				return h.ok(h.registry.docsStyleDocument(context.Background(), request(map[string]any{
					"document_id": "doc",
					"style": map[string]any{
						"page_size":                    map[string]any{"width_pt": 595.3, "height_pt": 841.9},
						"margin_top_pt":                56.7,
						"background_color":             "#FFFFFF",
						"use_first_page_header_footer": true,
						"use_even_page_header_footer":  false,
						"flip_page_orientation":        false,
						"default_header_id":            "kix.head",
						"page_number_start":            1.0,
					},
				})))
			},
			want: []string{"updateDocumentStyle", "pageSize", "background", "defaultHeaderId"},
		},
		{
			name: "a header for one section",
			send: func(h *harness) string {
				return h.ok(h.registry.docsAddHeaderFooter(context.Background(), request(map[string]any{
					"document_id": "doc", "kind": "footer", "section_break_index": 539.0,
				})))
			},
			want: []string{"createFooter", "sectionBreakLocation", `"index": 539`},
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t))
			probe.send(h)
			body := string(h.bodyOf(t, 0))
			for _, want := range probe.want {
				if !strings.Contains(body, want) {
					t.Errorf("the request should carry %s, got %s", want, body)
				}
			}
		})
	}
}

// TestDocsCallsAreRefusedBeforeTheyReachGoogle covers the arguments that cannot mean
// anything, so a caller gets a sentence rather than a 400 about a field name.
func TestDocsCallsAreRefusedBeforeTheyReachGoogle(t *testing.T) {
	for _, probe := range []struct {
		name string
		send func(*harness) string
		want string
	}{
		{
			name: "an empty range",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleText(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 10.0, "end_index": 10.0,
					"style": map[string]any{"bold": true},
				})))
			},
			want: "must be past",
		},
		{
			name: "a style naming nothing",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleText(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 1.0, "end_index": 5.0,
					"style": map[string]any{},
				})))
			},
			want: "names nothing",
		},
		{
			name: "a paragraph style naming nothing",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleParagraph(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 1.0, "end_index": 5.0,
					"style": map[string]any{},
				})))
			},
			want: "names nothing",
		},
		{
			name: "a style that is not an object",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleText(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 1.0, "end_index": 5.0, "style": "bold",
				})))
			},
			want: "must be an object",
		},
		{
			name: "a colour that is not one",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleText(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 1.0, "end_index": 5.0,
					"style": map[string]any{"color": "красный"},
				})))
			},
			want: "hex colour",
		},
		{
			name: "a number that is not one",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleText(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 1.0, "end_index": 5.0,
					"style": map[string]any{"font_size_pt": "большой"},
				})))
			},
			want: "must be a number",
		},
		{
			name: "a boolean that is not one",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleText(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 1.0, "end_index": 5.0,
					"style": map[string]any{"bold": "да"},
				})))
			},
			want: "true or false",
		},
		{
			name: "a table with no rows",
			send: func(h *harness) string {
				return h.fail(h.registry.docsInsertTable(context.Background(), request(map[string]any{
					"document_id": "doc", "rows": 0.0, "columns": 2.0,
				})))
			},
			want: "at least one row",
		},
		{
			name: "a table styling that asks for nothing",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleTable(context.Background(), request(map[string]any{
					"document_id": "doc", "table_start_index": 129.0,
				})))
			},
			want: "nothing to do",
		},
		{
			name: "a cell with no style",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleTable(context.Background(), request(map[string]any{
					"document_id": "doc", "table_start_index": 129.0,
					"cells": []any{map[string]any{"row": 0.0, "column": 0.0}},
				})))
			},
			want: "needs a style",
		},
		{
			name: "a cell with no row",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleTable(context.Background(), request(map[string]any{
					"document_id": "doc", "table_start_index": 129.0,
					"cells": []any{map[string]any{"column": 0.0, "style": map[string]any{"background_color": "#FFF000"}}},
				})))
			},
			want: "needs a row",
		},
		{
			name: "a width with no columns",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleTable(context.Background(), request(map[string]any{
					"document_id": "doc", "table_start_index": 129.0,
					"column_widths": []any{map[string]any{"width_pt": 200.0}},
				})))
			},
			want: "columns is required",
		},
		{
			name: "a row height naming nothing",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleTable(context.Background(), request(map[string]any{
					"document_id": "doc", "table_start_index": 129.0,
					"row_heights": []any{map[string]any{"rows": []any{0.0}}},
				})))
			},
			want: "names nothing",
		},
		{
			name: "a header that is neither",
			send: func(h *harness) string {
				return h.fail(h.registry.docsAddHeaderFooter(context.Background(), request(map[string]any{
					"document_id": "doc", "kind": "sidebar",
				})))
			},
			want: "header or footer",
		},
		{
			name: "a named style with no style",
			send: func(h *harness) string {
				return h.fail(h.registry.docsStyleNamed(context.Background(), request(map[string]any{
					"document_id": "doc", "named_style": "TITLE",
				})))
			},
			want: "text_style, a paragraph_style, or both",
		},
		{
			name: "a table deletion with no side named",
			send: func(h *harness) string {
				return h.fail(h.registry.docsDelete(context.Background(), request(map[string]any{
					"document_id": "doc", "table_start_index": 129.0,
				})))
			},
			want: "what=row or what=column",
		},
		{
			name: "a deletion running backwards",
			send: func(h *harness) string {
				return h.fail(h.registry.docsDelete(context.Background(), request(map[string]any{
					"document_id": "doc", "start_index": 30.0, "end_index": 20.0,
				})))
			},
			want: "must be past",
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t))
			if message := probe.send(h); !strings.Contains(message, probe.want) {
				t.Errorf("the refusal should carry %q, got %q", probe.want, message)
			}
		})
	}
}

// TestDocsDeleteReachesEverySegmentAndObject covers the rest of the removal targets.
func TestDocsDeleteReachesEverySegmentAndObject(t *testing.T) {
	for _, probe := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"a header", map[string]any{"header_id": "kix.head"}, "deleteHeader"},
		{"a footer", map[string]any{"footer_id": "kix.foot"}, "deleteFooter"},
		{"a floating object", map[string]any{"positioned_object_id": "kix.float"}, "deletePositionedObject"},
		{"a table column", map[string]any{"table_start_index": 129.0, "column": 1.0, "what": "column"}, "deleteTableColumn"},
		{"a stretch of a header", map[string]any{"segment_id": "kix.head", "start_index": 0.0, "end_index": 3.0}, "deleteContentRange"},
	} {
		t.Run(probe.name, func(t *testing.T) {
			h := newHarness(t, newFakeGoogle(t))
			h.ok(h.registry.docsDelete(context.Background(), request(
				merged(map[string]any{"document_id": "doc"}, probe.args))))

			if body := string(h.bodyOf(t, 0)); !strings.Contains(body, probe.want) {
				t.Errorf("the request should be a %s, got %s", probe.want, body)
			}
		})
	}
}

// TestDocsWritingIntoAHeader is what segment_id is for: the same tools, aimed at a
// segment rather than at the body.
func TestDocsWritingIntoAHeader(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	h.ok(h.registry.docsAppend(context.Background(), request(map[string]any{
		"document_id": "doc", "text": "Ждем ответа", "segment_id": "kix.foot",
	})))

	if body := string(h.bodyOf(t, 0)); !strings.Contains(body, `"segmentId": "kix.foot"`) {
		t.Errorf("the insertion should aim at the segment, got %s", body)
	}
}

func merged(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}

	return out
}
