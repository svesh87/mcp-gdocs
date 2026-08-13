package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// themeMask reads a deck's styling rather than its content: the palette, the master's
// background, and the placeholders of every layout with the styles they impose.
//
// This is where a slide's look actually lives. A title that reports no size of its own is
// not sizeless — it is 28 pt because the layout says so, and a deck built without reading
// the layouts is a deck whose every size has to be set by hand on every slide.
const themeMask = "presentationId,title,pageSize," +
	"masters(objectId,pageProperties(colorScheme,pageBackgroundFill)," +
	"pageElements(objectId,size,transform,shape(shapeType,placeholder," +
	"shapeProperties(shapeBackgroundFill,outline,contentAlignment,autofit)," +
	"text(textElements(paragraphMarker(style),textRun(content,style(" + google.TextStyleFields + ")))))))," +
	"layouts(objectId,layoutProperties(name,displayName),pageProperties(pageBackgroundFill)," +
	"pageElements(objectId,size,transform,shape(shapeType,placeholder," +
	"shapeProperties(shapeBackgroundFill,outline,contentAlignment,autofit)," +
	"text(textElements(paragraphMarker(style),textRun(content,style(" + google.TextStyleFields + ")))))))"

// registerSlidesTheme adds the tools that read and write a deck's styling: its palette,
// the styles its layouts impose, and the paragraph spacing that decides whether a block
// of text looks like the sample's or merely says the same words.
func (r *registry) registerSlidesTheme(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_slides_read_theme",
		mcp.WithDescription("Read a deck's styling: the twelve theme colours, the master's background and "+
			"default text style, and every layout with its placeholders — their boxes and the font, size, "+
			"weight and colour each one imposes. This is what a slide inherits, so it is what a deck built "+
			"from scratch has to set, and what a deck copied from a sample has to match. "+
			"Read the sample's theme before building anything: sizes set on the layout do not need setting "+
			"on every slide, and sizes the layout does not set have to be."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.slidesReadTheme)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_slides_set_paragraph_style",
		mcp.WithDescription("Set how paragraphs sit in a text box: alignment, line spacing, the space above and "+
			"below them, and their indents. gdocs_slides_set_text_style handles the letters; this handles "+
			"the room around them, which is half of why a rebuilt slide looks different from its sample "+
			"even when every font matches."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Text box or placeholder to style.")),
		mcp.WithString("scope", mcp.DefaultString(scopeAll), mcp.Description(
			"Which paragraphs: "+scopeAll+" for all of them, "+scopeTitle+" for the first line, "+
				"a nesting level like 0, 1 or 2 for one level of a list, or paragraph:0, paragraph:1 "+
				"and so on for a single paragraph by number.")),
		mcp.WithString("alignment", mcp.Description("START, CENTER, END or JUSTIFIED.")),
		mcp.WithNumber("line_spacing", mcp.Description(
			"Line spacing as a percentage: 100 is single, 150 is one and a half.")),
		mcp.WithNumber("space_above_pt", mcp.Description("Space before each paragraph, in points.")),
		mcp.WithNumber("space_below_pt", mcp.Description("Space after each paragraph, in points.")),
		mcp.WithNumber("indent_start_emu", mcp.Description("Indent of the whole paragraph from the left, in EMU.")),
		mcp.WithNumber("indent_first_line_emu", mcp.Description("Indent of the first line only, in EMU.")),
		mcp.WithString("direction", mcp.Description("LEFT_TO_RIGHT or RIGHT_TO_LEFT.")),
		mcp.WithString("spacing_mode", mcp.Description(
			"Whether the space above a paragraph survives beside the space below the one before it: "+
				"NEVER_COLLAPSE keeps both, COLLAPSE_LISTS — the default — merges them inside lists. "+
				"A sample that sets ten points above its headings and NEVER_COLLAPSE renders them ten "+
				"points lower than a copy that sets only the ten points, and every paragraph after them "+
				"inherits the difference.")),
		mcp.WithString("bullet_preset", mcp.Description(
			"Make these paragraphs a list again with this preset, e.g. "+defaultBulletPreset+". "+
				"A bullet takes its colour and size from the text of its paragraph at the moment it is "+
				"made, so a list built before the text was styled keeps black markers over coloured "+
				"words. Style the text first, then name this to have the markers made again.")),
		mcp.WithBoolean("remove_bullets", mcp.Description(
			"Strip the list markers from these paragraphs, leaving the text.")),
		mcp.WithArray("reset", mcp.WithStringItems(), mcp.Description(
			"Fields to give back to the layout: alignment, lineSpacing, spaceAbove, spaceBelow, "+
				"indentStart, indentFirstLine, indentEnd, direction. A sample that sets none of these "+
				"is not a sample with no spacing — it inherits, and a copy that has them set explicitly "+
				"stays different however close the numbers are. There is no value meaning \"inherit\": "+
				"clearing is the only way back.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesSetParagraphStyle)

	srv.AddTool(mcp.NewTool("gdocs_slides_style_layout",
		mcp.WithDescription("Write a style into a layout or the master, instead of onto each slide. A title set "+
			"to 25 pt on the layout is 25 pt on every slide that follows it, including slides made later; "+
			"the same size set slide by slide has to be set again on the next one. This is how a deck gets "+
			"a look of its own rather than a look pasted onto each page. "+
			"Read the target first with gdocs_slides_read_theme — it lists every layout and the identifier "+
			"of every placeholder on it."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description(
			"Placeholder on a layout or master, by identifier from gdocs_slides_read_theme.")),
		mcp.WithNumber("font_size", mcp.Description("Font size in points.")),
		mcp.WithString("font_family", mcp.Description("Font family, e.g. Rubik.")),
		mcp.WithBoolean("bold", mcp.Description("Bold the text.")),
		mcp.WithBoolean("italic", mcp.Description("Italicise the text.")),
		mcp.WithObject("foreground_color", mcp.Description(
			"Text colour as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}.")),
		mcp.WithString("theme_color", mcp.Description(
			"Text colour by theme name — DARK1, LIGHT1, ACCENT1…ACCENT6, HYPERLINK — instead of a value. "+
				"A colour set this way follows the theme when it changes; a literal one does not.")),
		mcp.WithString("alignment", mcp.Description("Paragraph alignment: START, CENTER, END or JUSTIFIED.")),
		mcp.WithNumber("line_spacing", mcp.Description("Line spacing as a percentage: 100 is single.")),
		mcp.WithNumber("space_above_pt", mcp.Description("Space before each paragraph, in points.")),
		mcp.WithNumber("space_below_pt", mcp.Description("Space after each paragraph, in points.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesStyleLayout)

	srv.AddTool(mcp.NewTool("gdocs_slides_set_theme_colors",
		mcp.WithDescription("Replace a deck's palette: the twelve colours every theme colour name refers to. "+
			"Slides only accepts a whole palette and only on the master, so pass all twelve — "+
			"gdocs_slides_read_theme reports the current ones, and changing one accent means sending the "+
			"other eleven back unchanged. Everything that referred to a name follows the change; anything "+
			"painted with a literal colour does not."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("master_object_id", mcp.Description(
			"Master to write the palette to. With one master, which is usual, it can be left out.")),
		mcp.WithObject("colors", mcp.Required(), mcp.Description(
			"All twelve colours by name, as hex: {\"DARK1\": \"#000000\", \"LIGHT1\": \"#FFFFFF\", "+
				"\"DARK2\", \"LIGHT2\", \"ACCENT1\"…\"ACCENT6\", \"HYPERLINK\", \"FOLLOWED_HYPERLINK\"}.")),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesSetThemeColors)
}

// describedPlaceholder is one slot of a layout with the style it imposes.
type describedPlaceholder struct {
	ObjectID    string `json:"object_id"`
	Placeholder string `json:"placeholder,omitempty"`
	Index       *int   `json:"placeholder_index,omitempty"`
	ShapeType   string `json:"shape_type,omitempty"`
	// Text is the prompt the layout carries, like "Click to add title". It is reported
	// because it is what shows on a slide where the placeholder was never filled.
	Text   string  `json:"text,omitempty"`
	X      float64 `json:"x_emu"`
	Y      float64 `json:"y_emu"`
	Width  float64 `json:"width_emu"`
	Height float64 `json:"height_emu"`

	FontSize   float64 `json:"font_size_pt,omitempty"`
	FontFamily string  `json:"font_family,omitempty"`
	Bold       *bool   `json:"bold,omitempty"`
	Italic     *bool   `json:"italic,omitempty"`
	Color      string  `json:"text_color,omitempty"`
	ThemeColor string  `json:"theme_color,omitempty"`
	Alignment  string  `json:"alignment,omitempty"`
	// LineSpacing is a percentage, the way Slides reports it: 100 is single spacing.
	LineSpacing  float64        `json:"line_spacing,omitempty"`
	SpaceAbovePT float64        `json:"space_above_pt,omitempty"`
	SpaceBelowPT float64        `json:"space_below_pt,omitempty"`
	Fill         *describedFill `json:"fill,omitempty"`
	AutofitType  string         `json:"autofit_type,omitempty"`
	FontScale    float64        `json:"font_scale,omitempty"`
	ContentAlign string         `json:"content_alignment,omitempty"`
	InheritsFrom string         `json:"inherits_from,omitempty"`
}

// describedLayout is a layout with everything a slide following it will inherit.
type describedLayout struct {
	ObjectID     string                 `json:"object_id"`
	Name         string                 `json:"name,omitempty"`
	Background   *describedFill         `json:"background,omitempty"`
	Placeholders []describedPlaceholder `json:"placeholders"`
}

// slidesReadTheme reads a deck's styling.
func (r *registry) slidesReadTheme(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.Presentation(ctx, presentationID, themeMask)
	if err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{
		"presentation_id": presentation.PresentationID,
		"title":           presentation.Title,
	}

	if presentation.PageSize != nil && presentation.PageSize.Width != nil && presentation.PageSize.Height != nil {
		payload["page_size_emu"] = map[string]float64{
			"width":  presentation.PageSize.Width.Magnitude,
			"height": presentation.PageSize.Height.Magnitude,
		}
	}

	masters := make([]map[string]any, 0, len(presentation.Masters))
	for index := range presentation.Masters {
		master := presentation.Masters[index]
		entry := map[string]any{
			"object_id":    master.ObjectID,
			"placeholders": describePlaceholders(master.PageElements),
		}
		if background := describeFill(pageBackgroundFill(&master)); background != nil {
			entry["background"] = background
		}
		if master.PageProperties != nil && master.PageProperties.ColorScheme != nil {
			entry["colors"] = describeColorScheme(master.PageProperties.ColorScheme)
		}
		masters = append(masters, entry)
	}
	payload["masters"] = masters

	layouts := make([]describedLayout, 0, len(presentation.Layouts))
	for index := range presentation.Layouts {
		layout := presentation.Layouts[index]
		described := describedLayout{
			ObjectID:     layout.ObjectID,
			Background:   describeFill(pageBackgroundFill(&layout)),
			Placeholders: describePlaceholders(layout.PageElements),
		}
		if layout.LayoutProperties != nil {
			described.Name = layout.LayoutProperties.DisplayName
			if described.Name == "" {
				described.Name = layout.LayoutProperties.Name
			}
		}
		layouts = append(layouts, described)
	}
	payload["layouts"] = layouts

	return resultJSON(payload)
}

// describeColorScheme renders a palette as names to hex.
func describeColorScheme(scheme *google.ColorScheme) map[string]string {
	colors := map[string]string{}
	for _, pair := range scheme.Colors {
		colors[pair.Type] = slideColor(pair.Color)
	}

	return colors
}

// describePlaceholders reports the slots of a layout or master with the style each imposes.
func describePlaceholders(elements []google.PageElement) []describedPlaceholder {
	described := make([]describedPlaceholder, 0, len(elements))

	for index := range elements {
		element := elements[index]
		if element.Shape == nil {
			continue
		}

		entry := describedPlaceholder{
			ObjectID:  element.ObjectID,
			ShapeType: element.Shape.ShapeType,
			Text:      shapeText(element.Shape),
		}

		if placeholder := element.Shape.Placeholder; placeholder != nil {
			entry.Placeholder = placeholder.Type
			entry.Index = placeholder.Index
			entry.InheritsFrom = placeholder.ParentObjectID
		}

		if width, height, err := elementBox(&element); err == nil {
			entry.Width, entry.Height = width, height
		}
		if element.Transform != nil {
			entry.X, entry.Y = element.Transform.TranslateX, element.Transform.TranslateY
		}

		if properties := element.Shape.Properties; properties != nil {
			entry.ContentAlign = properties.ContentAlignment
			if properties.BackgroundFill != nil {
				entry.Fill = describeFill(&google.PageBackgroundFill{
					PropertyState: properties.BackgroundFill.PropertyState,
					SolidFill:     properties.BackgroundFill.SolidFill,
				})
			}
			if autofit := properties.Autofit; autofit != nil {
				entry.AutofitType, entry.FontScale = autofit.AutofitType, autofit.FontScale
			}
		}

		// The style of the placeholder's own prompt text is the style it imposes: that is
		// what a slide's text inherits when it sets nothing of its own.
		for _, item := range shapeElements(element.Shape) {
			if marker := item.ParagraphMarker; marker != nil && marker.Style != nil {
				if entry.Alignment == "" {
					entry.Alignment = marker.Style.Alignment
				}
				if entry.LineSpacing == 0 {
					entry.LineSpacing = marker.Style.LineSpacing
				}
				if entry.SpaceAbovePT == 0 && marker.Style.SpaceAbove != nil {
					entry.SpaceAbovePT = marker.Style.SpaceAbove.InPoints()
				}
				if entry.SpaceBelowPT == 0 && marker.Style.SpaceBelow != nil {
					entry.SpaceBelowPT = marker.Style.SpaceBelow.InPoints()
				}
			}

			run := item.TextRun
			if run == nil || run.Style == nil {
				continue
			}
			style := run.Style
			if entry.FontSize == 0 && style.FontSize != nil {
				entry.FontSize = style.FontSize.InPoints()
			}
			if entry.FontFamily == "" {
				entry.FontFamily = style.FontFamily
			}
			if entry.Bold == nil {
				entry.Bold = style.Bold
			}
			if entry.Italic == nil {
				entry.Italic = style.Italic
			}
			if entry.Color == "" && entry.ThemeColor == "" &&
				style.ForegroundColor != nil && style.ForegroundColor.OpaqueColor != nil {
				entry.Color = slideColor(style.ForegroundColor.OpaqueColor.RGBColor)
				entry.ThemeColor = style.ForegroundColor.OpaqueColor.ThemeColor
			}
		}

		described = append(described, entry)
	}

	return described
}

// paragraphStyleFields is every field a paragraph style request may name, for the reset
// list. A field outside it is refused here rather than by the API, where the error names
// a mask and not the argument that produced it.
var paragraphStyleFields = map[string]bool{
	"alignment": true, "lineSpacing": true, "spaceAbove": true, "spaceBelow": true,
	"indentStart": true, "indentEnd": true, "indentFirstLine": true, "direction": true,
	"spacingMode": true,
}

// namesField says whether a mask already carries a field, so a field both set and reset
// is named once rather than twice.
func namesField(fields []string, want string) bool {
	for _, field := range fields {
		if field == want {
			return true
		}
	}

	return false
}

// paragraphStyleFrom reads a paragraph style out of the arguments, with its mask.
func paragraphStyleFrom(req mcp.CallToolRequest) (*google.ParagraphStyle, []string, error) {
	style := &google.ParagraphStyle{}
	var fields []string

	alignment := strings.ToUpper(optionalString(req, "alignment"))
	switch alignment {
	case "":
	case "START", "CENTER", "END", "JUSTIFIED":
		style.Alignment = alignment
		fields = append(fields, "alignment")
	default:
		return nil, nil, fmt.Errorf("alignment %q is not one of START, CENTER, END, JUSTIFIED", alignment)
	}

	if spacing := req.GetFloat("line_spacing", 0); spacing > 0 {
		// Slides counts line spacing in percent, not multiples: single spacing is 100.
		// A caller passing 1.5 would get lines on top of each other, so it is refused.
		if spacing < 10 {
			return nil, nil, fmt.Errorf("line_spacing is %g: it is a percentage, so single spacing is 100 "+
				"and one and a half is 150", spacing)
		}
		style.LineSpacing = spacing
		fields = append(fields, "lineSpacing")
	}

	for _, space := range []struct {
		name   string
		field  string
		target **google.Dimension
	}{
		{"space_above_pt", "spaceAbove", &style.SpaceAbove},
		{"space_below_pt", "spaceBelow", &style.SpaceBelow},
	} {
		raw, ok := req.GetArguments()[space.name]
		if !ok || raw == nil {
			continue
		}
		points := req.GetFloat(space.name, 0)
		if points < 0 {
			return nil, nil, fmt.Errorf("%s is %g: it cannot be negative", space.name, points)
		}
		*space.target = google.PT(points)
		fields = append(fields, space.field)
	}

	for _, indent := range []struct {
		name   string
		field  string
		target **google.Dimension
	}{
		{"indent_start_emu", "indentStart", &style.IndentStart},
		{"indent_first_line_emu", "indentFirstLine", &style.IndentFirstLine},
	} {
		raw, ok := req.GetArguments()[indent.name]
		if !ok || raw == nil {
			continue
		}
		*indent.target = google.EMU(req.GetFloat(indent.name, 0))
		fields = append(fields, indent.field)
	}

	if direction := strings.ToUpper(optionalString(req, "direction")); direction != "" {
		switch direction {
		case "LEFT_TO_RIGHT", "RIGHT_TO_LEFT":
		default:
			return nil, nil, fmt.Errorf("direction %q is not LEFT_TO_RIGHT or RIGHT_TO_LEFT", direction)
		}
		style.Direction = direction
		fields = append(fields, "direction")
	}

	if mode := strings.ToUpper(optionalString(req, "spacing_mode")); mode != "" {
		switch mode {
		case "NEVER_COLLAPSE", "COLLAPSE_LISTS":
		default:
			return nil, nil, fmt.Errorf("spacing_mode %q is not NEVER_COLLAPSE or COLLAPSE_LISTS", mode)
		}
		style.SpacingMode = mode
		fields = append(fields, "spacingMode")
	}

	return style, fields, nil
}

// slidesSetParagraphStyle sets the spacing and indents of paragraphs on a slide.
func (r *registry) slidesSetParagraphStyle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	style, fields, err := paragraphStyleFrom(req)
	if err != nil {
		return toolError(err), nil
	}

	// Fields named here go into the mask with nothing behind them, which is what tells
	// Slides to take the layout's value again.
	for _, field := range req.GetStringSlice("reset", nil) {
		if !paragraphStyleFields[field] {
			return toolError(fmt.Errorf("reset: %q is not a paragraph style field", field)), nil
		}
		if !namesField(fields, field) {
			fields = append(fields, field)
		}
	}

	bulletPreset := optionalString(req, "bullet_preset")
	removeBullets := req.GetBool("remove_bullets", false)
	if bulletPreset != "" && removeBullets {
		return toolError(fmt.Errorf("bullet_preset and remove_bullets are opposites: name one")), nil
	}

	if len(fields) == 0 && bulletPreset == "" && !removeBullets {
		return toolError(fmt.Errorf("nothing to set: name alignment, line_spacing, space_above_pt, " +
			"space_below_pt, indent_start_emu, indent_first_line_emu, direction, bullet_preset, " +
			"remove_bullets or reset")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	scope := strings.ToLower(req.GetString("scope", scopeAll))
	ranges, err := styleRanges(ctx, client, req, presentationID, objectID, scope)
	if err != nil {
		return toolError(err), nil
	}
	if len(ranges) == 0 {
		return toolError(fmt.Errorf("%s has nothing at that scope", objectID)), nil
	}

	requests := make([]google.Request, 0, len(ranges))
	for _, textRange := range ranges {
		if len(fields) > 0 {
			requests = append(requests, google.Request{
				UpdateParagraphStyle: &google.UpdateParagraphStyleRequest{
					ObjectID:  objectID,
					TextRange: textRange,
					Style:     style,
					Fields:    strings.Join(fields, ","),
				},
			})
		}

		// Markers last, and made from scratch: a bullet takes its colour and size from the
		// paragraph's text as it stands when the bullet is made. A list built before the
		// words were coloured keeps black markers over red text until they are remade.
		if removeBullets {
			requests = append(requests, google.Request{
				DeleteParagraphBullets: &google.DeleteParagraphBulletsRequest{
					ObjectID: objectID, TextRange: textRange,
				},
			})
		}
		if bulletPreset != "" {
			requests = append(requests,
				google.Request{DeleteParagraphBullets: &google.DeleteParagraphBulletsRequest{
					ObjectID: objectID, TextRange: textRange,
				}},
				google.Request{CreateParagraphBullets: &google.CreateParagraphBulletsRequest{
					ObjectID: objectID, TextRange: textRange, BulletPreset: bulletPreset,
				}},
			)
		}
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"object_id":       objectID,
		"scope":           scope,
		"changed":         fields,
		"replies":         len(response.Replies),
	})
}

// slidesStyleLayout writes a style into a layout or the master.
func (r *registry) slidesStyleLayout(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	style := &google.TextStyle{}
	var textFields []string

	if size := req.GetFloat("font_size", 0); size > 0 {
		style.FontSize = google.PT(size)
		textFields = append(textFields, "fontSize")
	}
	if family := optionalString(req, "font_family"); family != "" {
		style.FontFamily = family
		textFields = append(textFields, "fontFamily")
	}
	for _, flag := range []struct {
		name   string
		field  string
		target **bool
	}{
		{"bold", "bold", &style.Bold},
		{"italic", "italic", &style.Italic},
	} {
		if _, ok := req.GetArguments()[flag.name]; !ok {
			continue
		}
		value := req.GetBool(flag.name, false)
		*flag.target = &value
		textFields = append(textFields, flag.field)
	}

	colour, err := parseColor(req, "foreground_color")
	if err != nil {
		return toolError(err), nil
	}
	themeColor := strings.ToUpper(optionalString(req, "theme_color"))

	if colour != nil && themeColor != "" {
		return toolError(fmt.Errorf("foreground_color and theme_color are alternatives: name one")), nil
	}
	switch {
	case colour != nil:
		style.ForegroundColor = &google.OptionalColor{OpaqueColor: &google.OpaqueColor{RGBColor: colour}}
		textFields = append(textFields, "foregroundColor")
	case themeColor != "":
		style.ForegroundColor = &google.OptionalColor{OpaqueColor: &google.OpaqueColor{ThemeColor: themeColor}}
		textFields = append(textFields, "foregroundColor")
	}

	paragraphStyle, paragraphFields, err := paragraphStyleFrom(req)
	if err != nil {
		return toolError(err), nil
	}

	if len(textFields) == 0 && len(paragraphFields) == 0 {
		return toolError(fmt.Errorf("nothing to set: name font_size, font_family, bold, italic, " +
			"foreground_color, theme_color, alignment, line_spacing, space_above_pt or space_below_pt")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	var requests []google.Request

	// The whole placeholder, not a measured range: a layout's placeholder holds only its
	// prompt text, and the style is what slides inherit rather than what that prompt looks
	// like. Ranges measured against the prompt would style the words "Click to add title".
	if len(textFields) > 0 {
		requests = append(requests, google.Request{
			UpdateTextStyle: &google.UpdateTextStyleRequest{
				ObjectID:  objectID,
				TextRange: google.AllText(),
				Style:     style,
				Fields:    strings.Join(textFields, ","),
			},
		})
	}
	if len(paragraphFields) > 0 {
		requests = append(requests, google.Request{
			UpdateParagraphStyle: &google.UpdateParagraphStyleRequest{
				ObjectID:  objectID,
				TextRange: google.AllText(),
				Style:     paragraphStyle,
				Fields:    strings.Join(paragraphFields, ","),
			},
		})
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"object_id":       objectID,
		"changed":         append(textFields, paragraphFields...),
		"replies":         len(response.Replies),
	})
}

// slidesSetThemeColors replaces a deck's palette.
func (r *registry) slidesSetThemeColors(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	raw, ok := req.GetArguments()["colors"]
	if !ok || raw == nil {
		return toolError(fmt.Errorf("colors is required")), nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return toolError(fmt.Errorf("colors must be an object of names to hex colours, got %T", raw)), nil
	}

	scheme := &google.ColorScheme{}
	var missing []string

	for _, name := range google.ThemeColorTypes {
		value, present := object[name]
		if !present {
			missing = append(missing, name)
			continue
		}

		text, ok := value.(string)
		if !ok {
			return toolError(fmt.Errorf("colors.%s must be a hex colour like \"#1A73E8\", got %T", name, value)), nil
		}

		colour, err := parseHexColor(text)
		if err != nil {
			return toolError(fmt.Errorf("colors.%s: %w", name, err)), nil
		}

		scheme.Colors = append(scheme.Colors, google.ThemeColorPair{Type: name, Color: colour})
	}

	// All twelve or none: the API replaces the palette rather than editing it, and a
	// partial one is refused outright — better to say which are missing than to pass it on.
	if len(missing) > 0 {
		return toolError(fmt.Errorf("a palette has to carry all twelve colours, missing: %s",
			strings.Join(missing, ", "))), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	masterObjectID := optionalString(req, "master_object_id")
	if masterObjectID == "" {
		presentation, err := client.Presentation(ctx, presentationID, "masters(objectId)")
		if err != nil {
			return toolError(err), nil
		}
		switch len(presentation.Masters) {
		case 0:
			return toolError(fmt.Errorf("this presentation reports no master to write a palette to")), nil
		case 1:
			masterObjectID = presentation.Masters[0].ObjectID
		default:
			names := make([]string, 0, len(presentation.Masters))
			for _, master := range presentation.Masters {
				names = append(names, master.ObjectID)
			}
			return toolError(fmt.Errorf("this presentation has %d masters, so name master_object_id: %s",
				len(presentation.Masters), strings.Join(names, ", "))), nil
		}
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		UpdatePageProperties: &google.UpdatePagePropertiesRequest{
			ObjectID:       masterObjectID,
			PageProperties: &google.PageProperties{ColorScheme: scheme},
			Fields:         "colorScheme",
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id":  response.PresentationID,
		"master_object_id": masterObjectID,
		"colors":           len(scheme.Colors),
		"replies":          len(response.Replies),
	})
}

// parseHexColor reads a colour written the way a person writes one.
func parseHexColor(text string) (*google.RGBColor, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(text), "#")
	if len(trimmed) != 6 {
		return nil, fmt.Errorf("%q is not a hex colour like \"#1A73E8\"", text)
	}

	var components [3]float64
	for index := 0; index < 3; index++ {
		var value int
		if _, err := fmt.Sscanf(trimmed[index*2:index*2+2], "%02x", &value); err != nil {
			return nil, fmt.Errorf("%q is not a hex colour like \"#1A73E8\"", text)
		}
		components[index] = float64(value) / 255
	}

	return &google.RGBColor{Red: components[0], Green: components[1], Blue: components[2]}, nil
}
