package tools

import (
	"context"
	"strings"
	"testing"
)

// documentWithEverything is a document the way one comes back: a section break, a styled
// paragraph, a list item, a table with a painted cell, a picture, and a header.
const documentWithEverything = `{
  "documentId": "doc",
  "title": "Оффер",
  "revisionId": "rev1",
  "documentStyle": {
    "background": {"color": {}},
    "defaultHeaderId": "kix.head",
    "useFirstPageHeaderFooter": true,
    "useCustomHeaderFooterMargins": true,
    "marginTop": {"magnitude": 56.7, "unit": "PT"},
    "pageSize": {"width": {"magnitude": 595.3, "unit": "PT"}, "height": {"magnitude": 841.9, "unit": "PT"}}
  },
  "namedStyles": {"styles": [
    {"namedStyleType": "NORMAL_TEXT",
     "textStyle": {"fontSize": {"magnitude": 12, "unit": "PT"},
       "weightedFontFamily": {"fontFamily": "Roboto", "weight": 400}},
     "paragraphStyle": {"namedStyleType": "NORMAL_TEXT", "direction": "LEFT_TO_RIGHT"}}
  ]},
  "lists": {"kix.list": {"listProperties": {"nestingLevels": [
    {"glyphSymbol": "●", "glyphFormat": "%0", "indentStart": {"magnitude": 36, "unit": "PT"}}
  ]}}},
  "headers": {"kix.head": {"content": [
    {"endIndex": 2, "paragraph": {"elements": [
      {"endIndex": 2, "textRun": {"content": "ш\n", "textStyle": {"italic": true}}}]}}
  ]}},
  "inlineObjects": {"kix.pic": {"objectId": "kix.pic", "inlineObjectProperties": {"embeddedObject": {
    "imageProperties": {"contentUri": "https://lh7-rt.example/pic"},
    "size": {"width": {"magnitude": 510.2, "unit": "PT"}, "height": {"magnitude": 132.4, "unit": "PT"}}}}}},
  "positionedObjects": {"kix.float": {"objectId": "kix.float", "positionedObjectProperties": {
    "positioning": {"layout": "BEHIND_TEXT", "leftOffset": {"magnitude": -52, "unit": "PT"}},
    "embeddedObject": {"imageProperties": {"contentUri": "https://lh7-rt.example/banner"}}}}},
  "body": {"content": [
    {"endIndex": 1, "sectionBreak": {"sectionStyle": {"sectionType": "CONTINUOUS"}}},
    {"startIndex": 1, "endIndex": 14, "paragraph": {
      "elements": [{"startIndex": 1, "endIndex": 14,
        "textRun": {"content": "Анна Соколова\n",
          "textStyle": {"bold": true, "fontSize": {"magnitude": 26, "unit": "PT"},
            "foregroundColor": {"color": {"rgbColor": {"red": 1, "green": 1, "blue": 1}}}}}}],
      "paragraphStyle": {"namedStyleType": "TITLE", "alignment": "CENTER",
        "spaceAbove": {"magnitude": 18, "unit": "PT"},
        "shading": {"backgroundColor": {"color": {"rgbColor": {"red": 0.95, "green": 0.95, "blue": 0.95}}}},
        "borderTop": {"color": {"color": {"rgbColor": {}}}, "width": {"unit": "PT"}, "dashStyle": "SOLID"}},
      "positionedObjectIds": ["kix.float"]}},
    {"startIndex": 14, "endIndex": 30, "paragraph": {
      "elements": [{"startIndex": 14, "endIndex": 30, "textRun": {"content": "Работа с K8s;\n"}}],
      "paragraphStyle": {"namedStyleType": "NORMAL_TEXT", "indentStart": {"magnitude": 36, "unit": "PT"}},
      "bullet": {"listId": "kix.list", "nestingLevel": 0}}},
    {"startIndex": 30, "endIndex": 32, "paragraph": {
      "elements": [{"startIndex": 30, "endIndex": 31, "inlineObjectElement": {"inlineObjectId": "kix.pic"}},
                   {"startIndex": 31, "endIndex": 32, "textRun": {"content": "\n"}}]}},
    {"startIndex": 32, "endIndex": 60, "table": {"rows": 1, "columns": 2,
      "tableStyle": {"tableColumnProperties": [
        {"widthType": "FIXED_WIDTH", "width": {"magnitude": 233.5, "unit": "PT"}},
        {"widthType": "EVENLY_DISTRIBUTED"}]},
      "tableRows": [{"startIndex": 33, "tableRowStyle": {"minRowHeight": {"magnitude": 116.2, "unit": "PT"}},
        "tableCells": [
          {"startIndex": 34, "endIndex": 45,
           "tableCellStyle": {"rowSpan": 1, "columnSpan": 1,
             "backgroundColor": {"color": {"rgbColor": {"red": 0.95, "green": 0.95, "blue": 0.95}}},
             "paddingLeft": {"magnitude": 2.8, "unit": "PT"},
             "borderTop": {"color": {"color": {"rgbColor": {"red": 0.72, "green": 0.72, "blue": 0.72}}},
               "width": {"unit": "PT"}, "dashStyle": "DOT"}},
           "content": [{"startIndex": 35, "endIndex": 45, "paragraph": {
             "elements": [{"startIndex": 35, "endIndex": 45, "textRun": {"content": "Оформление"}}]}}]},
          {"startIndex": 46, "endIndex": 58, "content": [{"paragraph": {"elements": [
             {"textRun": {"content": "ТК РФ"}}]}}]}]}]}}
  ]}
}`

// TestDocsReadStructureReportsWhatRebuildsADocument pins the reading a copy is built
// from: the paragraph styles, the runs, the list a paragraph belongs to, the table's own
// numbers, the page setup, and the two kinds of object with what can be done about each.
func TestDocsReadStructureReportsWhatRebuildsADocument(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/documents/doc", documentWithEverything))

	answer := h.ok(h.registry.docsReadStructure(context.Background(), request(map[string]any{
		"document_id": "doc",
	})))

	for _, want := range []string{
		`"kind": "section_break"`,
		`"named_style": "TITLE"`,
		`"alignment": "CENTER"`,
		`"space_above_pt": 18`,
		`"shading_color": "#F2F2F2"`,
		`"dash_style": "SOLID"`,
		`"color": "#FFFFFF"`,
		`"font_size_pt": 26`,
		`"bullet"`,
		`"list_id": "kix.list"`,
		`"glyph_symbol": "●"`,
		`"inline_object_id": "kix.pic"`,
		`"content_uri": "https://lh7-rt.example/pic"`,
		`"width_pt": 233.5`,
		`"min_height_pt": 116.2`,
		`"background_color": "#F2F2F2"`,
		`"dash_style": "DOT"`,
		`"Оформление"`,
		`"page_size"`,
		`"margin_top_pt": 56.7`,
		`"kix.head"`,
		`"font_family": "Roboto"`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the structure reading should carry %s", want)
		}
	}
}

// TestDocsReadStructureSaysWhatCannotBeRebuilt is the point of reading the objects at
// all: a floating picture is reported so a copy can account for it, together with the
// reason it will not be there — the API has no request that makes one.
func TestDocsReadStructureSaysWhatCannotBeRebuilt(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/documents/doc", documentWithEverything))

	answer := h.ok(h.registry.docsReadStructure(context.Background(), request(map[string]any{
		"document_id": "doc",
	})))

	if !strings.Contains(answer, `"positioned_objects"`) {
		t.Fatal("the floating object should be reported")
	}
	if !strings.Contains(answer, `"writable": false`) {
		t.Error("a floating object should say it cannot be written back")
	}
	if !strings.Contains(answer, "no request that creates a positioned object") {
		t.Error("the reason should be stated where it is met, not left to be rediscovered")
	}
	if !strings.Contains(answer, `"positioned_object_ids"`) {
		t.Error("the paragraph holding the floating object should name it")
	}
}

// TestDocsReadStructureKeepsTheThreeStatesOfAColour apart: absent, none, and black. They
// are three different instructions to a rebuild, and an empty rgbColor is black rather
// than nothing at all.
func TestDocsReadStructureKeepsTheThreeStatesOfAColour(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/documents/doc", documentWithEverything))

	answer := h.ok(h.registry.docsReadStructure(context.Background(), request(map[string]any{
		"document_id": "doc",
	})))

	// The page background is {"color": {}} — a page with no colour of its own.
	if !strings.Contains(answer, `"background_color": "none"`) {
		t.Error("a colourless background should read as none")
	}
	// The paragraph border's colour is {"color": {"rgbColor": {}}} — black.
	if !strings.Contains(answer, `"color": "#000000"`) {
		t.Error("an rgbColor with every component left out is black")
	}
}

func TestDocsReadStructureNarrowsToARange(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/documents/doc", documentWithEverything))

	answer := h.ok(h.registry.docsReadStructure(context.Background(), request(map[string]any{
		"document_id": "doc",
		"start_index": 14,
		"end_index":   30,
	})))

	if strings.Contains(answer, "Анна Соколова") {
		t.Error("an element outside the range should not be reported")
	}
	if !strings.Contains(answer, "Работа с K8s") {
		t.Error("the element inside the range should be")
	}
}
