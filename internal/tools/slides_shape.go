package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// shapeFamily is a group of shape types with what tells them apart.
type shapeFamily struct {
	Family string   `json:"family"`
	Note   string   `json:"note"`
	Types  []string `json:"types"`
}

// shapeCatalogue is what Slides can draw, grouped so a caller can find the right one
// rather than guess.
//
// The grouping earns its place on the corners. A sample's panel is usually stored as
// ROUND_RECTANGLE with the radius handle pulled in — and that handle is an adjustment
// value the API neither reports nor accepts. Copying the type gives a panel with corners
// twice as round as the sample's. FLOW_CHART_ALTERNATE_PROCESS is the same rectangle with
// a small fixed radius and matches what such a panel looks like. Nothing in the API says
// so; it is found by drawing both and looking.
var shapeCatalogue = []shapeFamily{
	{
		Family: "rectangles",
		Note: "ROUND_RECTANGLE has a large corner radius; FLOW_CHART_ALTERNATE_PROCESS is the same " +
			"rectangle with a small one and is usually what a sample's panel looks like. The radius " +
			"itself cannot be set: it is an adjustment value, and the API has no field for it. " +
			"When a sample's corners look wrong after copying its shape_type, try the other one and " +
			"compare renders.",
		Types: []string{"TEXT_BOX", "RECTANGLE", "ROUND_RECTANGLE", "FLOW_CHART_ALTERNATE_PROCESS",
			"ROUND_1_RECTANGLE", "ROUND_2_SAME_RECTANGLE", "ROUND_2_DIAGONAL_RECTANGLE",
			"SNIP_1_RECTANGLE", "SNIP_2_SAME_RECTANGLE", "SNIP_2_DIAGONAL_RECTANGLE",
			"SNIP_ROUND_RECTANGLE", "PLAQUE", "FRAME", "HALF_FRAME", "BEVEL"},
	},
	{
		Family: "ovals and circles",
		Note:   "ELLIPSE is the plain oval; the rest are cut or joined variants.",
		Types: []string{"ELLIPSE", "DONUT", "PIE", "TEARDROP", "BLOCK_ARC", "CHORD", "ARC",
			"ELLIPSE_RIBBON", "ELLIPSE_RIBBON_2"},
	},
	{
		Family: "arrows",
		Note: "For an arrow between two things, a line with an arrowhead is usually better than an " +
			"arrow shape: use gdocs_slides_create_line with start_arrow or end_arrow.",
		Types: []string{"RIGHT_ARROW", "LEFT_ARROW", "UP_ARROW", "DOWN_ARROW", "LEFT_RIGHT_ARROW",
			"UP_DOWN_ARROW", "BENT_ARROW", "BENT_UP_ARROW", "CURVED_RIGHT_ARROW", "CURVED_LEFT_ARROW",
			"CURVED_UP_ARROW", "CURVED_DOWN_ARROW", "STRIPED_RIGHT_ARROW", "NOTCHED_RIGHT_ARROW",
			"HOME_PLATE", "CHEVRON", "RIGHT_ARROW_CALLOUT", "LEFT_ARROW_CALLOUT", "UP_ARROW_CALLOUT",
			"DOWN_ARROW_CALLOUT"},
	},
	{
		Family: "callouts and speech",
		Note:   "Bubbles with a tail, for a note pointing at something.",
		Types: []string{"WEDGE_RECTANGLE_CALLOUT", "WEDGE_ROUND_RECTANGLE_CALLOUT",
			"WEDGE_ELLIPSE_CALLOUT", "CLOUD_CALLOUT", "RECTANGULAR_CALLOUT", "ROUND_RECTANGULAR_CALLOUT"},
	},
	{
		Family: "flowchart",
		Note: "The flowchart set doubles as a source of plain shapes with modest corners — " +
			"FLOW_CHART_ALTERNATE_PROCESS above all.",
		Types: []string{"FLOW_CHART_PROCESS", "FLOW_CHART_ALTERNATE_PROCESS", "FLOW_CHART_DECISION",
			"FLOW_CHART_INPUT_OUTPUT", "FLOW_CHART_PREDEFINED_PROCESS", "FLOW_CHART_INTERNAL_STORAGE",
			"FLOW_CHART_DOCUMENT", "FLOW_CHART_MULTIDOCUMENT", "FLOW_CHART_TERMINATOR",
			"FLOW_CHART_PREPARATION", "FLOW_CHART_MANUAL_INPUT", "FLOW_CHART_MANUAL_OPERATION",
			"FLOW_CHART_CONNECTOR", "FLOW_CHART_CARD", "FLOW_CHART_DELAY", "FLOW_CHART_MERGE"},
	},
	{
		Family: "stars, banners and marks",
		Note:   "Decoration: a badge over a number, a ribbon across a corner.",
		Types: []string{"STAR_4", "STAR_5", "STAR_6", "STAR_7", "STAR_8", "STAR_10", "STAR_12",
			"STAR_16", "STAR_24", "STAR_32", "RIBBON", "RIBBON_2", "VERTICAL_SCROLL",
			"HORIZONTAL_SCROLL", "WAVE", "DOUBLE_WAVE", "EXPLOSION1", "EXPLOSION2", "CLOUD",
			"HEART", "LIGHTNING_BOLT", "SUN", "MOON", "SMILEY_FACE", "NO_SMOKING"},
	},
	{
		Family: "polygons and braces",
		Note:   "Triangles, diamonds and the brackets that group things.",
		Types: []string{"TRIANGLE", "RIGHT_TRIANGLE", "DIAMOND", "PARALLELOGRAM", "TRAPEZOID",
			"PENTAGON", "HEXAGON", "HEPTAGON", "OCTAGON", "DECAGON", "DODECAGON", "PLUS",
			"MATH_PLUS", "MATH_MINUS", "MATH_MULTIPLY", "MATH_DIVIDE", "MATH_EQUAL",
			"MATH_NOT_EQUAL", "LEFT_BRACE", "RIGHT_BRACE", "LEFT_BRACKET", "RIGHT_BRACKET",
			"BRACE_PAIR", "BRACKET_PAIR", "CUBE", "CAN", "FOLDED_CORNER", "DIAGONAL_STRIPE"},
	},
}

// registerSlidesShape adds the tools that draw and style the things a slide is made of
// besides text: filled panels, arrows, lines, and the pictures already on it.
func (r *registry) registerSlidesShape(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_reference",
		mcp.WithDescription("The values these tools accept that are nowhere else to be found: shape names, "+
			"bullet presets, arrowheads, dash styles, theme colour names, export formats, placeholder "+
			"kinds, units. Every one of them is an enum of Google's that a caller would otherwise have to "+
			"guess at, and a guess comes back as \"invalid argument\" naming nothing. "+
			"Each topic carries the notes that matter when a sample has to be matched — where the API "+
			"cannot express what a sample does, and what to use instead. Four topics are not enums but "+
			"behaviour of the APIs that costs an hour to rediscover: charts (a range that does not grow, "+
			"a chart on a slide that does not refresh itself), tables (what removing a row does to the "+
			"indexes and to merged cells), text (paragraphs against lists, the colour of a link) and "+
			"rendering (what a thumbnail and a PDF show and what they leave out). files says where "+
			"exported files land and how a client reaches them. Every answer carries server, the "+
			"version of this build."),
		mcp.WithString("topic", mcp.Description(
			"shapes, bullets, arrows, dashes, theme_colors, export_formats, placeholders, units, "+
				"alignments, charts, tables, text, rendering, files. Without one, everything.")),
		mcp.WithString("family", mcp.Description(
			"For topic=shapes, one family: rectangles, ovals and circles, arrows, callouts and speech, "+
				"flowchart, stars, banners and marks, polygons and braces.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.reference)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_slides_style_shape",
		mcp.WithDescription("Restyle a shape that is already on a slide: its fill, its outline, how the text "+
			"inside it is aligned vertically. Works on placeholders too, which is how a title gets the "+
			"sample's coloured panel behind it. Read the sample's with gdocs_slides_inspect_page first — "+
			"the colours it reports go straight back in here."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Shape to restyle.")),
		mcp.WithObject("fill_color", mcp.Description("Fill colour as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}.")),
		mcp.WithString("fill_theme_color", mcp.Description(
			"Fill by palette name instead of by value — DARK1, LIGHT1, ACCENT1...ACCENT6, HYPERLINK and "+
				"the TEXT1/BACKGROUND1 aliases. A fill painted this way follows "+
				"gdocs_slides_set_theme_colors; a literal one does not. This is what makes a series of "+
				"decks recolourable in one call instead of shape by shape.")),
		mcp.WithNumber("fill_alpha", mcp.Description("Fill opacity from 0 to 1. Default 1.")),
		mcp.WithBoolean("no_fill", mcp.Description("Make the shape transparent.")),
		mcp.WithBoolean("inherit_fill", mcp.Description("Give the fill back to the layout.")),
		mcp.WithObject("outline_color", mcp.Description("Outline colour, same shape as fill_color.")),
		mcp.WithString("outline_theme_color", mcp.Description(
			"Outline by palette name, the same names as fill_theme_color.")),
		mcp.WithNumber("outline_weight_emu", mcp.Description("Outline thickness in EMU. 12700 is one point.")),
		mcp.WithString("outline_dash", mcp.Description(
			"SOLID, DOT, DASH, DASH_DOT, LONG_DASH or LONG_DASH_DOT.")),
		mcp.WithBoolean("no_outline", mcp.Description("Remove the border.")),
		mcp.WithString("content_alignment", mcp.Description(
			"Where the text sits in the box: TOP, MIDDLE or BOTTOM.")),
		mcp.WithString("autofit", mcp.Description(
			"Turn off shrinking text to fit the box: NONE is the only value the API accepts. "+
				"Shrinking is why a sample's title reported as 28 pt measures 25 on screen — it is 28 × "+
				"the font_scale gdocs_slides_inspect_page reports — and it cannot be switched on from "+
				"here at all. Match such a title by setting its font size to size × that scale.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesStyleShape)

	srv.AddTool(mcp.NewTool("gdocs_slides_style_image",
		mcp.WithDescription("Restyle a picture already on a slide: crop it, make it see-through, adjust its "+
			"brightness or contrast, give it a border. A picture that fills a circle in a sample is a "+
			"cropped picture, not a smaller one — copying the box without the crop shows the parts the "+
			"author hid."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Picture to restyle.")),
		mcp.WithObject("crop", mcp.Description(
			"How much of each side to cut away, as fractions of the picture: "+
				"{\"left\": 0..1, \"right\": 0..1, \"top\": 0..1, \"bottom\": 0..1, \"angle\": radians}. "+
				"These are the numbers gdocs_slides_inspect_page reports for a sample's picture.")),
		mcp.WithNumber("transparency", mcp.Description("0 is opaque, 1 is invisible.")),
		mcp.WithNumber("brightness", mcp.Description("From -1 to 1, 0 leaves it alone.")),
		mcp.WithNumber("contrast", mcp.Description("From -1 to 1, 0 leaves it alone.")),
		mcp.WithObject("outline_color", mcp.Description("Border colour as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}.")),
		mcp.WithString("outline_theme_color", mcp.Description(
			"Border by palette name — DARK1, LIGHT1, ACCENT1...ACCENT6 — instead of by value, so it "+
				"follows the theme when the palette changes.")),
		mcp.WithNumber("outline_weight_emu", mcp.Description("Border thickness in EMU. 12700 is one point.")),
		mcp.WithString("outline_dash", mcp.Description("SOLID, DOT, DASH, DASH_DOT, LONG_DASH or LONG_DASH_DOT.")),
		mcp.WithBoolean("no_outline", mcp.Description("Remove the border.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesStyleImage)

	srv.AddTool(mcp.NewTool("gdocs_slides_create_line",
		mcp.WithDescription("Draw a line, an arrow or a connector between two points of a slide. A diagram in a "+
			"sample is boxes plus the lines between them, and a copy without the lines is a copy of half "+
			"the slide."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description("Slide to draw on.")),
		mcp.WithNumber("x", mcp.Required(), mcp.Description("Left edge of the line's box in EMU.")),
		mcp.WithNumber("y", mcp.Required(), mcp.Description("Top edge of the line's box in EMU.")),
		mcp.WithNumber("width", mcp.Required(), mcp.Description("Width of the line's box in EMU. May be 0 for a vertical line.")),
		mcp.WithNumber("height", mcp.Required(), mcp.Description("Height of the line's box in EMU. May be 0 for a horizontal line.")),
		mcp.WithString("category", mcp.Description("STRAIGHT (default), BENT or CURVED.")),
		mcp.WithString("object_id", mcp.Description("Identifier to give it. Without one it is generated.")),
		mcp.WithObject("color", mcp.Description("Line colour as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}.")),
		mcp.WithNumber("weight_emu", mcp.Description("Thickness in EMU. 12700 is one point.")),
		mcp.WithString("dash", mcp.Description("SOLID, DOT, DASH, DASH_DOT, LONG_DASH or LONG_DASH_DOT.")),
		mcp.WithString("start_arrow", mcp.Description(
			"NONE, STEALTH_ARROW, FILL_ARROW, FILL_CIRCLE, FILL_SQUARE, FILL_DIAMOND, "+
				"OPEN_ARROW, OPEN_CIRCLE, OPEN_SQUARE or OPEN_DIAMOND.")),
		mcp.WithString("end_arrow", mcp.Description("Same names as start_arrow.")),
	), r.slidesCreateLine)
}

// shapeStyle is a shape's look with the mask that says which parts of it to apply.
type shapeStyle struct {
	style  *google.ShapeProperties
	fields []string
}

// shapeStyleFrom reads the fill, outline and content alignment out of the arguments.
//
// The mask is built alongside the values rather than being a constant: naming a field
// that was not set resets it to the theme's default, so a caller changing only the fill
// would silently lose the outline the sample had.
func shapeStyleFrom(req mcp.CallToolRequest) (*shapeStyle, error) {
	style := &google.ShapeProperties{}
	var fields []string

	fillColor, err := parseColor(req, "fill_color")
	if err != nil {
		return nil, err
	}
	fillTheme, err := paletteColor(req, "fill_theme_color")
	if err != nil {
		return nil, err
	}

	noFill := req.GetBool("no_fill", false)
	inheritFill := req.GetBool("inherit_fill", false)

	chosen := 0
	for _, set := range []bool{fillColor != nil, fillTheme != "", noFill, inheritFill} {
		if set {
			chosen++
		}
	}
	if chosen > 1 {
		return nil, fmt.Errorf("fill_color, fill_theme_color, no_fill and inherit_fill are alternatives: name one")
	}

	switch {
	case fillColor != nil, fillTheme != "":
		alpha := req.GetFloat("fill_alpha", 1)
		if alpha < 0 || alpha > 1 {
			return nil, fmt.Errorf("fill_alpha is %g: opacity runs from 0 to 1", alpha)
		}
		colour := &google.OpaqueColor{RGBColor: fillColor}
		if fillTheme != "" {
			colour = &google.OpaqueColor{ThemeColor: fillTheme}
		}
		style.BackgroundFill = &google.ShapeBackgroundFill{
			PropertyState: "RENDERED",
			SolidFill:     &google.SolidFill{Color: colour, Alpha: alpha},
		}
		fields = append(fields, "shapeBackgroundFill")
	case noFill:
		style.BackgroundFill = &google.ShapeBackgroundFill{PropertyState: "NOT_RENDERED"}
		fields = append(fields, "shapeBackgroundFill")
	case inheritFill:
		style.BackgroundFill = &google.ShapeBackgroundFill{PropertyState: "INHERIT"}
		fields = append(fields, "shapeBackgroundFill")
	}

	outline, err := outlineFrom(req, "outline_color", "outline_theme_color",
		"outline_weight_emu", "outline_dash", "no_outline")
	if err != nil {
		return nil, err
	}
	if outline != nil {
		style.Outline = outline
		fields = append(fields, "outline")
	}

	if alignment := strings.ToUpper(optionalString(req, "content_alignment")); alignment != "" {
		switch alignment {
		case "TOP", "MIDDLE", "BOTTOM":
		default:
			return nil, fmt.Errorf("content_alignment is %q: it has to be TOP, MIDDLE or BOTTOM", alignment)
		}
		style.ContentAlignment = alignment
		fields = append(fields, "contentAlignment")
	}

	// Autofit is why a title reported as 28 pt measures 25 on screen: the size is 28 and
	// Slides shrank it by the scale it computed. Only NONE can be written — the API answers
	// anything else with "Autofit types other than NONE are not supported" — so shrinking
	// cannot be switched on, and a sample that has it is matched by setting the size the
	// scale works out to.
	if autofit := strings.ToUpper(optionalString(req, "autofit")); autofit != "" {
		if autofit != "NONE" {
			return nil, fmt.Errorf("autofit is %q, and Slides accepts only NONE here: shrinking text to "+
				"fit cannot be switched on through the API. A sample whose box shrinks its text reports "+
				"font_scale below 1 — reproduce it by setting the font size to size × that scale with "+
				"gdocs_slides_set_text_style", autofit)
		}
		style.Autofit = &google.Autofit{AutofitType: autofit}
		fields = append(fields, "autofit.autofitType")
	}

	if len(fields) == 0 {
		return nil, nil
	}

	return &shapeStyle{style: style, fields: fields}, nil
}

// outlineFrom reads a border out of the arguments, under whatever names the tool uses.
func outlineFrom(req mcp.CallToolRequest, colorName, themeName, weightName, dashName, noneName string,
) (*google.Outline, error) {
	colour, err := parseColor(req, colorName)
	if err != nil {
		return nil, err
	}
	theme, err := paletteColor(req, themeName)
	if err != nil {
		return nil, err
	}
	if colour != nil && theme != "" {
		return nil, fmt.Errorf("%s and %s are alternatives: name one", colorName, themeName)
	}

	weight := req.GetFloat(weightName, 0)
	dash := strings.ToUpper(optionalString(req, dashName))
	none := req.GetBool(noneName, false)

	if none {
		if colour != nil || theme != "" || weight > 0 || dash != "" {
			return nil, fmt.Errorf("%s cannot be combined with %s, %s, %s or %s",
				noneName, colorName, themeName, weightName, dashName)
		}
		return &google.Outline{PropertyState: "NOT_RENDERED"}, nil
	}

	if colour == nil && theme == "" && weight == 0 && dash == "" {
		return nil, nil
	}

	if dash != "" {
		switch dash {
		case "SOLID", "DOT", "DASH", "DASH_DOT", "LONG_DASH", "LONG_DASH_DOT":
		default:
			return nil, fmt.Errorf("%s is %q: it has to be SOLID, DOT, DASH, DASH_DOT, LONG_DASH or LONG_DASH_DOT",
				dashName, dash)
		}
	}

	outline := &google.Outline{PropertyState: "RENDERED", DashStyle: dash}
	if colour != nil || theme != "" {
		painted := &google.OpaqueColor{RGBColor: colour}
		if theme != "" {
			painted = &google.OpaqueColor{ThemeColor: theme}
		}
		outline.OutlineFill = &google.OutlineFill{
			SolidFill: &google.SolidFill{Color: painted, Alpha: 1},
		}
	}
	if weight > 0 {
		outline.Weight = google.EMU(weight)
	}

	return outline, nil
}

// referenceTopics are the enums a caller cannot know, with what matters about each.
//
// They are here rather than in a document because a caller reaching for a value has this
// server in front of it and not the documentation, and because half of what matters is
// not in the documentation at all: which shape matches a sample whose corner radius the
// API will not report, why a bullet has to be made after the text is coloured, that an
// indent comes back in points while a position comes back in EMU.
var referenceTopics = map[string]any{
	"bullets": map[string]any{
		"note": "Presets for gdocs_slides_set_list and the bullet_preset of " +
			"gdocs_slides_set_paragraph_style. Slides picks the marker per level from the preset, so " +
			"a nested list gets the markers the template would have used. A bullet takes its colour " +
			"and size from its paragraph's text at the moment it is made: style the text first, then " +
			"make the list, or the markers stay black over coloured words.",
		"presets": []string{
			"BULLET_DISC_CIRCLE_SQUARE", "BULLET_DIAMONDX_ARROW3D_SQUARE", "BULLET_CHECKBOX",
			"BULLET_ARROW_DIAMOND_DISC", "BULLET_STAR_CIRCLE_SQUARE", "BULLET_ARROW3D_CIRCLE_SQUARE",
			"BULLET_LEFTTRIANGLE_DIAMOND_DISC", "BULLET_DIAMONDX_HOLLOWDIAMOND_SQUARE",
			"BULLET_DIAMOND_CIRCLE_SQUARE", "NUMBERED_DIGIT_ALPHA_ROMAN",
			"NUMBERED_DIGIT_ALPHA_ROMAN_PARENS", "NUMBERED_DIGIT_NESTED", "NUMBERED_UPPERALPHA_ALPHA_ROMAN",
			"NUMBERED_UPPERROMAN_UPPERALPHA_DIGIT", "NUMBERED_ZERODIGIT_ALPHA_ROMAN",
		},
	},
	"arrows": map[string]any{
		"note": "Arrowheads for gdocs_slides_create_line, at either end.",
		"styles": []string{"NONE", "STEALTH_ARROW", "FILL_ARROW", "FILL_CIRCLE", "FILL_SQUARE",
			"FILL_DIAMOND", "OPEN_ARROW", "OPEN_CIRCLE", "OPEN_SQUARE", "OPEN_DIAMOND"},
	},
	"dashes": map[string]any{
		"note":   "Dash styles for outlines and lines.",
		"styles": []string{"SOLID", "DOT", "DASH", "DASH_DOT", "LONG_DASH", "LONG_DASH_DOT"},
	},
	"theme_colors": map[string]any{
		"note": "The twelve names a colour can be written as instead of a value. A colour set by name " +
			"follows the theme when it changes; the same colour written literally does not. " +
			"gdocs_slides_read_theme reports what each name currently is; gdocs_slides_set_theme_colors " +
			"replaces all twelve at once, on the master only.",
		"names": google.ThemeColorTypes,
		"aliases": "TEXT1, BACKGROUND1, TEXT2 and BACKGROUND2 are aliases of DARK1, LIGHT1, DARK2 and " +
			"LIGHT2; Slides reports them but ignores them on the way in.",
	},
	"export_formats": map[string]any{
		"note": "Formats for gdocs_drive_export_file. A deck exports to pdf and pptx; a document to docx, " +
			"odt, txt, html, rtf, epub and pdf; a spreadsheet to xlsx, ods, csv and pdf.",
		"formats": []string{"pdf", "pptx", "odp", "docx", "odt", "xlsx", "ods", "csv", "txt",
			"html", "rtf", "epub"},
		"deck_as_pdf": "A deck exported to pdf is the whole deck in one file, one page per slide, in " +
			"deck order — the cheapest way to look through every slide at once. Hidden slides are in " +
			"it too: the export is of the file, not of the slideshow, so page N is slide N counting " +
			"the hidden ones. Speaker notes are not in it.",
	},
	"placeholders": map[string]any{
		"note": "The slots a layout offers. Filling a placeholder keeps the template's styling; a new " +
			"text box inherits nothing. A placeholder nobody fills is not invisible — it renders as " +
			"\"Click to add text\" over whatever was placed on top, so fill it or delete it.",
		"kinds": []string{"TITLE", "CENTERED_TITLE", "SUBTITLE", "BODY", "OBJECT", "PICTURE", "CHART",
			"TABLE", "DIAGRAM", "MEDIA", "SLIDE_NUMBER", "DATE_AND_TIME", "FOOTER", "HEADER",
			"CLIP_ART", "SLIDE_IMAGE"},
	},
	"units": map[string]any{
		"note": "Slides answers in whichever unit a value was stored in, and the tools convert on the " +
			"way out so a field named _emu is always EMU and a field named _pt is always points. " +
			"Positions and sizes are EMU; font sizes and paragraph spacing are points; paragraph " +
			"indents come back from Google in points and are reported here in EMU.",
		"emu_per_point": google.EMUPerPoint,
		"emu_per_inch":  914400,
		"slide_16_9":    "9144000 × 5143500 EMU",
	},
	"alignments": map[string]any{
		"note":       "Horizontal alignment belongs to the paragraph; vertical alignment to the shape.",
		"paragraph":  []string{"START", "CENTER", "END", "JUSTIFIED"},
		"content":    []string{"TOP", "MIDDLE", "BOTTOM"},
		"directions": []string{"LEFT_TO_RIGHT", "RIGHT_TO_LEFT"},
		"baseline":   []string{"NONE", "SUPERSCRIPT", "SUBSCRIPT"},
	},
	// The four below are not enums. They are what the APIs do that their reference pages do
	// not say in one place, each learned by a deck or a workbook coming out wrong; they live
	// here because this is the one tool every connection has, whichever window it opened.
	"charts": map[string]any{
		"chart_id": "A chart is addressed by a number inside its workbook. gdocs_sheets_info lists the " +
			"charts of every tab with theirs, and gdocs_sheets_add_chart answers with the one it made.",
		"range_is_fixed": "A chart draws the rows its ranges named when it was made. Rows appended " +
			"under them are in the tab and not in the chart, and nothing says so. " +
			"gdocs_sheets_append_rows names the charts that read the tab it appended to, and with " +
			"extend_charts grows the ones whose ranges ended on the last row; " +
			"gdocs_sheets_update_chart with start_row and end_row moves a range by hand.",
		"on_a_slide": "A linked chart on a slide is a snapshot taken when it was put there or last " +
			"refreshed. Changing the workbook — its numbers, or the chart itself with " +
			"gdocs_sheets_update_chart — changes nothing on the slide until " +
			"gdocs_slides_refresh_sheets_chart is called for that element.",
		"not_linked": "A chart put on a slide with linked=false is a picture. It keeps no thread to the " +
			"workbook, gdocs_slides_inspect_page reports it as an image with no sheets_chart, and a " +
			"refresh has nothing to refresh. To bring it up to date put the chart on again with " +
			"gdocs_slides_add_sheets_chart and same_place_as naming the picture, then take the " +
			"picture off with gdocs_slides_delete.",
		"whole_spec": "updateChartSpec has no field mask: what is sent becomes the chart. " +
			"gdocs_sheets_update_chart reads the stored specification, changes what was named and " +
			"sends the whole of it back, so a new title does not cost the chart its data.",
	},
	"tables": map[string]any{
		"indexes_shift": "Removing a row of a table on a slide moves every row under it up by one, at " +
			"once: after rows 2 and 3 are gone, what was row 4 is row 2. A second removal built on the " +
			"reading that preceded the first removes the wrong row — read the table again with " +
			"gdocs_slides_read_table, or count from rows_left in the answer. Columns behave the same.",
		"merged_cells": "deleteTableRow removes every row the cell it names spans, so a row that " +
			"crosses a cell merged down several rows takes its neighbours with it. " +
			"gdocs_slides_delete refuses such a removal before sending anything and names the merged " +
			"cell: take the merge apart with gdocs_slides_edit_table what=unmerge, remove the row, and " +
			"merge again with gdocs_slides_style_table.",
		"coordinates": "A cell is addressed by the coordinates it reports, not by its place in its " +
			"row: a merge takes the cells it swallowed out of the table.",
	},
	"text": map[string]any{
		"paragraphs_are_not_a_list": "Newlines in gdocs_slides_set_text make paragraphs, and that is " +
			"all they make: no markers and no indents, so several lines read as one run-on block. " +
			"A list is made by gdocs_slides_set_list, which takes the lines with their levels.",
		"links": "A link takes the theme's HYPERLINK colour and an underline the moment it is made, " +
			"in a text box and in a table cell alike, whatever colour the text had. To have it another " +
			"colour, style the same stretch with gdocs_slides_set_text_style after linking, or change " +
			"HYPERLINK for the whole deck with gdocs_slides_set_theme_colors.",
	},
	"rendering": map[string]any{
		"thumbnail": "gdocs_slides_export_thumbnail and gdocs_slides_export_images render the slide " +
			"as the audience sees it. Speaker notes are not on it — read them back with " +
			"gdocs_slides_inspect_page, which reports speaker_notes.",
		"overflow": "The API does not say whether text fits its box: a shape reports its size and its " +
			"text and nothing about how the two met. gdocs_slides_inspect_page adds " +
			"text_overflow_estimate where its own arithmetic says the text is taller than the box — " +
			"an estimate from character counts and the font size the runs carry, not a measurement; a " +
			"box whose font size is inherited from its layout is not estimated at all. The render is " +
			"what settles it.",
		"pdf": "gdocs_drive_export_file with format=pdf writes the whole deck, one page per slide, " +
			"hidden slides included, without the speaker notes.",
	},
}

// reference answers with the values a caller cannot know.
func (r *registry) reference(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := r.referenceTopic(req)
	if err != nil || result == nil || result.IsError {
		return result, err
	}

	// The build is in every answer rather than under a topic of its own: whoever reads a
	// transcript in which a tool misbehaved has the version beside the call that showed it.
	if payload, ok := result.StructuredContent.(map[string]any); ok {
		payload["server"] = r.serverInfo()
		return resultJSON(payload)
	}

	return result, nil
}

// serverInfo is what this build says about itself.
func (r *registry) serverInfo() map[string]any {
	version := r.opts.Version
	if version == "" {
		version = "dev"
	}

	return map[string]any{"name": "mcp-gdocs", "version": version}
}

// filesTopic says where this server keeps files and how a client reaches them.
//
// It is built per call rather than kept in referenceTopics because it carries a fact about
// this process — the directory it was started with — and because the half that matters is
// one no reference page could hold: the path in an answer is a path on the server's side.
func (r *registry) filesTopic() map[string]any {
	topic := map[string]any{
		"note": "The file tools — gdocs_drive_export_file, gdocs_drive_download_file, " +
			"gdocs_slides_export_images, gdocs_drive_import_file and the file argument of " +
			"gdocs_slides_insert_image — read and write inside one directory, the one the server was " +
			"started with in --files-dir, and nowhere else. Names are given relative to it.",
		"two_sides": "A path in an answer is a path as the server sees it. A server in a container " +
			"sees the directory under the name it was mounted at — /files by convention — while the " +
			"client that called the tool sees the same directory under its own name on the host; " +
			"clients keep that name in GDOCS_FILES_DIR. So every answer carries two things: path, the " +
			"whole path on the server's side, and relative_path, the same file named from the " +
			"directory. On the client's side the file is at $GDOCS_FILES_DIR/<relative_path>, and a " +
			"file put at $GDOCS_FILES_DIR/<relative_path> is imported, or inserted as a picture, by " +
			"passing <relative_path>.",
	}

	if r.opts.FilesDir == "" {
		topic["files_dir"] = nil
		topic["available"] = "this server was started without --files-dir, so the file tools are " +
			"not offered"
	} else {
		topic["files_dir"] = filepath.Clean(r.opts.FilesDir)
	}

	return topic
}

func (r *registry) referenceTopic(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	topic := strings.ToLower(optionalString(req, "topic"))
	family := strings.ToLower(optionalString(req, "family"))

	shapes := func() any {
		if family == "" {
			return shapeCatalogue
		}
		for _, group := range shapeCatalogue {
			if strings.EqualFold(group.Family, family) {
				return []shapeFamily{group}
			}
		}
		return nil
	}

	if topic == "shapes" {
		found := shapes()
		if found == nil {
			names := make([]string, 0, len(shapeCatalogue))
			for _, group := range shapeCatalogue {
				names = append(names, group.Family)
			}
			return toolError(fmt.Errorf("no shape family %q: there are %s",
				family, strings.Join(names, ", "))), nil
		}
		return resultJSON(map[string]any{"shapes": found})
	}

	if topic == "files" {
		return resultJSON(map[string]any{"files": r.filesTopic()})
	}

	if topic != "" {
		entry, ok := referenceTopics[topic]
		if !ok {
			names := make([]string, 0, len(referenceTopics)+2)
			names = append(names, "shapes", "files")
			for name := range referenceTopics {
				names = append(names, name)
			}
			return toolError(fmt.Errorf("no topic %q: there are %s",
				topic, strings.Join(sortedStrings(names), ", "))), nil
		}
		return resultJSON(map[string]any{topic: entry})
	}

	everything := map[string]any{"shapes": shapes(), "files": r.filesTopic()}
	for name, entry := range referenceTopics {
		everything[name] = entry
	}

	return resultJSON(everything)
}

// slidesStyleShape restyles a shape that is already on a slide.
func (r *registry) slidesStyleShape(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	properties, err := shapeStyleFrom(req)
	if err != nil {
		return toolError(err), nil
	}
	if properties == nil {
		return toolError(fmt.Errorf("nothing to change: name a fill, an outline or content_alignment")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		UpdateShapeProperties: &google.UpdateShapePropertiesRequest{
			ObjectID:        objectID,
			ShapeProperties: properties.style,
			Fields:          strings.Join(properties.fields, ","),
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"object_id":       objectID,
		"changed":         properties.fields,
		"replies":         len(response.Replies),
	})
}

// slidesStyleImage crops or adjusts a picture already on a slide.
func (r *registry) slidesStyleImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	properties := &google.ImageProperties{}
	var fields []string
	arguments := req.GetArguments()

	if raw, ok := arguments["crop"]; ok && raw != nil {
		object, ok := raw.(map[string]any)
		if !ok {
			return toolError(fmt.Errorf("crop must be an object with left, right, top, bottom and angle, got %T",
				raw)), nil
		}

		crop := &google.CropProperties{}
		for _, side := range []struct {
			name   string
			target *float64
			// Offsets are fractions of a side and cannot exceed it; the angle is radians
			// and is not bounded the same way.
			bounded bool
		}{
			{"left", &crop.LeftOffset, true},
			{"right", &crop.RightOffset, true},
			{"top", &crop.TopOffset, true},
			{"bottom", &crop.BottomOffset, true},
			{"angle", &crop.Angle, false},
		} {
			value, present := object[side.name]
			if !present {
				continue
			}
			number, ok := value.(float64)
			if !ok {
				return toolError(fmt.Errorf("crop.%s must be a number, got %T", side.name, value)), nil
			}
			if side.bounded && (number < 0 || number > 1) {
				return toolError(fmt.Errorf("crop.%s is %g: offsets are fractions of the picture, from 0 to 1",
					side.name, number)), nil
			}
			*side.target = number
		}

		properties.CropProperties = crop
		fields = append(fields, "cropProperties")
	}

	for _, adjustment := range []struct {
		name   string
		target *float64
		min    float64
	}{
		{"transparency", &properties.Transparency, 0},
		{"brightness", &properties.Brightness, -1},
		{"contrast", &properties.Contrast, -1},
	} {
		raw, ok := arguments[adjustment.name]
		if !ok || raw == nil {
			continue
		}
		number, ok := raw.(float64)
		if !ok {
			return toolError(fmt.Errorf("%s must be a number, got %T", adjustment.name, raw)), nil
		}
		if number < adjustment.min || number > 1 {
			return toolError(fmt.Errorf("%s is %g: it runs from %g to 1", adjustment.name, number, adjustment.min)), nil
		}
		*adjustment.target = number
		fields = append(fields, adjustment.name)
	}

	outline, err := outlineFrom(req, "outline_color", "outline_theme_color",
		"outline_weight_emu", "outline_dash", "no_outline")
	if err != nil {
		return toolError(err), nil
	}
	if outline != nil {
		properties.Outline = outline
		fields = append(fields, "outline")
	}

	if len(fields) == 0 {
		return toolError(fmt.Errorf("nothing to change: name a crop, an adjustment or an outline")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		UpdateImageProperties: &google.UpdateImagePropertiesRequest{
			ObjectID:        objectID,
			ImageProperties: properties,
			Fields:          strings.Join(fields, ","),
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"object_id":       objectID,
		"changed":         fields,
		"replies":         len(response.Replies),
	})
}

// slidesCreateLine draws a line, an arrow or a connector.
func (r *registry) slidesCreateLine(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "page_object_id")
	if err != nil {
		return toolError(err), nil
	}

	width, height := req.GetFloat("width", 0), req.GetFloat("height", 0)
	if width < 0 || height < 0 {
		return toolError(fmt.Errorf("width and height are in EMU and cannot be negative")), nil
	}
	if width == 0 && height == 0 {
		return toolError(fmt.Errorf("a line with no width and no height has no length")), nil
	}

	category := strings.ToUpper(optionalString(req, "category"))
	if category == "" {
		category = "STRAIGHT"
	}
	switch category {
	case "STRAIGHT", "BENT", "CURVED":
	default:
		return toolError(fmt.Errorf("category is %q: it has to be STRAIGHT, BENT or CURVED", category)), nil
	}

	objectID := optionalString(req, "object_id")
	if objectID == "" {
		objectID = r.objectID("line")
	}

	requests := []google.Request{{
		CreateLine: &google.CreateLineRequest{
			ObjectID: objectID,
			Category: category,
			ElementProperties: &google.ElementProperties{
				PageObjectID: pageObjectID,
				Size:         &google.Size{Width: google.EMU(width), Height: google.EMU(height)},
				Transform: &google.Transform{
					ScaleX: 1, ScaleY: 1,
					TranslateX: req.GetFloat("x", 0),
					TranslateY: req.GetFloat("y", 0),
					Unit:       "EMU",
				},
			},
		},
	}}

	style, fields, err := linePropertiesFrom(req)
	if err != nil {
		return toolError(err), nil
	}
	if len(fields) > 0 {
		requests = append(requests, google.Request{
			UpdateLineProperties: &google.UpdateLinePropertiesRequest{
				ObjectID:       objectID,
				LineProperties: style,
				Fields:         strings.Join(fields, ","),
			},
		})
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"object_id":       objectID,
		"category":        category,
		"replies":         len(response.Replies),
	})
}

// arrowStyles are the arrowheads Slides draws.
var arrowStyles = map[string]bool{
	"NONE": true, "STEALTH_ARROW": true, "FILL_ARROW": true, "FILL_CIRCLE": true,
	"FILL_SQUARE": true, "FILL_DIAMOND": true, "OPEN_ARROW": true, "OPEN_CIRCLE": true,
	"OPEN_SQUARE": true, "OPEN_DIAMOND": true,
}

// linePropertiesFrom reads a line's look out of the arguments, with the mask for it.
func linePropertiesFrom(req mcp.CallToolRequest) (*google.LineProperties, []string, error) {
	properties := &google.LineProperties{}
	var fields []string

	colour, err := parseColor(req, "color")
	if err != nil {
		return nil, nil, err
	}
	if colour != nil {
		properties.LineFill = &google.LineFill{
			SolidFill: &google.SolidFill{Color: &google.OpaqueColor{RGBColor: colour}, Alpha: 1},
		}
		fields = append(fields, "lineFill")
	}

	if weight := req.GetFloat("weight_emu", 0); weight > 0 {
		properties.Weight = google.EMU(weight)
		fields = append(fields, "weight")
	}

	if dash := strings.ToUpper(optionalString(req, "dash")); dash != "" {
		switch dash {
		case "SOLID", "DOT", "DASH", "DASH_DOT", "LONG_DASH", "LONG_DASH_DOT":
		default:
			return nil, nil, fmt.Errorf("dash is %q: it has to be SOLID, DOT, DASH, DASH_DOT, LONG_DASH "+
				"or LONG_DASH_DOT", dash)
		}
		properties.DashStyle = dash
		fields = append(fields, "dashStyle")
	}

	for _, end := range []struct {
		name   string
		field  string
		target *string
	}{
		{"start_arrow", "startArrow", &properties.StartArrow},
		{"end_arrow", "endArrow", &properties.EndArrow},
	} {
		arrow := strings.ToUpper(optionalString(req, end.name))
		if arrow == "" {
			continue
		}
		if !arrowStyles[arrow] {
			return nil, nil, fmt.Errorf("%s is %q: it has to be one of NONE, STEALTH_ARROW, FILL_ARROW, "+
				"FILL_CIRCLE, FILL_SQUARE, FILL_DIAMOND, OPEN_ARROW, OPEN_CIRCLE, OPEN_SQUARE, OPEN_DIAMOND",
				end.name, arrow)
		}
		*end.target = arrow
		fields = append(fields, end.field)
	}

	return properties, fields, nil
}
