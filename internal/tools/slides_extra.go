package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerSlidesExtra adds the rest of what Slides v1 can do to a deck: growing a table
// after it exists, its borders, swapping pictures, alt text, connectors, live charts from
// a workbook, and video.
//
// The table borders are the notable one. Everything else about a table is written per
// cell, and the lines are not: they are addressed by position across a rectangle — all of
// them, the outside, the inside, or one side — which is why a table's frame is one request
// rather than a loop that leaves seams where two cells disagree.
func (r *registry) registerSlidesExtra(srv *server.MCPServer) {
	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_slides_edit_table",
		mcp.WithDescription("Change a table's shape after it exists: add rows or columns beside a cell, or "+
			"take a merged rectangle apart. The size a table was created with is not final."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("table_object_id", mcp.Required(), mcp.Description("Table to change.")),
		mcp.WithString("what", mcp.Required(), mcp.Description("insert_rows, insert_columns or unmerge.")),
		mcp.WithNumber("row", mcp.Required(), mcp.Description("Row of the cell to work beside, from 0.")),
		mcp.WithNumber("column", mcp.Required(), mcp.Description("Column of that cell, from 0.")),
		mcp.WithNumber("count", mcp.DefaultNumber(1), mcp.Description("How many rows or columns to add.")),
		mcp.WithBoolean("after", mcp.DefaultBool(true), mcp.Description(
			"Add below or to the right. False adds before.")),
		mcp.WithNumber("row_span", mcp.Description("For unmerge: how many rows the merged rectangle covers.")),
		mcp.WithNumber("column_span", mcp.Description("For unmerge: how many columns it covers.")),
	), r.slidesEditTable)

	srv.AddTool(mcp.NewTool("gdocs_slides_set_table_borders",
		mcp.WithDescription("Draw the lines of a table: colour, weight and dash style, for all of them, the "+
			"outside, the inside, or one side at a time. Borders are the one part of a table that is not "+
			"written per cell — a rectangle and a position, and the whole frame comes out even."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("table_object_id", mcp.Required(), mcp.Description("Table to draw.")),
		mcp.WithString("position", mcp.DefaultString("ALL"), mcp.Description(
			"ALL, BOTTOM, INNER, INNER_HORIZONTAL, INNER_VERTICAL, LEFT, OUTER, RIGHT or TOP.")),
		mcp.WithString("color", mcp.Description("Line colour as #RRGGBB, or a theme colour name.")),
		mcp.WithNumber("weight_emu", mcp.Description("Line thickness in EMU. 9525 EMU is one point.")),
		mcp.WithString("dash_style", mcp.Description("SOLID, DOT, DASH, DASH_DOT, LONG_DASH, LONG_DASH_DOT.")),
		mcp.WithNumber("row", mcp.Description("Restrict to a rectangle: first row, from 0.")),
		mcp.WithNumber("column", mcp.Description("First column, from 0.")),
		mcp.WithNumber("row_span", mcp.Description("How many rows the rectangle covers.")),
		mcp.WithNumber("column_span", mcp.Description("How many columns it covers.")),
	), r.slidesSetTableBorders)

	srv.AddTool(mcp.NewTool("gdocs_slides_replace_image",
		mcp.WithDescription("Swap a picture's content while it keeps its place, its size and its crop. This "+
			"is how a template's placeholder picture becomes this deck's picture without anybody "+
			"positioning anything."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("image_object_id", mcp.Required(), mcp.Description("Picture to replace.")),
		mcp.WithString("url", mcp.Required(), mcp.Description("Address of the new picture, reachable by Google.")),
		mcp.WithString("method", mcp.DefaultString("CENTER_CROP"), mcp.Description(
			"CENTER_CROP fills the old frame and crops the overflow.")),
	), r.slidesReplaceImage)

	srv.AddTool(mcp.NewTool("gdocs_slides_replace_text",
		mcp.WithDescription("Swap one stretch of text for another wherever it appears in the deck, keeping "+
			"the styling of the words it stands among. This is how a word, a marker or a date is changed "+
			"across twenty slides: gdocs_slides_set_text replaces a box's whole text and drops the "+
			"paragraphs' styling with it, so a panel of a bold heading over grey bullets comes back as "+
			"one flat block. Replacing an empty string is refused — it would insert the replacement "+
			"before every character in the deck."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("find", mcp.Required(), mcp.Description("Text to look for.")),
		mcp.WithString("replace", mcp.Required(), mcp.Description(
			"Text to put in its place. An empty string deletes what was found.")),
		mcp.WithBoolean("match_case", mcp.DefaultBool(true), mcp.Description("Match upper and lower case exactly.")),
		mcp.WithArray("page_object_ids", mcp.WithStringItems(), mcp.Description(
			"Limit to these slides. Without them, the whole deck.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesReplaceText)

	srv.AddTool(mcp.NewTool("gdocs_slides_replace_shapes_with_image",
		mcp.WithDescription("Turn every shape whose text matches into a picture, keeping the shape's place "+
			"and size. A template marked up with {{photo}} boxes becomes an illustrated deck in one call."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("contains_text", mcp.Required(), mcp.Description("Text the shape has to contain.")),
		mcp.WithString("url", mcp.Required(), mcp.Description("Address of the picture.")),
		mcp.WithBoolean("match_case", mcp.DefaultBool(true), mcp.Description("Match upper and lower case exactly.")),
		mcp.WithArray("page_object_ids", mcp.WithStringItems(), mcp.Description(
			"Limit to these slides. Without them, the whole deck.")),
		mcp.WithString("method", mcp.DefaultString("CENTER_INSIDE"), mcp.Description(
			"CENTER_INSIDE fits the picture inside the shape; CENTER_CROP fills and crops.")),
	), r.slidesReplaceShapesWithImage)

	srv.AddTool(mcp.NewTool("gdocs_slides_replace_shapes_with_chart",
		mcp.WithDescription("Turn every shape whose text matches into a chart from a spreadsheet, keeping "+
			"the shape's place and size. The template says where the chart goes; the workbook says what "+
			"it shows."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("contains_text", mcp.Required(), mcp.Description("Text the shape has to contain.")),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description("Workbook the chart lives in.")),
		mcp.WithNumber("chart_id", mcp.Required(), mcp.Description("Chart identifier inside that workbook.")),
		mcp.WithBoolean("linked", mcp.DefaultBool(true), mcp.Description("Keep it linked so it can be refreshed.")),
		mcp.WithBoolean("match_case", mcp.DefaultBool(true), mcp.Description("Match upper and lower case exactly.")),
		mcp.WithArray("page_object_ids", mcp.WithStringItems(), mcp.Description(
			"Limit to these slides. Without them, the whole deck.")),
	), r.slidesReplaceShapesWithChart)

	srv.AddTool(mcp.NewTool("gdocs_slides_set_alt_text",
		mcp.WithDescription("Give an element the title and description a screen reader reads out. A deck "+
			"that will be shared outside the team needs these, and nothing else in the API writes them."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Element to describe.")),
		mcp.WithString("title", mcp.Description("Short title.")),
		mcp.WithString("description", mcp.Description("What the element shows.")),
	), r.slidesSetAltText)

	srv.AddTool(mcp.NewTool("gdocs_slides_route_line",
		mcp.WithDescription("Set how a connector runs between the shapes it joins — straight, bent or "+
			"curved — and make it find its way again after they have moved. A connector that was drawn "+
			"before its shapes were positioned stays where it was drawn until this is called."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Connector to route.")),
		mcp.WithString("category", mcp.Description("STRAIGHT, BENT or CURVED. Omit to only reroute.")),
		mcp.WithBoolean("reroute", mcp.DefaultBool(true), mcp.Description(
			"Recompute the path between the shapes it is attached to.")),
	), r.slidesRouteLine)

	srv.AddTool(mcp.NewTool("gdocs_slides_add_sheets_chart",
		mcp.WithDescription("Put a chart from a spreadsheet onto a slide. Linked, it keeps a thread back to "+
			"the workbook and is brought up to date with refresh=true later; not linked, it is a picture "+
			"of the chart as it is now. The chart identifier comes from gdocs_sheets_info."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description("Slide to put it on.")),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description("Workbook the chart lives in.")),
		mcp.WithNumber("chart_id", mcp.Required(), mcp.Description("Chart identifier inside that workbook.")),
		mcp.WithBoolean("linked", mcp.DefaultBool(true), mcp.Description(
			"Keep it linked to the workbook so it can be refreshed.")),
		mcp.WithNumber("x_emu", mcp.Description("Left edge in EMU.")),
		mcp.WithNumber("y_emu", mcp.Description("Top edge in EMU.")),
		mcp.WithNumber("width_emu", mcp.Description("Width in EMU.")),
		mcp.WithNumber("height_emu", mcp.Description("Height in EMU.")),
	), r.slidesAddSheetsChart)

	srv.AddTool(mcp.NewTool("gdocs_slides_refresh_sheets_chart",
		mcp.WithDescription("Pull the current state of a linked chart from its workbook. A deck built last "+
			"quarter shows last quarter's numbers until this is called."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description("Chart on the slide.")),
	), r.slidesRefreshSheetsChart)

	srv.AddTool(mcp.NewTool("gdocs_slides_add_video",
		mcp.WithDescription("Put a video on a slide, from YouTube or from Drive, and set how it plays."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description("Slide to put it on.")),
		mcp.WithString("video_id", mcp.Required(), mcp.Description(
			"YouTube identifier, or the Drive file identifier for source=DRIVE.")),
		mcp.WithString("source", mcp.DefaultString("YOUTUBE"), mcp.Description("YOUTUBE or DRIVE.")),
		mcp.WithNumber("x_emu", mcp.Description("Left edge in EMU.")),
		mcp.WithNumber("y_emu", mcp.Description("Top edge in EMU.")),
		mcp.WithNumber("width_emu", mcp.Description("Width in EMU.")),
		mcp.WithNumber("height_emu", mcp.Description("Height in EMU.")),
		mcp.WithBoolean("autoplay", mcp.Description("Start playing when the slide is shown.")),
		mcp.WithBoolean("mute", mcp.Description("Play without sound.")),
		mcp.WithNumber("start_seconds", mcp.Description("Start at this many seconds in.")),
		mcp.WithNumber("end_seconds", mcp.Description("Stop at this many seconds in.")),
	), r.slidesAddVideo)
}

func (r *registry) slidesEditTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	tableID, err := requiredString(req, "table_object_id")
	if err != nil {
		return toolError(err), nil
	}

	cell := &google.CellLocation{RowIndex: req.GetInt("row", 0), ColumnIndex: req.GetInt("column", 0)}
	count := req.GetInt("count", 1)
	if count < 1 {
		count = 1
	}
	after := req.GetBool("after", true)

	request := google.Request{}
	switch what := strings.ToLower(optionalString(req, "what")); what {
	case "insert_rows":
		request.InsertTableRows = &google.InsertTableRowsRequest{
			TableObjectID: tableID, CellLocation: cell, InsertBelow: after, Number: count,
		}
	case "insert_columns":
		request.InsertTableColumns = &google.InsertTableColumnsRequest{
			TableObjectID: tableID, CellLocation: cell, InsertRight: after, Number: count,
		}
	case "unmerge":
		request.UnmergeTableCells = &google.UnmergeTableCellsRequest{
			ObjectID: tableID,
			TableRange: &google.TableRange{
				Location:   cell,
				RowSpan:    req.GetInt("row_span", 1),
				ColumnSpan: req.GetInt("column_span", 1),
			},
		}
	default:
		return toolError(fmt.Errorf("what is insert_rows, insert_columns or unmerge, got %q", what)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{request}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"table_object_id": tableID,
		"what":            optionalString(req, "what"),
	})
}

func (r *registry) slidesSetTableBorders(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	tableID, err := requiredString(req, "table_object_id")
	if err != nil {
		return toolError(err), nil
	}

	properties := &google.TableBorderProperties{}
	var fields []string

	if colour := optionalString(req, "color"); colour != "" {
		// A line's colour is either a literal or a name from the theme, and a theme name
		// is the one that keeps following the deck when its palette changes.
		opaque := &google.OpaqueColor{}
		if strings.HasPrefix(colour, "#") {
			rgb, err := parseHexColor(colour)
			if err != nil {
				return toolError(err), nil
			}
			opaque.RGBColor = rgb
		} else {
			opaque.ThemeColor = strings.ToUpper(colour)
		}

		properties.Fill = &google.TableBorderFill{SolidFill: &google.SolidFill{Color: opaque, Alpha: 1}}
		fields = append(fields, "tableBorderFill")
	}
	if weight := req.GetFloat("weight_emu", 0); weight > 0 {
		properties.Weight = google.EMU(weight)
		fields = append(fields, "weight")
	}
	if dash := optionalString(req, "dash_style"); dash != "" {
		properties.DashStyle = dash
		fields = append(fields, "dashStyle")
	}

	if len(fields) == 0 {
		return toolError(fmt.Errorf("nothing to draw: give color, weight_emu or dash_style")), nil
	}

	request := google.UpdateTableBorderRequest{
		ObjectID:   tableID,
		Position:   strings.ToUpper(req.GetString("position", "ALL")),
		Properties: properties,
		Fields:     strings.Join(fields, ","),
	}

	// A rectangle is optional: without one the position applies to the whole table, which
	// is what "give this table a frame" means.
	if _, given := req.GetArguments()["row"]; given {
		request.TableRange = &google.TableRange{
			Location:   &google.CellLocation{RowIndex: req.GetInt("row", 0), ColumnIndex: req.GetInt("column", 0)},
			RowSpan:    req.GetInt("row_span", 1),
			ColumnSpan: req.GetInt("column_span", 1),
		}
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{UpdateTableBorder: &request}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"table_object_id": tableID,
		"position":        request.Position,
		"fields":          request.Fields,
	})
}

func (r *registry) slidesReplaceImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "image_object_id")
	if err != nil {
		return toolError(err), nil
	}
	url, err := requiredString(req, "url")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		ReplaceImage: &google.ReplaceImageRequest{
			ImageObjectID: objectID, URL: url,
			Method: req.GetString("method", "CENTER_CROP"),
		},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"presentation_id": presentationID, "image_object_id": objectID})
}

func (r *registry) slidesReplaceText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	find, err := requiredString(req, "find")
	if err != nil {
		return toolError(err), nil
	}
	replacement, err := req.RequireString("replace")
	if err != nil {
		return toolError(err), nil
	}

	pages, err := stringListField(req.GetArguments(), "page_object_ids")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	replies, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		ReplaceAllText: &google.ReplaceAllTextRequest{
			ContainsText:  &google.SlidesTextMatch{Text: find, MatchCase: req.GetBool("match_case", true)},
			ReplaceText:   replacement,
			PageObjectIDs: pages,
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	// Google answers with how many boxes it touched. A caller that asked for a word it
	// misspelled gets a silent success otherwise, and finds out on the render.
	occurrences := 0
	if replies != nil && len(replies.Replies) > 0 && replies.Replies[0].ReplaceAllText != nil {
		occurrences = replies.Replies[0].ReplaceAllText.OccurrencesChanged
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"find":            find,
		"replace":         replacement,
		"occurrences":     occurrences,
	})
}

func (r *registry) slidesReplaceShapesWithImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	text, err := requiredString(req, "contains_text")
	if err != nil {
		return toolError(err), nil
	}
	url, err := requiredString(req, "url")
	if err != nil {
		return toolError(err), nil
	}

	pages, err := stringListField(req.GetArguments(), "page_object_ids")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		ReplaceShapesWithImage: &google.ReplaceShapesWithImageRequest{
			ContainsText:  &google.SlidesTextMatch{Text: text, MatchCase: req.GetBool("match_case", true)},
			ImageURL:      url,
			Method:        req.GetString("method", "CENTER_INSIDE"),
			PageObjectIDs: pages,
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	replaced := 0
	for _, reply := range response.Replies {
		if reply.ReplaceAllShapesWithImage != nil {
			replaced += reply.ReplaceAllShapesWithImage.OccurrencesChanged
		}
	}

	payload := map[string]any{
		"presentation_id": presentationID,
		"contains_text":   text,
		"shapes_replaced": replaced,
	}
	if replaced == 0 {
		payload["note"] = "no shape matched, so nothing changed: check contains_text, match_case " +
			"and page_object_ids against what gdocs_slides_inspect_page reports"
	}

	return resultJSON(payload)
}

func (r *registry) slidesReplaceShapesWithChart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	text, err := requiredString(req, "contains_text")
	if err != nil {
		return toolError(err), nil
	}
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	chartID, err := req.RequireInt("chart_id")
	if err != nil {
		return toolError(err), nil
	}

	pages, err := stringListField(req.GetArguments(), "page_object_ids")
	if err != nil {
		return toolError(err), nil
	}

	linking := "NOT_LINKED_IMAGE"
	if req.GetBool("linked", true) {
		linking = "LINKED"
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		ReplaceShapesWithChart: &google.ReplaceShapesWithChartRequest{
			ContainsText:  &google.SlidesTextMatch{Text: text, MatchCase: req.GetBool("match_case", true)},
			SpreadsheetID: spreadsheetID,
			ChartID:       chartID,
			LinkingMode:   linking,
			PageObjectIDs: pages,
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	// A batch that matched no shape succeeds, so the count is the whole of the answer:
	// without it "the chart is on the slide" and "the text matched nothing" look the same
	// from here, and the second one is only noticed when somebody opens the deck.
	replaced := 0
	for _, reply := range response.Replies {
		if reply.ReplaceAllShapesWithSheetsChart != nil {
			replaced += reply.ReplaceAllShapesWithSheetsChart.OccurrencesChanged
		}
	}

	payload := map[string]any{
		"presentation_id": presentationID,
		"contains_text":   text,
		"linking_mode":    linking,
		"shapes_replaced": replaced,
	}
	if replaced == 0 {
		payload["note"] = "no shape matched, so nothing changed: check contains_text, match_case " +
			"and page_object_ids against what gdocs_slides_inspect_page reports"
	}

	return resultJSON(payload)
}

func (r *registry) slidesSetAltText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	title := optionalString(req, "title")
	description := optionalString(req, "description")
	if title == "" && description == "" {
		return toolError(fmt.Errorf("give a title, a description, or both")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		UpdateAltText: &google.UpdateAltTextRequest{ObjectID: objectID, Title: title, Description: description},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"presentation_id": presentationID, "object_id": objectID})
}

func (r *registry) slidesRouteLine(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	var requests []google.Request
	if category := strings.ToUpper(optionalString(req, "category")); category != "" {
		requests = append(requests, google.Request{
			UpdateLineCategory: &google.UpdateLineCategoryRequest{ObjectID: objectID, LineCategory: category},
		})
	}
	if req.GetBool("reroute", true) {
		requests = append(requests, google.Request{
			RerouteLine: &google.RerouteLineRequest{ObjectID: objectID},
		})
	}

	if len(requests) == 0 {
		return toolError(fmt.Errorf("nothing to do: give a category, or leave reroute on")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SlidesBatchUpdate(ctx, presentationID, requests); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"requests":        len(requests),
	})
}

func (r *registry) slidesAddSheetsChart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageID, err := requiredString(req, "page_object_id")
	if err != nil {
		return toolError(err), nil
	}
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	chartID, err := req.RequireInt("chart_id")
	if err != nil {
		return toolError(err), nil
	}

	linking := "NOT_LINKED_IMAGE"
	if req.GetBool("linked", true) {
		linking = "LINKED"
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	objectID := r.objectID("chart")

	if _, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		CreateSheetsChart: &google.CreateSheetsChartRequest{
			ObjectID:      objectID,
			SpreadsheetID: spreadsheetID,
			ChartID:       chartID,
			LinkingMode:   linking,
			Element:       elementPlacement(req, pageID),
		},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"linking_mode":    linking,
	})
}

func (r *registry) slidesRefreshSheetsChart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	if _, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		RefreshSheetsChart: &google.RefreshSheetsChartRequest{ObjectID: objectID},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"presentation_id": presentationID, "object_id": objectID})
}

func (r *registry) slidesAddVideo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageID, err := requiredString(req, "page_object_id")
	if err != nil {
		return toolError(err), nil
	}
	videoID, err := requiredString(req, "video_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	objectID := r.objectID("video")
	requests := []google.Request{{
		CreateVideo: &google.CreateVideoRequest{
			ObjectID: objectID,
			ID:       videoID,
			Source:   strings.ToUpper(req.GetString("source", "YOUTUBE")),
			Element:  elementPlacement(req, pageID),
		},
	}}

	properties := &google.VideoProperties{}
	var fields []string
	if _, given := req.GetArguments()["autoplay"]; given {
		value := req.GetBool("autoplay", false)
		properties.AutoPlay = &value
		fields = append(fields, "autoPlay")
	}
	if _, given := req.GetArguments()["mute"]; given {
		value := req.GetBool("mute", false)
		properties.Mute = &value
		fields = append(fields, "mute")
	}
	if seconds := req.GetInt("start_seconds", -1); seconds >= 0 {
		properties.Start = &seconds
		fields = append(fields, "start")
	}
	if seconds := req.GetInt("end_seconds", -1); seconds >= 0 {
		properties.End = &seconds
		fields = append(fields, "end")
	}

	if len(fields) > 0 {
		requests = append(requests, google.Request{
			UpdateVideoProperties: &google.UpdateVideoPropertiesRequest{
				ObjectID: objectID, Properties: properties, Fields: strings.Join(fields, ","),
			},
		})
	}

	if _, err := client.SlidesBatchUpdate(ctx, presentationID, requests); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"presentation_id": presentationID, "object_id": objectID})
}

// elementPlacement is where a new element goes, when the caller said. Without a position
// Slides puts it at a default place and size, which is what "just put it on the slide"
// should mean.
func elementPlacement(req mcp.CallToolRequest, pageID string) *google.ElementProperties {
	placement := &google.ElementProperties{PageObjectID: pageID}

	width := req.GetFloat("width_emu", 0)
	height := req.GetFloat("height_emu", 0)
	if width > 0 || height > 0 {
		placement.Size = &google.Size{}
		if width > 0 {
			placement.Size.Width = google.EMU(width)
		}
		if height > 0 {
			placement.Size.Height = google.EMU(height)
		}
	}

	x := req.GetFloat("x_emu", 0)
	y := req.GetFloat("y_emu", 0)
	if x != 0 || y != 0 || placement.Size != nil {
		placement.Transform = &google.Transform{
			ScaleX: 1, ScaleY: 1, TranslateX: x, TranslateY: y, Unit: "EMU",
		}
	}

	return placement
}
