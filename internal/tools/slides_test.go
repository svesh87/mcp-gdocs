package tools

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// presentationWithBody is a deck holding one title box and one body box, styled the way
// a template styles them.
const presentationWithBody = `{
  "presentationId": "deck",
  "title": "Квартальный отчёт",
  "pageSize": {"width": {"magnitude": 9144000, "unit": "EMU"}, "height": {"magnitude": 5143500, "unit": "EMU"}},
  "slides": [
    {
      "objectId": "slide1",
      "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Заголовок и текст"},
      "pageElements": [
        {
          "objectId": "title1",
          "size": {"width": {"magnitude": 8000000, "unit": "EMU"}, "height": {"magnitude": 800000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 500000, "translateY": 300000, "unit": "EMU"},
          "shape": {
            "shapeType": "TEXT_BOX",
            "placeholder": {"type": "TITLE"},
            "text": {"textElements": [
              {"paragraphMarker": {}, "startIndex": 0, "endIndex": 10},
              {"textRun": {"content": "Итоги квартала\n", "style": {
                 "fontFamily": "Roboto",
                 "fontSize": {"magnitude": 28, "unit": "PT"},
                 "bold": true,
                 "weightedFontFamily": {"fontFamily": "Roboto", "weight": 700},
                 "foregroundColor": {"opaqueColor": {"rgbColor": {"red": 0.1, "green": 0.1, "blue": 0.1}}}
               }}}
            ]}
          }
        },
        {
          "objectId": "body1",
          "shape": {
            "shapeType": "TEXT_BOX",
            "placeholder": {"type": "BODY"},
            "text": {"textElements": [
              {"paragraphMarker": {"bullet": {"nestingLevel": 0}}, "startIndex": 0, "endIndex": 12},
              {"textRun": {"content": "Старый текст\n"}}
            ]}
          }
        },
        {"objectId": "table1", "table": {"rows": 2, "columns": 3}}
      ]
    }
  ],
  "layouts": [
    {"objectId": "layout_body", "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Заголовок и текст"}}
  ]
}`

const emptyBatchReply = `{"presentationId": "deck", "replies": [{}]}`

// TestSetListWithPlainHeading pins the batch that rebuilds a body slide with a heading
// above the list.
//
// This is the sequence the whole server exists for: bullets off, text out, text in,
// native bullets over everything below the heading, then the heading's own indent reset.
// Any change to it changes how every rebuilt slide looks.
func TestSetListWithPlainHeading(t *testing.T) {
	fake := newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody)
	h := newHarness(t, fake)

	result, err := h.registry.slidesSetList(context.Background(), request(map[string]any{
		"presentation_id":  "deck",
		"object_id":        "body1",
		"plain_first_line": true,
		"items": []any{
			map[string]any{"text": "Что сделали", "level": float64(0)},
			map[string]any{"text": "Инфраструктура", "level": float64(0)},
			map[string]any{"text": "Переехали на новый кластер", "level": float64(1)},
			map[string]any{"text": "Сократили холодный старт вдвое", "level": float64(1)},
			map[string]any{"text": "Продукт", "level": float64(0)},
			map[string]any{"text": "Выкатили отчёты", "level": float64(1)},
		},
	}))

	requireOK(t, result, err)
	// Request 0 reads what is in the body box, request 1 is the batch.
	checkGolden(t, "replace_body_nested_list.json", h.bodyOf(t, 1))
}

// TestSetListIsALlBulletsByDefault is the case that a sample deck turned out to need: a
// body where every line is a bullet, three levels deep, with nothing standing outside the
// list. The old shape of this tool could not express it — it always kept the first line
// out — and the slide came back flat, unbulleted and cut short.
func TestSetListIsAllBulletsByDefault(t *testing.T) {
	fake := newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody)
	h := newHarness(t, fake)

	h.ok(h.registry.slidesSetList(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"items": []any{
			map[string]any{"text": "Восстановили графики в табло", "level": float64(0)},
			map[string]any{"text": "Саше спасибо за помощь", "level": float64(1)},
			map[string]any{"text": "Переделали процесс On Call", "level": float64(0)},
			map[string]any{"text": "Что было", "level": float64(1)},
			map[string]any{"text": "Дежурство было строго добровольным", "level": float64(2)},
		},
	})))

	checkGolden(t, "set_list_all_bullets.json", h.bodyOf(t, 1))
}

// TestSetListPutsWordsInAndNothingElse keeps the tool at one job. It used to be able to
// fetch a heading's style from another box on the way past, which meant a caller could
// change how a slide looks without ever seeing a number. Styling is a separate call now.
func TestSetListPutsWordsInAndNothingElse(t *testing.T) {
	fake := newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody)
	h := newHarness(t, fake)

	h.ok(h.registry.slidesSetList(context.Background(), request(map[string]any{
		"presentation_id":  "deck",
		"object_id":        "body1",
		"plain_first_line": true,
		"items": []any{
			map[string]any{"text": "Что сделали", "level": float64(0)},
			map[string]any{"text": "Инфраструктура", "level": float64(0)},
			map[string]any{"text": "Переехали", "level": float64(1)},
		},
	})))

	if body := string(h.bodyOf(t, 1)); strings.Contains(body, "updateTextStyle") {
		t.Errorf("putting words in should not restyle anything: %s", body)
	}
}

func TestSetListRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "no lines",
			args: map[string]any{"presentation_id": "deck", "object_id": "body1", "items": []any{}},
			want: "items is empty",
		},
		{
			name: "a line with no text",
			args: map[string]any{"presentation_id": "deck", "object_id": "body1",
				"items": []any{map[string]any{"text": "", "level": float64(0)}}},
			want: "items[0].text is empty",
		},
		{
			// A newline would make paragraphs the levels do not describe, and a tab is how
			// depth is spelled on the wire — a line carrying one lands a level deeper than
			// it says.
			name: "a line carrying its own newline",
			args: map[string]any{"presentation_id": "deck", "object_id": "body1",
				"items": []any{map[string]any{"text": "первая\nвторая"}}},
			want: "one object per line",
		},
		{
			name: "a level deeper than Slides nests",
			args: map[string]any{"presentation_id": "deck", "object_id": "body1",
				"items": []any{map[string]any{"text": "глубоко", "level": float64(9)}}},
			want: "eight deep",
		},
		{
			name: "a level that counts backwards",
			args: map[string]any{"presentation_id": "deck", "object_id": "body1",
				"items": []any{map[string]any{"text": "строка", "level": float64(-1)}}},
			want: "levels count from 0",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := h.registry.slidesSetList(context.Background(), request(test.args))
			if message := requireError(t, result, err); !strings.Contains(message, test.want) {
				t.Errorf("expected a refusal mentioning %q, got %q", test.want, message)
			}
		})
	}
}

// TestCreateTableWithText pins the batch that makes a real table.
func TestCreateTableWithText(t *testing.T) {
	fake := newFakeGoogle(t).answer("/presentations/deck:batchUpdate",
		`{"presentationId": "deck", "replies": [{"createTable": {"objectId": "table_test"}}]}`)
	h := newHarness(t, fake)

	result, err := h.registry.slidesCreateTableWithText(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"rows": []any{
			[]any{"Метрика", "Было", "Стало"},
			[]any{"Холодный старт", float64(4.2), float64(2)},
		},
		"x":                 float64(457200),
		"y":                 float64(1200000),
		"width":             float64(8229600),
		"height":            float64(1200000),
		"font_size":         float64(11),
		"header_font_size":  float64(12),
		"column_widths":     []any{float64(3000000), float64(2600000), float64(2629600)},
		"font_family":       "Roboto",
		"foreground_color":  map[string]any{"red": 0.1, "green": 0.1, "blue": 0.1},
		"column_alignments": []any{"START", "END", "END"},
		"header_alignments": []any{"START", "CENTER", "CENTER"},
	}))

	requireOK(t, result, err)
	checkGolden(t, "create_table_with_text.json", h.bodyOf(t, 0))
}

func TestCreateTableRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "ragged rows",
			args: map[string]any{"presentation_id": "deck", "page_object_id": "s", "x": 0.0, "y": 0.0,
				"width": 100.0, "height": 100.0,
				"rows": []any{[]any{"a", "b"}, []any{"c"}}},
			want: "rectangular",
		},
		{
			name: "column widths of the wrong length",
			args: map[string]any{"presentation_id": "deck", "page_object_id": "s", "x": 0.0, "y": 0.0,
				"width": 100.0, "height": 100.0,
				"rows": []any{[]any{"a", "b"}}, "column_widths": []any{1.0}},
			want: "column_widths has 1 entries",
		},
		{
			name: "colour out of range",
			args: map[string]any{"presentation_id": "deck", "page_object_id": "s", "x": 0.0, "y": 0.0,
				"width": 100.0, "height": 100.0,
				"rows": []any{[]any{"a"}}, "foreground_color": map[string]any{"red": 255.0}},
			want: "0 to 1",
		},
		{
			name: "unknown alignment",
			args: map[string]any{"presentation_id": "deck", "page_object_id": "s", "x": 0.0, "y": 0.0,
				"width": 100.0, "height": 100.0,
				"rows": []any{[]any{"a"}}, "column_alignments": []any{"MIDDLE"}},
			want: "START, CENTER, END or JUSTIFIED",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := h.registry.slidesCreateTableWithText(context.Background(), request(test.args))
			if message := requireError(t, result, err); !strings.Contains(message, test.want) {
				t.Errorf("expected a refusal mentioning %q, got %q", test.want, message)
			}
		})
	}
}

// TestCopyTitleStyle pins the mask: the source's own fields plus the ones being reset.
// TestInspectTitleStyleCarriesTheFontWeight keeps the pair honest at its most awkward
// point: the weight of a font is a field of its own, and a reading that reports the
// family without it lets a caller write a heading back regular.
func TestInspectTitleStyleCarriesTheFontWeight(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	answer := h.ok(h.registry.slidesInspectTitleStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "title1",
	})))

	if !strings.Contains(answer, "weightedFontFamily") {
		t.Errorf("the weight has to be reported with the font, got %s", answer)
	}
}

// TestResetTitleIndent pins putting a shifted title back in line.
//
// The dedicated tool for this is gone: it did what set_paragraph_style does with a scope
// of "title" and zero indents, and two tools for one job is how a caller ends up guessing
// which one is meant. The batch it sends is the same one, so the golden file stays.
func TestResetTitleIndent(t *testing.T) {
	fake := newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody)
	h := newHarness(t, fake)

	result, err := h.registry.slidesSetParagraphStyle(context.Background(), request(map[string]any{
		"presentation_id":       "deck",
		"object_id":             "title1",
		"scope":                 scopeTitle,
		"alignment":             "START",
		"indent_start_emu":      float64(0),
		"indent_first_line_emu": float64(0),
	}))

	requireOK(t, result, err)
	checkGolden(t, "reset_title_indent.json", h.bodyOf(t, 1))
}

func TestInspectTextStructure(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	answer := h.ok(h.registry.slidesInspectTextStructure(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
	})))

	for _, want := range []string{`"has_bullet": true`, `"nesting_level": 0`, "Старый текст"} {
		if !strings.Contains(answer, want) {
			t.Errorf("the structure should report %s, got %s", want, answer)
		}
	}
}

func TestInspectTitleStyle(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	answer := h.ok(h.registry.slidesInspectTitleStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "title1",
	})))

	for _, want := range []string{"Roboto", `"bold": true`, "fontFamily,fontSize"} {
		if !strings.Contains(answer, want) {
			t.Errorf("the style should report %s, got %s", want, answer)
		}
	}
}

func TestInspectWrongKindOfObject(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	result, err := h.registry.slidesInspectTextStructure(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "table1",
	}))

	// A table is no longer the wrong kind of object, only an address without its cell, and
	// the refusal has to say which half is missing rather than "not a text box".
	if message := requireError(t, result, err); !strings.Contains(message, "name row and column") {
		t.Errorf("expected the refusal to say how to reach a cell, got %q", message)
	}
}

func TestInspectMissingObject(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	result, err := h.registry.slidesInspectTextStructure(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "nothing",
	}))

	if message := requireError(t, result, err); !strings.Contains(message, "no object nothing") {
		t.Errorf("expected the refusal to name the object, got %q", message)
	}
}

// TestSlidesCreate pins the request that makes an empty deck. The title is the whole body:
// a presentation cannot be created with a theme, a page size or anything else, and a caller
// that expects otherwise gets an empty deck and no error to say so.
func TestSlidesCreate(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations", `{"presentationId": "new_deck", "title": "DevOPS Demo Template"}`))

	answer := h.ok(h.registry.slidesCreate(context.Background(), request(map[string]any{
		"title": "DevOPS Demo Template",
	})))

	if !strings.Contains(answer, "new_deck") {
		t.Errorf("the new deck's identifier should come back, got %s", answer)
	}

	sent := h.google.requests[0]
	if sent.Method != http.MethodPost || sent.Path != "/v1/presentations" {
		t.Errorf("a deck is made by POST /v1/presentations, got %s %s", sent.Method, sent.Path)
	}
	if !strings.Contains(string(sent.Body), `"title":"DevOPS Demo Template"`) {
		t.Errorf("the title should be the body of the request, got %s", sent.Body)
	}
}

// TestSlidesCreateNeedsTitle keeps the refusal on this side of the API: without a name
// Google makes an untitled deck, and an untitled deck on someone's Drive is litter.
func TestSlidesCreateNeedsTitle(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	result, err := h.registry.slidesCreate(context.Background(), request(map[string]any{}))
	if message := requireError(t, result, err); !strings.Contains(message, "title") {
		t.Errorf("expected the refusal to name the missing title, got %q", message)
	}
}

// TestSlidesCreateReportsRefusal keeps Google's own refusal visible. Creating a file is
// where a quota or a policy on the account shows up, and a caller told only "failed" would
// go looking for the fault in the deck it has not made yet.
func TestSlidesCreateReportsRefusal(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		fail("/presentations", http.StatusForbidden, `{"error": {"message": "quota exceeded"}}`))

	message := h.fail(h.registry.slidesCreate(context.Background(), request(map[string]any{
		"title": "DevOPS Demo Template",
	})))

	if !strings.Contains(message, "quota exceeded") {
		t.Errorf("Google's own words should reach the caller, got %q", message)
	}
}

func TestSlidesList(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	answer := h.ok(h.registry.slidesList(context.Background(), request(map[string]any{
		"presentation_id": "deck",
	})))

	for _, want := range []string{`"object_id": "slide1"`, `"placeholder": "TITLE"`, `"kind": "table"`, "page_size_emu"} {
		if !strings.Contains(answer, want) {
			t.Errorf("the map of the deck should report %s, got %s", want, answer)
		}
	}
}

func TestSlidesListLayouts(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	answer := h.ok(h.registry.slidesListLayouts(context.Background(), request(map[string]any{
		"presentation_id": "deck",
	})))

	if !strings.Contains(answer, "layout_body") {
		t.Errorf("the layouts should be listed, got %s", answer)
	}
}

func TestAddSlide(t *testing.T) {
	fake := newFakeGoogle(t).answer("/presentations/deck:batchUpdate",
		`{"presentationId": "deck", "replies": [{"createSlide": {"objectId": "slide_new"}}]}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.slidesAddSlide(context.Background(), request(map[string]any{
		"presentation_id":  "deck",
		"layout_object_id": "layout_body",
		"insertion_index":  float64(2),
	})))

	if !strings.Contains(answer, "slide_new") {
		t.Errorf("the new slide's identifier should come back, got %s", answer)
	}

	checkGolden(t, "add_slide.json", h.bodyOf(t, 0))
}

func TestAddSlideNeedsALayout(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	result, err := h.registry.slidesAddSlide(context.Background(), request(map[string]any{
		"presentation_id": "deck",
	}))

	if message := requireError(t, result, err); !strings.Contains(message, "name a layout") {
		t.Errorf("expected a refusal asking for a layout, got %q", message)
	}

	result, err = h.registry.slidesAddSlide(context.Background(), request(map[string]any{
		"presentation_id":   "deck",
		"layout_object_id":  "layout_body",
		"predefined_layout": "TITLE_AND_BODY",
	}))

	if message := requireError(t, result, err); !strings.Contains(message, "alternatives") {
		t.Errorf("expected a refusal about naming both, got %q", message)
	}
}

func TestSetText(t *testing.T) {
	fake := newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody)
	h := newHarness(t, fake)

	h.ok(h.registry.slidesSetText(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "title1",
		"text":            "Итоги квартала",
	})))

	// Request 0 reads what is in the box, request 1 is the batch that replaces it.
	checkGolden(t, "set_text.json", h.bodyOf(t, 1))
}

func TestSetTextEmptyOnlyDeletes(t *testing.T) {
	// An empty text means "clear the box": bullets off, text out, and nothing inserted,
	// because inserting an empty string is an error in the API.
	requests := setTextRequests("title1", "", true, nil)

	if len(requests) != 2 {
		t.Fatalf("clearing a box should be two removals, got %d", len(requests))
	}
	if requests[0].DeleteParagraphBullets == nil {
		t.Error("the list formatting has to come off, or the next text arrives as list items")
	}
	if requests[1].DeleteText == nil {
		t.Error("the text has to go")
	}
}

// TestSetTextIntoEmptyPlaceholder is the case a deck built from layouts hits on every
// slide: the placeholder is empty, and Slides refuses a deleteText over an empty range
// with "startIndex 0 must be less than the endIndex 0".
func TestSetTextIntoEmptyPlaceholder(t *testing.T) {
	requests := setTextRequests("title1", "Итоги квартала", false, nil)

	if len(requests) != 1 || requests[0].InsertText == nil {
		t.Fatalf("filling an empty box should be one insert, got %+v", requests)
	}
}

func TestSetTextThroughTheTool(t *testing.T) {
	// The same case end to end: the deck reports an empty placeholder, so the batch that
	// goes out carries no delete at all.
	empty := `{"presentationId": "deck", "slides": [{"objectId": "slide1", "pageElements": [
	    {"objectId": "title1", "shape": {"shapeType": "TEXT_BOX", "text": {"textElements": []}}}]}]}`

	fake := newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", empty)
	h := newHarness(t, fake)

	h.ok(h.registry.slidesSetText(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "title1",
		"text":            "Итоги квартала",
	})))

	checkGolden(t, "set_text_into_empty.json", h.bodyOf(t, 1))
}

func TestReplaceBodyIntoEmptyPlaceholder(t *testing.T) {
	// Same for the body: a slide just added from a layout has an empty BODY, and the two
	// removals would each be refused.
	empty := `{"presentationId": "deck", "slides": [{"objectId": "slide1", "pageElements": [
	    {"objectId": "body1", "shape": {"shapeType": "TEXT_BOX", "text": {"textElements": []}}}]}]}`

	fake := newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", empty)
	h := newHarness(t, fake)

	h.ok(h.registry.slidesSetList(context.Background(), request(map[string]any{
		"presentation_id":  "deck",
		"object_id":        "body1",
		"plain_first_line": true,
		"items": []any{
			map[string]any{"text": "Что сделали", "level": float64(0)},
			map[string]any{"text": "Инфраструктура", "level": float64(0)},
			map[string]any{"text": "Переехали", "level": float64(1)},
		},
	})))

	checkGolden(t, "replace_body_into_empty.json", h.bodyOf(t, 1))
}

func TestExportThumbnail(t *testing.T) {
	fake := newFakeGoogle(t).answer("/thumbnail",
		`{"contentUrl": "https://example.invalid/thumb.png", "width": 1600, "height": 900}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.slidesExportThumbnail(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
	})))

	if !strings.Contains(answer, "thumb.png") {
		t.Errorf("the address of the picture should come back, got %s", answer)
	}

	query := h.google.requests[0].Query
	for _, want := range []string{"thumbnailProperties.mimeType=PNG", "thumbnailProperties.thumbnailSize=LARGE"} {
		if !strings.Contains(query, want) {
			t.Errorf("the request should carry %s, got %s", want, query)
		}
	}
}

func TestExportThumbnailRefusesUnknownFormat(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	result, err := h.registry.slidesExportThumbnail(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"mime_type":       "WEBP",
	}))

	if message := requireError(t, result, err); !strings.Contains(message, "PNG or JPEG") {
		t.Errorf("expected a refusal naming the formats, got %q", message)
	}
}

func TestCopyPresentation(t *testing.T) {
	fake := newFakeGoogle(t).answer("/files/template/copy",
		`{"id": "new_deck", "name": "Отчёт за квартал", "webViewLink": "https://example.invalid/d/new_deck"}`)
	h := newHarness(t, fake)

	answer := h.ok(h.registry.slidesCopyPresentation(context.Background(), request(map[string]any{
		"template_id":      "template",
		"name":             "Отчёт за квартал",
		"parent_folder_id": "folder1",
	})))

	if !strings.Contains(answer, "new_deck") {
		t.Errorf("the copy's identifier should come back, got %s", answer)
	}

	checkGolden(t, "copy_presentation.json", h.bodyOf(t, 0))

	if query := h.google.requests[0].Query; !strings.Contains(query, "supportsAllDrives=true") {
		t.Errorf("a template usually lives on a shared drive, so the copy has to say so: %s", query)
	}
}

// TestListTextTabs is the detail the whole approach rests on: depth is a tab, and Slides
// works the indents out from it.
func TestListTextTabs(t *testing.T) {
	text := listText([]listItem{
		{Text: "Заголовок"},
		{Text: "Секция"},
		{Text: "Пункт", Level: 1},
		{Text: "Ещё глубже", Level: 2},
	})

	if text != "Заголовок\nСекция\n\tПункт\n\t\tЕщё глубже" {
		t.Errorf("the list text is %q", text)
	}
}

func TestNestedListRequestsRefusesAHeadingOnly(t *testing.T) {
	if _, err := nestedListRequests("body1", "Только заголовок", "", true, true, nil); err == nil {
		t.Error("a heading with no list under it should be refused")
	}

	// Without a plain heading the same one line is a perfectly good one-item list.
	if _, err := nestedListRequests("body1", "Одна строка", "", true, false, nil); err != nil {
		t.Errorf("a single bulleted line is a list: %v", err)
	}
}
