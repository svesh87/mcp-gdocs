package tools

import (
	"context"
	"strings"
	"testing"
)

// presentationWithLook is a deck whose slide carries the things a plain listing does not
// report: a background picture, a filled and outlined shape, a rotated label, a group, an
// empty placeholder and a page of speaker notes.
const presentationWithLook = `{
  "presentationId": "deck",
  "title": "Отчёт",
  "pageSize": {"width": {"magnitude": 9144000, "unit": "EMU"}, "height": {"magnitude": 5143500, "unit": "EMU"}},
  "slides": [
    {
      "objectId": "slide1",
      "pageProperties": {"pageBackgroundFill": {
        "propertyState": "RENDERED",
        "stretchedPictureFill": {"contentUrl": "https://example.invalid/bg.png"}
      }},
      "slideProperties": {
        "layoutObjectId": "layout_body",
        "notesPage": {
          "notesProperties": {"speakerNotesObjectId": "notes1"},
          "pageElements": [
            {"objectId": "notes_thumb", "shape": {"text": {"textElements": [{"textRun": {"content": "не эти\n"}}]}}},
            {"objectId": "notes1", "shape": {"text": {"textElements": [{"textRun": {"content": "Сказать про квоту\n"}}]}}}
          ]
        }
      },
      "pageElements": [
        {
          "objectId": "panel1",
          "size": {"width": {"magnitude": 4000000, "unit": "EMU"}, "height": {"magnitude": 1000000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 400000, "translateY": 200000, "unit": "EMU"},
          "shape": {
            "shapeType": "ROUND_RECTANGLE",
            "shapeProperties": {
              "shapeBackgroundFill": {"propertyState": "RENDERED", "solidFill": {
                 "color": {"rgbColor": {"red": 1, "green": 1, "blue": 1}}, "alpha": 1}},
              "outline": {"propertyState": "RENDERED", "weight": {"magnitude": 12700, "unit": "EMU"},
                 "dashStyle": "SOLID",
                 "outlineFill": {"solidFill": {"color": {"rgbColor": {"red": 0.8, "green": 0, "blue": 0}}}}},
              "shadow": {"propertyState": "NOT_RENDERED"},
              "contentAlignment": "MIDDLE"
            },
            "text": {"textElements": [{"textRun": {"content": "Белым по тёмному\n"}}]}
          }
        },
        {
          "objectId": "over_panel",
          "size": {"width": {"magnitude": 3000000, "unit": "EMU"}, "height": {"magnitude": 800000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 500000, "translateY": 300000, "unit": "EMU"},
          "shape": {
            "shapeType": "TEXT_BOX",
            "shapeProperties": {
              "shapeBackgroundFill": {"solidFill": {"color": {"rgbColor": {}}, "alpha": 0}}
            },
            "text": {"textElements": [{"textRun": {"content": "Поверх плашки\n"}}]}
          }
        },
        {
          "objectId": "empty_body",
          "size": {"width": {"magnitude": 4000000, "unit": "EMU"}, "height": {"magnitude": 1000000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 0, "translateY": 0, "unit": "EMU"},
          "shape": {"shapeType": "TEXT_BOX", "placeholder": {"type": "BODY"}}
        },
        {
          "objectId": "tilted",
          "size": {"width": {"magnitude": 2000000, "unit": "EMU"}, "height": {"magnitude": 500000, "unit": "EMU"}},
          "transform": {"scaleX": 0.86602540378, "shearX": -0.5, "shearY": 0.5, "scaleY": 0.86602540378,
                        "translateX": 1000000, "translateY": 2000000, "unit": "EMU"},
          "shape": {"shapeType": "TEXT_BOX", "text": {"textElements": [{"textRun": {"content": "Наклон\n"}}]}}
        },
        {
          "objectId": "photo",
          "size": {"width": {"magnitude": 1000000, "unit": "EMU"}, "height": {"magnitude": 1000000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 5000000, "translateY": 1000000, "unit": "EMU"},
          "image": {"contentUrl": "https://example.invalid/photo.png",
            "imageProperties": {"transparency": 0.25,
              "cropProperties": {"leftOffset": 0.1, "rightOffset": 0.1}}}
        },
        {
          "objectId": "group1",
          "size": {"width": {"magnitude": 2000000, "unit": "EMU"}, "height": {"magnitude": 2000000, "unit": "EMU"}},
          "transform": {"scaleX": 2, "scaleY": 2, "translateX": 1000000, "translateY": 3000000, "unit": "EMU"},
          "elementGroup": {"children": [
            {
              "objectId": "in_group",
              "size": {"width": {"magnitude": 500000, "unit": "EMU"}, "height": {"magnitude": 500000, "unit": "EMU"}},
              "transform": {"scaleX": 1, "scaleY": 1, "translateX": 100000, "translateY": 50000, "unit": "EMU"},
              "shape": {"shapeType": "TEXT_BOX", "text": {"textElements": [{"textRun": {"content": "Внутри\n"}}]}}
            }
          ]}
        }
      ]
    }
  ],
  "layouts": [
    {"objectId": "layout_body", "layoutProperties": {"name": "TITLE_AND_BODY", "displayName": "Заголовок и текст"}}
  ]
}`

func TestInspectPageReportsWhatAListingDoesNot(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithLook))

	answer := h.ok(h.registry.slidesInspectPage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
	})))

	for _, want := range []string{
		// The background is the whole reason this tool exists: importing a theme does not
		// bring the picture an author put on one slide.
		`"picture_url": "https://example.invalid/bg.png"`,
		// White is reported rather than swallowed: white text on a dark panel is a decision.
		`"color": "#FFFFFF"`,
		`"content_alignment": "MIDDLE"`,
		// An unfilled placeholder is visible on screen as "Click to add text", over
		// whatever was placed on top of it.
		`"placeholder_empty": true`,
		`"rotation_deg": 30`,
		`"transparency": 0.25`,
		`"speaker_notes": "Сказать про квоту"`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the reading should carry %s, got %s", want, answer)
		}
	}

	// A shadow Slides reports as NOT_RENDERED is not a shadow; saying otherwise would have
	// a caller copying shadows onto everything.
	if strings.Contains(answer, `"has_shadow": true`) {
		t.Errorf("a shadow that is not rendered should not be reported, got %s", answer)
	}
}

// TestInspectPageReportsATransparentFill is the reading a real deck corrected. A text box
// sitting on a coloured panel carries a solid fill of black at alpha 0 — "no fill" written
// the long way. Reported as a colour with the alpha dropped, it reads as a black box, and
// a slide rebuilt from that reading paints one over the panel.
func TestInspectPageReportsATransparentFill(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithLook))

	answer := h.ok(h.registry.slidesInspectPage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
	})))

	for _, want := range []string{`"transparent": true`, `"alpha": 0`} {
		if !strings.Contains(answer, want) {
			t.Errorf("a fill at alpha 0 should say so, %s missing from %s", want, answer)
		}
	}
}

// TestInspectPageComposesGroupTransforms keeps a child of a group from being reported
// where it would sit if it stood on the slide by itself.
func TestInspectPageComposesGroupTransforms(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithLook))

	answer := h.ok(h.registry.slidesInspectPage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
	})))

	// The group sits at 1000000 and doubles its children, so a child at 100000 inside it
	// lands at 1000000 + 2×100000. Read raw, it would be reported at 100000.
	if !strings.Contains(answer, `"x_emu": 1200000`) {
		t.Errorf("a child of a group should be reported at its place on the slide, got %s", answer)
	}
}

func TestInspectPageRefusesAnUnknownSlide(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithLook))

	message := h.fail(h.registry.slidesInspectPage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "nope",
	})))

	if !strings.Contains(message, "no slide nope") {
		t.Errorf("the refusal should name the slide, got %q", message)
	}
}

func TestSetPageBackgroundPicture(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesSetPageBackground(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"picture_url":     "https://example.invalid/bg.png",
	})))

	checkGolden(t, "set_page_background_picture.json", h.bodyOf(t, 0))
}

func TestSetPageBackgroundColorAndInherit(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesSetPageBackground(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"color":           map[string]any{"red": 0.1, "green": 0.2, "blue": 0.3},
	})))
	checkGolden(t, "set_page_background_color.json", h.bodyOf(t, 0))

	h.ok(h.registry.slidesSetPageBackground(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"inherit":         true,
	})))
	checkGolden(t, "set_page_background_inherit.json", h.bodyOf(t, 1))
}

func TestSetPageBackgroundRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	for _, test := range []struct {
		title string
		args  map[string]any
		want  string
	}{
		{
			"nothing named",
			map[string]any{"presentation_id": "deck", "page_object_id": "slide1"},
			"name one of color, picture_url or inherit",
		},
		{
			"two at once",
			map[string]any{"presentation_id": "deck", "page_object_id": "slide1",
				"inherit": true, "picture_url": "https://example.invalid/x.png"},
			"alternatives",
		},
		{
			"a picture Google cannot fetch",
			map[string]any{"presentation_id": "deck", "page_object_id": "slide1", "picture_url": "file:///etc/passwd"},
			"http or https",
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			if message := h.fail(h.registry.slidesSetPageBackground(context.Background(),
				request(test.args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should say %q, got %q", test.want, message)
			}
		})
	}
}

func TestOrderElements(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesOrderElements(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{"photo", "panel1"},
		"operation":       "send_to_back",
	})))

	checkGolden(t, "order_elements.json", h.bodyOf(t, 0))

	message := h.fail(h.registry.slidesOrderElements(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{"photo"},
		"operation":       "UPWARDS",
	})))
	if !strings.Contains(message, "BRING_TO_FRONT") {
		t.Errorf("the refusal should list the operations, got %q", message)
	}
}

func TestGroupAndUngroup(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesGroup(context.Background(), request(map[string]any{
		"presentation_id":    "deck",
		"group_object_ids":   []any{"panel1", "photo"},
		"ungroup_object_ids": []any{"group1"},
	})))

	checkGolden(t, "group_objects.json", h.bodyOf(t, 0))

	if message := h.fail(h.registry.slidesGroup(context.Background(), request(map[string]any{
		"presentation_id":  "deck",
		"group_object_ids": []any{"panel1"},
	}))); !strings.Contains(message, "at least two") {
		t.Errorf("a group of one should be refused, got %q", message)
	}

	if message := h.fail(h.registry.slidesGroup(context.Background(), request(map[string]any{
		"presentation_id": "deck",
	}))); !strings.Contains(message, "group_object_ids") {
		t.Errorf("a call with nothing to do should be refused, got %q", message)
	}
}

// TestDuplicateMakesEveryCopyFromTheOriginal pins the one way to reproduce a shape whose
// look the API neither reports nor accepts — a corner radius among it. Every copy is made
// from the original rather than from the previous copy, so editing the first one later
// does not travel into the rest.
func TestDuplicateMakesEveryCopyFromTheOriginal(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate",
		`{"presentationId":"deck","replies":[{"duplicateObject":{"objectId":"panel_a"}},`+
			`{"duplicateObject":{"objectId":"panel_b"}},{"duplicateObject":{"objectId":"gen_3"}}]}`))

	result := h.ok(h.registry.slidesDuplicate(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "panel",
		"copies":          float64(3),
		"new_object_ids":  []any{"panel_a", "panel_b"},
	})))

	checkGolden(t, "duplicate_object.json", h.bodyOf(t, 0))

	// The identifiers come back because the next call has to address the copies, and the
	// third one is Google's own: naming fewer than were asked for is allowed.
	for _, want := range []string{"panel_a", "panel_b", "gen_3"} {
		if !strings.Contains(result, want) {
			t.Errorf("the reply should report the copy %s, got %s", want, result)
		}
	}
}

func TestDuplicateRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	if message := h.fail(h.registry.slidesDuplicate(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "panel",
		"copies":          float64(0),
	}))); !strings.Contains(message, "at least 1") {
		t.Errorf("asking for no copies should be refused, got %q", message)
	}

	// More names than copies is a caller who miscounted; inventing which ones to drop
	// would put the panel they meant to address somewhere they cannot find it.
	if message := h.fail(h.registry.slidesDuplicate(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "panel",
		"copies":          float64(1),
		"new_object_ids":  []any{"a", "b"},
	}))); !strings.Contains(message, "new_object_ids") {
		t.Errorf("more names than copies should be refused, got %q", message)
	}
}

// TestHideSlides pins hiding a slide rather than removing it. Authors keep slides that
// way — last period's numbers, a backup explanation — and a copy that shows them says
// more than the original does.
func TestHideSlides(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesHide(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{"slide1", "slide2"},
		"hidden":          true,
	})))

	checkGolden(t, "hide_slides.json", h.bodyOf(t, 0))
}

func TestHideSlidesRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	// Omitting the flag is not the same as passing false: one is a caller who forgot to
	// say what they want, the other unhides slides somebody hid on purpose.
	if message := h.fail(h.registry.slidesHide(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{"slide1"},
	}))); !strings.Contains(message, "hidden is required") {
		t.Errorf("a call without the flag should be refused, got %q", message)
	}

	if message := h.fail(h.registry.slidesHide(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{},
		"hidden":          true,
	}))); !strings.Contains(message, "object_ids") {
		t.Errorf("a call naming no slides should be refused, got %q", message)
	}
}

// TestInspectPageReportsHidden keeps a hidden slide from being reproduced as a visible
// one: it looks like any other slide in a listing.
func TestInspectPageReportsHidden(t *testing.T) {
	deck := strings.Replace(presentationWithLook,
		`"slideProperties": {`, `"slideProperties": {"isSkipped": true,`, 1)
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", deck))

	answer := h.ok(h.registry.slidesInspectPage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
	})))

	if !strings.Contains(answer, `"hidden": true`) {
		t.Errorf("a hidden slide should be reported as hidden, got %s", answer)
	}
}

// TestSetSpeakerNotes pins that existing notes are cleared before new ones go in, and
// that the text goes to the shape the notes page names rather than to the other one.
func TestSetSpeakerNotes(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLook))

	h.ok(h.registry.slidesSetSpeakerNotes(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"text":            "Новый текст заметок",
	})))

	checkGolden(t, "set_speaker_notes.json", h.bodyOf(t, 1))
}

func TestSetSpeakerNotesWithoutANotesPage(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody))

	message := h.fail(h.registry.slidesSetSpeakerNotes(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"text":            "…",
	})))

	if !strings.Contains(message, "no speaker notes shape") {
		t.Errorf("the refusal should say there is nowhere to write, got %q", message)
	}
}

// TestReference is the answer to "what values does this take". Every one of them is an
// enum of Google's that a caller would otherwise guess at, and a guess comes back as
// "invalid argument" naming nothing.
func TestReference(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	everything := h.ok(h.registry.reference(context.Background(), request(map[string]any{})))
	for _, want := range []string{"shapes", "bullets", "arrows", "theme_colors", "units"} {
		if !strings.Contains(everything, want) {
			t.Errorf("the reference should carry %s, got %s", want, everything[:200])
		}
	}

	// The note that this exists for: a panel copied by the sample's own shape_type comes
	// out with corners the API cannot change.
	shapes := h.ok(h.registry.reference(context.Background(), request(map[string]any{
		"topic":  "shapes",
		"family": "rectangles",
	})))
	if !strings.Contains(shapes, "FLOW_CHART_ALTERNATE_PROCESS") ||
		!strings.Contains(shapes, "adjustment value") {
		t.Errorf("the rectangles family should name the alternatives and why, got %s", shapes)
	}

	units := h.ok(h.registry.reference(context.Background(), request(map[string]any{"topic": "units"})))
	if !strings.Contains(units, "12700") {
		t.Errorf("the units topic should carry the points-to-EMU factor, got %s", units)
	}
}

func TestReferenceRefusesWhatItDoesNotHave(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t))

	if message := h.fail(h.registry.reference(context.Background(), request(map[string]any{
		"topic": "gradients",
	}))); !strings.Contains(message, "shapes") {
		t.Errorf("an unknown topic should be answered with the list of topics, got %q", message)
	}

	if message := h.fail(h.registry.reference(context.Background(), request(map[string]any{
		"topic":  "shapes",
		"family": "squiggles",
	}))); !strings.Contains(message, "flowchart") {
		t.Errorf("an unknown family should be answered with the families, got %q", message)
	}
}

func TestStyleShape(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesStyleShape(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "panel1",
		"fill_color":      map[string]any{"red": 0.85, "green": 0.1, "blue": 0.1},
		"fill_alpha":      0.9,
		// Written as floats because that is what arrives over JSON; an integer here would
		// be a test that exercises a shape the protocol never produces.
		"outline_color":      map[string]any{"red": float64(0), "green": float64(0), "blue": float64(0)},
		"outline_weight_emu": float64(12700),
		"outline_dash":       "DASH",
		"content_alignment":  "MIDDLE",
	})))

	checkGolden(t, "style_shape.json", h.bodyOf(t, 0))
}

// TestStyleShapePaintsFromThePalette is the difference between a deck that can be
// recoloured for a season and one that has to be repainted shape by shape: a fill that
// names a palette colour follows gdocs_slides_set_theme_colors, a literal one does not.
func TestStyleShapePaintsFromThePalette(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesStyleShape(context.Background(), request(map[string]any{
		"presentation_id":     "deck",
		"object_id":           "panel1",
		"fill_theme_color":    "accent2",
		"outline_theme_color": "ACCENT3",
	})))

	body := string(h.bodyOf(t, 0))
	for _, want := range []string{`"themeColor": "ACCENT2"`, `"themeColor": "ACCENT3"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}
	// A named colour carries no value of its own; sending both is how a fill stops
	// following the theme without anybody noticing.
	if strings.Contains(body, "rgbColor") {
		t.Errorf("a palette colour must not be sent as a value too: %s", body)
	}
}

// TestStyleShapeRefusesAColourOutsideThePalette keeps the mistake local. Google answers a
// wrong name with "Invalid requests[0]" and nothing about which field or which name.
func TestStyleShapeRefusesAColourOutsideThePalette(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	message := h.fail(h.registry.slidesStyleShape(context.Background(), request(map[string]any{
		"presentation_id":  "deck",
		"object_id":        "panel1",
		"fill_theme_color": "PUMPKIN",
	})))

	for _, want := range []string{"PUMPKIN", "ACCENT1", "LIGHT1"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal should name the mistake and the choices, got %q", message)
		}
	}

	if message := h.fail(h.registry.slidesStyleShape(context.Background(), request(map[string]any{
		"presentation_id":  "deck",
		"object_id":        "panel1",
		"fill_color":       map[string]any{"red": 1.0, "green": 0.0, "blue": 0.0},
		"fill_theme_color": "ACCENT1",
	}))); !strings.Contains(message, "alternatives") {
		t.Errorf("a value and a name together should be refused, got %q", message)
	}
}

// TestStyleShapeSetsAutofit pins what Slides actually accepts. A sample's title measuring
// 25 pt where it reports 28 has its text shrunk to fit, and that cannot be switched on:
// the API answers anything but NONE with "Autofit types other than NONE are not
// supported". Refusing here says so before the round trip, and names the way across.
func TestStyleShapeSetsAutofit(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesStyleShape(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "title1",
		"autofit":         "none",
	})))

	body := string(h.bodyOf(t, 0))
	for _, want := range []string{`"autofitType": "NONE"`, "autofit.autofitType"} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}
	// The scale is Slides' own answer to "does this fit"; sending it is rejected outright.
	if strings.Contains(body, "fontScale") {
		t.Errorf("the scale is read-only and must not be sent: %s", body)
	}

	if message := h.fail(h.registry.slidesStyleShape(context.Background(),
		request(map[string]any{
			"presentation_id": "deck",
			"object_id":       "title1",
			"autofit":         "SHRINK_ON_OVERFLOW",
		}))); !strings.Contains(message, "font_scale") {
		t.Errorf("the refusal should name the way across, got %q", message)
	}
}

func TestStyleShapeRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	for _, test := range []struct {
		title string
		args  map[string]any
		want  string
	}{
		{
			"nothing to change",
			map[string]any{"presentation_id": "deck", "object_id": "panel1"},
			"nothing to change",
		},
		{
			"two fills",
			map[string]any{"presentation_id": "deck", "object_id": "panel1",
				"no_fill": true, "inherit_fill": true},
			"alternatives",
		},
		{
			"an outline that is both there and not",
			map[string]any{"presentation_id": "deck", "object_id": "panel1",
				"no_outline": true, "outline_weight_emu": float64(12700)},
			"cannot be combined",
		},
		{
			"a dash Slides does not draw",
			map[string]any{"presentation_id": "deck", "object_id": "panel1", "outline_dash": "WAVY"},
			"LONG_DASH_DOT",
		},
		{
			"alignment that is not one",
			map[string]any{"presentation_id": "deck", "object_id": "panel1", "content_alignment": "CENTRE"},
			"TOP, MIDDLE or BOTTOM",
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			if message := h.fail(h.registry.slidesStyleShape(context.Background(),
				request(test.args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should say %q, got %q", test.want, message)
			}
		})
	}
}

func TestStyleImage(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesStyleImage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "photo",
		"crop":            map[string]any{"left": 0.1, "right": 0.1, "top": 0.2, "bottom": 0.0},
		"transparency":    0.25,
	})))

	checkGolden(t, "style_image.json", h.bodyOf(t, 0))

	if message := h.fail(h.registry.slidesStyleImage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "photo",
		"crop":            map[string]any{"left": 1.5},
	}))); !strings.Contains(message, "fractions of the picture") {
		t.Errorf("a crop beyond the picture should be refused, got %q", message)
	}

	if message := h.fail(h.registry.slidesStyleImage(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "photo",
	}))); !strings.Contains(message, "nothing to change") {
		t.Errorf("a call with nothing to change should be refused, got %q", message)
	}
}

func TestCreateLine(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesCreateLine(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"x":               float64(1000000),
		"y":               float64(2000000),
		"width":           float64(3000000),
		"height":          float64(0),
		"color":           map[string]any{"red": 0.2, "green": 0.2, "blue": 0.2},
		"weight_emu":      float64(25400),
		"end_arrow":       "FILL_ARROW",
	})))

	checkGolden(t, "create_line.json", h.bodyOf(t, 0))
}

func TestCreateLineRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	for _, test := range []struct {
		title string
		args  map[string]any
		want  string
	}{
		{
			"a line of no length",
			map[string]any{"presentation_id": "deck", "page_object_id": "slide1",
				"x": float64(0), "y": float64(0), "width": float64(0), "height": float64(0)},
			"no length",
		},
		{
			"a category that is not one",
			map[string]any{"presentation_id": "deck", "page_object_id": "slide1",
				"x": float64(0), "y": float64(0), "width": float64(100), "height": float64(0),
				"category": "ZIGZAG"},
			"STRAIGHT, BENT or CURVED",
		},
		{
			"an arrowhead Slides does not draw",
			map[string]any{"presentation_id": "deck", "page_object_id": "slide1",
				"x": float64(0), "y": float64(0), "width": float64(100), "height": float64(0),
				"end_arrow": "HARPOON"},
			"OPEN_DIAMOND",
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			if message := h.fail(h.registry.slidesCreateLine(context.Background(),
				request(test.args))); !strings.Contains(message, test.want) {
				t.Errorf("the refusal should say %q, got %q", test.want, message)
			}
		})
	}
}

// presentationWithLayoutGrid is the smallest deck that has a title on a layout: the grid a
// template imposes lives there, not on any slide.
const presentationWithLayoutGrid = `{
  "presentationId": "deck",
  "pageSize": {"width": {"magnitude": 9144000, "unit": "EMU"}, "height": {"magnitude": 5143500, "unit": "EMU"}},
  "slides": [
    {
      "objectId": "slide1",
      "pageElements": [
        {
          "objectId": "body1",
          "size": {"width": {"magnitude": 4000000, "unit": "EMU"}, "height": {"magnitude": 1000000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 311700, "translateY": 1152475, "unit": "EMU"},
          "shape": {"shapeType": "TEXT_BOX"}
        }
      ]
    }
  ],
  "layouts": [
    {
      "objectId": "layout_body",
      "pageElements": [
        {
          "objectId": "layout_title",
          "size": {"width": {"magnitude": 8520600, "unit": "EMU"}, "height": {"magnitude": 572700, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 311700, "translateY": 445025, "unit": "EMU"},
          "shape": {"shapeType": "TEXT_BOX", "placeholder": {"type": "TITLE"}}
        }
      ]
    }
  ],
  "masters": [
    {
      "objectId": "master",
      "pageElements": [
        {
          "objectId": "master_number",
          "size": {"width": {"magnitude": 548700, "unit": "EMU"}, "height": {"magnitude": 393600, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 8472457, "translateY": 4663216, "unit": "EMU"},
          "shape": {"shapeType": "TEXT_BOX", "placeholder": {"type": "SLIDE_NUMBER"}}
        }
      ]
    }
  ]
}`

// TestPlaceElementMovesALayoutPlaceholder is the difference between a template and a deck
// that merely looks like one. A grid set slide by slide is undone by the next slide somebody
// adds in the browser: that one comes off the layout, where the title never moved.
func TestPlaceElementMovesALayoutPlaceholder(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLayoutGrid))

	answer := h.ok(h.registry.slidesPlaceElement(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "layout_title",
		"x_emu":           float64(381000),
		"y_emu":           float64(190500),
		"height_emu":      float64(476400),
	})))

	if !strings.Contains(answer, `"y_emu": 190500`) {
		t.Errorf("the layout's title should move to the given place, got %s", answer)
	}

	body := string(h.bodyOf(t, 1))
	if !strings.Contains(body, "layout_title") {
		t.Errorf("the request should name the layout's element, got %s", body)
	}
}

// TestPlaceElementMovesAMasterPlaceholder covers the page every layout inherits from. The
// slide number sits there and nowhere else, so a deck that wants it elsewhere has no other
// place to move it from.
func TestPlaceElementMovesAMasterPlaceholder(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLayoutGrid))

	answer := h.ok(h.registry.slidesPlaceElement(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "master_number",
		"x_emu":           float64(200000),
		"y_emu":           float64(4700000),
	})))

	if !strings.Contains(answer, `"x_emu": 200000`) {
		t.Errorf("the master's slide number should move, got %s", answer)
	}
}

// TestPlaceElementSaysWhereItLooked keeps the refusal honest now that it looks in three
// kinds of page rather than one.
func TestPlaceElementSaysWhereItLooked(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithLayoutGrid))

	message := h.fail(h.registry.slidesPlaceElement(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "nothing",
		"x_emu":           float64(0),
	})))

	if !strings.Contains(message, "slide, layout or master") {
		t.Errorf("the refusal should say every kind of page it searched, got %q", message)
	}
}

// TestDeleteRemovesFurnitureFromALayout is the other half of putting it there: a band or a
// rule added to a layout has to come off the same way, or a template can only ever grow.
func TestDeleteRemovesFurnitureFromALayout(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLayoutGrid))

	answer := h.ok(h.registry.slidesDelete(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{"layout_title"},
	})))

	if !strings.Contains(answer, "layout_title") {
		t.Errorf("the layout's element should be removed, got %s", answer)
	}
}

// TestDeleteRefusesTheLayoutItself draws the line the deck cannot come back from: an
// element on a layout is ordinary editing, the layout is what every slide following it
// depends on.
func TestDeleteRefusesTheLayoutItself(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithLayoutGrid))

	message := h.fail(h.registry.slidesDelete(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_ids":      []any{"layout_body"},
	})))

	if !strings.Contains(message, "every slide that follows it") {
		t.Errorf("the refusal should say what removing a layout would take with it, got %q", message)
	}
}

// TestPlaceElementRotates pins the matrix a turn produces, because the angle is not a
// field: it is spread across the same four numbers as the scale, and getting it wrong
// puts the element somewhere else entirely rather than merely at the wrong angle.
func TestPlaceElementRotates(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLook))

	h.ok(h.registry.slidesPlaceElement(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "panel1",
		"x_emu":           float64(400000),
		"y_emu":           float64(200000),
		"rotation_deg":    float64(90),
	})))

	checkGolden(t, "place_element_rotated.json", h.bodyOf(t, 1))
}

// TestPlaceElementLikeCopiesTheTurn keeps a sample's tilted label from being reproduced
// upright: the position used to be copied without the angle that goes with it.
func TestPlaceElementLikeCopiesTheTurn(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLook))

	h.ok(h.registry.slidesPlaceElement(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "panel1",
		"like_object_id":  "tilted",
	})))

	checkGolden(t, "place_element_like_tilted.json", h.bodyOf(t, 1))
}

// TestPlaceElementAllowsHangingOffTheEdge is a rule a real deck overturned: a title bled
// past the left margin is deliberate, and the sample's own coordinates are negative.
// Refusing them made that slide impossible to reproduce, so it is reported instead.
func TestPlaceElementAllowsHangingOffTheEdge(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithLook))

	answer := h.ok(h.registry.slidesPlaceElement(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "panel1",
		"x_emu":           float64(-14246),
		"y_emu":           float64(4713450),
	})))

	if !strings.Contains(answer, `"off_slide": true`) {
		t.Errorf("an element hanging off the edge should be placed and reported, got %s", answer)
	}
	if !strings.Contains(answer, `"x_emu": -14246`) {
		t.Errorf("the coordinate should reach the API as given, got %s", answer)
	}
}

func TestCreateShapeWithFill(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesCreateShape(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"shape_type":      "round_rectangle",
		"text":            "Итоги",
		"x":               float64(400000),
		"y":               float64(200000),
		"width":           float64(4000000),
		"height":          float64(1000000),
		"fill_color":      map[string]any{"red": 0.9, "green": 0.9, "blue": 0.9},
		"font_size":       float64(18),
		"alignment":       "CENTER",
	})))

	checkGolden(t, "create_shape_panel.json", h.bodyOf(t, 0))
}

// TestCreateShapeDefaultsToATextBox keeps the common case one argument shorter: the tool
// that draws panels is the same one that adds a caption.
func TestCreateShapeDefaultsToATextBox(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	answer := h.ok(h.registry.slidesCreateShape(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"page_object_id":  "slide1",
		"text":            "Подпись под графиком",
		"x":               float64(400000),
		"y":               float64(4000000),
		"width":           float64(3000000),
		"height":          float64(400000),
	})))

	if !strings.Contains(answer, `"shape_type": "TEXT_BOX"`) {
		t.Errorf("a shape with no type named should be a text box, got %s", answer)
	}
}
