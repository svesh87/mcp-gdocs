package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// presentationWithLevels is a deck whose body is a nested list with a different size at
// each level, which is what a deck people have edited actually looks like.
const presentationWithLevels = `{
  "presentationId": "deck",
  "title": "Отчёт",
  "pageSize": {"width": {"magnitude": 9144000, "unit": "EMU"}, "height": {"magnitude": 5143500, "unit": "EMU"}},
  "slides": [
    {
      "objectId": "slide1",
      "slideProperties": {"layoutObjectId": "layout_body"},
      "pageElements": [
        {
          "objectId": "body1",
          "size": {"width": {"magnitude": 8000000, "unit": "EMU"}, "height": {"magnitude": 3000000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 500000, "translateY": 1500000, "unit": "EMU"},
          "shape": {
            "shapeType": "TEXT_BOX",
            "placeholder": {"type": "BODY", "parentObjectId": "layout_body_ph"},
            "text": {"textElements": [
              {"paragraphMarker": {"style": {"spaceAbove": {"magnitude": 10, "unit": "PT"},
                 "spaceBelow": {"unit": "PT"}, "spacingMode": "NEVER_COLLAPSE"}},
               "startIndex": 0, "endIndex": 12},
              {"textRun": {"content": "Что сделали\n", "style": {
                 "fontSize": {"magnitude": 20, "unit": "PT"}, "bold": true}}},
              {"paragraphMarker": {"bullet": {"nestingLevel": 0, "glyph": "●", "bulletStyle": {
                 "fontSize": {"magnitude": 14, "unit": "PT"},
                 "foregroundColor": {"opaqueColor": {"rgbColor": {"red": 0.6, "green": 0.1, "blue": 0.1}}}}}},
               "startIndex": 12, "endIndex": 27},
              {"textRun": {"content": "Инфраструктура\n", "style": {
                 "fontSize": {"magnitude": 14, "unit": "PT"}, "bold": true,
                 "link": {"url": "https://example.invalid/infra"}}}},
              {"paragraphMarker": {"bullet": {"nestingLevel": 1}}, "startIndex": 27, "endIndex": 36},
              {"textRun": {"content": "Кластер\n", "style": {"fontSize": {"magnitude": 11, "unit": "PT"}}}}
            ]}
          }
        }
      ]
    }
  ],
  "layouts": [
    {
      "objectId": "layout_body",
      "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Заголовок и текст"},
      "pageElements": [
        {
          "objectId": "layout_body_ph",
          "shape": {"placeholder": {"type": "BODY", "parentObjectId": "master_body"},
            "text": {"textElements": [{"textRun": {"content": "Текст\n", "style": {
               "fontFamily": "Rubik", "fontSize": {"magnitude": 18, "unit": "PT"}}}}]}}
        }
      ]
    }
  ],
  "masters": [
    {
      "objectId": "master1",
      "pageElements": [
        {"objectId": "master_body", "shape": {"placeholder": {"type": "BODY"},
          "text": {"textElements": [{"textRun": {"content": "Текст\n", "style": {
             "foregroundColor": {"opaqueColor": {"rgbColor": {"red": 0.2, "green": 0.2, "blue": 0.2}}}}}}]}}}
      ]
    }
  ]
}`

func TestSetTextStyle(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLevels))

	h.ok(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id":  "deck",
		"object_id":        "body1",
		"scope":            scopeAll,
		"font_size":        float64(14),
		"font_family":      "Rubik",
		"bold":             true,
		"foreground_color": map[string]any{"red": 0.8, "green": 0.0, "blue": 0.0},
		"alignment":        "START",
	})))

	checkGolden(t, "set_text_style.json", h.bodyOf(t, 0))
}

// TestSetTextStyleByLevel pins styling one level of a list, which is how a body slide
// with three sizes gets reproduced without flattening it to one.
func TestSetTextStyleByLevel(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLevels))

	h.ok(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"scope":           "1",
		"font_size":       float64(11),
	})))

	// Request 0 finds where the level sits, request 1 styles exactly that range.
	checkGolden(t, "set_text_style_level.json", h.bodyOf(t, 1))
}

// TestSetTextStyleTakesAThemeColour pins the case a real sample turned out to be: the
// author picked the colour from the theme's row, so the slide carries a name and not a
// value. Copying it as a literal colour would stop it following the theme.
func TestSetTextStyleTakesAThemeColour(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLevels))

	h.ok(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"scope":           scopeAll,
		"italic":          true,
		"theme_color":     "light1",
	})))

	checkGolden(t, "set_text_style_theme_colour.json", h.bodyOf(t, 0))
}

// TestSetTextStyleByParagraph pins styling one paragraph of a box. A real slide puts a
// bold heading, a plain body and a grey footnote in one text box, and none of them is a
// list item — so a nesting level cannot tell them apart and each has to be addressed.
func TestSetTextStyleByParagraph(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLevels))

	h.ok(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"scope":           "paragraph:2",
		"italic":          true,
	})))

	checkGolden(t, "set_text_style_paragraph.json", h.bodyOf(t, 1))

	if message := h.fail(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"scope":           "paragraph:9",
		"italic":          true,
	}))); !strings.Contains(message, "has 3 paragraphs") {
		t.Errorf("a paragraph past the end should be refused by count, got %q", message)
	}
}

func TestSetTextStyleRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLevels))

	if message := h.fail(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
	}))); !strings.Contains(message, "nothing to set") {
		t.Errorf("a call that changes nothing should be refused, got %q", message)
	}

	if message := h.fail(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"font_size":       float64(14),
		"scope":           "деепричастие",
	}))); !strings.Contains(message, "scope") {
		t.Errorf("a scope that is not one should be refused, got %q", message)
	}

	if message := h.fail(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"font_size":       float64(14),
		"scope":           "7",
	}))); !strings.Contains(message, "no paragraphs at level 7") {
		t.Errorf("a level the box does not have should be refused, got %q", message)
	}
}

// TestResetTextStyle pins giving text back to the layout, which is the only way to find
// out what a template would have done with it.
func TestResetTextStyle(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLevels))

	h.ok(h.registry.slidesResetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"scope":           scopeAll,
		"fields":          []any{"fontSize", "foregroundColor"},
	})))

	checkGolden(t, "reset_text_style.json", h.bodyOf(t, 0))
}

// TestResetTextStyleOverARange is what a rebuilt bullet needs: the sample reads "Итог:"
// in bold and the rest in whatever the layout says, and only the rest may be given back.
// Clearing the whole box would take the heading's bold with it.
func TestResetTextStyleOverARange(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLevels))

	h.ok(h.registry.slidesResetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"scope":           scopeRange,
		"start_index":     float64(390),
		"end_index":       float64(482),
		"fields":          []any{"bold"},
	})))

	body := string(h.bodyOf(t, 0))
	for _, want := range []string{`"startIndex": 390`, `"endIndex": 482`, `"type": "FIXED_RANGE"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should clear only the stretch, %s missing from %s", want, body)
		}
	}

	if message := h.fail(h.registry.slidesResetTextStyle(context.Background(),
		request(map[string]any{
			"presentation_id": "deck",
			"object_id":       "body1",
			"scope":           scopeRange,
		}))); !strings.Contains(message, "start_index") {
		t.Errorf("a range without bounds should be refused, got %q", message)
	}
}

// TestStyleReadingAndWritingAgree is the pair the server rests on now that there is no
// copying tool: what the reading reports about a stretch of text is exactly what the
// writing takes back, in the same units. Whoever is in between — an agent building a deck
// from a dozen samples — decides what to keep and what to change.
func TestStyleReadingAndWritingAgree(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithMixedRuns))

	// Read: the line is bold up to the colon and plain after it.
	reading := h.ok(h.registry.slidesInspectTextStructure(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "mixed1",
	})))

	var answer struct {
		Paragraphs []struct {
			Runs []struct {
				StartIndex int64  `json:"start_index"`
				EndIndex   int64  `json:"end_index"`
				Bold       *bool  `json:"bold"`
				Color      string `json:"text_color"`
			} `json:"runs"`
		} `json:"paragraphs"`
	}
	if err := json.Unmarshal([]byte(reading), &answer); err != nil {
		t.Fatalf("the reading is not JSON: %v", err)
	}
	if len(answer.Paragraphs) == 0 || len(answer.Paragraphs[0].Runs) != 2 {
		t.Fatalf("the mixed line should come back as two runs, got %s", reading)
	}

	// Write: the same numbers go straight back in, on another box.
	first := answer.Paragraphs[0].Runs[0]
	h.ok(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "mixed2",
		"scope":           scopeRange,
		"start_index":     float64(first.StartIndex),
		"end_index":       float64(first.EndIndex),
		"bold":            first.Bold != nil && *first.Bold,
		"foreground_color": map[string]any{
			"red": 0.1, "green": 0.5, "blue": 0.2,
		},
	})))

	checkGolden(t, "style_range_from_reading.json", h.bodyOf(t, len(h.google.requests)-1))
}

func TestSetTextStyleRangeRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithMixedRuns))

	if message := h.fail(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "mixed2",
		"scope":           scopeRange,
		"bold":            true,
	}))); !strings.Contains(message, "start_index and end_index") {
		t.Errorf("a range with no bounds should be refused, got %q", message)
	}

	if message := h.fail(h.registry.slidesSetTextStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "mixed2",
		"scope":           scopeRange,
		"start_index":     float64(20),
		"end_index":       float64(5),
		"bold":            true,
	}))); !strings.Contains(message, "backwards") {
		t.Errorf("a backwards range should be refused, got %q", message)
	}
}

// TestInspectTextStructureReportsRuns keeps mixed styling visible: reported per paragraph
// only, a half-bold line comes back as one style and is rebuilt uniformly bold.
// TestInspectTextStructureReportsAnExplicitZero is the difference a real deck turned on.
// A heading that sets space below to zero sits tight against the line under it; one that
// sets nothing inherits the master's twelve points. Slides stores the zero as a dimension
// with no magnitude, and reporting it as absent makes the two indistinguishable — the copy
// then runs twelve points long for every paragraph after it.
func TestInspectTextStructureReportsAnExplicitZero(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithLevels))

	answer := h.ok(h.registry.slidesInspectTextStructure(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
	})))

	for _, want := range []string{`"space_below_pt": 0`, `"space_above_pt": 10`,
		`"spacing_mode": "NEVER_COLLAPSE"`,
		// The marker's own size is not the text's, and a bigger marker makes a taller
		// line: a copy whose markers took the text's size runs short on every paragraph.
		`"bullet_size_pt": 14`} {
		if !strings.Contains(answer, want) {
			t.Errorf("the reading should carry %s, got %s", want, answer)
		}
	}
}

func TestInspectTextStructureReportsRuns(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithMixedRuns))

	answer := h.ok(h.registry.slidesInspectTextStructure(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "mixed1",
	})))

	if !strings.Contains(answer, `"runs"`) || !strings.Contains(answer, `"Разбили на 3 смены:"`) {
		t.Errorf("the runs of a mixed line should be reported, got %s", answer)
	}
}

// presentationWithMixedRuns is the shape of a real slide: one paragraph, two styles.
const presentationWithMixedRuns = `{
  "presentationId": "deck",
  "slides": [
    {
      "objectId": "slide1",
      "pageElements": [
        {
          "objectId": "mixed1",
          "shape": {"shapeType": "TEXT_BOX", "text": {"textElements": [
            {"paragraphMarker": {"bullet": {"nestingLevel": 0}}, "startIndex": 0, "endIndex": 48},
            {"startIndex": 0, "endIndex": 20, "textRun": {"content": "Разбили на 3 смены:", "style": {
               "bold": true, "foregroundColor": {"opaqueColor": {"rgbColor": {"red": 0.1, "green": 0.5, "blue": 0.2}}}}}},
            {"startIndex": 20, "endIndex": 48, "textRun": {"content": " Ночь, День, Вечер\n", "style": {
               "foregroundColor": {"opaqueColor": {"rgbColor": {"red": 0.2, "green": 0.2, "blue": 0.2}}}}}}
          ]}}
        },
        {
          "objectId": "mixed2",
          "shape": {"shapeType": "TEXT_BOX", "text": {"textElements": [
            {"paragraphMarker": {"bullet": {"nestingLevel": 0}}, "startIndex": 0, "endIndex": 48},
            {"startIndex": 0, "endIndex": 48, "textRun": {"content": "Разбили на 3 смены: Ночь, День, Вечер\n"}}
          ]}}
        }
      ]
    }
  ]
}`

// TestCopyingIsItsOwnGroup keeps the line the copy groups draw where it is.
//
// Copying between documents was once refused outright, and the reason was about learning
// rather than about the work: a deck that matched its sample by being copied proved nothing
// about whether the server could build one. That is settled, and the tools exist — but they
// are the class of thing an operator may want to switch off on its own, so every tool that
// carries content in from another document lands in a *-copy group and nowhere else.
//
// The two exceptions are named rather than derived. Copying a file is Drive's ordinary
// writing and is how every deck starts, from a copy of a template; putting it behind the
// copy switch would break the main way this server is used.
func TestCopyingIsItsOwnGroup(t *testing.T) {
	wholeFiles := map[string]bool{
		"gdocs_drive_copy":               true,
		"gdocs_slides_copy_presentation": true,
	}

	found := 0
	for _, name := range registeredTools(t, true) {
		if !strings.Contains(name, "copy_") {
			continue
		}

		group, err := GroupOf(name)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}

		if wholeFiles[name] {
			if !strings.HasSuffix(string(group), "-write") {
				t.Errorf("%s copies a whole file and belongs in a write group, not %s", name, group)
			}
			continue
		}

		found++
		if !strings.HasSuffix(string(group), "-copy") {
			t.Errorf("%s carries content in from another document and belongs in a copy group, not %s",
				name, group)
		}
	}

	if found == 0 {
		t.Error("no copying tools are registered at all, which is not what this server offers")
	}
}

// TestCopyingIsNotInAFamilyShorthand: naming a family asks for reading and writing it.
// Carrying content in from somewhere else is a separate decision, and an operator who
// spelled out --tools=slides did not make it.
func TestCopyingIsNotInAFamilyShorthand(t *testing.T) {
	enabled, err := ParseGroups("slides")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if enabled[SlidesCopy] {
		t.Error("--tools=slides should not switch copying on by itself")
	}
	if !enabled[SlidesWrite] {
		t.Error("--tools=slides should still switch writing on")
	}

	// A configuration that says nothing gets copying, because the work needs it.
	if !defaultGroups()[SlidesCopy] {
		t.Error("the default set should offer copying")
	}
}

func TestLinkText(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLevels))

	h.ok(h.registry.slidesLinkText(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"links": []any{
			map[string]any{"text": "Кластер", "url": "https://example.invalid/cluster"},
		},
	})))

	checkGolden(t, "link_text.json", h.bodyOf(t, 1))
}

// TestLinkTextRefusesTextThatIsNotThere keeps a link from landing on the wrong words: the
// range is measured against the text, so a phrase that is not in the box has no range.
func TestLinkTextRefusesTextThatIsNotThere(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLevels))

	message := h.fail(h.registry.slidesLinkText(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"links": []any{
			map[string]any{"text": "такого текста нет", "url": "https://example.invalid/x"},
		},
	})))

	if !strings.Contains(message, "такого текста нет") {
		t.Errorf("the refusal should quote the text it could not find, got %q", message)
	}
}

// TestInspectTitleStyleReportsTheEffectiveSize is the answer to "how big is this text
// really": the slide sets 20 pt, the layout would have said 18, the master supplies the
// colour, and what a person sees is the merge of the three.
func TestInspectTitleStyleReportsTheEffectiveSize(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithLevels))

	answer := h.ok(h.registry.slidesInspectTitleStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
	})))

	for _, want := range []string{
		`"effective_font_size_pt": 20`,
		`"fontSize": "text"`,
		// The colour is nowhere on the slide: it comes from the master, three levels up.
		`"foregroundColor": "master"`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the effective style should carry %s, got %s", want, answer)
		}
	}
}

// TestInspectTextStructureReportsSpacing keeps the paragraph spacing in the same reading
// as the text: it used to need a tool of its own, and two tools for one box is how a
// caller ends up reading half of what it needs.
func TestInspectTextStructureReportsSpacing(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithSpacing))

	answer := h.ok(h.registry.slidesInspectTextStructure(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
	})))

	for _, want := range []string{
		`"line_spacing": 115`,
		`"space_above_pt": 6`,
		`"indent_start_emu": 457200`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the structure reading should carry %s, got %s", want, answer)
		}
	}
}

// presentationWithSpacing is a body box whose paragraphs carry spacing and indents.
const presentationWithSpacing = `{
  "presentationId": "deck",
  "slides": [
    {
      "objectId": "slide1",
      "pageElements": [
        {
          "objectId": "body1",
          "shape": {"shapeType": "TEXT_BOX", "text": {"textElements": [
            {"paragraphMarker": {"style": {"alignment": "START", "lineSpacing": 115,
               "spaceAbove": {"magnitude": 6, "unit": "PT"},
               "indentStart": {"magnitude": 457200, "unit": "EMU"}}}, "startIndex": 0, "endIndex": 6},
            {"textRun": {"content": "Абзац\n"}}
          ]}}
        }
      ]
    }
  ]
}`
