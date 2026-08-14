package tools

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// levelsMask reads what a level-by-level style copy needs: every paragraph's nesting and
// the style of the runs inside it.
const levelsMask = "slides(pageElements(objectId,shape(text(textElements(startIndex,endIndex," +
	"paragraphMarker(bullet(nestingLevel)),textRun(content,style(" +
	google.TextStyleFields + ",link)))))))"

// paragraphRun is one paragraph of a text box: how deep it sits and what its text looks
// like.
type paragraphRun struct {
	Level int
	Start int64
	End   int64
	Style google.TextStyle
}

// readParagraphs reads a text box as paragraphs with their nesting and their style.
func readParagraphs(ctx context.Context, client *google.Client, presentationID, objectID string) ([]paragraphRun, error) {
	presentation, err := client.Presentation(ctx, presentationID, levelsMask)
	if err != nil {
		return nil, err
	}

	shape, err := findShape(presentation, objectID)
	if err != nil {
		return nil, err
	}

	var paragraphs []paragraphRun
	var current *paragraphRun

	for _, element := range shapeElements(shape) {
		switch {
		case element.ParagraphMarker != nil:
			run := paragraphRun{Level: 0}
			if element.StartIndex != nil {
				run.Start = *element.StartIndex
			}
			if element.EndIndex != nil {
				// The end index of a paragraph includes its newline, and styling that
				// newline is what bleeds a style into the next paragraph.
				run.End = *element.EndIndex - 1
			}
			if bullet := element.ParagraphMarker.Bullet; bullet != nil {
				// A top-level bullet reports no nesting level at all, which is level 0
				// with a bullet — one deeper than a line with no bullet at all.
				run.Level = 1
				if bullet.NestingLevel != nil {
					run.Level = *bullet.NestingLevel + 1
				}
			}

			paragraphs = append(paragraphs, run)
			current = &paragraphs[len(paragraphs)-1]

		case current != nil && element.TextRun != nil:
			if current.Style.IsEmpty() && element.TextRun.Style != nil &&
				strings.TrimSpace(element.TextRun.Content) != "" {
				current.Style = *element.TextRun.Style
				// A link belongs to the run it was on, not to the level, and copying it
				// onto every paragraph of that level would point them all at one page.
				current.Style.Link = nil
			}
		}
	}

	return paragraphs, nil
}

// registerSlidesLinks adds the tool that puts a link on a piece of text.
func (r *registry) registerSlidesLinks(srv *server.MCPServer) {
	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_slides_set_text_style",
		mcp.WithDescription("Set the style of text in a box directly: size, font, weight, colour, alignment — "+
			"on the whole box, on its first line, or on one nesting level of its list. "+
			"Use it when the sample's value is known but cannot be copied, which is the usual case for "+
			"sizes: a sample inherits its size from a layout that the target deck does not have, so the "+
			"number has to be set rather than copied. Read it with gdocs_slides_inspect_title_style, "+
			"which reports inherited values too."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Text box to style.")),
		mcp.WithString("scope", mcp.DefaultString(scopeAll), mcp.Description(
			"Which part to style: "+scopeAll+" for the whole box, "+scopeTitle+" for its first line, "+
				"or a number for one nesting level — 0 is the line outside the list, 1 the top-level "+
				"bullets, 2 the ones under them. "+
				"paragraph:0, paragraph:1 and so on style a single paragraph by number, which is what "+
				"a box holding a bold heading, a plain body and a grey footnote needs: those are all "+
				"one level, so a level cannot tell them apart. "+
				"range styles exactly the stretch named by start_index and end_index — the form "+
				"gdocs_slides_inspect_text_structure reports its runs in, so a style read off one deck "+
				"can be written into another without anything in between.")),
		mcp.WithNumber("start_index", mcp.Description(
			"With scope=range: where the stretch begins, counted in UTF-16 code units the way the API "+
				"counts. For Russian text a byte count is twice too large and lands mid-character.")),
		mcp.WithNumber("end_index", mcp.Description("With scope=range: where the stretch ends, exclusive.")),
		mcp.WithNumber("font_size", mcp.Description("Size in points.")),
		mcp.WithString("font_family", mcp.Description("Font family, e.g. Rubik.")),
		mcp.WithBoolean("bold", mcp.Description("Bold or not.")),
		mcp.WithBoolean("italic", mcp.Description("Italic or not.")),
		mcp.WithBoolean("underline", mcp.Description("Underlined or not.")),
		mcp.WithBoolean("strikethrough", mcp.Description("Struck through or not.")),
		mcp.WithBoolean("small_caps", mcp.Description("Small capitals or not.")),
		mcp.WithString("baseline_offset", mcp.Description(
			"NONE, SUPERSCRIPT or SUBSCRIPT — the footnote marks a real deck uses.")),
		mcp.WithNumber("font_weight", mcp.Description(
			"Numeric weight of the font, in hundreds: 400 is regular, 700 bold. It travels with "+
				"font_family; a family set without it can come out lighter than the sample.")),
		mcp.WithObject("background_color", mcp.Description(
			"Highlight behind the words as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}. Not the "+
				"shape's fill — that is gdocs_slides_style_shape.")),
		mcp.WithObject("foreground_color", mcp.Description(
			"Text colour as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}.")),
		mcp.WithString("theme_color", mcp.Description(
			"Text colour by theme name — LIGHT1, DARK1, ACCENT1…ACCENT6, HYPERLINK — instead of a value. "+
				"This is what a sample reports when its author picked a colour from the theme's row rather "+
				"than a custom one, and copying it as a literal colour stops it following the theme.")),
		mcp.WithString("alignment", mcp.Description("Paragraph alignment: START, CENTER, END or JUSTIFIED.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesSetTextStyle)

	srv.AddTool(mcp.NewTool("gdocs_slides_reset_text_style",
		mcp.WithDescription("Give text back to its layout: clear the named style fields from a text box so "+
			"the font, size, weight or colour come from the template again. "+
			"Use it when a copied or hand-set style turned out wrong — clearing is the only way back, "+
			"because there is no value that means \"inherit\"."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Text box to clear.")),
		mcp.WithArray("fields", mcp.WithStringItems(), mcp.Description(
			"Style fields to clear, e.g. bold, fontSize, fontFamily, weightedFontFamily, foregroundColor. "+
				"Without any, all of them are cleared.")),
		mcp.WithString("scope", mcp.DefaultString(scopeAll), mcp.Description(
			"Which text: "+scopeAll+" for the whole box, "+scopeRange+" with start_index and end_index "+
				"for one stretch, "+scopeTitle+" for the first line, a nesting level, or paragraph:0, "+
				"paragraph:1 and so on. A sample whose bullet is bold up to the colon and plain after it "+
				"is reproduced by bolding the first words and clearing the rest — clearing the whole box "+
				"would take the bold with it.")),
		mcp.WithNumber("start_index", mcp.Description(
			"First UTF-16 code unit to clear, with scope "+scopeRange+".")),
		mcp.WithNumber("end_index", mcp.Description(
			"End of the stretch, exclusive, with scope "+scopeRange+".")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesResetTextStyle)

	srv.AddTool(mcp.NewTool("gdocs_slides_link_text",
		mcp.WithDescription("Turn a piece of text in a text box into a link. "+
			"Rebuilding a slide replaces its text, and a link in the original — the incident number, the "+
			"board, the document — is not part of that text: it has to be put back, or the new slide "+
			"quietly loses what the old one pointed at. gdocs_slides_inspect_text_structure reports the "+
			"links a sample has."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Text box holding the text.")),
		mcp.WithArray("links", mcp.Required(), mcp.Description(
			"Links to place, as a list of objects: {\"text\": \"PM-101\", \"url\": \"https://…\"}. "+
				"Each text is found in the box and the first occurrence becomes the link."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string"},
					"url":  map[string]any{"type": "string"},
				},
				"required": []string{"text", "url"},
			})),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesLinkText)
}

// slidesSetTextStyle sets a style outright, for the cases a copy cannot cover.
func (r *registry) slidesSetTextStyle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	style := &google.TextStyle{}
	var fields []string

	if size := req.GetFloat("font_size", 0); size > 0 {
		style.FontSize = google.PT(size)
		fields = append(fields, "fontSize")
	}
	if family := optionalString(req, "font_family"); family != "" {
		style.FontFamily = family
		fields = append(fields, "fontFamily")
	}
	for _, flag := range []struct {
		name   string
		field  string
		target **bool
	}{
		{"bold", "bold", &style.Bold},
		{"italic", "italic", &style.Italic},
		{"underline", "underline", &style.Underline},
		{"strikethrough", "strikethrough", &style.Strikethrough},
		{"small_caps", "smallCaps", &style.SmallCaps},
	} {
		if _, ok := req.GetArguments()[flag.name]; !ok {
			continue
		}
		value := req.GetBool(flag.name, false)
		*flag.target = &value
		fields = append(fields, flag.field)
	}

	if offset := strings.ToUpper(optionalString(req, "baseline_offset")); offset != "" {
		switch offset {
		case "NONE", "SUPERSCRIPT", "SUBSCRIPT":
		default:
			return toolError(fmt.Errorf("baseline_offset %q is not NONE, SUPERSCRIPT or SUBSCRIPT", offset)), nil
		}
		style.BaselineOffset = offset
		fields = append(fields, "baselineOffset")
	}

	// The weight goes with the family, in one field: Slides carries them together, and a
	// weight sent without a family has nothing to weigh.
	if weight := int(req.GetFloat("font_weight", 0)); weight > 0 {
		family := style.FontFamily
		if family == "" {
			return toolError(fmt.Errorf("font_weight needs font_family: the weight is part of the font, " +
				"not a setting of its own")), nil
		}
		style.WeightedFontFamily = &google.WeightedFontFamily{FontFamily: family, Weight: weight}
		fields = append(fields, "weightedFontFamily")
	}

	background, err := parseColor(req, "background_color")
	if err != nil {
		return toolError(err), nil
	}
	if background != nil {
		style.BackgroundColor = &google.OptionalColor{OpaqueColor: &google.OpaqueColor{RGBColor: background}}
		fields = append(fields, "backgroundColor")
	}

	colour, err := parseColor(req, "foreground_color")
	if err != nil {
		return toolError(err), nil
	}
	themeColor, err := paletteColor(req, "theme_color")
	if err != nil {
		return toolError(err), nil
	}

	if colour != nil && themeColor != "" {
		return toolError(fmt.Errorf("foreground_color and theme_color are alternatives: name one")), nil
	}
	switch {
	case colour != nil:
		style.ForegroundColor = &google.OptionalColor{OpaqueColor: &google.OpaqueColor{RGBColor: colour}}
		fields = append(fields, "foregroundColor")
	case themeColor != "":
		style.ForegroundColor = &google.OptionalColor{OpaqueColor: &google.OpaqueColor{ThemeColor: themeColor}}
		fields = append(fields, "foregroundColor")
	}

	alignment := strings.ToUpper(optionalString(req, "alignment"))
	switch alignment {
	case "", "START", "CENTER", "END", "JUSTIFIED":
	default:
		return toolError(fmt.Errorf("alignment %q is not one of START, CENTER, END, JUSTIFIED", alignment)), nil
	}

	if len(fields) == 0 && alignment == "" {
		return toolError(fmt.Errorf("nothing to set: name font_size, font_family, bold, italic, " +
			"underline, foreground_color, theme_color or alignment")), nil
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

	var requests []google.Request
	for _, textRange := range ranges {
		if len(fields) > 0 {
			applied := *style
			requests = append(requests, google.Request{UpdateTextStyle: &google.UpdateTextStyleRequest{
				ObjectID:  objectID,
				TextRange: textRange,
				Style:     &applied,
				Fields:    strings.Join(fields, ","),
			}})
		}
		if alignment != "" {
			requests = append(requests, google.Request{UpdateParagraphStyle: &google.UpdateParagraphStyleRequest{
				ObjectID:  objectID,
				TextRange: textRange,
				Style:     &google.ParagraphStyle{Alignment: alignment},
				Fields:    "alignment",
			}})
		}
	}

	if len(requests) == 0 {
		return toolError(fmt.Errorf("%s has nothing at that scope", objectID)), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"scope":           scope,
		"fields":          strings.Join(fields, ","),
		"alignment":       alignment,
		"ranges":          len(ranges),
		"replies":         len(response.Replies),
	})
}

// styleRanges turns a scope into the ranges to apply a style over.
func styleRanges(ctx context.Context, client *google.Client, req mcp.CallToolRequest,
	presentationID, objectID, scope string) ([]*google.Range, error) {
	switch scope {
	case scopeAll, "":
		return []*google.Range{google.AllText()}, nil

	case scopeRange:
		// An explicit stretch, in the units the reading reports. This is what makes the
		// pair symmetrical: everything gdocs_slides_inspect_text_structure describes can
		// be written back with the numbers it gave, without a copying tool in between.
		arguments := req.GetArguments()
		_, hasStart := arguments["start_index"]
		_, hasEnd := arguments["end_index"]
		if !hasStart || !hasEnd {
			return nil, fmt.Errorf("scope %s needs start_index and end_index", scopeRange)
		}

		start := int64(req.GetFloat("start_index", 0))
		end := int64(req.GetFloat("end_index", 0))
		if start < 0 || end <= start {
			return nil, fmt.Errorf("the range [%d,%d) is empty or backwards", start, end)
		}

		return []*google.Range{google.FixedRange(start, end)}, nil

	case scopeTitle:
	}

	// A single paragraph by number, counted from zero. Real slides put a bold heading, a
	// plain body and a grey footnote in one box, and styling them apart is the difference
	// between a copy and a block of uniform text. Levels cannot separate them: paragraphs
	// with no bullet all sit at the same level.
	if number, ok := strings.CutPrefix(scope, "paragraph:"); ok {
		index, err := strconv.Atoi(number)
		if err != nil || index < 0 {
			return nil, fmt.Errorf("scope %q: a paragraph is named as paragraph:0, paragraph:1 and so on", scope)
		}

		paragraphs, err := readParagraphs(ctx, client, presentationID, objectID)
		if err != nil {
			return nil, err
		}
		if index >= len(paragraphs) {
			return nil, fmt.Errorf("%s has %d paragraphs, so there is no paragraph:%d",
				objectID, len(paragraphs), index)
		}

		paragraph := paragraphs[index]
		if paragraph.End <= paragraph.Start {
			return nil, fmt.Errorf("paragraph:%d of %s is empty", index, objectID)
		}

		return []*google.Range{google.FixedRange(paragraph.Start, paragraph.End)}, nil
	}

	switch scope {
	case scopeTitle:
		text, err := shapeTextOf(ctx, client, presentationID, objectID)
		if err != nil {
			return nil, err
		}
		end := firstLineEnd(text)
		if end == 0 {
			return nil, fmt.Errorf("%s starts with an empty line", objectID)
		}
		return []*google.Range{google.FixedRange(0, end)}, nil
	}

	level, err := strconv.Atoi(scope)
	if err != nil || level < 0 {
		return nil, fmt.Errorf("scope %q is not %s, %s, or a nesting level like 0, 1 or 2",
			scope, scopeAll, scopeTitle)
	}

	paragraphs, err := readParagraphs(ctx, client, presentationID, objectID)
	if err != nil {
		return nil, err
	}

	var ranges []*google.Range
	for _, paragraph := range paragraphs {
		if paragraph.Level == level && paragraph.End > paragraph.Start {
			ranges = append(ranges, google.FixedRange(paragraph.Start, paragraph.End))
		}
	}

	if len(ranges) == 0 {
		return nil, fmt.Errorf("%s has no paragraphs at level %d", objectID, level)
	}

	return ranges, nil
}

// slidesResetTextStyle clears style fields so the layout decides again.
func (r *registry) slidesResetTextStyle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	fields := req.GetStringSlice("fields", nil)
	if len(fields) == 0 {
		for name := range styleFieldNames {
			if name != "link" {
				fields = append(fields, name)
			}
		}
		sort.Strings(fields)
	}

	for _, field := range fields {
		if !styleFieldNames[field] {
			return toolError(fmt.Errorf("%q is not a text style field", field)), nil
		}
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	// Clearing a stretch matters as much as clearing a box. A sample whose bullet reads
	// "Итог:" in bold and the rest plain is reproduced by bolding the first words — but
	// only if the rest can be given back to what it inherits, and a whole-box reset would
	// take the heading's bold with it.
	scope := strings.ToLower(optionalString(req, "scope"))
	ranges, err := styleRanges(ctx, client, req, presentationID, objectID, scope)
	if err != nil {
		return toolError(err), nil
	}

	// An empty style with a mask is how the API is told "unset these": there is no value
	// that means inherited.
	requests := make([]google.Request, 0, len(ranges))
	for _, textRange := range ranges {
		requests = append(requests, google.Request{
			UpdateTextStyle: &google.UpdateTextStyleRequest{
				ObjectID:  objectID,
				TextRange: textRange,
				Style:     &google.TextStyle{},
				Fields:    strings.Join(fields, ","),
			},
		})
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"cleared":         fields,
		"ranges":          len(ranges),
		"replies":         len(response.Replies),
	})
}

// slidesLinkText puts links back on the text of a rebuilt slide.
func (r *registry) slidesLinkText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	links, err := objectList(req, "links")
	if err != nil {
		return toolError(err), nil
	}
	if len(links) == 0 {
		return toolError(fmt.Errorf("links is empty")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	text, err := shapeTextOf(ctx, client, presentationID, objectID)
	if err != nil {
		return toolError(err), nil
	}

	var requests []google.Request
	placed := make([]map[string]any, 0, len(links))

	for index, entry := range links {
		needle := stringField(entry, "text")
		address := stringField(entry, "url")

		if needle == "" || address == "" {
			return toolError(fmt.Errorf("links[%d] needs both text and url", index)), nil
		}
		if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
			return toolError(fmt.Errorf("links[%d].url is %q: a link has to be an http or https address",
				index, address)), nil
		}

		position := strings.Index(text, needle)
		if position < 0 {
			return toolError(fmt.Errorf("links[%d]: %q is not in this text box. "+
				"Put the text in first, then link it", index, needle)), nil
		}

		// Indexes are counted the way the API counts them, in UTF-16 code units, so the
		// range is measured on the text before the match rather than on its bytes.
		start := utf16Length(text[:position])
		end := start + utf16Length(needle)

		requests = append(requests, google.Request{
			UpdateTextStyle: &google.UpdateTextStyleRequest{
				ObjectID:  objectID,
				TextRange: google.FixedRange(start, end),
				Style:     &google.TextStyle{Link: &google.Link{URL: address}},
				Fields:    "link",
			},
		})

		placed = append(placed, map[string]any{"text": needle, "url": address, "start": start, "end": end})
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"links":           placed,
		"replies":         len(response.Replies),
	})
}
