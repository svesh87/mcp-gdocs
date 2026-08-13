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
		mcp.WithString("title", mcp.Description("New title above the chart.")),
		mcp.WithString("subtitle", mcp.Description("New subtitle.")),
		mcp.WithString("alt_text", mcp.Description("Description for a screen reader.")),
		mcp.WithString("font", mcp.Description("Font family for the chart's own text.")),
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

	if colour := optionalString(req, "border_color"); colour != "" {
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

	// The words on the chart are part of its specification rather than of its position,
	// so they go in their own request. Only what the caller named is sent: a specification
	// is replaced wholesale, and sending an empty one would strip the chart of everything
	// it draws.
	spec := &google.ChartSpec{
		Title:    optionalString(req, "title"),
		Subtitle: optionalString(req, "subtitle"),
		AltText:  optionalString(req, "alt_text"),
		FontName: optionalString(req, "font"),
	}
	if spec.Title != "" || spec.Subtitle != "" || spec.AltText != "" || spec.FontName != "" {
		requests = append(requests, google.SheetsRequest{
			UpdateChartSpec: &google.UpdateChartSpecRequest{ChartID: chartID, Spec: spec},
		})
	}

	if len(requests) == 0 {
		return toolError(fmt.Errorf("nothing to change: give a sheet_title to move it, a border_color, " +
			"or a title, subtitle, alt_text or font")), nil
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
