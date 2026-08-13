package tools

import (
	"context"
	"strings"
	"testing"
)

// presentationWithTheme is a deck whose look lives where a slide's look really lives: on
// the master's palette and on the layout's placeholders.
const presentationWithTheme = `{
  "presentationId": "deck",
  "title": "Шаблон",
  "pageSize": {"width": {"magnitude": 9144000, "unit": "EMU"}, "height": {"magnitude": 5143500, "unit": "EMU"}},
  "masters": [
    {
      "objectId": "master1",
      "pageProperties": {
        "colorScheme": {"colors": [
          {"type": "DARK1", "color": {"red": 0, "green": 0, "blue": 0}},
          {"type": "LIGHT1", "color": {"red": 1, "green": 1, "blue": 1}},
          {"type": "ACCENT1", "color": {"red": 0.8, "green": 0, "blue": 0}}
        ]},
        "pageBackgroundFill": {"propertyState": "RENDERED", "solidFill": {
           "color": {"rgbColor": {"red": 1, "green": 1, "blue": 1}}, "alpha": 1}}
      },
      "pageElements": [
        {
          "objectId": "master_body",
          "size": {"width": {"magnitude": 8000000, "unit": "EMU"}, "height": {"magnitude": 3000000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 500000, "translateY": 1500000, "unit": "EMU"},
          "shape": {"shapeType": "TEXT_BOX", "placeholder": {"type": "BODY"},
            "text": {"textElements": [
              {"paragraphMarker": {"style": {"alignment": "START", "lineSpacing": 100}}},
              {"textRun": {"content": "Текст\n", "style": {
                 "fontFamily": "Rubik", "fontSize": {"magnitude": 14, "unit": "PT"}}}}
            ]}}
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
          "objectId": "layout_title",
          "size": {"width": {"magnitude": 8000000, "unit": "EMU"}, "height": {"magnitude": 800000, "unit": "EMU"}},
          "transform": {"scaleX": 1, "scaleY": 1, "translateX": 500000, "translateY": 300000, "unit": "EMU"},
          "shape": {"shapeType": "TEXT_BOX", "placeholder": {"type": "TITLE"},
            "shapeProperties": {"autofit": {"autofitType": "SHRINK_ON_OVERFLOW", "fontScale": 0.89}},
            "text": {"textElements": [
              {"paragraphMarker": {"style": {"alignment": "START", "lineSpacing": 90,
                 "spaceAbove": {"magnitude": 4, "unit": "PT"}}}},
              {"textRun": {"content": "Заголовок\n", "style": {
                 "fontFamily": "Rubik", "fontSize": {"magnitude": 28, "unit": "PT"}, "bold": true,
                 "foregroundColor": {"opaqueColor": {"rgbColor": {"red": 0.8, "green": 0, "blue": 0}}}}}}
            ]}}
        }
      ]
    }
  ],
  "slides": []
}`

// TestReadThemeReportsWhatSlidesInherit pins that the numbers a slide does not carry are
// found where they actually live: on the layout and the master.
func TestReadThemeReportsWhatSlidesInherit(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithTheme))

	answer := h.ok(h.registry.slidesReadTheme(context.Background(), request(map[string]any{
		"presentation_id": "deck",
	})))

	for _, want := range []string{
		`"ACCENT1": "#CC0000"`,
		`"LIGHT1": "#FFFFFF"`,
		`"name": "Заголовок и текст"`,
		`"object_id": "layout_title"`,
		`"font_size_pt": 28`,
		// The size on screen is the size times the autofit scale, and that is the number a
		// person comparing two decks by eye is comparing.
		`"font_scale": 0.89`,
		`"line_spacing": 90`,
		`"space_above_pt": 4`,
		`"text_color": "#CC0000"`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the theme reading should carry %s, got %s", want, answer)
		}
	}
}

func TestSetParagraphStyle(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody))

	h.ok(h.registry.slidesSetParagraphStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"scope":           scopeAll,
		"line_spacing":    float64(115),
		"space_above_pt":  float64(6),
		"space_below_pt":  float64(0),
		"alignment":       "START",
	})))

	// The only request: a scope of "all" needs no reading first, unlike a scope naming a
	// nesting level, which has to find out where the levels are.
	checkGolden(t, "set_paragraph_style.json", h.bodyOf(t, 0))
}

// TestSetParagraphStyleCarriesSpacingMode pins the field a real sample needed: ten points
// of space above a heading render as none inside a list unless the collapsing is turned
// off, and every paragraph below then sits ten points too high.
func TestSetParagraphStyleCarriesSpacingMode(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody))

	h.ok(h.registry.slidesSetParagraphStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"scope":           scopeAll,
		"space_above_pt":  float64(10),
		"spacing_mode":    "never_collapse",
	})))

	body := string(h.bodyOf(t, 0))
	for _, want := range []string{"NEVER_COLLAPSE", `"fields": "spaceAbove,spacingMode"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request should carry %s, got %s", want, body)
		}
	}

	if message := h.fail(h.registry.slidesSetParagraphStyle(context.Background(),
		request(map[string]any{
			"presentation_id": "deck",
			"object_id":       "body1",
			"spacing_mode":    "SOMETIMES",
		}))); !strings.Contains(message, "NEVER_COLLAPSE") {
		t.Errorf("the refusal should name the modes, got %q", message)
	}
}

// TestSetParagraphStyleRefusesAMultiplier catches the mistake the API would accept and
// render as lines on top of each other: Slides counts spacing in percent, so 1.5 means
// one and a half percent of a line, not one and a half lines.
func TestSetParagraphStyleRefusesAMultiplier(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody))

	message := h.fail(h.registry.slidesSetParagraphStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"line_spacing":    1.5,
	})))

	if !strings.Contains(message, "percentage") {
		t.Errorf("the refusal should explain the unit, got %q", message)
	}
}

// TestSetParagraphStyleResets pins giving spacing back to the layout. A sample that sets
// none of these is not a sample with no spacing — it inherits — and a copy with them set
// explicitly stays different however close the numbers are.
func TestSetParagraphStyleResets(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithBody))

	h.ok(h.registry.slidesSetParagraphStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"scope":           scopeAll,
		"reset":           []any{"alignment", "lineSpacing"},
	})))

	checkGolden(t, "reset_paragraph_style.json", h.bodyOf(t, 0))

	if message := h.fail(h.registry.slidesSetParagraphStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
		"reset":           []any{"колонтитул"},
	}))); !strings.Contains(message, "not a paragraph style field") {
		t.Errorf("an unknown field should be refused by name, got %q", message)
	}
}

func TestSetParagraphStyleNeedsSomethingToSet(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithBody))

	message := h.fail(h.registry.slidesSetParagraphStyle(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "body1",
	})))

	if !strings.Contains(message, "nothing to set") {
		t.Errorf("a call that changes nothing should be refused, got %q", message)
	}
}

// TestStyleLayout pins writing a style into a layout rather than onto a slide, which is
// the difference between a deck with a look of its own and a deck with a look pasted onto
// every page one at a time.
func TestStyleLayout(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	h.ok(h.registry.slidesStyleLayout(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "layout_title",
		"font_size":       float64(25),
		"font_family":     "Rubik",
		"bold":            true,
		"theme_color":     "accent1",
		"line_spacing":    float64(90),
	})))

	checkGolden(t, "style_layout.json", h.bodyOf(t, 0))
}

func TestStyleLayoutRefusals(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck:batchUpdate", emptyBatchReply))

	if message := h.fail(h.registry.slidesStyleLayout(context.Background(), request(map[string]any{
		"presentation_id":  "deck",
		"object_id":        "layout_title",
		"foreground_color": map[string]any{"red": 1.0, "green": 0.0, "blue": 0.0},
		"theme_color":      "ACCENT1",
	}))); !strings.Contains(message, "alternatives") {
		t.Errorf("a literal colour and a theme colour at once should be refused, got %q", message)
	}

	if message := h.fail(h.registry.slidesStyleLayout(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"object_id":       "layout_title",
	}))); !strings.Contains(message, "nothing to set") {
		t.Errorf("a call that changes nothing should be refused, got %q", message)
	}
}

// wholePalette is the twelve colours Slides insists on, because an update replaces the
// palette rather than editing it.
var wholePalette = map[string]any{
	"DARK1": "#000000", "LIGHT1": "#FFFFFF", "DARK2": "#434343", "LIGHT2": "#EFEFEF",
	"ACCENT1": "#CC0000", "ACCENT2": "#1A73E8", "ACCENT3": "#188038", "ACCENT4": "#E37400",
	"ACCENT5": "#9334E6", "ACCENT6": "#12B5CB", "HYPERLINK": "#1155CC", "FOLLOWED_HYPERLINK": "#551A8B",
}

func TestSetThemeColors(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).
		answer("/presentations/deck:batchUpdate", emptyBatchReply).
		answer("/presentations/deck", presentationWithTheme))

	h.ok(h.registry.slidesSetThemeColors(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"colors":          wholePalette,
	})))

	// Request 0 finds the master, request 1 writes the palette.
	checkGolden(t, "set_theme_colors.json", h.bodyOf(t, 1))
}

func TestSetThemeColorsRefusesAPartialPalette(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithTheme))

	message := h.fail(h.registry.slidesSetThemeColors(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"colors":          map[string]any{"ACCENT1": "#CC0000"},
	})))

	if !strings.Contains(message, "all twelve") || !strings.Contains(message, "DARK1") {
		t.Errorf("the refusal should name what is missing, got %q", message)
	}
}

func TestSetThemeColorsRefusesNonsense(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/presentations/deck", presentationWithTheme))

	broken := map[string]any{}
	for name, value := range wholePalette {
		broken[name] = value
	}
	broken["ACCENT1"] = "красный"

	message := h.fail(h.registry.slidesSetThemeColors(context.Background(), request(map[string]any{
		"presentation_id": "deck",
		"colors":          broken,
	})))

	if !strings.Contains(message, "hex colour") {
		t.Errorf("the refusal should say what a colour looks like, got %q", message)
	}
}

func TestParseHexColor(t *testing.T) {
	colour, err := parseHexColor("#CC0000")
	if err != nil {
		t.Fatalf("reading a colour: %v", err)
	}
	if colour.Red < 0.79 || colour.Red > 0.81 || colour.Green != 0 || colour.Blue != 0 {
		t.Errorf("#CC0000 came out as %+v", colour)
	}

	if _, err := parseHexColor("#12345"); err == nil {
		t.Error("a colour of the wrong length should be refused")
	}
	if _, err := parseHexColor("#ZZZZZZ"); err == nil {
		t.Error("a colour that is not hex should be refused")
	}
}
