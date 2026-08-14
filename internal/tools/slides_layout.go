package tools

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// Horizontal and vertical anchors an element can be placed against.
const (
	alignLeft   = "LEFT"
	alignCenter = "CENTER"
	alignRight  = "RIGHT"

	alignTop    = "TOP"
	alignMiddle = "MIDDLE"
	alignBottom = "BOTTOM"
)

// defaultMargin is the margin an element is placed with when none was named. It is the
// margin Google's own layouts use on a 16:9 slide, so an element placed with it lines up
// with the placeholders around it.
const defaultMargin = 457200

// geometryElements is one page's worth of what a placement needs: every element's box and
// transform, and — for tables — the column widths and row heights that are their real size.
const geometryElements = "objectId,pageElements(objectId,size,transform," +
	"table(rows,columns,tableColumns(columnWidth),tableRows(rowHeight)))"

// geometryMask reads the page size and the geometry of every page there is — slides,
// layouts and the master.
//
// The layouts are the point. A deck gets its grid from them: move the title on the layout
// and every slide that follows it moves, including the ones a person adds in the browser
// tomorrow. Moving the same title slide by slide leaves the template saying one thing and
// the deck doing another.
const geometryMask = "pageSize,slides(" + geometryElements + "),layouts(" + geometryElements +
	"),masters(" + geometryElements + ")"

// tableMask reads the tables on every slide: cells, spans, column widths, row heights and
// the style of the text inside, which is what reproducing a sample's table needs.
const tableMask = "slides(objectId,pageElements(objectId,size,transform," +
	"table(rows,columns,tableColumns(columnWidth),tableRows(rowHeight," +
	"tableCells(location,rowSpan,columnSpan,tableCellProperties(tableCellBackgroundFill,contentAlignment)," +
	"text(textElements(paragraphMarker(style(alignment)),textRun(content,style(" +
	google.TextStyleFields + ")))))))))"

// registerSlidesLayout adds the tools that place things on a slide and work with tables
// that already exist. They are the difference between filling a template in and building
// a slide that matches one — which is what copying a deck's look actually requires.
func (r *registry) registerSlidesLayout(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_slides_read_table",
		mcp.WithDescription("Read a table on a slide: every cell, the column widths and the row heights. "+
			"This is how a table in one deck gets reproduced in another — read its shape here, then "+
			"pass the same widths to gdocs_slides_create_table_with_text."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Object identifier of the table.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.slidesReadTable)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_slides_place_element",
		mcp.WithDescription("Move or resize anything on a slide — a text box, a table, a picture — by naming "+
			"where it should sit rather than by inventing coordinates. Align it against an edge of the "+
			"slide or centre it, with a margin, and the tool works the numbers out from the slide's own "+
			"size and the element's own box. Exact positions in EMU are accepted too. "+
			"An element of a layout or of the master moves the same way, by its identifier from "+
			"gdocs_slides_read_theme, and that is where a deck's grid belongs: a title moved on the "+
			"layout moves on every slide that follows it, including the ones added later in the browser, "+
			"while the same move made slide by slide is undone by the next slide somebody adds."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Element to move.")),
		mcp.WithString("align", mcp.Description("LEFT, CENTER or RIGHT, against the slide.")),
		mcp.WithString("valign", mcp.Description("TOP, MIDDLE or BOTTOM, against the slide.")),
		mcp.WithNumber("margin_emu", mcp.Description(
			"Distance from the edge for LEFT/RIGHT/TOP/BOTTOM, in EMU. Default 457200, the margin "+
				"Google's own layouts use on a 16:9 slide.")),
		mcp.WithString("like_object_id", mcp.Description(
			"Copy the position and size of this element, so the result sits exactly where the sample's "+
				"does. The one argument that gets a slide 1:1 with its sample — but only if every "+
				"element of the slide is placed this way, title included: placing one against a "+
				"sample and leaving the rest where the theme put them is what makes things overlap.")),
		mcp.WithString("like_presentation_id", mcp.Description(
			"Presentation the like_object_id element is in, when it is a different deck.")),
		mcp.WithString("below_object_id", mcp.Description(
			"Put it under this element on the same slide, with gap_emu between them. This is how an "+
				"element goes under a title: a y taken from another deck lands wherever that deck's "+
				"title happened to end, and on a different theme that is on top of the title.")),
		mcp.WithNumber("gap_emu", mcp.Description(
			"Distance below the element named by below_object_id. Default 228600, half the standard margin.")),
		mcp.WithString("left_aligned_with_object_id", mcp.Description(
			"Line its left edge up with this element's left edge, so a table and the title above it start "+
				"at the same place.")),
		mcp.WithNumber("x_emu", mcp.Description("Exact left edge in EMU. Overrides align.")),
		mcp.WithNumber("y_emu", mcp.Description("Exact top edge in EMU. Overrides valign.")),
		mcp.WithNumber("width_emu", mcp.Description("Resize to this width in EMU.")),
		mcp.WithNumber("height_emu", mcp.Description("Resize to this height in EMU.")),
		mcp.WithNumber("rotation_deg", mcp.Description(
			"Turn the element by this many degrees clockwise, about its own centre. Slides has no "+
				"rotation field — the angle lives in the same matrix as the scale — so this is the only "+
				"way to reproduce a sample's tilted label. gdocs_slides_inspect_page reports the angle.")),
		mcp.WithBoolean("flip_horizontally", mcp.Description(
			"Mirror it left to right. Not the same as turning it 180°: an arrow mirrored still points "+
				"up, an arrow turned points down.")),
		mcp.WithBoolean("flip_vertically", mcp.Description("Mirror it top to bottom.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesPlaceElement)

	srv.AddTool(mcp.NewTool("gdocs_slides_create_shape",
		mcp.WithDescription("Add a shape to a slide at a given place, with its text, font, colour, fill and "+
			"outline. Left without a shape_type it makes a plain text box, which is the usual case: a "+
			"caption, a note beside a chart, a label above a table. Naming one makes the rest — a rounded "+
			"panel behind a block of text, an arrow between two boxes, a circle around a number. "+
			"For anything a layout does have a placeholder for, fill the placeholder instead: it carries "+
			"the template's styling and a new shape does not."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description("Slide to put it on.")),
		mcp.WithNumber("x", mcp.Required(), mcp.Description("Left edge in EMU.")),
		mcp.WithNumber("y", mcp.Required(), mcp.Description("Top edge in EMU.")),
		mcp.WithNumber("width", mcp.Required(), mcp.Description("Width in EMU.")),
		mcp.WithNumber("height", mcp.Required(), mcp.Description("Height in EMU.")),
		mcp.WithString("text", mcp.Description("Text for the shape. Newlines make paragraphs.")),
		mcp.WithString("shape_type", mcp.Description(
			"Slides shape name: TEXT_BOX (the default), RECTANGLE, ROUND_RECTANGLE, ELLIPSE, DIAMOND, "+
				"RIGHT_ARROW, CHEVRON, STAR_5, CLOUD and the rest of the API's list.")),
		mcp.WithString("object_id", mcp.Description("Identifier to give it. Without one it is generated.")),
		mcp.WithNumber("font_size", mcp.Description("Font size in points.")),
		mcp.WithString("font_family", mcp.Description("Font family, e.g. Rubik.")),
		mcp.WithBoolean("bold", mcp.Description("Bold the text.")),
		mcp.WithObject("foreground_color", mcp.Description(
			"Text colour as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}.")),
		mcp.WithString("alignment", mcp.Description("Paragraph alignment: START, CENTER, END or JUSTIFIED.")),
		mcp.WithObject("fill_color", mcp.Description(
			"Fill as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}. Without one the shape keeps the "+
				"theme's default fill, which for a text box is nothing.")),
		mcp.WithNumber("fill_alpha", mcp.Description("Fill opacity from 0 to 1. Default 1.")),
		mcp.WithBoolean("no_fill", mcp.Description("Leave the shape transparent.")),
		mcp.WithObject("outline_color", mcp.Description("Outline colour, same shape as fill_color.")),
		mcp.WithNumber("outline_weight_emu", mcp.Description("Outline thickness in EMU. 12700 is one point.")),
		mcp.WithString("outline_dash", mcp.Description(
			"SOLID, DOT, DASH, DASH_DOT, LONG_DASH or LONG_DASH_DOT.")),
		mcp.WithBoolean("no_outline", mcp.Description("Leave the shape without a border.")),
		mcp.WithString("content_alignment", mcp.Description(
			"Where the text sits in the box: TOP, MIDDLE or BOTTOM.")),
	), r.slidesCreateShape)

	srv.AddTool(mcp.NewTool("gdocs_slides_insert_image",
		mcp.WithDescription("Put a picture on a slide by address. Slides fetches it once and keeps its own "+
			"copy, so the address only has to work at that moment. Without a size the picture keeps its "+
			"own proportions."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description("Slide to put it on.")),
		mcp.WithString("url", mcp.Required(), mcp.Description("Address of the picture, reachable by Google.")),
		mcp.WithNumber("x", mcp.Description("Left edge in EMU.")),
		mcp.WithNumber("y", mcp.Description("Top edge in EMU.")),
		mcp.WithNumber("width", mcp.Description("Width in EMU.")),
		mcp.WithNumber("height", mcp.Description("Height in EMU.")),
		mcp.WithString("object_id", mcp.Description("Identifier to give it. Without one it is generated.")),
	), r.slidesInsertImage)

	srv.AddTool(mcp.NewTool("gdocs_slides_update_table_cells",
		mcp.WithDescription("Replace the text of cells in a table that already exists, keeping the table, its "+
			"widths and its styling. This is how a deck is refreshed for a new period: same table, new "+
			"numbers. Creating a second table beside the first is what makes a deck grow junk."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Object identifier of the table.")),
		mcp.WithArray("column_widths_emu", mcp.WithNumberItems(), mcp.Description(
			"New column widths in EMU, one per column. A table's width is the sum of these — it has no "+
				"width of its own, and gdocs_slides_place_element cannot resize it.")),
		mcp.WithArray("cells", mcp.Description(
			"Cells to change, as a list of objects: {\"row\": 0, \"column\": 1, \"text\": \"…\"}. "+
				"Rows and columns count from 0, and they are the coordinates the cells report, not "+
				"positions in the row: a merge takes the cells it swallowed out of the table, so under "+
				"a first column merged down five rows the next row starts at column 1. Naming a "+
				"swallowed coordinate is refused rather than written into the merged cell."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"row":    map[string]any{"type": "integer"},
					"column": map[string]any{"type": "integer"},
					"text":   map[string]any{"type": "string"},
				},
				"required": []string{"row", "column", "text"},
			})),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesUpdateTableCells)

	srv.AddTool(mcp.NewTool("gdocs_slides_delete",
		mcp.WithDescription("Remove a slide, or one element of a slide, by identifier. "+
			"This exists for two jobs and no others: taking a copied deck down to the slides that apply "+
			"this time, and clearing away a slide a failed step left behind. It reaches nothing outside "+
			"the presentation — no files, no folders, no spreadsheet tabs, no rows. "+
			"Name the identifier you read from gdocs_slides_list, never a guess."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithArray("object_ids", mcp.Required(), mcp.WithStringItems(), mcp.Description(
			"Identifiers to remove: slides, or elements on them.")),
		mcp.WithDestructiveHintAnnotation(true),
	), r.slidesDelete)

	srv.AddTool(mcp.NewTool("gdocs_slides_reorder",
		mcp.WithDescription("Put slides in a given order. Pass every slide identifier in the order the deck "+
			"should end up in, and the deck is arranged to match. Reproducing a deck means reproducing "+
			"its order too: the same slides in a different sequence tell a different story."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithArray("order", mcp.Required(), mcp.WithStringItems(), mcp.Description(
			"Slide identifiers, first to last. Every slide of the presentation has to appear exactly once.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesReorder)

	srv.AddTool(mcp.NewTool("gdocs_slides_style_table",
		mcp.WithDescription("Style a table that already exists: merge cells, fill them with a colour, align "+
			"their content, set row heights. Merging is what a real deck's table does with a heading that "+
			"covers several rows — a grid where that heading is repeated or left blank reads as a table "+
			"nobody finished."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Object identifier of the table.")),
		mcp.WithArray("merge", mcp.Description(
			"Rectangles to merge, as a list of objects: "+
				"{\"row\": 1, \"column\": 0, \"row_span\": 5, \"column_span\": 1}."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"row":         map[string]any{"type": "integer"},
					"column":      map[string]any{"type": "integer"},
					"row_span":    map[string]any{"type": "integer"},
					"column_span": map[string]any{"type": "integer"},
				},
				"required": []string{"row", "column", "row_span", "column_span"},
			})),
		mcp.WithArray("fill", mcp.Description(
			"Cells to fill, as a list of objects: "+
				"{\"row\": 0, \"column\": 0, \"row_span\": 1, \"column_span\": 3, "+
				"\"color\": {\"red\": 0.94, \"green\": 0.94, \"blue\": 0.94}}. "+
				"An entry may name a palette colour instead — {\"theme_color\": \"ACCENT2\"} — and then "+
				"the cell follows gdocs_slides_set_theme_colors rather than keeping the value."),
			mcp.Items(map[string]any{"type": "object"})),
		mcp.WithArray("cell_styles", mcp.Description(
			"How the text of particular cells looks, as a list of objects: "+
				"{\"row\": 0, \"column\": 1, \"bold\": true, \"italic\": false, \"font_size\": 14, "+
				"\"font_family\": \"Rubik\", \"alignment\": \"CENTER\", "+
				"\"text_color\": {\"red\": 0.26, \"green\": 0.26, \"blue\": 0.26}}. "+
				"\"theme_color\": \"DARK2\" paints the words from the palette instead. "+
				"This is the shape gdocs_slides_read_table reports, cell for cell: a table whose header "+
				"is centred and whose first column is bold cannot be described by one style per column, "+
				"and a copy made that way reads as a different table."),
			mcp.Items(map[string]any{"type": "object"})),
		mcp.WithString("content_alignment", mcp.Description(
			"Vertical alignment of cell content: TOP, MIDDLE or BOTTOM. Applies to the whole table.")),
		mcp.WithNumber("header_row_height_emu", mcp.Description("Minimum height of the first row, in EMU.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesStyleTable)
}

// slidesDelete removes slides or elements, and nothing else anywhere.
//
// Every identifier is checked against the presentation first, so a typo is a refusal
// rather than a deletion of something else, and the answer names what went.
func (r *registry) slidesDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	objectIDs := req.GetStringSlice("object_ids", nil)
	if len(objectIDs) == 0 {
		return toolError(fmt.Errorf("object_ids is empty")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.Presentation(ctx, presentationID, geometryMask)
	if err != nil {
		return toolError(err), nil
	}

	type removal struct {
		ObjectID string `json:"object_id"`
		Kind     string `json:"kind"`
	}

	removals := make([]removal, 0, len(objectIDs))
	var requests []google.Request
	slides := 0

	for _, objectID := range objectIDs {
		kind := ""

		for _, page := range presentation.Slides {
			if page.ObjectID == objectID {
				kind = "slide"
				break
			}
			for _, element := range page.PageElements {
				if element.ObjectID == objectID {
					kind = elementKind(element)
					break
				}
			}
			if kind != "" {
				break
			}
		}

		// A band or a rule put on a layout is part of the template and comes off the same
		// way it went on. The layout itself is not on the table: it is not a slide, and
		// removing one takes every slide that follows it with it.
		if kind == "" {
			for _, page := range append(append([]google.Page{}, presentation.Layouts...), presentation.Masters...) {
				if page.ObjectID == objectID {
					return toolError(fmt.Errorf("%s is a layout or the master, not something on one; "+
						"a layout cannot be removed here, and removing it would take every slide "+
						"that follows it", objectID)), nil
				}
				for _, element := range page.PageElements {
					if element.ObjectID == objectID {
						kind = elementKind(element)
						break
					}
				}
				if kind != "" {
					break
				}
			}
		}

		if kind == "" {
			return toolError(fmt.Errorf("no slide, or element of a slide, a layout or the master, "+
				"named %s in this presentation: read the identifiers with gdocs_slides_list or "+
				"gdocs_slides_read_theme and pass them exactly", objectID)), nil
		}

		if kind == "slide" {
			slides++
		}

		removals = append(removals, removal{ObjectID: objectID, Kind: kind})
		requests = append(requests, google.Request{DeleteObject: &google.DeleteObjectRequest{ObjectID: objectID}})
	}

	// A presentation with no slides is not a presentation, and Slides itself allows it.
	if slides > 0 && slides >= len(presentation.Slides) {
		return toolError(fmt.Errorf("that would remove every slide of the presentation: keep at least one")), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"removed":         removals,
		"replies":         len(response.Replies),
	})
}

// slidesReorder arranges the deck in the order it was given.
//
// The order is given in full rather than as moves, because a sequence of moves is a
// sequence a caller has to reason about, and the whole point is to end up with a known
// order. Every slide is moved to its place one at a time, from the front: after the first
// n are settled, moving the next to index n cannot disturb them.
func (r *registry) slidesReorder(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	order := req.GetStringSlice("order", nil)
	if len(order) == 0 {
		return toolError(fmt.Errorf("order is empty: pass every slide identifier, first to last")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.Presentation(ctx, presentationID, "slides(objectId)")
	if err != nil {
		return toolError(err), nil
	}

	current := make([]string, 0, len(presentation.Slides))
	known := map[string]bool{}
	for _, slide := range presentation.Slides {
		current = append(current, slide.ObjectID)
		known[slide.ObjectID] = true
	}

	seen := map[string]bool{}
	for _, objectID := range order {
		if !known[objectID] {
			return toolError(fmt.Errorf("no slide %s in this presentation", objectID)), nil
		}
		if seen[objectID] {
			return toolError(fmt.Errorf("%s appears twice in order", objectID)), nil
		}
		seen[objectID] = true
	}

	if len(order) != len(current) {
		return toolError(fmt.Errorf("order names %d slides but the presentation has %d: "+
			"pass all of them, in the order they should end up in", len(order), len(current))), nil
	}

	var requests []google.Request
	for index, objectID := range order {
		requests = append(requests, google.Request{
			UpdateSlidesPosition: &google.UpdateSlidesPositionRequest{
				SlideObjectIDs: []string{objectID},
				InsertionIndex: index,
			},
		})
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"order":           order,
		"replies":         len(response.Replies),
	})
}

// slidesStyleTable merges, fills and aligns cells of an existing table.
func (r *registry) slidesStyleTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	presentation, err := client.Presentation(ctx, presentationID, tableMask)
	if err != nil {
		return toolError(err), nil
	}

	_, element, err := findElement(presentation, objectID)
	if err != nil {
		return toolError(err), nil
	}
	if element.Table == nil {
		return toolError(fmt.Errorf("%s is a %s, not a table", objectID, elementKind(*element))), nil
	}

	var requests []google.Request
	arguments := req.GetArguments()

	// Merges go first: filling a cell that is about to be merged away wastes the fill,
	// and Slides addresses a merged block by its top-left corner afterwards.
	if _, ok := arguments["merge"]; ok {
		merges, err := objectList(req, "merge")
		if err != nil {
			return toolError(err), nil
		}

		for index, entry := range merges {
			area, err := tableRangeOf(entry, element.Table)
			if err != nil {
				return toolError(fmt.Errorf("merge[%d]: %w", index, err)), nil
			}
			requests = append(requests, google.Request{
				MergeTableCells: &google.MergeTableCellsRequest{ObjectID: objectID, TableRange: area},
			})
		}
	}

	if _, ok := arguments["fill"]; ok {
		fills, err := objectList(req, "fill")
		if err != nil {
			return toolError(err), nil
		}

		for index, entry := range fills {
			area, err := tableRangeOf(entry, element.Table)
			if err != nil {
				return toolError(fmt.Errorf("fill[%d]: %w", index, err)), nil
			}

			// A cell is filled either by value or by a name from the palette. The name is
			// what lets a whole deck be recoloured with set_theme_colors afterwards, so a
			// table in a themed series is filled that way and a one-off by value.
			painted := &google.OpaqueColor{}
			themed, ok := entry["theme_color"].(string)
			colour, hasColour := entry["color"].(map[string]any)

			switch {
			case ok && themed != "" && hasColour:
				return toolError(fmt.Errorf("fill[%d]: color and theme_color are alternatives: name one", index)), nil
			case ok && themed != "":
				name := strings.ToUpper(strings.TrimSpace(themed))
				if !paletteNames[name] {
					return toolError(fmt.Errorf("fill[%d].theme_color is %q, which is not a colour of the "+
						"palette: use one of %s", index, themed, strings.Join(sortedPaletteNames(), ", "))), nil
				}
				painted.ThemeColor = name
			case hasColour:
				rgb := &google.RGBColor{}
				for name, target := range map[string]*float64{
					"red": &rgb.Red, "green": &rgb.Green, "blue": &rgb.Blue,
				} {
					if value, present := colour[name]; present {
						number, ok := value.(float64)
						if !ok || number < 0 || number > 1 {
							return toolError(fmt.Errorf("fill[%d].color.%s must be a number from 0 to 1",
								index, name)), nil
						}
						*target = number
					}
				}
				painted.RGBColor = rgb
			default:
				return toolError(fmt.Errorf("fill[%d] has neither color nor theme_color: give red, green and "+
					"blue from 0 to 1, or a palette name", index)), nil
			}

			requests = append(requests, google.Request{
				UpdateTableCellProperties: &google.UpdateTableCellPropertiesRequest{
					ObjectID:   objectID,
					TableRange: area,
					TableCellProperties: &google.TableCellProperties{
						BackgroundFill: &google.TableCellBackgroundFill{
							SolidFill: &google.SolidFill{Color: painted, Alpha: 1},
						},
					},
					Fields: "tableCellBackgroundFill.solidFill.color",
				},
			})
		}
	}

	if alignment := strings.ToUpper(optionalString(req, "content_alignment")); alignment != "" {
		switch alignment {
		case "TOP", "MIDDLE", "BOTTOM":
		default:
			return toolError(fmt.Errorf("content_alignment %q is not one of TOP, MIDDLE, BOTTOM", alignment)), nil
		}

		requests = append(requests, google.Request{
			UpdateTableCellProperties: &google.UpdateTableCellPropertiesRequest{
				ObjectID:            objectID,
				TableCellProperties: &google.TableCellProperties{ContentAlignment: alignment},
				Fields:              "contentAlignment",
			},
		})
	}

	if height := req.GetFloat("header_row_height_emu", 0); height > 0 {
		requests = append(requests, google.Request{
			UpdateTableRowProperties: &google.UpdateTableRowPropertiesRequest{
				ObjectID:           objectID,
				RowIndices:         []int{0},
				TableRowProperties: &google.TableRowProperties{MinRowHeight: google.EMU(height)},
				Fields:             "minRowHeight",
			},
		})
	}

	if _, ok := arguments["cell_styles"]; ok {
		styles, err := objectList(req, "cell_styles")
		if err != nil {
			return toolError(err), nil
		}

		for index, entry := range styles {
			cellRequests, err := cellStyleRequests(objectID, entry, element.Table)
			if err != nil {
				return toolError(fmt.Errorf("cell_styles[%d]: %w", index, err)), nil
			}
			requests = append(requests, cellRequests...)
		}
	}

	if len(requests) == 0 {
		return toolError(fmt.Errorf("nothing to do: name merge, fill, cell_styles, content_alignment " +
			"or header_row_height_emu")), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"changes":         len(requests),
		"replies":         len(response.Replies),
	})
}

// cellStyleRequests turns one entry of cell_styles into the requests that apply it.
//
// A cell is styled through its own text range, not through the table's: the table has no
// text of its own, and the API addresses a cell's words by naming the cell alongside the
// range. Empty cells are skipped — Slides refuses a text update on a cell with no text,
// and the empty ones in a real table are the halves of merged blocks.
func cellStyleRequests(objectID string, entry map[string]any, table *google.Table) ([]google.Request, error) {
	row, ok := intField(entry, "row")
	if !ok || row < 0 {
		return nil, fmt.Errorf("row is missing or negative")
	}
	column, ok := intField(entry, "column")
	if !ok || column < 0 {
		return nil, fmt.Errorf("column is missing or negative")
	}
	if row >= table.Rows || column >= table.Columns {
		return nil, fmt.Errorf("the table is %d×%d, so there is no cell (%d,%d)",
			table.Rows, table.Columns, row, column)
	}

	if strings.TrimSpace(cellTextAt(table, row, column)) == "" {
		return nil, nil
	}

	location := &google.CellLocation{RowIndex: row, ColumnIndex: column}

	style := &google.TextStyle{}
	var fields []string

	if size, ok := entry["font_size"].(float64); ok && size > 0 {
		style.FontSize = google.PT(size)
		fields = append(fields, "fontSize")
	}
	if family := stringField(entry, "font_family"); family != "" {
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
	} {
		value, present := entry[flag.name].(bool)
		if !present {
			continue
		}
		set := value
		*flag.target = &set
		fields = append(fields, flag.field)
	}

	if colour, ok := entry["text_color"].(map[string]any); ok {
		rgb := &google.RGBColor{}
		for name, target := range map[string]*float64{
			"red": &rgb.Red, "green": &rgb.Green, "blue": &rgb.Blue,
		} {
			number, ok := colour[name].(float64)
			if !ok {
				continue
			}
			if number < 0 || number > 1 {
				return nil, fmt.Errorf("text_color.%s is %g: colour components run from 0 to 1", name, number)
			}
			*target = number
		}
		style.ForegroundColor = &google.OptionalColor{OpaqueColor: &google.OpaqueColor{RGBColor: rgb}}
		fields = append(fields, "foregroundColor")
	} else if themed := stringField(entry, "theme_color"); themed != "" {
		// By name, so the cell follows the palette the way a themed shape does.
		name := strings.ToUpper(strings.TrimSpace(themed))
		if !paletteNames[name] {
			return nil, fmt.Errorf("theme_color is %q, which is not a colour of the palette: use one of %s",
				themed, strings.Join(sortedPaletteNames(), ", "))
		}
		style.ForegroundColor = &google.OptionalColor{OpaqueColor: &google.OpaqueColor{ThemeColor: name}}
		fields = append(fields, "foregroundColor")
	}

	var requests []google.Request

	if len(fields) > 0 {
		requests = append(requests, google.Request{
			UpdateTextStyle: &google.UpdateTextStyleRequest{
				ObjectID:     objectID,
				CellLocation: location,
				TextRange:    google.AllText(),
				Style:        style,
				Fields:       strings.Join(fields, ","),
			},
		})
	}

	if alignment := strings.ToUpper(stringField(entry, "alignment")); alignment != "" {
		switch alignment {
		case "START", "CENTER", "END", "JUSTIFIED":
		default:
			return nil, fmt.Errorf("alignment %q is not one of START, CENTER, END, JUSTIFIED", alignment)
		}
		requests = append(requests, google.Request{
			UpdateParagraphStyle: &google.UpdateParagraphStyleRequest{
				ObjectID:     objectID,
				CellLocation: location,
				TextRange:    google.AllText(),
				Style:        &google.ParagraphStyle{Alignment: alignment},
				Fields:       "alignment",
			},
		})
	}

	return requests, nil
}

// cellTextAt is the text of one cell, or nothing when the cell is not there.
func cellTextAt(table *google.Table, row, column int) string {
	if row >= len(table.TableRows) {
		return ""
	}
	cells := table.TableRows[row].TableCells
	for _, cell := range cells {
		if cell.Location != nil && cell.Location.RowIndex == row && cell.Location.ColumnIndex == column {
			return tableCellText(cell)
		}
	}

	return ""
}

// tableRangeOf reads a rectangle of a table out of a decoded object and checks it fits.
func tableRangeOf(entry map[string]any, table *google.Table) (*google.TableRange, error) {
	row, ok := intField(entry, "row")
	if !ok || row < 0 {
		return nil, fmt.Errorf("row is missing or negative")
	}
	column, ok := intField(entry, "column")
	if !ok || column < 0 {
		return nil, fmt.Errorf("column is missing or negative")
	}

	rowSpan, ok := intField(entry, "row_span")
	if !ok || rowSpan < 1 {
		rowSpan = 1
	}
	columnSpan, ok := intField(entry, "column_span")
	if !ok || columnSpan < 1 {
		columnSpan = 1
	}

	if row+rowSpan > table.Rows || column+columnSpan > table.Columns {
		return nil, fmt.Errorf("reaches past the table, which is %d×%d", table.Rows, table.Columns)
	}

	return &google.TableRange{
		Location:   &google.CellLocation{RowIndex: row, ColumnIndex: column},
		RowSpan:    rowSpan,
		ColumnSpan: columnSpan,
	}, nil
}

// slidesReadTable reports a table cell by cell, with the widths that make it look the way
// it does.
func (r *registry) slidesReadTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	presentation, err := client.Presentation(ctx, presentationID, tableMask)
	if err != nil {
		return toolError(err), nil
	}

	_, element, err := findElement(presentation, objectID)
	if err != nil {
		return toolError(err), nil
	}
	if element.Table == nil {
		return toolError(fmt.Errorf("%s is a %s, not a table", objectID, elementKind(*element))), nil
	}

	table := element.Table

	type cellStyle struct {
		Row        int     `json:"row"`
		Column     int     `json:"column"`
		RowSpan    int     `json:"row_span,omitempty"`
		ColumnSpan int     `json:"column_span,omitempty"`
		FontFamily string  `json:"font_family,omitempty"`
		FontSize   float64 `json:"font_size_pt,omitempty"`
		Bold       *bool   `json:"bold,omitempty"`
		Color      string  `json:"text_color,omitempty"`
		Background string  `json:"background,omitempty"`
		Alignment  string  `json:"alignment,omitempty"`
		Vertical   string  `json:"content_alignment,omitempty"`
	}

	rows := make([][]string, 0, len(table.TableRows))
	var styles []cellStyle

	for rowIndex, row := range table.TableRows {
		cells := make([]string, 0, len(row.TableCells))
		for columnIndex, cell := range row.TableCells {
			cells = append(cells, strings.TrimSuffix(tableCellText(cell), "\n"))

			// Style is reported per cell rather than per table because a header row
			// usually differs from the body, and that difference is the thing worth
			// copying.
			described := cellStyle{Row: rowIndex, Column: columnIndex}
			if cell.RowSpan > 1 {
				described.RowSpan = cell.RowSpan
			}
			if cell.ColumnSpan > 1 {
				described.ColumnSpan = cell.ColumnSpan
			}
			if cell.Properties != nil {
				described.Vertical = cell.Properties.ContentAlignment
				described.Background = describeSolidFill(cell.Properties.BackgroundFill)
			}

			if style, alignment, ok := firstCellStyle(cell); ok {
				described.FontFamily = style.FontFamily
				if style.FontSize != nil {
					described.FontSize = style.FontSize.InPoints()
				}
				described.Bold = style.Bold
				if style.ForegroundColor != nil && style.ForegroundColor.OpaqueColor != nil {
					described.Color = slideColor(style.ForegroundColor.OpaqueColor.RGBColor)
				}
				described.Alignment = alignment
			}

			if described != (cellStyle{Row: rowIndex, Column: columnIndex}) {
				styles = append(styles, described)
			}
		}
		rows = append(rows, cells)
	}

	widths := make([]float64, 0, len(table.TableColumns))
	for _, column := range table.TableColumns {
		if column.ColumnWidth != nil {
			widths = append(widths, column.ColumnWidth.InEMU())
		}
	}

	heights := make([]float64, 0, len(table.TableRows))
	for _, row := range table.TableRows {
		if row.RowHeight != nil {
			heights = append(heights, row.RowHeight.InEMU())
		}
	}

	payload := map[string]any{
		"presentation_id":   presentationID,
		"object_id":         objectID,
		"rows":              table.Rows,
		"columns":           table.Columns,
		"values":            rows,
		"cell_styles":       styles,
		"column_widths_emu": widths,
		"row_heights_emu":   heights,
		"reproduce_with":    "gdocs_slides_create_table_with_text, then gdocs_slides_style_table for the merges and fills",
	}

	if element.Transform != nil {
		payload["x_emu"] = element.Transform.TranslateX
		payload["y_emu"] = element.Transform.TranslateY
	}
	if width, height, err := elementBox(element); err == nil {
		payload["width_emu"] = width
		payload["height_emu"] = height
	}

	return resultJSON(payload)
}

// firstCellStyle is the style of the first run of text in a cell, and the alignment of
// its first paragraph.
func firstCellStyle(cell google.TableCell) (google.TextStyle, string, bool) {
	if cell.Text == nil {
		return google.TextStyle{}, "", false
	}

	alignment := ""
	for _, element := range cell.Text.TextElements {
		if element.ParagraphMarker != nil && element.ParagraphMarker.Style != nil && alignment == "" {
			alignment = element.ParagraphMarker.Style.Alignment
		}
		if element.TextRun != nil && strings.TrimSpace(element.TextRun.Content) != "" {
			if element.TextRun.Style != nil {
				return *element.TextRun.Style, alignment, true
			}
			return google.TextStyle{}, alignment, alignment != ""
		}
	}

	return google.TextStyle{}, alignment, alignment != ""
}

// describeSolidFill renders a cell's background as hex, or nothing when it is inherited.
//
// White is reported like any other colour here, unlike in a spreadsheet: a table cell on
// a slide is white because somebody made it white, and a header row that loses its fill
// on the way into a copy is a visibly different table.
func describeSolidFill(fill *google.TableCellBackgroundFill) string {
	if fill == nil || fill.SolidFill == nil || fill.SolidFill.Color == nil {
		return ""
	}

	return slideColor(fill.SolidFill.Color.RGBColor)
}

// cellText is the text of one table cell.
func tableCellText(cell google.TableCell) string {
	if cell.Text == nil {
		return ""
	}

	var builder strings.Builder
	for _, item := range cell.Text.TextElements {
		if item.TextRun != nil {
			builder.WriteString(item.TextRun.Content)
		}
	}

	return builder.String()
}

// slidesUpdateTableCells replaces the text of cells in place.
func (r *registry) slidesUpdateTableCells(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	var changes []map[string]any
	if _, ok := req.GetArguments()["cells"]; ok {
		changes, err = objectList(req, "cells")
		if err != nil {
			return toolError(err), nil
		}
	}

	widths := req.GetFloatSlice("column_widths_emu", nil)

	if len(changes) == 0 && len(widths) == 0 {
		return toolError(fmt.Errorf("nothing to do: name cells, column_widths_emu, or both")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.Presentation(ctx, presentationID, tableMask)
	if err != nil {
		return toolError(err), nil
	}

	_, element, err := findElement(presentation, objectID)
	if err != nil {
		return toolError(err), nil
	}
	if element.Table == nil {
		return toolError(fmt.Errorf("%s is a %s, not a table", objectID, elementKind(*element))), nil
	}

	var requests []google.Request

	if len(widths) > 0 {
		if len(widths) != element.Table.Columns {
			return toolError(fmt.Errorf("column_widths_emu has %d entries, but the table has %d columns",
				len(widths), element.Table.Columns)), nil
		}

		for index, width := range widths {
			if width <= 0 {
				return toolError(fmt.Errorf("column_widths_emu[%d] is %g: widths are in EMU and have to be positive",
					index, width)), nil
			}

			requests = append(requests, google.Request{
				UpdateTableColumnProperties: &google.UpdateTableColumnPropertiesRequest{
					ObjectID:              objectID,
					ColumnIndices:         []int{index},
					TableColumnProperties: &google.TableColumnProperties{ColumnWidth: google.EMU(width)},
					Fields:                "columnWidth",
				},
			})
		}
	}

	for index, change := range changes {
		row, ok := intField(change, "row")
		if !ok {
			return toolError(fmt.Errorf("cells[%d].row is missing or not a whole number", index)), nil
		}
		column, ok := intField(change, "column")
		if !ok {
			return toolError(fmt.Errorf("cells[%d].column is missing or not a whole number", index)), nil
		}

		if row < 0 || row >= element.Table.Rows || column < 0 || column >= element.Table.Columns {
			return toolError(fmt.Errorf("cells[%d] is at row %d column %d, and the table is %d×%d",
				index, row, column, element.Table.Rows, element.Table.Columns)), nil
		}

		// A merged-away coordinate is not an empty cell. Slides accepts the write and puts
		// the text into the merged cell instead, on top of what is already there — the
		// heading of a merged first column ends up holding every row's text at once, and
		// the columns beside it look shifted by one. Refusing says where the text would
		// have gone and what to write instead.
		if cellAt(element.Table, row, column) == nil {
			if owner := mergedInto(element.Table, row, column); owner != nil {
				return toolError(fmt.Errorf("cells[%d] is at row %d column %d, which is inside the "+
					"merge that starts at row %d column %d: writing there would go into that cell "+
					"instead. Read the table with gdocs_slides_read_table and name the coordinates "+
					"the cells actually report",
					index, row, column, owner.RowIndex, owner.ColumnIndex)), nil
			}
		}

		text, _ := change["text"].(string)
		location := &google.CellLocation{RowIndex: row, ColumnIndex: column}

		// Emptying comes first and only where there is something to empty: Slides
		// refuses a delete over an empty cell the same way it refuses one over an empty
		// box.
		if existing := currentCellText(element.Table, row, column); existing != "" {
			requests = append(requests, google.Request{DeleteText: &google.DeleteTextRequest{
				ObjectID: objectID, CellLocation: location, TextRange: google.AllText(),
			}})
		}

		if text != "" {
			requests = append(requests, google.Request{InsertText: &google.InsertTextRequest{
				ObjectID: objectID, CellLocation: location, Text: text, InsertionIndex: 0,
			}})
		}
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"cells":           len(changes),
		"replies":         len(response.Replies),
	})
}

// currentCellText is what is in one cell now.
func currentCellText(table *google.Table, row, column int) string {
	cell := cellAt(table, row, column)
	if cell == nil {
		return ""
	}

	return strings.TrimSuffix(tableCellText(*cell), "\n")
}

// cellAt finds a cell by the coordinates it reports, not by its place in the row.
//
// A merge takes the cells it swallowed out of the row entirely: in a table whose first
// column is merged down five rows, the rows below hold two cells, and the one at index 0
// is column 1. Counting positions there reads the wrong cell — the old text is looked for
// where there is none, so it is never cleared and the new text lands in front of it.
func cellAt(table *google.Table, row, column int) *google.TableCell {
	if row < 0 || row >= len(table.TableRows) {
		return nil
	}

	cells := table.TableRows[row].TableCells
	for index := range cells {
		location := cells[index].Location
		if location == nil {
			// Without coordinates the position is all there is, and it is right as long
			// as nothing in this row was merged away.
			if index == column {
				return &cells[index]
			}
			continue
		}
		if location.RowIndex == row && location.ColumnIndex == column {
			return &cells[index]
		}
	}

	return nil
}

// mergedInto names the cell that swallowed the coordinates given, if one did.
//
// The distinction matters to a caller: a cell that does not exist because the table is
// smaller is a miscount, and a cell that does not exist because it was merged away is a
// table whose shape the caller has not read. Writing to the latter is silently answered by
// Slides — the text goes into the merged cell, on top of what is already there.
func mergedInto(table *google.Table, row, column int) *google.CellLocation {
	for rowIndex := range table.TableRows {
		cells := table.TableRows[rowIndex].TableCells
		for index := range cells {
			cell := cells[index]
			if cell.Location == nil {
				continue
			}

			rowSpan, columnSpan := cell.RowSpan, cell.ColumnSpan
			if rowSpan < 1 {
				rowSpan = 1
			}
			if columnSpan < 1 {
				columnSpan = 1
			}
			if rowSpan == 1 && columnSpan == 1 {
				continue
			}

			top, left := cell.Location.RowIndex, cell.Location.ColumnIndex
			if row >= top && row < top+rowSpan && column >= left && column < left+columnSpan {
				return cell.Location
			}
		}
	}

	return nil
}

// intField reads a whole number out of a decoded object, accepting the float JSON hands
// over.
func intField(object map[string]any, name string) (int, bool) {
	switch value := object[name].(type) {
	case float64:
		if value != float64(int(value)) {
			return 0, false
		}
		return int(value), true
	case int:
		return value, true
	default:
		return 0, false
	}
}

// slidesPlaceElement puts an element where a person would describe it: against an edge of
// the slide, or centred, with a margin.
//
// The arithmetic is done here rather than by the caller on purpose. An agent that works
// out coordinates itself gets them nearly right — a table forty thousand EMU off the
// margin the rest of the deck uses — and nearly right is what makes a deck look
// hand-made. Here the numbers come from the slide's own size and the element's own box.
func (r *registry) slidesPlaceElement(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	horizontal := strings.ToUpper(optionalString(req, "align"))
	vertical := strings.ToUpper(optionalString(req, "valign"))

	arguments := req.GetArguments()
	_, hasX := arguments["x_emu"]
	_, hasY := arguments["y_emu"]
	_, hasWidth := arguments["width_emu"]
	_, hasHeight := arguments["height_emu"]

	like := optionalString(req, "like_object_id")
	below := optionalString(req, "below_object_id")
	leftWith := optionalString(req, "left_aligned_with_object_id")

	if horizontal == "" && vertical == "" && !hasX && !hasY && !hasWidth && !hasHeight &&
		like == "" && below == "" && leftWith == "" {
		return toolError(fmt.Errorf("nothing to do: name like_object_id, below_object_id, " +
			"left_aligned_with_object_id, align, valign, x_emu, y_emu, width_emu or height_emu")), nil
	}

	switch horizontal {
	case "", alignLeft, alignCenter, alignRight:
	default:
		return toolError(fmt.Errorf("align %q is not one of LEFT, CENTER, RIGHT", horizontal)), nil
	}
	switch vertical {
	case "", alignTop, alignMiddle, alignBottom:
	default:
		return toolError(fmt.Errorf("valign %q is not one of TOP, MIDDLE, BOTTOM", vertical)), nil
	}

	margin := req.GetFloat("margin_emu", defaultMargin)
	if margin < 0 {
		return toolError(fmt.Errorf("margin_emu %g is negative", margin)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.Presentation(ctx, presentationID, geometryMask)
	if err != nil {
		return toolError(err), nil
	}

	if presentation.PageSize == nil || presentation.PageSize.Width == nil || presentation.PageSize.Height == nil {
		return toolError(fmt.Errorf("this presentation does not report its page size, " +
			"so there is nothing to measure a placement against")), nil
	}

	_, element, err := findElement(presentation, objectID)
	if err != nil {
		return toolError(err), nil
	}

	width, height, err := elementBox(element)
	if err != nil {
		return toolError(err), nil
	}

	// The transform is taken apart rather than edited in place. Scale, rotation and
	// mirroring share one 2×2 matrix, so setting a new width by writing scaleX into a
	// rotated element's matrix does not resize it — it unrotates it by an arbitrary amount.
	placement := decomposeTransform(element.Transform)

	// A table cannot be resized this way and pretending otherwise would silently scale
	// its text along with it.
	if element.Table != nil && (hasWidth || hasHeight) {
		return toolError(fmt.Errorf("a table's size is its column widths and row heights, not a scale: "+
			"use gdocs_slides_update_table_cells with column_widths_emu to change how wide %s is",
			objectID)), nil
	}

	// Resizing is scaling: Slides keeps the size an element was created with and applies
	// the transform on top, so a new width is a new scale against that original.
	if hasWidth {
		requested := req.GetFloat("width_emu", width)
		if requested <= 0 {
			return toolError(fmt.Errorf("width_emu %g is not positive", requested)), nil
		}
		placement.scaleX = requested / element.Size.Width.Magnitude
		width = requested
	}
	if hasHeight {
		requested := req.GetFloat("height_emu", height)
		if requested <= 0 {
			return toolError(fmt.Errorf("height_emu %g is not positive", requested)), nil
		}
		placement.scaleY = requested / element.Size.Height.Magnitude
		height = requested
	}

	pageWidth := presentation.PageSize.Width.Magnitude
	pageHeight := presentation.PageSize.Height.Magnitude

	// Copying a sample's geometry comes first, so an anchor named alongside it still wins.
	if like != "" {
		sampleDeck := presentationID
		if other := optionalString(req, "like_presentation_id"); other != "" {
			sampleDeck = other
		}

		sample := presentation
		if sampleDeck != presentationID {
			sample, err = client.Presentation(ctx, sampleDeck, geometryMask)
			if err != nil {
				return toolError(err), nil
			}
		}

		_, model, err := findElement(sample, like)
		if err != nil {
			return toolError(fmt.Errorf("the element to copy the placement from: %w", err)), nil
		}

		modelWidth, modelHeight, err := elementBox(model)
		if err != nil {
			return toolError(fmt.Errorf("the element to copy the placement from: %w", err)), nil
		}

		placement.x = model.Transform.TranslateX
		placement.y = model.Transform.TranslateY

		// The sample's turn and mirroring come with its position. Copying only the two
		// translations is what reproduced a tilted label upright, in the right place and
		// still visibly wrong.
		modelPlacement := decomposeTransform(model.Transform)
		placement.rotationDeg = modelPlacement.rotationDeg
		placement.flipH, placement.flipV = modelPlacement.flipH, modelPlacement.flipV

		// A table takes its size from its columns, so only its position is copied; the
		// widths travel separately, through update_table_cells.
		if element.Table == nil && element.Size != nil &&
			element.Size.Width != nil && element.Size.Height != nil {
			placement.scaleX = modelWidth / element.Size.Width.Magnitude
			placement.scaleY = modelHeight / element.Size.Height.Magnitude
			width, height = modelWidth, modelHeight
		}
	}

	// Relative placement: under an element of this slide, and lined up with one.
	if below != "" {
		_, anchor, err := findElement(presentation, below)
		if err != nil {
			return toolError(fmt.Errorf("the element to sit under: %w", err)), nil
		}

		_, anchorHeight, err := elementBox(anchor)
		if err != nil {
			return toolError(fmt.Errorf("the element to sit under: %w", err)), nil
		}

		placement.y = anchor.Transform.TranslateY + anchorHeight + req.GetFloat("gap_emu", defaultMargin/2)
	}

	if leftWith != "" {
		_, anchor, err := findElement(presentation, leftWith)
		if err != nil {
			return toolError(fmt.Errorf("the element to line up with: %w", err)), nil
		}
		placement.x = anchor.Transform.TranslateX
	}

	switch horizontal {
	case alignLeft:
		placement.x = margin
	case alignCenter:
		placement.x = (pageWidth - width) / 2
	case alignRight:
		placement.x = pageWidth - width - margin
	}

	switch vertical {
	case alignTop:
		placement.y = margin
	case alignMiddle:
		placement.y = (pageHeight - height) / 2
	case alignBottom:
		placement.y = pageHeight - height - margin
	}

	// An exact position outranks an anchor: a caller that measured something in the
	// sample deck means that number.
	if hasX {
		placement.x = req.GetFloat("x_emu", placement.x)
	}
	if hasY {
		placement.y = req.GetFloat("y_emu", placement.y)
	}

	// A turn named here outranks one copied from a sample, the way an exact position does.
	if _, ok := arguments["rotation_deg"]; ok {
		placement.rotationDeg = req.GetFloat("rotation_deg", 0)
	}
	if _, ok := arguments["flip_horizontally"]; ok {
		placement.flipH = req.GetBool("flip_horizontally", false)
	}
	if _, ok := arguments["flip_vertically"]; ok {
		placement.flipV = req.GetBool("flip_vertically", false)
	}

	// Hanging off an edge is reported, not refused. Real decks do it deliberately — a
	// title bled to the left margin, a picture running off the side — and a sample copied
	// with like_object_id carries those coordinates as they are. Refusing them made a
	// slide impossible to reproduce; the caller is told instead.
	offSlide := placement.x < 0 || placement.y < 0 ||
		placement.x+width > pageWidth || placement.y+height > pageHeight

	transform := placement.transform(element.Size, width, height)

	response, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		UpdatePageElementTransform: &google.UpdatePageElementTransformRequest{
			ObjectID:  objectID,
			Transform: transform,
			// ABSOLUTE replaces the whole transform, so the result does not depend on
			// where the element happened to be. RELATIVE would accumulate.
			ApplyMode: "ABSOLUTE",
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"x_emu":           transform.TranslateX,
		"y_emu":           transform.TranslateY,
		"width_emu":       width,
		"height_emu":      height,
		"page_size_emu":   map[string]float64{"width": pageWidth, "height": pageHeight},
		"replies":         len(response.Replies),
	}
	if offSlide {
		payload["off_slide"] = true
	}
	if placement.rotationDeg != 0 {
		payload["rotation_deg"] = placement.rotationDeg
	}

	return resultJSON(payload)
}

// slidesCreateTextBox adds a text box with the styling it was asked for.
func (r *registry) slidesCreateShape(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "page_object_id")
	if err != nil {
		return toolError(err), nil
	}

	// Text is optional: a panel drawn behind other elements has none, and demanding an
	// empty string for it would be a rule with no reason behind it.
	text := req.GetString("text", "")

	shapeType := strings.ToUpper(optionalString(req, "shape_type"))
	if shapeType == "" {
		shapeType = "TEXT_BOX"
	}

	box, err := boxArgs(req)
	if err != nil {
		return toolError(err), nil
	}

	objectID := optionalString(req, "object_id")
	if objectID == "" {
		objectID = r.objectID("textbox")
	}

	colour, err := parseColor(req, "foreground_color")
	if err != nil {
		return toolError(err), nil
	}

	alignment := strings.ToUpper(optionalString(req, "alignment"))
	switch alignment {
	case "", "START", "CENTER", "END", "JUSTIFIED":
	default:
		return toolError(fmt.Errorf("alignment %q is not one of START, CENTER, END, JUSTIFIED", alignment)), nil
	}

	requests := []google.Request{{
		CreateShape: &google.CreateShapeRequest{
			ObjectID:  objectID,
			ShapeType: shapeType,
			ElementProperties: &google.ElementProperties{
				PageObjectID: pageObjectID,
				Size: &google.Size{
					Width:  google.EMU(box.width),
					Height: google.EMU(box.height),
				},
				Transform: &google.Transform{
					ScaleX: 1, ScaleY: 1,
					TranslateX: box.x, TranslateY: box.y, Unit: "EMU",
				},
			},
		},
	}}

	// The look of the shape itself, before its text: createShape takes no properties of
	// its own, so a fill or an outline is a second request against what was just made.
	appearance, err := shapeStyleFrom(req)
	if err != nil {
		return toolError(err), nil
	}
	if appearance != nil {
		requests = append(requests, google.Request{
			UpdateShapeProperties: &google.UpdateShapePropertiesRequest{
				ObjectID:        objectID,
				ShapeProperties: appearance.style,
				Fields:          strings.Join(appearance.fields, ","),
			},
		})
	}

	if text != "" {
		requests = append(requests, google.Request{
			InsertText: &google.InsertTextRequest{ObjectID: objectID, Text: text, InsertionIndex: 0},
		})
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
	if _, ok := req.GetArguments()["bold"]; ok {
		bold := req.GetBool("bold", false)
		style.Bold = &bold
		fields = append(fields, "bold")
	}
	if colour != nil {
		style.ForegroundColor = &google.OptionalColor{OpaqueColor: &google.OpaqueColor{RGBColor: colour}}
		fields = append(fields, "foregroundColor")
	}

	if len(fields) > 0 && text != "" {
		requests = append(requests, google.Request{UpdateTextStyle: &google.UpdateTextStyleRequest{
			ObjectID:  objectID,
			TextRange: google.AllText(),
			Style:     style,
			Fields:    strings.Join(fields, ","),
		}})
	}

	if alignment != "" && text != "" {
		requests = append(requests, google.Request{UpdateParagraphStyle: &google.UpdateParagraphStyleRequest{
			ObjectID:  objectID,
			TextRange: google.AllText(),
			Style:     &google.ParagraphStyle{Alignment: alignment},
			Fields:    "alignment",
		}})
	}

	api, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	result, err := api.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"page_object_id":  pageObjectID,
		"object_id":       objectID,
		"shape_type":      shapeType,
		"characters":      utf16Length(text),
		"replies":         len(result.Replies),
	})
}

// slidesInsertImage puts a picture on a slide.
func (r *registry) slidesInsertImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "page_object_id")
	if err != nil {
		return toolError(err), nil
	}
	address, err := requiredString(req, "url")
	if err != nil {
		return toolError(err), nil
	}
	if !strings.HasPrefix(address, "https://") && !strings.HasPrefix(address, "http://") {
		return toolError(fmt.Errorf("url has to be an http or https address Google can fetch, got %q", address)), nil
	}

	objectID := optionalString(req, "object_id")
	if objectID == "" {
		objectID = r.objectID("image")
	}

	properties := &google.ElementProperties{PageObjectID: pageObjectID}

	arguments := req.GetArguments()
	_, hasWidth := arguments["width"]
	_, hasHeight := arguments["height"]
	_, hasX := arguments["x"]
	_, hasY := arguments["y"]

	if hasWidth != hasHeight {
		return toolError(fmt.Errorf("width and height go together: name both, or neither and let the " +
			"picture keep its own proportions")), nil
	}

	if hasWidth {
		width := req.GetFloat("width", 0)
		height := req.GetFloat("height", 0)
		if width <= 0 || height <= 0 {
			return toolError(fmt.Errorf("width and height are in EMU and have to be positive")), nil
		}
		properties.Size = &google.Size{Width: google.EMU(width), Height: google.EMU(height)}
	}

	if hasX || hasY {
		properties.Transform = &google.Transform{
			ScaleX: 1, ScaleY: 1,
			TranslateX: req.GetFloat("x", 0),
			TranslateY: req.GetFloat("y", 0),
			Unit:       "EMU",
		}
	}

	api, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	result, err := api.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		CreateImage: &google.CreateImageRequest{
			ObjectID:          objectID,
			URL:               address,
			ElementProperties: properties,
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"page_object_id":  pageObjectID,
		"object_id":       objectID,
		"replies":         len(result.Replies),
	})
}

// box is a rectangle in EMU.
type box struct {
	x, y, width, height float64
}

// boxArgs reads the four numbers that place a new element.
func boxArgs(req mcp.CallToolRequest) (box, error) {
	var result box

	for _, field := range []struct {
		name   string
		target *float64
	}{
		{"x", &result.x},
		{"y", &result.y},
		{"width", &result.width},
		{"height", &result.height},
	} {
		value, err := req.RequireFloat(field.name)
		if err != nil {
			return box{}, err
		}
		*field.target = value
	}

	if result.width <= 0 || result.height <= 0 {
		return box{}, fmt.Errorf("width and height are in EMU and have to be positive, got %g and %g",
			result.width, result.height)
	}

	return result, nil
}

// findElement locates any element, not just a shape: a table or a picture is placed as
// readily as a text box.
//
// Slides first, then the layouts and the master, because that is the order of how often an
// identifier belongs to each — and because a layout's placeholder has to be findable at all:
// it is the only way a template's grid can be changed in one place instead of on every
// slide. Whether the page carrying an element is a slide makes no difference to the request
// that moves it.
func findElement(presentation *google.Presentation, objectID string) (*google.Page, *google.PageElement, error) {
	for _, pages := range [][]google.Page{presentation.Slides, presentation.Layouts, presentation.Masters} {
		for pageIndex := range pages {
			page := &pages[pageIndex]
			for elementIndex := range page.PageElements {
				if page.PageElements[elementIndex].ObjectID == objectID {
					return page, &page.PageElements[elementIndex], nil
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("no object %s on any slide, layout or master of this presentation", objectID)
}

// placement is a transform taken apart into the things a caller reasons about: where the
// element sits, how much it is scaled, how far it is turned and whether it is mirrored.
//
// Slides keeps all of that in one 2×2 matrix plus a translation, and the parts are not
// separable by reading a field: a turned element has its scale spread across scaleX and
// shearY. Editing the matrix directly — writing a new scaleX to resize something — works
// only while nothing is turned, and silently unturns it when something is.
type placement struct {
	scaleX, scaleY float64
	// RotationDeg is clockwise, the direction Slides' own rotation handle turns.
	rotationDeg  float64
	flipH, flipV bool
	// X and Y are the top-left corner of the box before any turning, which is what every
	// alignment here is measured against.
	x, y float64
}

// decomposeTransform takes a transform apart.
func decomposeTransform(transform *google.Transform) placement {
	if transform == nil {
		return placement{scaleX: 1, scaleY: 1}
	}

	scaleX, shearX := unitScale(transform.ScaleX), transform.ShearX
	scaleY, shearY := unitScale(transform.ScaleY), transform.ShearY

	// The columns of the matrix are the element's own axes after the transform: the length
	// of each is its scale, and the angle of the first is the rotation.
	lengthX := math.Hypot(scaleX, shearY)
	lengthY := math.Hypot(shearX, scaleY)
	if lengthX == 0 {
		lengthX = 1
	}
	if lengthY == 0 {
		lengthY = 1
	}

	angle := math.Atan2(shearY, scaleX) * 180 / math.Pi

	// A negative determinant means the element is mirrored rather than merely turned. The
	// two look alike on a symmetrical shape and nothing alike on an arrow.
	mirrored := scaleX*scaleY-shearX*shearY < 0

	return placement{
		scaleX:      lengthX,
		scaleY:      lengthY,
		rotationDeg: math.Round(angle*10) / 10,
		flipH:       mirrored,
		x:           transform.TranslateX,
		y:           transform.TranslateY,
	}
}

// transform builds the matrix to send.
//
// Without a turn this is the plain scale-and-translate the API has always been given, so
// an element that is not rotated comes out with exactly the numbers it used to. With a
// turn, the translation is corrected so the element pivots about its own centre — the way
// Slides' rotation handle does — instead of about the slide's origin, which would fling
// it off the page.
func (p placement) transform(size *google.Size, width, height float64) *google.Transform {
	scaleX, scaleY := p.scaleX, p.scaleY
	if p.flipH {
		scaleX = -scaleX
	}
	if p.flipV {
		scaleY = -scaleY
	}

	if p.rotationDeg == 0 && !p.flipH && !p.flipV {
		return &google.Transform{
			ScaleX: scaleX, ScaleY: scaleY,
			TranslateX: p.x, TranslateY: p.y,
			Unit: "EMU",
		}
	}

	radians := p.rotationDeg * math.Pi / 180
	cos, sin := zeroTiny(math.Cos(radians)), zeroTiny(math.Sin(radians))

	built := &google.Transform{
		ScaleX: cos * scaleX,
		ShearX: -sin * scaleY,
		ShearY: sin * scaleX,
		ScaleY: cos * scaleY,
		Unit:   "EMU",
	}

	// The matrix multiplies the element's own coordinates, which start at its top-left
	// corner. To keep the centre where the caller placed it, the translation is the wanted
	// centre minus where the matrix sends the element's own centre.
	var halfWidth, halfHeight float64
	if size != nil && size.Width != nil && size.Height != nil {
		halfWidth, halfHeight = size.Width.Magnitude/2, size.Height.Magnitude/2
	}

	built.TranslateX = p.x + width/2 - (built.ScaleX*halfWidth + built.ShearX*halfHeight)
	built.TranslateY = p.y + height/2 - (built.ShearY*halfWidth + built.ScaleY*halfHeight)

	return built
}

// zeroTiny rounds away the dust left by trigonometry.
//
// The cosine of 90° comes out as 6e-17 rather than 0, and a transform carrying that is
// a transform no two runs agree on. Nothing a slide is measured in is smaller than this,
// so rounding it away costs no accuracy and makes the request bodies readable.
func zeroTiny(value float64) float64 {
	if math.Abs(value) < 1e-12 {
		return 0
	}

	return value
}

// elementBox is how much room an element takes on the slide.
//
// Slides reports a size and a transform separately, and the size is the untransformed
// box: an element scaled by two reports the same size and covers twice the width. Placing
// it by the reported size alone puts a scaled element in the wrong place, which is the
// kind of near-miss this tool exists to prevent.
func elementBox(element *google.PageElement) (float64, float64, error) {
	if element.Transform == nil {
		return 0, 0, fmt.Errorf("%s does not report a transform", element.ObjectID)
	}

	// The scale is the length of each column of the matrix, not the scaleX and scaleY
	// fields: on a turned element the two are spread across all four numbers, and reading
	// the fields alone reports an element turned by 30° as 13% smaller than it is.
	turned := decomposeTransform(element.Transform)
	scaleX, scaleY := turned.scaleX, turned.scaleY

	// A table lies about its size. Slides reports 3000000×3000000 for every table it has
	// ever made, whatever was asked for at creation, because a table's real extent is the
	// sum of its column widths and row heights. Centring one by the reported size puts it
	// a long way off.
	if element.Table != nil {
		width, height := tableExtent(element.Table)
		if width > 0 && height > 0 {
			return width * scaleX, height * scaleY, nil
		}
	}

	if element.Size == nil || element.Size.Width == nil || element.Size.Height == nil {
		return 0, 0, fmt.Errorf("%s does not report a size, so it cannot be placed by its edges",
			element.ObjectID)
	}

	return element.Size.Width.Magnitude * scaleX, element.Size.Height.Magnitude * scaleY, nil
}

// tableExtent is how much room a table actually takes: its columns across, its rows down.
func tableExtent(table *google.Table) (float64, float64) {
	var width, height float64

	for _, column := range table.TableColumns {
		if column.ColumnWidth != nil {
			width += column.ColumnWidth.Magnitude
		}
	}
	for _, row := range table.TableRows {
		if row.RowHeight != nil {
			height += row.RowHeight.Magnitude
		}
	}

	return width, height
}
