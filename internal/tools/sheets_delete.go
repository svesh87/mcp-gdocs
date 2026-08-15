package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerSheetsDelete adds removal inside a workbook, on the same terms as the other two:
// what a workbook contains can be taken out, the file itself cannot be touched.
//
// A workbook is where removal is most easily regretted — a column of data does not come
// back with an undo an agent can press — so this tool insists on naming exactly one thing,
// says in its answer what went, and refuses a range that names no rows or columns rather
// than guessing that the whole tab was meant.
func (r *registry) registerSheetsDelete(srv *server.MCPServer) {
	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_sheets_delete",
		mcp.WithDescription("Remove something inside a workbook: rows or columns, the contents of a "+
			"rectangle with the rest shifted up or left, a whole tab, a grouping, a banding, a rule "+
			"that colours by content, a protection, a named range, a filter view, duplicate rows, a "+
			"chart or slicer, or a table object. Exactly one of those per call. It reaches nothing "+
			"outside the workbook — no files, no folders, no other workbooks. There is no undo here: "+
			"take the indexes from a reading made after the last edit, and remember that rows and "+
			"columns are counted from 0 with the end exclusive."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("what", mcp.Required(), mcp.Description(
			"rows, columns, cells, tab, group, banding, conditional_format, protection, named_range, "+
				"filter_view, duplicates, chart, table, metadata.")),
		mcp.WithString("sheet_title", mcp.Description("Tab to work in, for anything that names a range.")),
		mcp.WithNumber("sheet_id", mcp.Description("Tab by identifier, as an alternative to its title.")),
		mcp.WithNumber("start", mcp.Description("First row or column to remove, counting from 0.")),
		mcp.WithNumber("end", mcp.Description("One past the last one.")),
		mcp.WithNumber("start_row", mcp.Description("For cells and duplicates: first row of the rectangle.")),
		mcp.WithNumber("end_row", mcp.Description("One past its last row.")),
		mcp.WithNumber("start_column", mcp.Description("First column of the rectangle.")),
		mcp.WithNumber("end_column", mcp.Description("One past its last column.")),
		mcp.WithString("shift", mcp.DefaultString("ROWS"), mcp.Description(
			"For cells: ROWS pulls what is below upwards, COLUMNS pulls what is to the right leftwards.")),
		mcp.WithNumber("object_id", mcp.Description(
			"Identifier of the thing to remove, for banding, protection, filter_view and chart.")),
		mcp.WithString("named_range_id", mcp.Description("Named range to forget; the cells stay.")),
		mcp.WithString("table_id", mcp.Description("Table object to remove; the values stay.")),
		mcp.WithNumber("index", mcp.Description("For conditional_format: position of the rule in the tab's list.")),
		mcp.WithNumber("metadata_id", mcp.Description("For metadata: the label's identifier.")),
		mcp.WithString("key", mcp.Description("For metadata: remove every label under this key instead.")),
		mcp.WithDestructiveHintAnnotation(true),
	), r.sheetsDelete)
}

func (r *registry) sheetsDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	what := strings.ToLower(optionalString(req, "what"))

	// A tab is named by title far more often than by identifier, and the identifier is
	// what every request here takes; resolving it once keeps that out of each branch.
	sheetID := req.GetInt("sheet_id", -1)
	if sheetID < 0 {
		if title := optionalString(req, "sheet_title"); title != "" {
			resolved, err := r.sheetIDOf(ctx, client, spreadsheetID, title)
			if err != nil {
				return toolError(err), nil
			}
			sheetID = resolved
		}
	}

	request, described, err := r.sheetsDeleteRequest(req, what, sheetID)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{*request}); err != nil {
		return toolError(err), nil
	}

	described["spreadsheet_id"] = spreadsheetID

	return resultJSON(described)
}

// sheetsDeleteRequest works out which single thing a call names.
func (r *registry) sheetsDeleteRequest(req mcp.CallToolRequest, what string, sheetID int) (*google.SheetsRequest, map[string]any, error) {
	objectID := req.GetInt("object_id", -1)

	dimension := func(kind string) (google.DimensionRange, error) {
		start := req.GetInt("start", -1)
		end := req.GetInt("end", -1)
		if start < 0 || end <= start {
			return google.DimensionRange{}, fmt.Errorf(
				"removing %s needs start and end, counted from 0 with end exclusive", kind)
		}
		if sheetID < 0 {
			return google.DimensionRange{}, fmt.Errorf("name the tab: sheet_title or sheet_id")
		}

		return google.DimensionRange{
			SheetID:    sheetID,
			Dimension:  kind,
			StartIndex: &start,
			EndIndex:   &end,
		}, nil
	}

	rectangle := func() (google.GridRange, error) {
		if sheetID < 0 {
			return google.GridRange{}, fmt.Errorf("name the tab: sheet_title or sheet_id")
		}
		startRow := req.GetInt("start_row", -1)
		endRow := req.GetInt("end_row", -1)
		startColumn := req.GetInt("start_column", -1)
		endColumn := req.GetInt("end_column", -1)
		if startRow < 0 || endRow <= startRow || startColumn < 0 || endColumn <= startColumn {
			return google.GridRange{}, fmt.Errorf(
				"name the rectangle: start_row, end_row, start_column and end_column, end exclusive")
		}

		return google.GridRange{
			SheetID:          sheetID,
			StartRowIndex:    &startRow,
			EndRowIndex:      &endRow,
			StartColumnIndex: &startColumn,
			EndColumnIndex:   &endColumn,
		}, nil
	}

	switch what {
	case "rows", "columns":
		kind := "ROWS"
		if what == "columns" {
			kind = "COLUMNS"
		}
		span, err := dimension(kind)
		if err != nil {
			return nil, nil, err
		}

		return &google.SheetsRequest{DeleteDimension: &google.DeleteDimensionRequest{Range: span}},
			map[string]any{"removed": what, "start": *span.StartIndex, "end": *span.EndIndex}, nil

	case "cells":
		area, err := rectangle()
		if err != nil {
			return nil, nil, err
		}
		shift := strings.ToUpper(req.GetString("shift", "ROWS"))
		if shift != "ROWS" && shift != "COLUMNS" {
			return nil, nil, fmt.Errorf("shift is ROWS or COLUMNS, got %q", shift)
		}

		return &google.SheetsRequest{DeleteRange: &google.DeleteRangeRequest{Range: area, ShiftDimension: shift}},
			map[string]any{"removed": "cells", "shift": shift}, nil

	case "tab":
		if sheetID < 0 {
			return nil, nil, fmt.Errorf("name the tab: sheet_title or sheet_id")
		}
		if err := r.mayRemoveWholePage(SheetsDelete); err != nil {
			return nil, nil, err
		}

		return &google.SheetsRequest{DeleteSheet: &google.DeleteSheetRequest{SheetID: sheetID}},
			map[string]any{"removed": "tab", "sheet_id": sheetID}, nil

	case "group":
		kind := strings.ToUpper(req.GetString("shift", "ROWS"))
		span, err := dimension(kind)
		if err != nil {
			return nil, nil, err
		}

		return &google.SheetsRequest{DeleteGroup: &google.DeleteDimensionGroupReq{Range: span}},
			map[string]any{"removed": "group", "dimension": kind}, nil

	case "banding":
		if objectID < 0 {
			return nil, nil, fmt.Errorf("removing a banding needs its object_id, as gdocs_sheets_read_format reports it")
		}

		return &google.SheetsRequest{DeleteBanding: &google.DeleteBandingRequest{BandedRangeID: objectID}},
			map[string]any{"removed": "banding", "object_id": objectID}, nil

	case "conditional_format":
		index := req.GetInt("index", -1)
		if index < 0 || sheetID < 0 {
			return nil, nil, fmt.Errorf("removing a rule needs the tab and the index of the rule in its list")
		}

		return &google.SheetsRequest{DeleteConditional: &google.DeleteConditionalRequest{SheetID: sheetID, Index: index}},
			map[string]any{"removed": "conditional_format", "index": index}, nil

	case "protection":
		if objectID < 0 {
			return nil, nil, fmt.Errorf("removing a protection needs its object_id")
		}

		return &google.SheetsRequest{DeleteProtected: &google.DeleteProtectedRequest{ProtectedRangeID: objectID}},
			map[string]any{"removed": "protection", "object_id": objectID}, nil

	case "named_range":
		id := optionalString(req, "named_range_id")
		if id == "" {
			return nil, nil, fmt.Errorf("removing a named range needs its named_range_id")
		}

		return &google.SheetsRequest{DeleteNamedRange: &google.DeleteNamedRangeRequest{NamedRangeID: id}},
			map[string]any{"removed": "named_range", "named_range_id": id,
				"note": "the name is gone, the cells it covered are untouched"}, nil

	case "filter_view":
		if objectID < 0 {
			return nil, nil, fmt.Errorf("removing a filter view needs its object_id")
		}

		return &google.SheetsRequest{DeleteFilterView: &google.DeleteFilterViewRequest{FilterID: objectID}},
			map[string]any{"removed": "filter_view", "object_id": objectID}, nil

	case "duplicates":
		area, err := rectangle()
		if err != nil {
			return nil, nil, err
		}

		return &google.SheetsRequest{DeleteDuplicates: &google.DeleteDuplicatesRequest{Range: area}},
			map[string]any{"removed": "duplicates"}, nil

	case "chart":
		if objectID < 0 {
			return nil, nil, fmt.Errorf("removing a chart or a slicer needs its object_id")
		}

		return &google.SheetsRequest{DeleteEmbedded: &google.DeleteEmbeddedRequest{ObjectID: objectID}},
			map[string]any{"removed": "chart", "object_id": objectID}, nil

	case "table":
		id := optionalString(req, "table_id")
		if id == "" {
			return nil, nil, fmt.Errorf("removing a table object needs its table_id")
		}

		return &google.SheetsRequest{DeleteTable: &google.DeleteTableRequest{TableID: id}},
			map[string]any{"removed": "table", "table_id": id,
				"note": "the table object is gone, the values in the cells stay"}, nil

	case "metadata":
		id := req.GetInt("metadata_id", 0)
		key := optionalString(req, "key")
		if id == 0 && key == "" {
			return nil, nil, fmt.Errorf("removing a label needs its metadata_id or its key")
		}

		return &google.SheetsRequest{DeleteMetadata: &google.DeleteMetadataRequest{
				Filter: google.DeveloperMetadataFilter{
					DeveloperMetadataLookup: &google.MetadataLookup{MetadataID: id, MetadataKey: key},
				},
			}},
			map[string]any{"removed": "metadata", "metadata_id": id, "key": key}, nil

	default:
		return nil, nil, fmt.Errorf("what is rows, columns, cells, tab, group, banding, conditional_format, "+
			"protection, named_range, filter_view, duplicates, chart or table, got %q", what)
	}
}
