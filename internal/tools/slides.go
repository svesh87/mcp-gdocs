package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// Default look of a nested list. Slides picks the marker per level from the preset, so
// the deck gets the markers its template would have used.
const defaultBulletPreset = "BULLET_DISC_CIRCLE_SQUARE"

// How much of a text box a style covers.
const (
	scopeTitle = "title"
	scopeAll   = "all"
	// scopeRange is the stretch the caller names, in the units the reading reports. It is
	// what makes reading and writing symmetrical: a run described by
	// gdocs_slides_inspect_text_structure is written back with the numbers it gave.
	scopeRange = "range"
)

// styleFieldNames is every field a style request may name. A caller asking to reset
// something outside this list is refused here rather than by the API, where the error
// names a field mask and not the argument that produced it.
//
// Naming a field with no value in it resets that field to what the layout says, which is
// how a size set on a slide is given back: a title that inherited a size from a template
// it no longer belongs to keeps that size until the field is named and left empty.
var styleFieldNames = map[string]bool{
	"bold": true, "italic": true, "underline": true, "strikethrough": true,
	"smallCaps": true, "baselineOffset": true, "fontFamily": true, "fontSize": true,
	"weightedFontFamily": true, "foregroundColor": true, "backgroundColor": true, "link": true,
}

// textStyleMask is the fields mask for reading a style off a shape, together with the
// layouts and masters it inherits from.
//
// The inheritance is the point. In a real deck a title sets no style of its own — the
// size, the font and the colour all come from the layout — so reading only what the slide
// says returns an empty style and tells a caller nothing about how the slide looks.
const textStyleMask = "slides(objectId,slideProperties(layoutObjectId)," +
	"pageElements(objectId,shape(placeholder(type,parentObjectId)," +
	"shapeProperties(autofit(autofitType,fontScale,lineSpacingReduction))," +
	"text(textElements(textRun(content,style(" +
	google.TextStyleFields + "))))))),layouts(objectId,pageElements(objectId," +
	"shape(placeholder(type,parentObjectId),text(textElements(textRun(style(" +
	google.TextStyleFields + "))))))),masters(objectId,pageElements(objectId," +
	"shape(placeholder(type,parentObjectId),text(textElements(textRun(style(" +
	google.TextStyleFields + ")))))))"

// textContentMask reads just the text of the shapes on every slide.
const textContentMask = "slides(pageElements(objectId,shape(text(textElements(textRun(content))))))"

// structureMask reads paragraphs with their bullets and the links inside them: what is in
// a text box before it is changed. Links are part of that — a rebuilt slide that lost the
// link to the incident it describes is a slide somebody has to fix by hand.
// The paragraph marker's whole style is read, not just its alignment: line spacing and
// the space around a paragraph decide how a block of text sits as much as the font size
// does, and a copy made without them comes out tighter or looser than its sample with
// every font matching.
const structureMask = "slides(pageElements(objectId,shape(text(textElements(startIndex,endIndex," +
	"paragraphMarker(bullet(nestingLevel,glyph,bulletStyle(foregroundColor,fontSize))," +
	"style),textRun(content,style(" +
	google.TextStyleFields + ",link)))))))"

// pagesMask lists the slides and what sits on them.
const pagesMask = "presentationId,title,pageSize,layouts(objectId,layoutProperties(name,displayName))," +
	"slides(objectId,slideProperties(layoutObjectId,isSkipped),layoutProperties(name,displayName)," +
	"pageElements(objectId,title,size,transform,shape(shapeType,placeholder,text(textElements(textRun(content))))," +
	"table(rows,columns,tableColumns(columnWidth),tableRows(rowHeight)),image(contentUrl),video(url),line(lineType)," +
	"sheetsChart(spreadsheetId,chartId,contentUrl)))"

// layoutsMask lists the layouts a new slide can follow.
const layoutsMask = "layouts(objectId,layoutProperties(name,displayName))"

func (r *registry) registerSlides(srv *server.MCPServer) {
	r.registerSlidesLayout(srv)
	r.registerSlidesCopy(srv)
	r.registerCopyBridges(srv)
	r.registerSlidesLinks(srv)
	r.registerSlidesPage(srv)
	r.registerSlidesShape(srv)
	r.registerSlidesTheme(srv)
	r.registerSlidesExtra(srv)

	srv.AddTool(mcp.NewTool("gdocs_slides_inspect_text_structure",
		mcp.WithDescription("Read what is inside one text box of a slide: its paragraphs, their text, "+
			"and how deep each one sits in the list. Look here before changing a slide — the object "+
			"identifiers and the existing structure are what every other slides tool takes as input."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Object identifier of the text box.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.slidesInspectTextStructure)

	srv.AddTool(mcp.NewTool("gdocs_slides_inspect_title_style",
		mcp.WithDescription("Read the style of the first line of a text box: font, size, weight and colours. "+
			"This is what gdocs_slides_copy_title_style would copy, and reading it first is how a caller "+
			"finds out whether two slides really differ."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Object identifier of the text box.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.slidesInspectTitleStyle)

	srv.AddTool(mcp.NewTool("gdocs_slides_list",
		mcp.WithDescription("List the slides of a presentation with the objects on each: identifiers, kinds, "+
			"placeholders, sizes and the beginning of any text. This is the map a caller needs before it "+
			"can address anything on a slide."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.slidesList)

	srv.AddTool(mcp.NewTool("gdocs_slides_list_layouts",
		mcp.WithDescription("List the layouts of a presentation, which are the slide kinds its template offers. "+
			"Adding a slide means naming one of these, and a deck built on its own template's layouts is "+
			"the one that keeps looking right."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.slidesListLayouts)

	srv.AddTool(mcp.NewTool("gdocs_slides_export_thumbnail",
		mcp.WithDescription("Render one slide to a picture and return the address it can be fetched from. "+
			"Use it to look at what a change actually did instead of trusting the batch that made it. "+
			"The address is short-lived."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description("Object identifier of the slide.")),
		mcp.WithString("mime_type", mcp.DefaultString("PNG"), mcp.Description("PNG or JPEG.")),
		mcp.WithString("size", mcp.DefaultString("LARGE"), mcp.Description("SMALL, MEDIUM or LARGE.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.slidesExportThumbnail)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_slides_create",
		mcp.WithDescription("Create an empty presentation with a title. It arrives on Google's default "+
			"theme, and no request brings another deck's theme into it: the look has to be built here, "+
			"with gdocs_slides_set_theme_colors for the palette and gdocs_slides_style_layout for what "+
			"the layouts and the master impose. To start from an existing deck's look instead of "+
			"building one, copy that deck with gdocs_slides_copy_presentation."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Name of the new presentation.")),
	), r.slidesCreate)

	srv.AddTool(mcp.NewTool("gdocs_slides_copy_presentation",
		mcp.WithDescription("Copy a presentation, which is how a new deck is started from a template: the copy "+
			"keeps the master, the layouts, the fonts and the colours, so everything added afterwards "+
			"inherits them. A deck made with gdocs_slides_create starts on the default theme instead, "+
			"and looks nothing like the rest until that theme is built."),
		mcp.WithString("template_id", mcp.Required(), mcp.Description("File identifier of the template presentation.")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name for the new presentation.")),
		mcp.WithString("parent_folder_id", mcp.Description("Folder to put it in. Without one it lands beside the template.")),
	), r.slidesCopyPresentation)

	srv.AddTool(mcp.NewTool("gdocs_slides_add_slide",
		mcp.WithDescription("Add a slide following one of the presentation's own layouts. "+
			"Take the layout identifier from gdocs_slides_list_layouts rather than guessing a predefined one: "+
			"a template's layouts carry its styling, the predefined ones do not."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("layout_object_id", mcp.Description("Layout identifier from gdocs_slides_list_layouts.")),
		mcp.WithString("predefined_layout", mcp.Description(
			"A Google predefined layout, e.g. TITLE_AND_BODY. Only when the presentation has no suitable layout of its own.")),
		mcp.WithNumber("insertion_index", mcp.Description("Where to put it, counting from 0. Without it the slide goes last.")),
		mcp.WithString("object_id", mcp.Description("Identifier to give the new slide. Without one Google assigns it.")),
	), r.slidesAddSlide)

	srv.AddTool(mcp.NewTool("gdocs_slides_set_text",
		mcp.WithDescription("Replace the text of one text box with plain text, keeping the box where it is. "+
			"The new text inherits the styling of its placeholder, which is what keeps a filled-in slide "+
			"looking like the template it came from. For a title plus bullets use "+
			"gdocs_slides_replace_body_nested_list instead."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Object identifier of the text box.")),
		mcp.WithString("text", mcp.Required(), mcp.Description("The text to put in it. Newlines make paragraphs.")),
	), r.slidesSetText)

	srv.AddTool(mcp.NewTool("gdocs_slides_set_list",
		mcp.WithDescription("Replace a text box with a native bullet list of any depth. Each line carries the "+
			"level it sits at, and Slides works out the indents, the markers and the spacing itself. "+
			"This is the tool for a bullet slide: writing the markers as text, or placing the lines as "+
			"separate boxes, is what produces decks with drifting indents and wrong markers. "+
			"Take the lines from gdocs_slides_inspect_text_structure, which reports each paragraph's level "+
			"and its whole text — gdocs_slides_list shortens text for its overview, and a line copied from "+
			"there lands in the deck with the ellipsis included."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Object identifier of the text box.")),
		mcp.WithArray("items", mcp.Required(), mcp.Description(
			"Lines as a list of objects: {\"text\": \"…\", \"level\": 0}. Level 0 is a top-level bullet, "+
				"1 is nested under it, and so on to any depth the sample has."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text":  map[string]any{"type": "string"},
					"level": map[string]any{"type": "integer"},
				},
				"required": []string{"text"},
			})),
		mcp.WithBoolean("plain_first_line", mcp.Description(
			"Keep the first line out of the list, as a heading above it, and reset the indent it would "+
				"otherwise inherit. Some layouts want that; a body that is a list all the way up does not.")),
		mcp.WithString("bullet_preset", mcp.DefaultString(defaultBulletPreset),
			mcp.Description("Google bullet preset, e.g. "+defaultBulletPreset+". "+
				"This tool puts the words in; how they look is gdocs_slides_set_text_style and "+
				"gdocs_slides_set_paragraph_style, which take exactly what the reading tools report.")),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesSetList)

	srv.AddTool(mcp.NewTool("gdocs_slides_create_table_with_text",
		mcp.WithDescription("Create a real Slides table on a slide and fill its cells, with column widths, "+
			"fonts, colours and per-column alignment, and a header row styled apart. "+
			"A table has to be a table: rows of text boxes look similar until somebody edits one, and "+
			"they cannot be sorted, resized or pasted anywhere as a table. "+
			"Positions and widths are in EMU (914400 per inch), the unit Slides itself uses."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description("Slide to put the table on.")),
		mcp.WithArray("rows", mcp.Required(), mcp.Description(
			"Rows as a list of lists of cell values. Every row must have the same number of cells.")),
		mcp.WithNumber("x", mcp.Required(), mcp.Description("Left edge in EMU.")),
		mcp.WithNumber("y", mcp.Required(), mcp.Description("Top edge in EMU.")),
		mcp.WithNumber("width", mcp.Required(), mcp.Description("Width in EMU.")),
		mcp.WithNumber("height", mcp.Required(), mcp.Description("Height in EMU.")),
		mcp.WithString("object_id", mcp.Description("Identifier to give the table. Without one it is generated.")),
		mcp.WithNumber("font_size", mcp.DefaultNumber(12), mcp.Description("Body font size in points.")),
		mcp.WithBoolean("header_row", mcp.DefaultBool(true), mcp.Description("Style the first row as a header.")),
		mcp.WithNumber("header_font_size", mcp.Description("Header font size in points. Defaults to font_size.")),
		mcp.WithBoolean("header_bold", mcp.DefaultBool(true), mcp.Description("Bold the header row.")),
		mcp.WithArray("column_widths", mcp.WithNumberItems(), mcp.Description(
			"Per-column widths in EMU. Must have one entry per column.")),
		mcp.WithString("font_family", mcp.Description("Font for every cell, e.g. Roboto.")),
		mcp.WithObject("foreground_color", mcp.Description(
			"Text colour as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}.")),
		mcp.WithArray("column_alignments", mcp.WithStringEnumItems([]string{"START", "CENTER", "END", "JUSTIFIED"}),
			mcp.Description("Per-column alignment for the body rows. One entry per column.")),
		mcp.WithArray("header_alignments", mcp.WithStringEnumItems([]string{"START", "CENTER", "END", "JUSTIFIED"}),
			mcp.Description("Per-column alignment for the header row. One entry per column.")),
	), r.slidesCreateTableWithText)
}

const presentationIDHelp = "Presentation identifier, the part of its address between /d/ and /edit."

// slidesInspectTextStructure reports the paragraphs of a text box before anything is
// changed. Looking first is the habit this whole package is built around.
func (r *registry) slidesInspectTextStructure(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.Presentation(ctx, presentationID, structureMask)
	if err != nil {
		return toolError(err), nil
	}

	shape, err := findShape(presentation, objectID)
	if err != nil {
		return toolError(err), nil
	}

	// A link belongs to a run of text, not to a paragraph, so it is reported with the
	// text it covers: that pair is what rebuilding the paragraph needs.
	type link struct {
		Text string `json:"text"`
		URL  string `json:"url"`
	}
	// run is a stretch of text with one style, with the range it covers.
	//
	// A paragraph is reported as a whole and again in runs, because real slides style
	// parts of a line: "Разделили на 3 смены:" in bold and the rest of the sentence
	// plain, a term coloured inside a grey sentence. Reported per paragraph only, that
	// line comes back as one style and is rebuilt uniformly bold.
	type run struct {
		StartIndex int64   `json:"start_index"`
		EndIndex   int64   `json:"end_index"`
		Text       string  `json:"text"`
		FontSize   float64 `json:"font_size_pt,omitempty"`
		FontFamily string  `json:"font_family,omitempty"`
		// FontWeight is the numeric weight that travels with the family. A family reported
		// without it lets a caller write a heading back regular.
		FontWeight    int   `json:"font_weight,omitempty"`
		Bold          *bool `json:"bold,omitempty"`
		Italic        *bool `json:"italic,omitempty"`
		Underline     *bool `json:"underline,omitempty"`
		Strikethrough *bool `json:"strikethrough,omitempty"`
		SmallCaps     *bool `json:"small_caps,omitempty"`
		// BaselineOffset is SUPERSCRIPT or SUBSCRIPT — the footnote marks a real deck uses.
		BaselineOffset string `json:"baseline_offset,omitempty"`
		Color          string `json:"text_color,omitempty"`
		ThemeColor     string `json:"theme_color,omitempty"`
		// Background is the highlight behind the words, which is not the shape's fill.
		Background string `json:"background_color,omitempty"`
		URL        string `json:"url,omitempty"`
	}
	type paragraph struct {
		StartIndex   *int64 `json:"start_index,omitempty"`
		EndIndex     *int64 `json:"end_index,omitempty"`
		HasBullet    bool   `json:"has_bullet"`
		NestingLevel *int   `json:"nesting_level,omitempty"`
		// BulletColor and BulletGlyph are what the marker itself looks like. A bullet takes
		// its colour from the paragraph's text when it is made, so a list built before the
		// words were coloured has black markers over red text — visible on the slide and
		// invisible in the text style.
		BulletColor string `json:"bullet_color,omitempty"`
		BulletGlyph string `json:"bullet_glyph,omitempty"`
		// BulletSizePT is the marker's own size, and it is not the text's: a list made
		// while the paragraph was 14 pt keeps 14 pt markers after the words are set to
		// 11.5. A bigger marker makes a taller line, so a copy whose markers took the
		// smaller size runs a few pixels short on every paragraph, and the difference
		// piles up down the block.
		BulletSizePT *float64 `json:"bullet_size_pt,omitempty"`
		Text         string   `json:"text"`
		Links        []link   `json:"links,omitempty"`
		// Runs are reported only when the paragraph is not all one style: on a plain line
		// they would repeat what the paragraph already says.
		Runs []run `json:"runs,omitempty"`
		// FontSize and the rest are what this paragraph sets for itself, over whatever
		// the layout says. In a deck people have edited, most of it is set here rather
		// than inherited, and that is what has to be reproduced — the layout of a copy
		// will not supply it.
		FontSize   float64 `json:"font_size_pt,omitempty"`
		FontFamily string  `json:"font_family,omitempty"`
		Bold       *bool   `json:"bold,omitempty"`
		Color      string  `json:"text_color,omitempty"`
		Alignment  string  `json:"alignment,omitempty"`
		// The room around the paragraph, which gdocs_slides_set_paragraph_style takes back
		// in the same units: spacing as a percentage, spaces in points, indents in EMU.
		//
		// Pointers, because zero and unset are different answers and the difference is
		// visible: a heading that sets space below to zero sits tight against the line
		// under it, while one that sets nothing inherits the master's twelve points. Both
		// read as "no value" if zero is dropped as empty, and a copy made from that
		// reading is twelve points out for every paragraph that follows.
		LineSpacing        *float64 `json:"line_spacing,omitempty"`
		SpaceAbovePT       *float64 `json:"space_above_pt,omitempty"`
		SpaceBelowPT       *float64 `json:"space_below_pt,omitempty"`
		IndentStartEMU     *float64 `json:"indent_start_emu,omitempty"`
		IndentFirstLineEMU *float64 `json:"indent_first_line_emu,omitempty"`
		Direction          string   `json:"direction,omitempty"`
		// SpacingMode decides whether space above survives beside space below. A sample
		// that sets it and a copy that does not put the same paragraph in two different
		// places, and nothing about the numbers says why.
		SpacingMode string `json:"spacing_mode,omitempty"`
	}

	var paragraphs []paragraph
	var current *paragraph

	for _, element := range shapeElements(shape) {
		switch {
		case element.ParagraphMarker != nil:
			paragraphs = append(paragraphs, paragraph{
				StartIndex: element.StartIndex,
				EndIndex:   element.EndIndex,
				HasBullet:  element.ParagraphMarker.Bullet != nil,
			})
			current = &paragraphs[len(paragraphs)-1]
			if bullet := element.ParagraphMarker.Bullet; bullet != nil {
				current.NestingLevel = bullet.NestingLevel
				current.BulletGlyph = bullet.Glyph
				if bullet.BulletStyle != nil {
					if colour := bullet.BulletStyle.ForegroundColor; colour != nil && colour.OpaqueColor != nil {
						current.BulletColor = slideColor(colour.OpaqueColor.RGBColor)
					}
					if size := bullet.BulletStyle.FontSize; size != nil {
						points := size.InPoints()
						current.BulletSizePT = &points
					}
				}
			}
			if style := element.ParagraphMarker.Style; style != nil {
				current.Alignment = style.Alignment
				current.Direction = style.Direction
				current.SpacingMode = style.SpacingMode
				if style.LineSpacing != 0 {
					spacing := style.LineSpacing
					current.LineSpacing = &spacing
				}
				if style.SpaceAbove != nil {
					points := style.SpaceAbove.InPoints()
					current.SpaceAbovePT = &points
				}
				if style.SpaceBelow != nil {
					points := style.SpaceBelow.InPoints()
					current.SpaceBelowPT = &points
				}
				if style.IndentStart != nil {
					// Converted, not passed through: Slides answers in whichever unit the
					// value was stored in, and an indent usually comes back in points.
					// Reported raw, 36 points would be written back as 36 EMU.
					emu := style.IndentStart.InEMU()
					current.IndentStartEMU = &emu
				}
				if style.IndentFirstLine != nil {
					emu := style.IndentFirstLine.InEMU()
					current.IndentFirstLineEMU = &emu
				}
			}
		case current != nil && element.TextRun != nil:
			content := strings.TrimSuffix(element.TextRun.Content, "\n")
			current.Text += content

			piece := run{Text: content}
			if element.StartIndex != nil {
				piece.StartIndex = *element.StartIndex
			}
			if element.EndIndex != nil {
				// The trailing newline belongs to the paragraph, not to the run: a range
				// that includes it styles the line break and Slides refuses some of it.
				piece.EndIndex = *element.EndIndex
				if strings.HasSuffix(element.TextRun.Content, "\n") && piece.EndIndex > piece.StartIndex {
					piece.EndIndex--
				}
			}

			if style := element.TextRun.Style; style != nil {
				if style.Link != nil && style.Link.URL != "" {
					current.Links = append(current.Links, link{Text: content, URL: style.Link.URL})
					piece.URL = style.Link.URL
				}

				// The first styled run of the paragraph stands for the paragraph as a
				// whole; the runs below carry the rest, for the lines that mix styles.
				if current.FontSize == 0 && style.FontSize != nil {
					current.FontSize = style.FontSize.InPoints()
				}
				if current.FontFamily == "" {
					current.FontFamily = style.FontFamily
				}
				if current.Bold == nil {
					current.Bold = style.Bold
				}
				if current.Color == "" && style.ForegroundColor != nil && style.ForegroundColor.OpaqueColor != nil {
					current.Color = slideColor(style.ForegroundColor.OpaqueColor.RGBColor)
				}

				// Every field, not a chosen few: one word small and bold beside another
				// large and italic is what a real slide does, and anything left unread is
				// a difference nobody can explain afterwards.
				piece.FontFamily, piece.Bold, piece.Italic, piece.Underline =
					style.FontFamily, style.Bold, style.Italic, style.Underline
				piece.Strikethrough, piece.SmallCaps = style.Strikethrough, style.SmallCaps
				piece.BaselineOffset = style.BaselineOffset
				if style.FontSize != nil {
					piece.FontSize = style.FontSize.InPoints()
				}
				if style.WeightedFontFamily != nil {
					piece.FontWeight = style.WeightedFontFamily.Weight
					if piece.FontFamily == "" {
						piece.FontFamily = style.WeightedFontFamily.FontFamily
					}
				}
				if style.ForegroundColor != nil && style.ForegroundColor.OpaqueColor != nil {
					piece.Color = slideColor(style.ForegroundColor.OpaqueColor.RGBColor)
					piece.ThemeColor = style.ForegroundColor.OpaqueColor.ThemeColor
				}
				if style.BackgroundColor != nil && style.BackgroundColor.OpaqueColor != nil {
					piece.Background = slideColor(style.BackgroundColor.OpaqueColor.RGBColor)
				}
			}

			if piece.EndIndex > piece.StartIndex {
				current.Runs = append(current.Runs, piece)
			}
		}
	}

	// A paragraph in one style says everything twice; the runs are dropped there so the
	// reading stays about the slide rather than about its internals.
	sameStyle := func(a, b run) bool {
		return samePointer(a.Bold, b.Bold) && samePointer(a.Italic, b.Italic) &&
			samePointer(a.Underline, b.Underline) && samePointer(a.Strikethrough, b.Strikethrough) &&
			samePointer(a.SmallCaps, b.SmallCaps) &&
			a.FontSize == b.FontSize && a.FontFamily == b.FontFamily && a.FontWeight == b.FontWeight &&
			a.BaselineOffset == b.BaselineOffset &&
			a.Color == b.Color && a.ThemeColor == b.ThemeColor &&
			a.Background == b.Background && a.URL == b.URL
	}

	for index := range paragraphs {
		runs := paragraphs[index].Runs
		uniform := true
		for _, piece := range runs {
			if !sameStyle(runs[0], piece) {
				uniform = false
				break
			}
		}
		if uniform {
			paragraphs[index].Runs = nil
		}
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"paragraphs":      paragraphs,
	})
}

// slidesInspectTitleStyle reports the style of the first line of a text box.
func (r *registry) slidesInspectTitleStyle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	text, style, err := firstStyledRun(ctx, client, presentationID, objectID)
	if err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"text":            text,
		"style":           style,
		"fields":          strings.Join(style.Fields(), ","),
	}

	// What the text actually looks like, which is almost never what it sets for itself:
	// the rest comes from the layout and the master. Without this a caller reproducing a
	// slide in another deck has no number to set — the size it needs is not written
	// anywhere on the slide it is copying.
	if presentation, err := client.Presentation(ctx, presentationID, textStyleMask); err == nil {
		effective, origin := presentation.EffectiveTextStyle("", objectID)
		if !effective.IsEmpty() {
			payload["effective_style"] = effective
			payload["effective_from"] = origin

			if effective.FontSize != nil {
				size := effective.FontSize.InPoints()
				payload["effective_font_size_pt"] = size

				// What the editor shows is the size after Slides shrank the text to fit
				// its box. A caller matching a sample by eye is matching this number, not
				// the one above.
				if scale := autofitScale(presentation, objectID); scale > 0 && scale != 1 {
					payload["autofit_font_scale"] = scale
					payload["displayed_font_size_pt"] = size * scale
				}
			}
		}
	}

	// An empty style is not a failure and not a blank slide: it means the text carries
	// nothing of its own and takes everything from the layout. Saying so plainly stops a
	// caller from reading "{}" as "no styling" and inventing some.
	if style.IsEmpty() {
		payload["inherited"] = true
		payload["note"] = "this text sets no style of its own — font, size and colour all come " +
			"from the layout, so there is nothing here to copy"
	}

	return resultJSON(payload)
}

// slidesCreate makes an empty deck. What comes back is the identifier every other slides
// tool takes, so it is the only thing worth reporting besides the title.
func (r *registry) slidesCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := requiredString(req, "title")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.CreatePresentation(ctx, title)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentation.PresentationID,
		"title":           presentation.Title,
	})
}

// slidesList maps a presentation: its slides and what is on each of them.
func (r *registry) slidesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.Presentation(ctx, presentationID, pagesMask)
	if err != nil {
		return toolError(err), nil
	}

	type element struct {
		ObjectID    string `json:"object_id"`
		Kind        string `json:"kind"`
		ShapeType   string `json:"shape_type,omitempty"`
		Placeholder string `json:"placeholder,omitempty"`
		Text        string `json:"text,omitempty"`
		// ContentURL is where a picture can be fetched from. It is the only handle on a
		// picture a caller gets, and reproducing a slide that has one means using it
		// while it is still good — Slides hands out a short-lived address of its own copy.
		ContentURL string `json:"content_url,omitempty"`
		Rows       int    `json:"rows,omitempty"`
		Columns    int    `json:"columns,omitempty"`
		// A chart standing on a slide names the workbook and the chart it came from. Those
		// two are what refreshing it takes, and what tells a reader whether the picture on
		// the slide still follows the numbers or is a snapshot of them.
		SpreadsheetID string `json:"spreadsheet_id,omitempty"`
		ChartID       int    `json:"chart_id,omitempty"`
		// Geometry is always reported, zeroes included: an element at x=0 is an element
		// at the left edge, and a field that disappears when it is zero is a field a
		// caller cannot place anything against.
		X      float64 `json:"x_emu"`
		Y      float64 `json:"y_emu"`
		Width  float64 `json:"width_emu"`
		Height float64 `json:"height_emu"`
	}
	type slide struct {
		ObjectID string `json:"object_id"`
		Layout   string `json:"layout,omitempty"`
		// LayoutObjectID is the layout this slide follows. Reproducing a slide means
		// creating it on the layout with the same name in the target deck: the sizes and
		// colours a slide does not set itself come from there, so the same text on a
		// different layout comes out a different size.
		LayoutObjectID string `json:"layout_object_id,omitempty"`
		// Hidden is a slide kept in the deck but skipped while presenting. It looks like
		// any other slide in a listing, and a copy that shows it says more than the
		// original does.
		Hidden   bool      `json:"hidden,omitempty"`
		Elements []element `json:"elements"`
	}

	// Layout identifiers on a slide mean nothing without their names: identifiers differ
	// between presentations, names are what match.
	layoutNames := map[string]string{}
	for _, layout := range presentation.Layouts {
		if layout.LayoutProperties == nil {
			continue
		}
		name := layout.LayoutProperties.DisplayName
		if name == "" {
			name = layout.LayoutProperties.Name
		}
		layoutNames[layout.ObjectID] = name
	}

	slides := make([]slide, 0, len(presentation.Slides))
	for _, page := range presentation.Slides {
		// An empty list rather than a missing one: a slide with nothing on it is normal —
		// a blank layout, a picture-only slide before its picture — and a caller looping
		// over the elements should get an empty loop, not a null.
		entry := slide{ObjectID: page.ObjectID, Elements: []element{}}
		if page.SlideProperties != nil {
			entry.LayoutObjectID = page.SlideProperties.LayoutObjectID
			entry.Hidden = page.SlideProperties.IsSkipped != nil && *page.SlideProperties.IsSkipped
			// The layout's name is not on the slide, it is on the layout, so it is looked
			// up: a caller reproducing this slide needs the name, because identifiers do
			// not travel between presentations.
			entry.Layout = layoutNames[entry.LayoutObjectID]
		}
		if entry.Layout == "" && page.LayoutProperties != nil {
			entry.Layout = page.LayoutProperties.DisplayName
			if entry.Layout == "" {
				entry.Layout = page.LayoutProperties.Name
			}
		}

		for index, item := range page.PageElements {
			described := element{ObjectID: item.ObjectID, Kind: elementKind(item)}

			if item.Shape != nil {
				described.ShapeType = item.Shape.ShapeType
				if item.Shape.Placeholder != nil {
					described.Placeholder = item.Shape.Placeholder.Type
				}
				described.Text = truncateText(shapeText(item.Shape), 200)
			}
			if item.Table != nil {
				described.Rows = item.Table.Rows
				described.Columns = item.Table.Columns
			}
			if item.Image != nil {
				described.ContentURL = item.Image.ContentURL
			}
			if chart := item.SheetsChart; chart != nil {
				described.SpreadsheetID = chart.SpreadsheetID
				described.ChartID = chart.ChartID
				described.ContentURL = chart.ContentURL
			}
			if item.Transform != nil {
				described.X = item.Transform.TranslateX
				described.Y = item.Transform.TranslateY
			}

			// The reported size is the element's untransformed box, and for anything
			// inherited from a layout that box is Slides' own 3000000×3000000 unit
			// square. What the element actually covers is that box times the scale in
			// its transform — and for a table, the sum of its columns and rows. Reporting
			// the raw size would hand a caller the number 3000000 for every element on
			// every slide, which is exactly the sort of thing that gets placed against.
			if width, height, err := elementBox(&page.PageElements[index]); err == nil {
				described.Width = width
				described.Height = height
			}

			entry.Elements = append(entry.Elements, described)
		}

		slides = append(slides, entry)
	}

	payload := map[string]any{
		"presentation_id": presentation.PresentationID,
		"title":           presentation.Title,
		"slides":          slides,
	}
	if presentation.PageSize != nil && presentation.PageSize.Width != nil && presentation.PageSize.Height != nil {
		payload["page_size_emu"] = map[string]float64{
			"width":  presentation.PageSize.Width.Magnitude,
			"height": presentation.PageSize.Height.Magnitude,
		}
	}

	return resultJSON(payload)
}

// slidesListLayouts reports the slide kinds a presentation's own template offers.
func (r *registry) slidesListLayouts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.Presentation(ctx, presentationID, layoutsMask)
	if err != nil {
		return toolError(err), nil
	}

	type layout struct {
		ObjectID    string `json:"object_id"`
		Name        string `json:"name,omitempty"`
		DisplayName string `json:"display_name,omitempty"`
	}

	layouts := make([]layout, 0, len(presentation.Layouts))
	for _, page := range presentation.Layouts {
		entry := layout{ObjectID: page.ObjectID}
		if page.LayoutProperties != nil {
			entry.Name = page.LayoutProperties.Name
			entry.DisplayName = page.LayoutProperties.DisplayName
		}
		layouts = append(layouts, entry)
	}

	return resultJSON(map[string]any{"presentation_id": presentationID, "layouts": layouts})
}

// slidesExportThumbnail renders a slide so an agent can look at what it did.
func (r *registry) slidesExportThumbnail(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "page_object_id")
	if err != nil {
		return toolError(err), nil
	}

	mimeType := strings.ToUpper(req.GetString("mime_type", "PNG"))
	size := strings.ToUpper(req.GetString("size", "LARGE"))

	switch mimeType {
	case "PNG", "JPEG":
	default:
		return toolError(fmt.Errorf("mime_type %q is not one Slides renders: use PNG or JPEG", mimeType)), nil
	}
	switch size {
	case "SMALL", "MEDIUM", "LARGE":
	default:
		return toolError(fmt.Errorf("size %q is not one Slides renders: use SMALL, MEDIUM or LARGE", size)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	thumbnail, err := client.Thumbnail(ctx, presentationID, pageObjectID, mimeType, size)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"page_object_id":  pageObjectID,
		"content_url":     thumbnail.ContentURL,
		"width":           thumbnail.Width,
		"height":          thumbnail.Height,
	})
}

// slidesCopyPresentation starts a deck from a template.
func (r *registry) slidesCopyPresentation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	templateID, err := requiredString(req, "template_id")
	if err != nil {
		return toolError(err), nil
	}
	name, err := requiredString(req, "name")
	if err != nil {
		return toolError(err), nil
	}

	var parents []string
	if folder := optionalString(req, "parent_folder_id"); folder != "" {
		parents = []string{folder}
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	file, err := client.CopyFile(ctx, templateID, name, parents)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": file.ID,
		"name":            file.Name,
		"web_view_link":   file.WebViewLink,
		"parents":         file.Parents,
	})
}

// slidesAddSlide adds a slide following a layout of the presentation.
func (r *registry) slidesAddSlide(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	layoutID := optionalString(req, "layout_object_id")
	predefined := optionalString(req, "predefined_layout")
	if layoutID == "" && predefined == "" {
		return toolError(fmt.Errorf("name a layout: layout_object_id from gdocs_slides_list_layouts, " +
			"or predefined_layout when the presentation has nothing suitable")), nil
	}
	if layoutID != "" && predefined != "" {
		return toolError(fmt.Errorf("layout_object_id and predefined_layout are alternatives, not both")), nil
	}

	request := google.CreateSlideRequest{
		ObjectID:             optionalString(req, "object_id"),
		SlideLayoutReference: &google.LayoutReference{LayoutID: layoutID, PredefinedLayout: predefined},
	}

	if _, ok := req.GetArguments()["insertion_index"]; ok {
		index := req.GetInt("insertion_index", 0)
		if index < 0 {
			return toolError(fmt.Errorf("insertion_index %d is before the first slide", index)), nil
		}
		request.InsertionIndex = &index
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{CreateSlide: &request}})
	if err != nil {
		return toolError(err), nil
	}

	created := request.ObjectID
	for _, reply := range response.Replies {
		if reply.CreateSlide != nil && reply.CreateSlide.ObjectID != "" {
			created = reply.CreateSlide.ObjectID
		}
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"slide_object_id": created,
	})
}

// slidesSetText replaces the text of a box, leaving its styling to the template.
func (r *registry) slidesSetText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}
	text, err := req.RequireString("text")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	existing, err := shapeTextOf(ctx, client, presentationID, objectID)
	if err != nil {
		return toolError(err), nil
	}

	requests := setTextRequests(objectID, text, existing != "")

	if _, err := client.SlidesBatchUpdate(ctx, presentationID, requests); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"characters":      utf16Length(text),
	})
}

// setTextRequests empties a text box and puts plain text in it.
//
// The delete is sent only when there is something to delete. Slides refuses a deleteText
// over an empty box with "startIndex 0 must be less than the endIndex 0", and an empty
// box is the normal state of a placeholder on a slide that was just added from a layout —
// which is exactly when a caller fills one in.
func setTextRequests(objectID, text string, hasText bool) []google.Request {
	var requests []google.Request

	if hasText {
		// Bullets come off before the text does. Deleting text leaves the list formatting
		// behind, and the next text typed into that box arrives as list items — which is
		// how plain paragraphs end up wearing the markers of whatever was there before.
		requests = append(requests,
			google.Request{DeleteParagraphBullets: &google.DeleteParagraphBulletsRequest{
				ObjectID: objectID, TextRange: google.AllText()}},
			google.Request{DeleteText: &google.DeleteTextRequest{
				ObjectID: objectID, TextRange: google.AllText()}},
		)
	}

	if text != "" {
		requests = append(requests, google.Request{
			InsertText: &google.InsertTextRequest{ObjectID: objectID, Text: text, InsertionIndex: 0},
		})
	}

	return requests
}

// shapeTextOf reads the text currently in a text box.
//
// It costs one read before a write, and it buys the difference between a box that has to
// be emptied first and one that must not be: both writing tools build a different batch
// for each.
func shapeTextOf(ctx context.Context, client *google.Client, presentationID, objectID string) (string, error) {
	presentation, err := client.Presentation(ctx, presentationID, textContentMask)
	if err != nil {
		return "", err
	}

	shape, err := findShape(presentation, objectID)
	if err != nil {
		return "", err
	}

	return shapeText(shape), nil
}

// slidesReplaceBodyNestedList rebuilds a body slide as a native nested list.
func (r *registry) slidesSetList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	items, err := parseListItems(req)
	if err != nil {
		return toolError(err), nil
	}

	plainFirstLine := req.GetBool("plain_first_line", false)

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	existing, err := shapeTextOf(ctx, client, presentationID, objectID)
	if err != nil {
		return toolError(err), nil
	}

	text := listText(items)
	requests, err := nestedListRequests(objectID, text, req.GetString("bullet_preset", defaultBulletPreset),
		existing != "", plainFirstLine)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	deepest := 0
	for _, item := range items {
		if item.Level > deepest {
			deepest = item.Level
		}
	}

	return resultJSON(map[string]any{
		"presentation_id":  presentationID,
		"object_id":        objectID,
		"lines":            len(items),
		"deepest_level":    deepest,
		"plain_first_line": plainFirstLine,
		"characters":       utf16Length(text),
		"replies":          len(response.Replies),
	})
}

// listItem is one line of a list and how deep it sits.
type listItem struct {
	Text  string
	Level int
}

// parseListItems reads the lines argument.
func parseListItems(req mcp.CallToolRequest) ([]listItem, error) {
	objects, err := objectList(req, "items")
	if err != nil {
		return nil, err
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("items is empty: a list needs at least one line")
	}

	items := make([]listItem, 0, len(objects))
	for index, object := range objects {
		text := stringField(object, "text")
		if text == "" {
			return nil, fmt.Errorf("items[%d].text is empty", index)
		}
		if strings.ContainsAny(text, "\n\t") {
			// Newlines would make paragraphs the levels do not describe, and a tab is how
			// depth is spelled on the wire: a line carrying one would come out a level
			// deeper than it says.
			return nil, fmt.Errorf("items[%d].text carries a newline or a tab: one object per line, "+
				"and depth goes in level", index)
		}

		level, _ := intField(object, "level")
		if level < 0 {
			return nil, fmt.Errorf("items[%d].level is %d: levels count from 0", index, level)
		}
		if level > 8 {
			return nil, fmt.Errorf("items[%d].level is %d: Slides nests eight deep at most", index, level)
		}

		items = append(items, listItem{Text: text, Level: level})
	}

	return items, nil
}

// listText turns the lines into what Slides takes: one paragraph each, depth as tabs.
//
// Tabs rather than indents on purpose. Slides reads the tabs when it makes the list and
// then owns the indentation itself, which is what keeps a rebuilt deck's bullets lined up
// with the template's rather than with numbers this server invented.
func listText(items []listItem) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, strings.Repeat("\t", item.Level)+item.Text)
	}

	return strings.Join(lines, "\n")
}

// nestedListRequests is the batch that replaces a body with a nested list.
//
// The order matters and is not incidental: bullets are removed before the text, because
// deleting text under a list leaves the list behind; the text goes in whole; the bullets
// are made over everything after the title line; and the title's own indent is reset
// last, because inserting into a list gives the first line the list's indent.
//
// The two removals are skipped when the box is empty — the usual state of a placeholder
// on a freshly added slide — because Slides refuses both over an empty range.
func nestedListRequests(objectID, text, bulletPreset string, hasText, plainFirstLine bool) ([]google.Request, error) {
	titleEnd := firstLineEnd(text)
	length := utf16Length(text)

	if plainFirstLine && titleEnd >= length {
		return nil, fmt.Errorf("with plain_first_line the box would hold a heading and nothing else")
	}

	if bulletPreset == "" {
		bulletPreset = defaultBulletPreset
	}

	var requests []google.Request

	if hasText {
		requests = append(requests,
			google.Request{DeleteParagraphBullets: &google.DeleteParagraphBulletsRequest{
				ObjectID: objectID, TextRange: google.AllText()}},
			google.Request{DeleteText: &google.DeleteTextRequest{
				ObjectID: objectID, TextRange: google.AllText()}},
		)
	}

	// Where the list starts. Without a plain heading it is the whole box — a body that is
	// a list all the way up, which is what most real slides are.
	bulleted := google.AllText()
	if plainFirstLine {
		bulleted = google.FixedRange(titleEnd+1, length)
	}

	requests = append(requests,
		google.Request{InsertText: &google.InsertTextRequest{ObjectID: objectID, Text: text, InsertionIndex: 0}},
		google.Request{CreateParagraphBullets: &google.CreateParagraphBulletsRequest{
			ObjectID:     objectID,
			TextRange:    bulleted,
			BulletPreset: bulletPreset,
		}},
	)

	// The heading's indent is reset only when there is a heading: inserting text into a
	// list gives the first line the list's indent, and on a box that is entirely a list
	// that indent is correct.
	if plainFirstLine {
		requests = append(requests, google.Request{UpdateParagraphStyle: titleIndentRequest(objectID, titleEnd)})
	}

	return requests, nil
}

// titleIndentRequest puts the first line back where a title belongs.
func titleIndentRequest(objectID string, titleEnd int64) *google.UpdateParagraphStyleRequest {
	return &google.UpdateParagraphStyleRequest{
		ObjectID:  objectID,
		TextRange: google.FixedRange(0, titleEnd),
		Style: &google.ParagraphStyle{
			Alignment:       "START",
			IndentStart:     google.PT(0),
			IndentFirstLine: google.PT(0),
		},
		Fields: "alignment,indentStart,indentFirstLine",
	}
}

// There is no tool that copies a style from one deck into another in one call.
//
// That is deliberate, and it was tried the other way first. A copying tool answers "make
// this look like that", which is not the job: the job is to build a deck that looks right,
// having read how a dozen others are built. Copying hides the numbers — the caller never
// learns that the sample's headings are 25 pt in the theme's ACCENT1, so it cannot decide
// to use 22 pt here, or to keep the size and change the colour.
//
// So the pair is symmetrical instead. Everything the reading reports — sizes, weights,
// colours by value or by theme name, the runs inside a line with their ranges — goes back
// in through gdocs_slides_set_text_style, in the same units, with scope=range for a
// stretch the reading described. Whoever is between the two decides what to keep.

// firstStyledRun reads the first non-blank run of a text box, with its style. This is
// what "the title's style" means everywhere in this package.
func firstStyledRun(ctx context.Context, client *google.Client, presentationID, objectID string) (string, google.TextStyle, error) {
	presentation, err := client.Presentation(ctx, presentationID, textStyleMask)
	if err != nil {
		return "", google.TextStyle{}, err
	}

	shape, err := findShape(presentation, objectID)
	if err != nil {
		return "", google.TextStyle{}, err
	}

	for _, element := range shapeElements(shape) {
		if element.TextRun == nil || strings.TrimSpace(element.TextRun.Content) == "" {
			continue
		}

		style := google.TextStyle{}
		if element.TextRun.Style != nil {
			style = *element.TextRun.Style
		}

		return strings.TrimSuffix(element.TextRun.Content, "\n"), style, nil
	}

	return "", google.TextStyle{}, fmt.Errorf("%s in %s has no text to read a style from", objectID, presentationID)
}

// findShape locates a text box by identifier, saying plainly when the object is there
// but is not a shape — a caller that pointed at a table wants to know that.
func findShape(presentation *google.Presentation, objectID string) (*google.Shape, error) {
	for _, page := range presentation.Slides {
		for _, element := range page.PageElements {
			if element.ObjectID != objectID {
				continue
			}
			if element.Shape == nil {
				return nil, fmt.Errorf("%s is a %s, not a text box", objectID, elementKind(element))
			}
			return element.Shape, nil
		}
	}

	return nil, fmt.Errorf("no object %s on any slide of this presentation", objectID)
}

// shapeElements is the text elements of a shape, or nothing if it has no text.
func shapeElements(shape *google.Shape) []google.TextElement {
	if shape == nil || shape.Text == nil {
		return nil
	}
	return shape.Text.TextElements
}

// shapeText is the text of a shape without the newline the API puts at its end.
func shapeText(shape *google.Shape) string {
	var builder strings.Builder
	for _, element := range shapeElements(shape) {
		if element.TextRun != nil {
			builder.WriteString(element.TextRun.Content)
		}
	}

	return strings.TrimSuffix(builder.String(), "\n")
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}

// samePointer compares two optional flags, where nil means "not set" and is not the same
// as false: a run that says nothing about weight inherits it, and one that says false is
// deliberately not bold.
func samePointer(a, b *bool) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func elementKind(element google.PageElement) string {
	switch {
	case element.Shape != nil:
		return "shape"
	case element.Table != nil:
		return "table"
	case element.Image != nil:
		return "image"
	case element.Video != nil:
		return "video"
	case element.Line != nil:
		return "line"
	case element.SheetsChart != nil:
		return "sheets_chart"
	default:
		return "element"
	}
}

// autofitScale is how much Slides shrank the text of a shape to fit its box, or 0 when it
// did not.
func autofitScale(presentation *google.Presentation, objectID string) float64 {
	for _, page := range presentation.Slides {
		for _, element := range page.PageElements {
			if element.ObjectID != objectID || element.Shape == nil {
				continue
			}
			if properties := element.Shape.Properties; properties != nil && properties.Autofit != nil {
				return properties.Autofit.FontScale
			}
		}
	}

	return 0
}

func truncateText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// newObjectID makes an identifier for something this server creates. Slides accepts
// letters, digits, underscores and hyphens, and wants at least five characters.
func newObjectID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		// The identifier only has to be unique within one presentation, so a failure of
		// the random source is not worth failing the call over.
		return prefix + "_fallback"
	}

	return prefix + "_" + hex.EncodeToString(buf)
}
