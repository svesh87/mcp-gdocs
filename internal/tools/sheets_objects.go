package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerSheetsObjects adds the things that sit on a tab rather than in its cells —
// charts, slicers, saved filter views — and the labels that survive the rows moving.
//
// The one worth explaining is developer metadata. Every other way of remembering "this is
// the row the total is on" is a row number, and a row number is wrong the moment somebody
// inserts a line above it. Metadata is attached to the row itself and travels with it, so
// a workbook an agent edits repeatedly can find its own landmarks again.
func (r *registry) registerSheetsObjects(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_sheets_list_metadata",
		mcp.WithDescription("Read the developer metadata of a workbook: the labels attached to rows, "+
			"columns, tabs or the workbook itself. Unlike a row number, a label moves with what it is "+
			"attached to, which is what makes it worth reading before an edit that shifts anything."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("key", mcp.Description("Only the labels under this key.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.sheetsListMetadata)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_sheets_update_chart",
		mcp.WithDescription("Change a chart that already exists: move it, resize it, give it a border, or "+
			"replace what it draws. Changing beats making a new one — a chart recreated loses its place "+
			"on the tab and every reference to it from a slide."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithNumber("chart_id", mcp.Required(), mcp.Description(
			"Chart identifier, as gdocs_sheets_info reports it.")),
		mcp.WithString("sheet_title", mcp.Description("Tab to move it to, or the one it is already on.")),
		mcp.WithNumber("anchor_row", mcp.Description("Row of the cell it hangs from, from 0.")),
		mcp.WithNumber("anchor_column", mcp.Description("Column of that cell, from 0.")),
		mcp.WithNumber("offset_x", mcp.Description("Pixels right of that cell's corner.")),
		mcp.WithNumber("offset_y", mcp.Description("Pixels below it.")),
		mcp.WithNumber("width", mcp.Description("Width in pixels.")),
		mcp.WithNumber("height", mcp.Description("Height in pixels.")),
		mcp.WithString("border_color", mcp.Description("Frame colour as #RRGGBB.")),
		mcp.WithBoolean("no_border", mcp.Description(
			"Take the frame off altogether. Not the same as painting it the colour behind it: a "+
				"border painted to match still draws, and on a panel's rounded corner it leaves thin "+
				"strokes along the edge. This is what makes a chart sit in a panel with no seam.")),
		mcp.WithString("title", mcp.Description("New title above the chart.")),
		mcp.WithString("subtitle", mcp.Description("New subtitle.")),
		mcp.WithString("alt_text", mcp.Description("Description for a screen reader.")),
		mcp.WithString("font", mcp.Description("Font family for the chart's own text.")),
		mcp.WithNumber("font_size_pt", mcp.Description(
			"Size of the chart's title text, in points. A chart squeezed onto a slide draws its "+
				"own text smaller than the shapes beside it.")),
		mcp.WithString("background_color", mcp.Description(
			"What the chart is drawn on, as #RRGGBB. A chart sitting inside a panel on a slide "+
				"arrives with a white rectangle and paints over it; match the panel here. A deck in a "+
				"dark variant cannot be built without this — the chart lives in the workbook and "+
				"inherits nothing from the deck's palette.")),
		mcp.WithBoolean("transparent_background", mcp.Description(
			"Refused, and here to say why: Google does not accept an alpha on a chart's "+
				"background at all. Match the panel with background_color instead.")),
		mcp.WithBoolean("data_labels", mcp.Description(
			"Print each value on its bar, column or point; false takes the numbers off again.")),
		mcp.WithBoolean("total_data_labels", mcp.Description(
			"Print the total over each stacked column; false takes it off. On a column stacked from "+
				"several categories this is the only place the total appears at all.")),
		mcp.WithString("data_sheet_title", mcp.Description(
			"Tab the numbers are on, when the data range is being changed.")),
		mcp.WithNumber("labels_column", mcp.Description(
			"Column of the names along the bottom, from 0. Part of changing the data range.")),
		mcp.WithArray("value_columns", mcp.WithNumberItems(), mcp.Description(
			"Columns of numbers to draw, from 0. Giving these points the chart at a new range, "+
				"keeping its number and so keeping every slide that shows it — which deleting and "+
				"drawing it again does not.")),
		mcp.WithNumber("start_row", mcp.Description("First row of the data, from 0.")),
		mcp.WithNumber("end_row", mcp.Description("Row to stop before.")),
		mcp.WithNumber("header_rows", mcp.Description("How many rows at the top are headings.")),
	), r.sheetsUpdateChart)

	srv.AddTool(mcp.NewTool("gdocs_sheets_filter_view",
		mcp.WithDescription("Save a way of looking at a range — its own filter and sort — without changing "+
			"what anybody else sees. This is the difference between a filter and a filter view: the "+
			"first one hides rows for everyone in the workbook, the second one only for whoever opens "+
			"it. Give a filter_view_id to change one, or omit it to make a new one."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the view is over.")),
		mcp.WithString("title", mcp.Description("Name of the view.")),
		mcp.WithNumber("filter_view_id", mcp.Description("Existing view to change, or to duplicate.")),
		mcp.WithBoolean("duplicate", mcp.DefaultBool(false), mcp.Description(
			"Copy the view named by filter_view_id instead of changing it.")),
		mcp.WithNumber("start_row", mcp.Description("First row of the range, from 0.")),
		mcp.WithNumber("end_row", mcp.Description("One past the last row.")),
		mcp.WithNumber("start_column", mcp.Description("First column, from 0.")),
		mcp.WithNumber("end_column", mcp.Description("One past the last column.")),
		mcp.WithNumber("sort_column", mcp.Description("Column to sort by, from 0.")),
		mcp.WithString("sort_order", mcp.DefaultString("ASCENDING"), mcp.Description("ASCENDING or DESCENDING.")),
	), r.sheetsFilterView)

	srv.AddTool(mcp.NewTool("gdocs_sheets_slicer",
		mcp.WithDescription("Put a slicer on a tab, or change one: the control a reader clicks to filter a "+
			"range by one column. It is the only filtering in a spreadsheet that a person can work "+
			"without touching the data."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the data is on.")),
		mcp.WithNumber("slicer_id", mcp.Description("Existing slicer to change. Omit to make a new one.")),
		mcp.WithNumber("start_row", mcp.Description("First row of the data, from 0.")),
		mcp.WithNumber("end_row", mcp.Description("One past the last row.")),
		mcp.WithNumber("start_column", mcp.Description("First column, from 0.")),
		mcp.WithNumber("end_column", mcp.Description("One past the last column.")),
		mcp.WithNumber("column_index", mcp.Description("Which column of that range the slicer filters, from 0.")),
		mcp.WithString("title", mcp.Description("Caption shown on the control.")),
		mcp.WithNumber("anchor_row", mcp.Description("Row of the cell the control hangs from.")),
		mcp.WithNumber("anchor_column", mcp.Description("Column of that cell.")),
		mcp.WithNumber("width", mcp.Description("Width in pixels.")),
		mcp.WithNumber("height", mcp.Description("Height in pixels.")),
	), r.sheetsSlicer)

	srv.AddTool(mcp.NewTool("gdocs_sheets_set_metadata",
		mcp.WithDescription("Attach a label to a row, a column, a tab or the workbook, or change one that "+
			"is already there. The label travels with what it is attached to, so it still points at the "+
			"right place after rows have been inserted above it — which a remembered row number does not."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("key", mcp.Required(), mcp.Description("Name of the label.")),
		mcp.WithString("value", mcp.Description("What it says.")),
		mcp.WithNumber("metadata_id", mcp.Description(
			"Existing label to change. Omit to attach a new one; give a number of your own to name it.")),
		mcp.WithString("sheet_title", mcp.Description("Tab to attach it to, or whose row or column to use.")),
		mcp.WithString("dimension", mcp.Description("ROWS or COLUMNS, when attaching to one of them.")),
		mcp.WithNumber("start", mcp.Description("First row or column, from 0.")),
		mcp.WithNumber("end", mcp.Description("One past the last one.")),
		mcp.WithString("visibility", mcp.DefaultString("DOCUMENT"), mcp.Description(
			"DOCUMENT means anybody who can open the workbook can read it; PROJECT means only this "+
				"application.")),
	), r.sheetsSetMetadata)
}

func (r *registry) sheetsListMetadata(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	book, err := client.Spreadsheet(ctx, spreadsheetID)
	if err != nil {
		return toolError(err), nil
	}

	wanted := optionalString(req, "key")
	labels := make([]map[string]any, 0)

	add := func(entry google.DeveloperMetadata, where string) {
		if wanted != "" && entry.MetadataKey != wanted {
			return
		}
		described := map[string]any{
			"id":    entry.MetadataID,
			"key":   entry.MetadataKey,
			"value": entry.MetadataValue,
			"where": where,
		}
		putString(described, "visibility", entry.Visibility)
		labels = append(labels, described)
	}

	for _, entry := range book.DeveloperMetadata {
		add(entry, "workbook")
	}
	for _, sheet := range book.Sheets {
		for _, entry := range sheet.DeveloperMetadata {
			add(entry, sheet.Properties.Title)
		}
	}

	return resultJSON(map[string]any{"spreadsheet_id": spreadsheetID, "metadata": labels})
}

func (r *registry) sheetsUpdateChart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	chartID, err := req.RequireInt("chart_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	var requests []google.SheetsRequest

	if title := optionalString(req, "sheet_title"); title != "" {
		sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, title)
		if err != nil {
			return toolError(err), nil
		}

		position := &google.EmbeddedObjPos{
			OverlayPosition: &google.OverlayPosition{
				AnchorCell: google.GridCoord{
					SheetID:     sheetID,
					RowIndex:    req.GetInt("anchor_row", 0),
					ColumnIndex: req.GetInt("anchor_column", 0),
				},
				OffsetXPixels: req.GetInt("offset_x", 0),
				OffsetYPixels: req.GetInt("offset_y", 0),
				WidthPixels:   req.GetInt("width", 0),
				HeightPixels:  req.GetInt("height", 0),
			},
		}

		requests = append(requests, google.SheetsRequest{
			UpdateEmbedded: &google.UpdateEmbeddedPosReq{
				ObjectID: chartID, NewPosition: position, Fields: "*",
			},
		})
	}

	// Taking the frame off is not the same as painting it the colour behind it, and the
	// difference is visible: a border painted to match still draws, and on the rounded corner
	// of a panel it leaves thin strokes along the edge. Clearing the colour outright — an
	// empty border with a mask of "*" — is what makes a chart sit in a panel with no seam.
	if req.GetBool("no_border", false) {
		if optionalString(req, "border_color") != "" {
			return toolError(fmt.Errorf("no_border takes the frame off and border_color paints it: " +
				"name one. Painting it the colour of what is behind it is the thing that leaves " +
				"visible strokes on a rounded corner, which is what no_border is for")), nil
		}
		// A colour with no alpha, not an absent colour: clearing the field leaves the frame
		// drawn in Google's default dark, which is more visible than the one it replaced.
		// Verified on a live chart, twice — once each way.
		requests = append(requests, google.SheetsRequest{
			UpdateEmbBorder: &google.UpdateEmbeddedBorderReq{
				ObjectID: chartID,
				Border: &google.EmbedBorder{Color: map[string]any{
					"rgbColor": map[string]any{"red": 1, "green": 1, "blue": 1, "alpha": 0},
				}},
				Fields: "colorStyle",
			},
		})
	} else if colour := optionalString(req, "border_color"); colour != "" {
		rgb, err := parseHexColor(colour)
		if err != nil {
			return toolError(err), nil
		}
		requests = append(requests, google.SheetsRequest{
			UpdateEmbBorder: &google.UpdateEmbeddedBorderReq{
				ObjectID: chartID,
				Border:   &google.EmbedBorder{Color: &google.ColorStyle{RGBColor: rgb}},
				Fields:   "colorStyle",
			},
		})
	}

	// Everything drawn on the chart lives in its specification, and updateChartSpec has no
	// field mask: whatever is sent becomes the chart. So the current specification is read
	// as Google stores it, changed, and sent back whole. Building a fresh one from the few
	// fields a caller named would replace the chart with a title and no data.
	if wantsSpecChange(req) {
		spec, err := client.ChartSpecOf(ctx, spreadsheetID, chartID)
		if err != nil {
			return toolError(err), nil
		}

		if err := r.patchChartSpec(ctx, client, spreadsheetID, spec, req); err != nil {
			return toolError(err), nil
		}

		requests = append(requests, google.SheetsRequest{
			UpdateChartSpec: &google.UpdateChartSpecRequest{ChartID: chartID, Spec: spec},
		})
	}

	if len(requests) == 0 {
		return toolError(fmt.Errorf("nothing to change: give a sheet_title to move it, a border_color, " +
			"a title, subtitle, alt_text or font, data_labels or total_data_labels, or a new data " +
			"range with data_sheet_title, labels_column, value_columns, start_row and end_row")), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, requests); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"chart_id":       chartID,
		"requests":       len(requests),
	})
}

// chartSpecArguments are the ones that change what the chart draws rather than where it sits.
var chartSpecArguments = []string{
	"title", "subtitle", "alt_text", "font", "font_size_pt",
	"data_labels", "total_data_labels",
	"background_color", "transparent_background",
	"data_sheet_title", "labels_column", "value_columns", "start_row", "end_row",
}

// patchChartBackground sets what the chart is drawn on.
//
// It matters more than it sounds. A chart standing inside a rounded panel on a slide arrives
// with a white rectangle of its own and paints over the panel: the panel's corners show and
// its field does not. And a deck that exists in a dark variant cannot be built at all
// without this — the chart lives in the workbook and inherits nothing from the deck's
// palette, so a white background on a dark panel is not "slightly off", it is an unreadable
// slide.
func patchChartBackground(spec map[string]any, req mcp.CallToolRequest) error {
	// Transparency is refused here rather than sent, because Google refuses it too and says
	// so in a way nobody would find twice: "chart.backgroundColorStyle.alpha not supported".
	// Verified on a live chart. The parameter stays so the answer explains the way round it.
	if req.GetBool("transparent_background", false) {
		return fmt.Errorf("a chart cannot be drawn on nothing: Google refuses an alpha on a " +
			"chart's background outright — \"chart.backgroundColorStyle.alpha not supported\". " +
			"Match the panel instead: read its fill with gdocs_slides_inspect_page and pass that " +
			"colour as background_color. On a deck with light and dark variants that means one " +
			"background_color per variant, and there is no way round it — the chart lives in the " +
			"workbook and inherits nothing from the deck's palette")
	}

	colour := optionalString(req, "background_color")
	if colour == "" {
		return nil
	}

	rgb, err := parseHexColor(colour)
	if err != nil {
		return err
	}

	// Both fields are written. Google keeps the older backgroundColor for readers that only
	// know it, and a chart left with the two disagreeing is drawn by whichever the renderer
	// happens to prefer.
	value := map[string]any{"red": rgb.Red, "green": rgb.Green, "blue": rgb.Blue}
	spec["backgroundColor"] = value
	spec["backgroundColorStyle"] = map[string]any{"rgbColor": value}

	return nil
}

func wantsSpecChange(req mcp.CallToolRequest) bool {
	arguments := req.GetArguments()
	for _, name := range chartSpecArguments {
		if _, given := arguments[name]; given {
			return true
		}
	}

	return false
}

// patchChartSpec changes the named parts of a chart's own specification, in place.
//
// It works on the map Google sent rather than on a struct, so everything this server does
// not model survives: a background colour, a per-series line style, a hidden-dimension
// strategy. Only the keys named here are touched.
func (r *registry) patchChartSpec(ctx context.Context, client *google.Client, spreadsheetID string,
	spec map[string]any, req mcp.CallToolRequest) error {
	// An empty string is a decision, not a missing argument. A chart whose name is written on
	// the slide beside it should carry no title of its own, and "title": "" is how a caller
	// says so — dropped as empty, it left the old title in place and looked like the call had
	// worked. Google ignores an empty value in a specification, so the key is removed instead.
	arguments := req.GetArguments()
	for _, word := range []struct{ argument, key string }{
		{"title", "title"},
		{"subtitle", "subtitle"},
		{"alt_text", "altText"},
		{"font", "fontName"},
	} {
		if _, given := arguments[word.argument]; !given {
			continue
		}
		if text := optionalString(req, word.argument); strings.TrimSpace(text) != "" {
			spec[word.key] = text
		} else {
			delete(spec, word.key)
		}
	}

	if err := patchChartBackground(spec, req); err != nil {
		return err
	}

	if size := req.GetFloat("font_size_pt", 0); size > 0 {
		// The size lives inside the chart's own text format, which is also where the font
		// name would go if it were set this way. Only the size is touched: replacing the
		// whole format would drop a colour or a weight somebody set in the editor.
		format, ok := spec["titleTextFormat"].(map[string]any)
		if !ok {
			format = map[string]any{}
		}
		format["fontSize"] = size
		spec["titleTextFormat"] = format
	}

	_, wantsLabels := arguments["data_labels"]
	_, wantsTotals := arguments["total_data_labels"]
	_, wantsRange := arguments["value_columns"]

	if !wantsLabels && !wantsTotals && !wantsRange {
		return nil
	}

	basic, ok := spec["basicChart"].(map[string]any)
	if !ok {
		return fmt.Errorf("this chart is not one of the kinds drawn against two axes, so it has " +
			"no series to label and no data range to change: labels and ranges apply to column, bar, " +
			"line, area, stepped area and scatter charts")
	}

	if wantsRange {
		if err := r.patchChartRange(ctx, client, spreadsheetID, basic, req); err != nil {
			return err
		}
	}

	if wantsTotals {
		if req.GetBool("total_data_labels", false) {
			basic["totalDataLabel"] = map[string]any{"type": "DATA"}
		} else {
			delete(basic, "totalDataLabel")
		}
	}

	if wantsLabels {
		series, _ := basic["series"].([]any)
		on := req.GetBool("data_labels", false)
		for _, item := range series {
			one, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if on {
				one["dataLabel"] = map[string]any{"type": "DATA"}
			} else {
				delete(one, "dataLabel")
			}
		}
	}

	return nil
}

// patchChartRange points an existing chart at different columns or a different span of rows.
//
// This is the difference between changing a chart and rebuilding it, and the difference
// matters outside the workbook: a chart put on a slide is linked by its number, and a chart
// deleted and made again comes back with a new one, leaving the slide pointing at nothing.
func (r *registry) patchChartRange(ctx context.Context, client *google.Client, spreadsheetID string,
	basic map[string]any, req mcp.CallToolRequest) error {
	dataSheet, err := requiredString(req, "data_sheet_title")
	if err != nil {
		return fmt.Errorf("changing the data range needs data_sheet_title, the tab the numbers are on: %w", err)
	}

	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, dataSheet)
	if err != nil {
		return err
	}

	startRow, err := req.RequireInt("start_row")
	if err != nil {
		return err
	}
	endRow, err := req.RequireInt("end_row")
	if err != nil {
		return err
	}
	if startRow < 0 || endRow <= startRow {
		return fmt.Errorf("the data is empty: end_row is exclusive and both count from 0")
	}

	labels, err := req.RequireInt("labels_column")
	if err != nil {
		return err
	}
	values, err := intList(req, "value_columns")
	if err != nil {
		return err
	}
	if len(values) == 0 {
		return fmt.Errorf("value_columns is empty: a chart needs at least one column of numbers")
	}

	column := func(index int) map[string]any {
		return map[string]any{"sourceRange": map[string]any{"sources": []any{map[string]any{
			"sheetId":          sheetID,
			"startRowIndex":    startRow,
			"endRowIndex":      endRow,
			"startColumnIndex": index,
			"endColumnIndex":   index + 1,
		}}}}
	}

	basic["domains"] = []any{map[string]any{"domain": column(labels)}}

	// The series that were there are replaced rather than edited, but each new one keeps
	// the look of the one it stands in for: dropping the colour of series three because the
	// range grew is a chart that changed appearance for no reason a reader can see.
	previous, _ := basic["series"].([]any)
	rebuilt := make([]any, 0, len(values))
	for index, valueColumn := range values {
		one := map[string]any{"series": column(valueColumn), "targetAxis": "LEFT_AXIS"}
		if index < len(previous) {
			if old, ok := previous[index].(map[string]any); ok {
				for _, keep := range []string{"targetAxis", "color", "colorStyle", "dataLabel",
					"lineStyle", "type", "pointStyle"} {
					if value, present := old[keep]; present {
						one[keep] = value
					}
				}
			}
		}
		rebuilt = append(rebuilt, one)
	}
	basic["series"] = rebuilt

	if header, given := req.GetArguments()["header_rows"]; given {
		_ = header
		basic["headerCount"] = req.GetInt("header_rows", 1)
	}

	return nil
}

func (r *registry) sheetsFilterView(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	title, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, title)
	if err != nil {
		return toolError(err), nil
	}

	viewID := req.GetInt("filter_view_id", 0)

	if req.GetBool("duplicate", false) {
		if viewID == 0 {
			return toolError(fmt.Errorf("duplicating needs the filter_view_id of the view to copy")), nil
		}
		if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
			DuplicateFilter: &google.DuplicateFilterRequest{FilterID: viewID},
		}}); err != nil {
			return toolError(err), nil
		}

		return resultJSON(map[string]any{"spreadsheet_id": spreadsheetID, "duplicated": viewID})
	}

	view := google.FilterView{Title: optionalString(req, "title")}

	if _, given := req.GetArguments()["start_row"]; given {
		startRow := req.GetInt("start_row", 0)
		endRow := req.GetInt("end_row", 0)
		startColumn := req.GetInt("start_column", 0)
		endColumn := req.GetInt("end_column", 0)
		view.Range = &google.GridRange{
			SheetID:          sheetID,
			StartRowIndex:    &startRow,
			EndRowIndex:      &endRow,
			StartColumnIndex: &startColumn,
			EndColumnIndex:   &endColumn,
		}
	} else {
		view.Range = &google.GridRange{SheetID: sheetID}
	}

	if _, given := req.GetArguments()["sort_column"]; given {
		view.SortSpecs = []google.SortSpec{{
			DimensionIndex: req.GetInt("sort_column", 0),
			SortOrder:      strings.ToUpper(req.GetString("sort_order", "ASCENDING")),
		}}
	}

	request := google.SheetsRequest{}
	if viewID > 0 {
		view.FilterViewID = viewID
		fields := "range"
		if view.Title != "" {
			fields += ",title"
		}
		if view.SortSpecs != nil {
			fields += ",sortSpecs"
		}
		request.UpdateFilterVw = &google.UpdateFilterViewRequest{Filter: view, Fields: fields}
	} else {
		request.AddFilterView = &google.AddFilterViewRequest{Filter: view}
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{request}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet_title":    title,
		"filter_view_id": viewID,
	})
}

func (r *registry) sheetsSlicer(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	title, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, title)
	if err != nil {
		return toolError(err), nil
	}

	spec := &google.SlicerSpec{Title: optionalString(req, "title")}

	if _, given := req.GetArguments()["start_row"]; given {
		startRow := req.GetInt("start_row", 0)
		endRow := req.GetInt("end_row", 0)
		startColumn := req.GetInt("start_column", 0)
		endColumn := req.GetInt("end_column", 0)
		spec.DataRange = &google.GridRange{
			SheetID:          sheetID,
			StartRowIndex:    &startRow,
			EndRowIndex:      &endRow,
			StartColumnIndex: &startColumn,
			EndColumnIndex:   &endColumn,
		}
	}
	if _, given := req.GetArguments()["column_index"]; given {
		column := req.GetInt("column_index", 0)
		spec.ColumnIndex = &column
	}

	if slicerID := req.GetInt("slicer_id", 0); slicerID > 0 {
		fields := "title"
		if spec.DataRange != nil {
			fields += ",dataRange"
		}
		if spec.ColumnIndex != nil {
			fields += ",columnIndex"
		}

		if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
			UpdateSlicer: &google.UpdateSlicerRequest{SlicerID: slicerID, Spec: spec, Fields: fields},
		}}); err != nil {
			return toolError(err), nil
		}

		return resultJSON(map[string]any{"spreadsheet_id": spreadsheetID, "slicer_id": slicerID})
	}

	if spec.DataRange == nil {
		return toolError(fmt.Errorf("a new slicer needs the range it filters: start_row, end_row, " +
			"start_column and end_column")), nil
	}

	slicer := google.Slicer{
		Spec: spec,
		Position: &google.EmbeddedObjPos{
			OverlayPosition: &google.OverlayPosition{
				AnchorCell: google.GridCoord{
					SheetID:     sheetID,
					RowIndex:    req.GetInt("anchor_row", 0),
					ColumnIndex: req.GetInt("anchor_column", 0),
				},
				WidthPixels:  req.GetInt("width", 0),
				HeightPixels: req.GetInt("height", 0),
			},
		},
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		AddSlicer: &google.AddSlicerRequest{Slicer: slicer},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"spreadsheet_id": spreadsheetID, "sheet_title": title})
}

func (r *registry) sheetsSetMetadata(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	key, err := requiredString(req, "key")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	entry := google.DeveloperMetadata{
		MetadataKey:   key,
		MetadataValue: optionalString(req, "value"),
		Visibility:    strings.ToUpper(req.GetString("visibility", "DOCUMENT")),
	}

	location := &google.MetadataLocation{}
	if title := optionalString(req, "sheet_title"); title != "" {
		sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, title)
		if err != nil {
			return toolError(err), nil
		}

		if dimension := strings.ToUpper(optionalString(req, "dimension")); dimension != "" {
			start := req.GetInt("start", -1)
			end := req.GetInt("end", -1)
			if start < 0 || end <= start {
				return toolError(fmt.Errorf("attaching to %s needs start and end, end exclusive", dimension)), nil
			}
			location.DimensionRnge = &google.DimensionRange{
				SheetID: sheetID, Dimension: dimension, StartIndex: &start, EndIndex: &end,
			}
		} else {
			location.SheetID = sheetID
		}
	} else {
		location.Spreadsheet = true
	}
	entry.Location = location

	request := google.SheetsRequest{}
	if id := req.GetInt("metadata_id", 0); id > 0 {
		entry.MetadataID = id
		request.UpdateMetadata = &google.UpdateMetadataRequest{
			DataFilters: []google.DeveloperMetadataFilter{{
				DeveloperMetadataLookup: &google.MetadataLookup{MetadataID: id},
			}},
			DeveloperMetadata: entry,
			Fields:            "metadataKey,metadataValue,location,visibility",
		}
	} else {
		request.CreateMetadata = &google.CreateMetadataRequest{DeveloperMetadata: entry}
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{request}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"spreadsheet_id": spreadsheetID, "key": key})
}
