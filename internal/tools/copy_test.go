package tools

import (
	"context"
	"strings"
	"testing"
)

// deckToCopyFrom is a slide with the things a rebuild has to carry: a title with two runs
// in one shape, a bulleted body, a table with a merge and a coloured cell, a picture with
// an address, and a chart pointing at a workbook.
const deckToCopyFrom = `{
  "presentationId": "sample",
  "title": "Образец",
  "pageSize": {"width": {"magnitude": 9144000, "unit": "EMU"}, "height": {"magnitude": 5143500, "unit": "EMU"}},
  "layouts": [
    {"objectId": "layout_body", "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Заголовок и текст"}}
  ],
  "slides": [
    {
      "objectId": "slide1",
      "slideProperties": {"layoutObjectId": "layout_body",
        "notesPage": {"notesProperties": {"speakerNotesObjectId": "notes1"},
          "pageElements": [{"objectId": "notes1", "shape": {"placeholder": {"type": "BODY"},
            "text": {"textElements": [{"textRun": {"content": "Сказать про квоту"}}]}}}]}},
      "pageElements": [
        {"objectId": "title1",
         "size": {"width": {"magnitude": 3000000, "unit": "EMU"}, "height": {"magnitude": 3000000, "unit": "EMU"}},
         "transform": {"scaleX": 2.8, "scaleY": 0.16, "translateX": 311700, "translateY": 190500, "unit": "EMU"},
         "shape": {"shapeType": "TEXT_BOX",
           "shapeProperties": {"contentAlignment": "MIDDLE",
             "shapeBackgroundFill": {"propertyState": "RENDERED",
               "solidFill": {"color": {"rgbColor": {"red": 0.95, "green": 0.96, "blue": 0.98}}, "alpha": 1}},
             "outline": {"propertyState": "INHERIT"}},
           "text": {"textElements": [
             {"paragraphMarker": {"style": {"alignment": "START"}}, "startIndex": 0, "endIndex": 18},
             {"startIndex": 0, "endIndex": 7, "textRun": {"content": "Итоги ",
               "style": {"bold": true, "fontSize": {"magnitude": 24, "unit": "PT"}}}},
             {"startIndex": 7, "endIndex": 18, "textRun": {"content": "августа\n",
               "style": {"italic": true}}}
           ]}}},
        {"objectId": "body1",
         "transform": {"scaleX": 1, "scaleY": 1, "translateX": 311700, "translateY": 900000, "unit": "EMU"},
         "shape": {"shapeType": "TEXT_BOX",
           "text": {"textElements": [
             {"paragraphMarker": {"bullet": {"nestingLevel": 0}, "style": {"alignment": "START"}},
              "startIndex": 0, "endIndex": 15},
             {"startIndex": 0, "endIndex": 15, "textRun": {"content": "Закрыли долг\n"}}
           ]}}},
        {"objectId": "table1",
         "transform": {"scaleX": 1, "scaleY": 1, "translateX": 500000, "translateY": 2000000, "unit": "EMU"},
         "table": {"rows": 2, "columns": 2,
           "tableColumns": [{"columnWidth": {"magnitude": 2000000, "unit": "EMU"}},
                            {"columnWidth": {"magnitude": 1500000, "unit": "EMU"}}],
           "tableRows": [
             {"tableCells": [
               {"location": {"rowIndex": 0, "columnIndex": 0}, "rowSpan": 1, "columnSpan": 2,
                "tableCellProperties": {"contentAlignment": "MIDDLE",
                  "tableCellBackgroundFill": {"solidFill": {"color": {"rgbColor": {"red": 0.9}}}}},
                "text": {"textElements": [{"textRun": {"content": "Команда", "style": {"bold": true}}}]}}
             ]},
             {"tableCells": [
               {"location": {"rowIndex": 1, "columnIndex": 0}, "rowSpan": 1, "columnSpan": 1,
                "text": {"textElements": [{"textRun": {"content": "SRE"}}]}},
               {"location": {"rowIndex": 1, "columnIndex": 1}, "rowSpan": 1, "columnSpan": 1,
                "text": {"textElements": [{"textRun": {"content": "12"}}]}}
             ]}
           ]}},
        {"objectId": "pic1",
         "transform": {"scaleX": 1, "scaleY": 1, "translateX": 6000000, "translateY": 900000, "unit": "EMU"},
         "image": {"contentUrl": "https://example.invalid/pic.png",
           "imageProperties": {"transparency": 0.25}}},
        {"objectId": "chart1",
         "transform": {"scaleX": 1, "scaleY": 1, "translateX": 500000, "translateY": 3500000, "unit": "EMU"},
         "sheetsChart": {"spreadsheetId": "book", "chartId": 4242}},
        {"objectId": "group1",
         "transform": {"scaleX": 1, "scaleY": 1, "translateX": 100, "translateY": 100, "unit": "EMU"},
         "elementGroup": {"children": [{"objectId": "inner1"}]}}
      ]
    }
  ]
}`

// deckToCopyInto is a target deck whose layout has the same name as the sample's.
const deckToCopyInto = `{
  "presentationId": "target",
  "title": "Наша",
  "layouts": [
    {"objectId": "target_layout", "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Заголовок и текст"}}
  ],
  "slides": []
}`

// TestCopySlideBuildsWhatItRead pins the batch a copied slide turns into. The request bodies
// are the point: what the slide ends up looking like is decided by them, not by which
// methods were called.
func TestCopySlideBuildsWhatItRead(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/sample", deckToCopyFrom).
		answer("/presentations/target:batchUpdate", `{"replies": [{"createSlide": {"objectId": "new1"}}]}`).
		answer("/presentations/target", deckToCopyInto))

	answer := h.ok(h.registry.slidesCopySlide(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_page_object_id":  "slide1",
		"target_presentation_id": "target",
	})))

	checkGolden(t, "copy_slide.json", h.bodyOf(t, 2))

	// A layout is matched by name, because the sample's layout identifier means nothing in
	// the target deck.
	if !strings.Contains(answer, "Заголовок и текст") {
		t.Errorf("the answer should name the layout the slide landed on, got %s", answer)
	}

	// The losses are named rather than left to be discovered on the finished slide.
	for _, want := range []string{"not_carried", "group of elements"} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should carry %s, got %s", want, answer)
		}
	}
}

// deckWithPlaceholders is a target whose layout offers the same slots as the sample's, which
// is the ordinary case: two decks built from the same template.
const deckWithPlaceholders = `{
  "presentationId": "target",
  "layouts": [
    {"objectId": "target_layout",
     "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Заголовок и текст"},
     "pageElements": [
       {"objectId": "L_title", "shape": {"placeholder": {"type": "TITLE", "index": 0}}},
       {"objectId": "L_body", "shape": {"placeholder": {"type": "BODY", "index": 0}}}
     ]}
  ],
  "slides": []
}`

// deckWithAPlaceholderTitle is a sample whose title is a placeholder — which is what a title
// is in any deck built from a template.
const deckWithAPlaceholderTitle = `{
  "presentationId": "sample",
  "layouts": [
    {"objectId": "layout_body", "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Заголовок и текст"}}
  ],
  "slides": [
    {
      "objectId": "slideT",
      "slideProperties": {"layoutObjectId": "layout_body"},
      "pageElements": [
        {"objectId": "t1",
         "transform": {"scaleX": 1, "scaleY": 1, "translateX": 311700, "translateY": 190500, "unit": "EMU"},
         "shape": {"shapeType": "TEXT_BOX", "placeholder": {"type": "TITLE", "index": 0},
           "text": {"textElements": [
             {"paragraphMarker": {}, "startIndex": 0, "endIndex": 9},
             {"startIndex": 0, "endIndex": 9, "textRun": {"content": "Прогресс\n"}}]}}}
      ]
    }
  ]
}`

// TestCopySlideWritesIntoTheLayoutsOwnSlots is the fix a live copy demanded. A title rebuilt
// as an ordinary text box comes out in the target's default grey, because almost everything
// about a title — its size, weight and colour — lives on the layout's placeholder and not on
// the slide. Asking createSlide for the layout's slots by name puts the text where the look
// already is.
func TestCopySlideWritesIntoTheLayoutsOwnSlots(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/sample", deckWithAPlaceholderTitle).
		answer("/presentations/target:batchUpdate", `{"replies": [{"createSlide": {"objectId": "new1"}}]}`).
		answer("/presentations/target", deckWithPlaceholders))

	answer := h.ok(h.registry.slidesCopySlide(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_page_object_id":  "slideT",
		"target_presentation_id": "target",
		"copy_speaker_notes":     false,
	})))

	body := string(h.bodyOf(t, 2))
	for _, want := range []string{`"placeholderIdMappings"`, `"type": "TITLE"`, `"objectId": "ph_test"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the slide should be created with the layout's slots, got %s", body)
		}
	}

	// Nothing is created for a placeholder, and no geometry is sent: the slot sits where
	// this deck's layout puts it, not where the sample's did.
	if strings.Contains(body, "createShape") {
		t.Errorf("a placeholder should be filled, not rebuilt as a shape, got %s", body)
	}
	if !strings.Contains(body, `"objectId": "ph_test",`) || !strings.Contains(body, "Прогресс") {
		t.Errorf("the text should go into the slot, got %s", body)
	}

	if !strings.Contains(answer, `"carried_whole": true`) {
		t.Errorf("nothing was lost, so the answer should say so: %s", answer)
	}
}

// TestCopySlideSaysWhenALayoutIsMissing: a target deck without the sample's layout gets a
// blank slide, and the answer says so. Silently landing on BLANK is how a copied slide comes
// out with the right words in the wrong font and nobody knows why.
func TestCopySlideSaysWhenALayoutIsMissing(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/sample", deckToCopyFrom).
		answer("/presentations/bare:batchUpdate", `{"replies": [{"createSlide": {"objectId": "new1"}}]}`).
		answer("/presentations/bare", `{"presentationId": "bare", "layouts": [], "slides": []}`))

	answer := h.ok(h.registry.slidesCopySlide(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_page_object_id":  "slide1",
		"target_presentation_id": "bare",
		"copy_speaker_notes":     false,
	})))

	if !strings.Contains(answer, "which the target deck does not have") {
		t.Errorf("the answer should say the layout was missing, got %s", answer)
	}
	if !strings.Contains(string(h.bodyOf(t, 2)), `"predefinedLayout": "BLANK"`) {
		t.Error("a slide with no matching layout should be created blank")
	}
}

// TestCopySlideInsideOneDeckDuplicates is the difference between a copy and the same slide.
//
// Rebuilding inside one deck would be strictly worse than what Google does itself: createShape
// cannot reproduce an authored corner radius, so a rounded panel comes back square, and a
// linked chart would be made again rather than carried. A deck whose "Metrics" slide is
// multiplied once per metric is exactly the case this matters in.
func TestCopySlideInsideOneDeckDuplicates(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/sample:batchUpdate", `{"replies": [{"duplicateObject": {"objectId": "dup1"}}]}`).
		answer("/presentations/sample", deckToCopyFrom))

	answer := h.ok(h.registry.slidesCopySlide(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_page_object_id":  "slide1",
		"target_presentation_id": "sample",
		"insert_at":              float64(3),
	})))

	body := string(h.bodyOf(t, 1))
	for _, want := range []string{"duplicateObject", "updateSlidesPosition", `"insertionIndex": 3`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}
	if strings.Contains(body, "createSlide") || strings.Contains(body, "createShape") {
		t.Errorf("nothing should be rebuilt inside one deck, got %s", body)
	}

	for _, want := range []string{`"method": "duplicate"`, `"carried_whole": true`, `"page_object_id": "dup1"`} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should carry %s, got %s", want, answer)
		}
	}
}

// TestCopySlideRefusesAnUnknownSlide: naming the slides there are beats "not found", because
// the identifiers are not guessable and a caller usually has the wrong deck open.
func TestCopySlideRefusesAnUnknownSlide(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/sample", deckToCopyFrom))

	message := h.fail(h.registry.slidesCopySlide(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_page_object_id":  "slide9",
		"target_presentation_id": "target",
	})))

	if !strings.Contains(message, "slide1") {
		t.Errorf("the refusal should name the slides there are, got %s", message)
	}
}

// TestCopyElementPlacesWhereItIsTold: an element copied to a named position keeps its scale
// and takes the new corner, which is what putting a sample's panel on our own slide means.
func TestCopyElementPlacesWhereItIsTold(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/sample", deckToCopyFrom).
		answer("/presentations/target:batchUpdate", `{"replies": [{}]}`))

	h.ok(h.registry.slidesCopyElement(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_object_id":       "title1",
		"target_presentation_id": "target",
		"target_page_object_id":  "slideA",
		"x_emu":                  float64(1000000),
		"y_emu":                  float64(2000000),
	})))

	checkGolden(t, "copy_element.json", h.bodyOf(t, 1))
}

// TestCopyElementRefusesWhatCannotBeBuilt: a group cannot be made in one request, and saying
// so beats creating half of it.
func TestCopyElementRefusesWhatCannotBeBuilt(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/sample", deckToCopyFrom))

	message := h.fail(h.registry.slidesCopyElement(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_object_id":       "group1",
		"target_presentation_id": "target",
		"target_page_object_id":  "slideA",
	})))

	if !strings.Contains(message, "gdocs_slides_group") {
		t.Errorf("the refusal should say what to do instead, got %s", message)
	}
}

// deckWithALine is a slide holding the two element kinds the fixture above leaves out: a
// line with its own drawing, and a video.
const deckWithALine = `{
  "presentationId": "sample",
  "layouts": [],
  "slides": [
    {
      "objectId": "slide2",
      "pageElements": [
        {"objectId": "line1",
         "transform": {"scaleX": 1, "scaleY": 1, "translateX": 200000, "translateY": 300000, "unit": "EMU"},
         "line": {"lineType": "STRAIGHT_LINE", "lineCategory": "BENT",
           "lineProperties": {"weight": {"magnitude": 9525, "unit": "EMU"}, "dashStyle": "DASH",
             "endArrow": "FILL_ARROW",
             "lineFill": {"solidFill": {"color": {"themeColor": "ACCENT1"}, "alpha": 1}}}}},
        {"objectId": "video1",
         "transform": {"scaleX": 1, "scaleY": 1, "translateX": 400000, "translateY": 500000, "unit": "EMU"},
         "video": {"url": "https://youtu.be/x", "source": "YOUTUBE", "id": "x"}},
        {"objectId": "picNoURL",
         "transform": {"scaleX": 1, "scaleY": 1, "translateX": 10, "translateY": 10, "unit": "EMU"},
         "image": {}}
      ]
    }
  ]
}`

// TestCopyElementRebuildsALine: a line copied without its drawing is a line of a different
// weight, colour and dash, which on a diagram reads as a different diagram.
func TestCopyElementRebuildsALine(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/sample", deckWithALine).
		answer("/presentations/target:batchUpdate", `{"replies": [{}]}`))

	h.ok(h.registry.slidesCopyElement(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_object_id":       "line1",
		"target_presentation_id": "target",
		"target_page_object_id":  "slideA",
	})))

	body := string(h.bodyOf(t, 1))
	for _, want := range []string{"createLine", `"category": "BENT"`, "updateLineProperties",
		`"fields": "lineFill,weight,dashStyle,endArrow"`, `"themeColor": "ACCENT1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}
}

// TestCopyElementRebuildsAVideo covers the last element kind that can be created.
func TestCopyElementRebuildsAVideo(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/sample", deckWithALine).
		answer("/presentations/target:batchUpdate", `{"replies": [{}]}`))

	h.ok(h.registry.slidesCopyElement(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_object_id":       "video1",
		"target_presentation_id": "target",
		"target_page_object_id":  "slideA",
	})))

	if body := string(h.bodyOf(t, 1)); !strings.Contains(body, `"source": "YOUTUBE"`) {
		t.Errorf("the request should carry the video's source, got %s", body)
	}
}

// TestCopyElementRefusesAPictureWithNoAddress: Slides hands out an address for a picture it
// can serve, and nothing for one it cannot. Without an address there is nothing to fetch,
// and a createImage with an empty URL is a refused batch with an unhelpful message.
func TestCopyElementRefusesAPictureWithNoAddress(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/sample", deckWithALine))

	message := h.fail(h.registry.slidesCopyElement(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_object_id":       "picNoURL",
		"target_presentation_id": "target",
		"target_page_object_id":  "slideA",
	})))

	if !strings.Contains(message, "cannot be fetched") {
		t.Errorf("the refusal should say why, got %s", message)
	}
}

// TestCopySlideCarriesSpeakerNotes: the notes live on a page behind the slide that does not
// exist until the slide does, so they are a second batch — and the tool has to read the new
// slide back to learn which shape takes them.
func TestCopySlideCarriesSpeakerNotes(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/sample", deckToCopyFrom).
		answer("/presentations/target:batchUpdate", `{"replies": [{"createSlide": {"objectId": "new1"}}]}`).
		answer("/presentations/target", `{"presentationId": "target",
          "layouts": [{"objectId": "L", "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Заголовок и текст"}}],
          "slides": [{"objectId": "new1", "slideProperties": {"notesPage":
            {"notesProperties": {"speakerNotesObjectId": "newnotes"}}}}]}`))

	answer := h.ok(h.registry.slidesCopySlide(context.Background(), request(map[string]any{
		"source_presentation_id": "sample",
		"source_page_object_id":  "slide1",
		"target_presentation_id": "target",
	})))

	if !strings.Contains(answer, `"speaker_notes": true`) {
		t.Errorf("the answer should say the notes came across, got %s", answer)
	}

	last := string(h.bodyOf(t, len(h.google.requests)-1))
	for _, want := range []string{"newnotes", "Сказать про квоту"} {
		if !strings.Contains(last, want) {
			t.Errorf("the last batch should write the notes, got %s", last)
		}
	}
}

// TestCopyRangeRefusesAnEmptyReading: a rectangle that came back with nothing usually means
// a tab renamed since somebody wrote the range down, and saying so beats writing nothing.
func TestCopyRangeRefusesAnEmptyReading(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/spreadsheets/source", `{"spreadsheetId": "source", "sheets": []}`))

	message := h.fail(h.registry.sheetsCopyRange(context.Background(), request(map[string]any{
		"source_spreadsheet_id": "source", "source_sheet_title": "Нет такой",
		"start_row": float64(0), "end_row": float64(1),
		"start_column": float64(0), "end_column": float64(1),
		"target_spreadsheet_id": "target", "target_sheet_title": "Лист",
	})))

	if !strings.Contains(message, "check the tab name") {
		t.Errorf("the refusal should say where to look, got %s", message)
	}
}

// TestCopySheetKeepsGooglesNameWhenTheRenameFails: the copy exists either way, and a tab
// called "Copy of …" is a recoverable outcome while a silent failure is not.
func TestCopySheetKeepsGooglesNameWhenTheRenameFails(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":copyTo", `{"sheetId": 91, "title": "Копия Цели"}`).
		fail("/spreadsheets/target:batchUpdate", 403, `{"error": {"message": "no access"}}`).
		answer("/spreadsheets/source", bookToCopyFrom))

	answer := h.ok(h.registry.sheetsCopySheet(context.Background(), request(map[string]any{
		"source_spreadsheet_id": "source", "source_sheet_title": "Цели",
		"target_spreadsheet_id": "target", "new_title": "Цели августа",
	})))

	for _, want := range []string{"rename_failed", `"title": "Копия Цели"`} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should carry %s, got %s", want, answer)
		}
	}
}

// TestDocsCopyRangeRefusesAnEmptyRange and its neighbours cover the arithmetic's edges,
// where an off-by-one writes into somebody else's paragraph.
func TestDocsCopyRangeRefusesAnEmptyRange(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	message := h.fail(h.registry.docsCopyRange(context.Background(), request(map[string]any{
		"source_document_id": "source", "start_index": float64(10), "end_index": float64(10),
		"target_document_id": "target",
	})))

	if !strings.Contains(message, "end_index is exclusive") {
		t.Errorf("the refusal should explain the range, got %s", message)
	}
}

// TestDocsCopyRangeRefusesWhenNothingCanBeCarried: a range holding only a table produces no
// requests at all, and answering "copied" would be a lie.
func TestDocsCopyRangeRefusesWhenNothingCanBeCarried(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/documents/source", documentToCopyFrom).
		answer("/documents/target", documentToCopyInto))

	message := h.fail(h.registry.docsCopyRange(context.Background(), request(map[string]any{
		"source_document_id": "source", "start_index": float64(45), "end_index": float64(90),
		"target_document_id": "target",
	})))

	if !strings.Contains(message, "nothing in that range can be carried") {
		t.Errorf("the refusal should say the range held nothing carryable, got %s", message)
	}
}

// TestDocsCopyRangeClipsAPartialRun: a range that starts inside a run takes the part inside
// it and no more. Taking the whole run is how a copy of one sentence brings the paragraph.
func TestDocsCopyRangeClipsAPartialRun(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/documents/source", documentToCopyFrom).
		answer("/documents/target:batchUpdate", `{"documentId": "target", "replies": [{}]}`).
		answer("/documents/target", documentToCopyInto))

	h.ok(h.registry.docsCopyRange(context.Background(), request(map[string]any{
		"source_document_id": "source", "start_index": float64(7), "end_index": float64(14),
		"target_document_id": "target", "target_index": float64(1),
	})))

	// The run "Условия работы\n" starts at index 1, so a range of 7..14 is offsets 6..13
	// inside it. Counted in bytes the same range would start in the middle of "и".
	body := string(h.bodyOf(t, 2))
	if !strings.Contains(body, `"text": "я работ"`) {
		t.Errorf("the run should be cut at the range's own edges, got %s", body)
	}
}

// bookToCopyFrom is a rectangle with what a copy has to carry: a formula rather than the
// number it produces today, a format, a note, a dropdown, a merge — and a rule that paints
// by content, which does not travel.
const bookToCopyFrom = `{
  "spreadsheetId": "source",
  "properties": {"title": "Образец"},
  "sheets": [
    {
      "properties": {"sheetId": 3, "title": "Цели", "index": 0},
      "merges": [{"sheetId": 3, "startRowIndex": 1, "endRowIndex": 2,
        "startColumnIndex": 1, "endColumnIndex": 3}],
      "conditionalFormats": [
        {"ranges": [{"sheetId": 3, "startRowIndex": 1, "endRowIndex": 5,
          "startColumnIndex": 1, "endColumnIndex": 3}]}
      ],
      "data": [
        {
          "startRow": 1,
          "startColumn": 1,
          "rowData": [
            {"values": [
              {"userEnteredValue": {"stringValue": "Статус"},
               "userEnteredFormat": {"backgroundColor": {"red": 1, "green": 1, "blue": 1},
                 "textFormat": {"bold": true}},
               "note": "заполняет дежурный",
               "dataValidation": {"condition": {"type": "ONE_OF_LIST",
                 "values": [{"userEnteredValue": "Всё ок"}]}, "strict": true}},
              {"userEnteredValue": {"formulaValue": "=SUM(D2:D9)"}}
            ]}
          ]
        }
      ]
    }
  ]
}`

// TestCopyRangeCarriesWhatWasTyped: the mask is the whole of what this tool claims, and the
// formula is the reason SpreadsheetGrid could not be reused — it asks for the value shown,
// which turns a formula into today's number and stops the copy following its inputs.
func TestCopyRangeCarriesWhatWasTyped(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/spreadsheets/source", bookToCopyFrom).
		answer("/spreadsheets/target:batchUpdate", `{"spreadsheetId": "target", "replies": [{}]}`).
		answer("/spreadsheets/target", spreadsheetInfo))

	answer := h.ok(h.registry.sheetsCopyRange(context.Background(), request(map[string]any{
		"source_spreadsheet_id": "source",
		"source_sheet_title":    "Цели",
		"start_row":             float64(1),
		"end_row":               float64(2),
		"start_column":          float64(1),
		"end_column":            float64(3),
		"target_spreadsheet_id": "target",
		"target_sheet_title":    "Сотрудники",
		"target_row":            float64(5),
		"target_column":         float64(0),
	})))

	checkGolden(t, "sheets_copy_range.json", h.bodyOf(t, 2))

	// The rule that paints by content overlaps the rectangle and does not travel, so it is
	// named. A rectangle whose colours came from a rule arrives grey otherwise.
	if !strings.Contains(answer, "conditional formatting") {
		t.Errorf("the answer should name what was left behind, got %s", answer)
	}
}

// TestCopySheetRenamesAfterwards: the copy exists either way, so the rename is a second
// request — a rename that fails should leave a tab called "Copy of …" rather than nothing.
func TestCopySheetRenamesAfterwards(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer(":copyTo", `{"sheetId": 91, "title": "Копия Цели", "index": 2}`).
		answer("/spreadsheets/target:batchUpdate", `{"spreadsheetId": "target", "replies": [{}]}`).
		answer("/spreadsheets/source", bookToCopyFrom))

	answer := h.ok(h.registry.sheetsCopySheet(context.Background(), request(map[string]any{
		"source_spreadsheet_id": "source",
		"source_sheet_title":    "Цели",
		"target_spreadsheet_id": "target",
		"new_title":             "Цели августа",
	})))

	for _, want := range []string{`"sheet_id": 91`, `"title": "Цели августа"`} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should carry %s, got %s", want, answer)
		}
	}

	if path := h.google.requests[1].Path; !strings.Contains(path, "/sheets/3:copyTo") {
		t.Errorf("the copy should address the tab by its number, got %s", path)
	}
}

// documentToCopyFrom is a stretch with a heading, a styled run, a bullet and a picture.
const documentToCopyFrom = `{
  "documentId": "source",
  "title": "Оффер",
  "inlineObjects": {
    "kix.pic1": {"inlineObjectProperties": {"embeddedObject": {
      "imageProperties": {"contentUri": "https://example.invalid/logo.png"},
      "size": {"width": {"magnitude": 120, "unit": "PT"}, "height": {"magnitude": 40, "unit": "PT"}}}}}
  },
  "body": {"content": [
    {"startIndex": 0, "endIndex": 1, "sectionBreak": {}},
    {"startIndex": 1, "endIndex": 20, "paragraph": {
      "paragraphStyle": {"namedStyleType": "HEADING_1", "alignment": "CENTER"},
      "elements": [
        {"startIndex": 1, "endIndex": 20, "textRun": {"content": "Условия работы\n",
          "textStyle": {"bold": true, "fontSize": {"magnitude": 18, "unit": "PT"}}}}
      ]}},
    {"startIndex": 20, "endIndex": 45, "paragraph": {
      "bullet": {"listId": "kix.l1", "nestingLevel": 0},
      "paragraphStyle": {"alignment": "START", "indentStart": {"magnitude": 36, "unit": "PT"}},
      "elements": [
        {"startIndex": 20, "endIndex": 44, "textRun": {"content": "Удалённо, гибкий график",
          "textStyle": {"italic": true}}},
        {"startIndex": 44, "endIndex": 45, "inlineObjectElement": {"inlineObjectId": "kix.pic1"}}
      ]}},
    {"startIndex": 45, "endIndex": 90, "table": {"rows": 2, "columns": 2}}
  ]}
}`

// documentToCopyInto is an empty target: Docs keeps a final newline nothing may be inserted
// after, which is why the end of the body is one before its end index.
const documentToCopyInto = `{
  "documentId": "target",
  "title": "Новый",
  "body": {"content": [{"startIndex": 0, "endIndex": 1, "sectionBreak": {}},
    {"startIndex": 1, "endIndex": 2, "paragraph": {"elements": [
      {"startIndex": 1, "endIndex": 2, "textRun": {"content": "\n"}}]}}]}
}`

// TestDocsCopyRangeWritesInTargetCoordinates is the arithmetic this tool is mostly made of:
// every style names a range in the target's indices, not the source's. A style applied at
// the source's indices lands wherever those happen to fall in the other document.
func TestDocsCopyRangeWritesInTargetCoordinates(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/documents/source", documentToCopyFrom).
		answer("/documents/target:batchUpdate", `{"documentId": "target", "replies": [{}]}`).
		answer("/documents/target", documentToCopyInto))

	answer := h.ok(h.registry.docsCopyRange(context.Background(), request(map[string]any{
		"source_document_id": "source",
		"start_index":        float64(1),
		"end_index":          float64(90),
		"target_document_id": "target",
	})))

	checkGolden(t, "docs_copy_range.json", h.bodyOf(t, 2))

	// A table in the range is named rather than half-built: its cells only get indices once
	// it exists, and text put in the wrong cells is worse than text not carried.
	if !strings.Contains(answer, "gdocs_docs_insert_table") {
		t.Errorf("the answer should say what to do about the table, got %s", answer)
	}
}

// TestDocsCopyRangeCountsInUTF16: the indices a document reports are UTF-16 code units. For
// Russian a byte offset is twice too large and a rune offset too small, and either cuts a
// range that does not line up with what the document said.
func TestDocsCopyRangeCountsInUTF16(t *testing.T) {
	for _, probe := range []struct {
		text          string
		from, to      int64
		want          string
		whatItIsAbout string
	}{
		{"Условия работы", 0, 7, "Условия", "a Cyrillic word cut at its own boundary"},
		{"Условия работы", 8, 14, "работы", "the second word, offset past the first"},
		{"👍 готово", 0, 2, "👍", "an emoji is two code units, not one"},
	} {
		if got := utf16Slice(probe.text, probe.from, probe.to); got != probe.want {
			t.Errorf("%s: utf16Slice(%q, %d, %d) = %q, want %q",
				probe.whatItIsAbout, probe.text, probe.from, probe.to, got, probe.want)
		}
	}
}
