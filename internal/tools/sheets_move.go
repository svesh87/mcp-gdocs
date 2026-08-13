package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerSheetsMove adds the operations that move values about inside a workbook rather
// than writing them one by one.
//
// They matter for a reason that is easy to miss: writing a rectangle cell by cell loses
// everything that is not a value — the formatting, the validation, the notes, the rules
// that paint by content — while a copy carries exactly what it is told to carry. A tab
// rebuilt with PASTE_FORMAT is a tab that looks like its sample without anybody having to
// describe the sample in words.
//
// Rectangles are given as rows and columns counted from zero, end exclusive, the way every
// other writing tool here takes them.
func (r *registry) registerSheetsMove(srv *server.MCPServer) {
	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_sheets_move_range",
		mcp.WithDescription("Copy or move a rectangle inside a workbook: values, formatting, formulas, "+
			"validation, or all of it. A copy repeats itself to fill a larger destination, the way "+
			"dragging a corner does; a move leaves nothing behind. Both carry what a cell-by-cell "+
			"rewrite would lose."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the rectangle is taken from.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row of the source, from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("One past its last row.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column of the source, from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("One past its last column.")),
		mcp.WithString("to_sheet_title", mcp.Description("Tab it goes to. Without one, the same tab.")),
		mcp.WithNumber("to_row", mcp.Required(), mcp.Description("Row of the top-left cell it goes to.")),
		mcp.WithNumber("to_column", mcp.Required(), mcp.Description("Column of that cell.")),
		mcp.WithNumber("to_end_row", mcp.Description(
			"For a copy: last row of the destination, exclusive. A destination bigger than the source "+
				"is filled by repeating it.")),
		mcp.WithNumber("to_end_column", mcp.Description("For a copy: last column of the destination, exclusive.")),
		mcp.WithBoolean("move", mcp.DefaultBool(false), mcp.Description(
			"Take it rather than copy it, leaving the source empty.")),
		mcp.WithString("what", mcp.DefaultString("NORMAL"), mcp.Description(
			"NORMAL is everything; VALUES only what the cells hold; FORMAT only how they look; "+
				"NO_BORDERS, FORMULA, DATA_VALIDATION, CONDITIONAL_FORMATTING.")),
		mcp.WithString("orientation", mcp.DefaultString("NORMAL"), mcp.Description(
			"TRANSPOSE turns rows into columns on the way.")),
	), r.sheetsMoveRange)

	srv.AddTool(mcp.NewTool("gdocs_sheets_paste_text",
		mcp.WithDescription("Put delimited text into a tab as if it had been pasted there: a CSV body, a "+
			"tab-separated block, or an HTML table. The splitting happens on Google's side, so a table "+
			"copied out of somewhere else lands as cells rather than as one long string."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to paste into.")),
		mcp.WithNumber("row", mcp.DefaultNumber(0), mcp.Description("Row of the top-left cell, from 0.")),
		mcp.WithNumber("column", mcp.DefaultNumber(0), mcp.Description("Column of the top-left cell, from 0.")),
		mcp.WithString("data", mcp.Required(), mcp.Description("The text to paste.")),
		mcp.WithString("delimiter", mcp.DefaultString(","), mcp.Description(
			"What separates the columns. Ignored when html is true.")),
		mcp.WithBoolean("html", mcp.DefaultBool(false), mcp.Description("The data is an HTML table.")),
		mcp.WithString("what", mcp.DefaultString("NORMAL"), mcp.Description("As in move_range.")),
	), r.sheetsPasteText)

	srv.AddTool(mcp.NewTool("gdocs_sheets_shape_range",
		mcp.WithDescription("Make room or shuffle: insert empty cells and push the rest aside, or put the "+
			"rows of a rectangle in random order. Inserting cells is not inserting rows — it moves only "+
			"what is inside the rectangle's own columns, which is how one column is realigned without "+
			"disturbing its neighbours."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to work in.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("One past the last row.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("One past the last column.")),
		mcp.WithString("what", mcp.Required(), mcp.Description("insert_cells or randomize.")),
		mcp.WithString("shift", mcp.DefaultString("ROWS"), mcp.Description(
			"For insert_cells: ROWS pushes what is below downwards, COLUMNS pushes what is to the right.")),
	), r.sheetsShapeRange)

	srv.AddTool(mcp.NewTool("gdocs_sheets_append_rows",
		mcp.WithDescription("Add rows after the last one that has anything in it. gdocs_sheets_append "+
			"writes values through the other API; this one goes through the batch, so it appends to the "+
			"grid itself and is the one to use when the tab's own shape matters."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to add to.")),
		mcp.WithArray("rows", mcp.Required(), mcp.Description(
			"Rows, each a list of cell values: [[\"2026-08-13\", 42], [\"2026-08-14\", 17]]. "+
				"A string that starts with = goes in as a formula."),
			mcp.Items(map[string]any{"type": "array"})),
	), r.sheetsAppendRows)
}

// pasteType is what a caller's shorthand means to the API.
func pasteType(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		value = "NORMAL"
	}
	if !strings.HasPrefix(value, "PASTE_") {
		value = "PASTE_" + value
	}

	return value
}

func (r *registry) sheetsMoveRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, sheetTitle, sheetID, bounds, failure := r.rangeArguments(ctx, req)
	if failure != nil {
		return failure, nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	targetID := sheetID
	if title := optionalString(req, "to_sheet_title"); title != "" && title != sheetTitle {
		targetID, err = r.sheetIDOf(ctx, client, spreadsheetID, title)
		if err != nil {
			return toolError(err), nil
		}
	}

	toRow := req.GetInt("to_row", 0)
	toColumn := req.GetInt("to_column", 0)

	request := google.SheetsRequest{}
	if req.GetBool("move", false) {
		request.CutPaste = &google.CutPasteRequest{
			Source:      gridRangeOf(sheetID, bounds),
			Destination: google.GridCoord{SheetID: targetID, RowIndex: toRow, ColumnIndex: toColumn},
			PasteType:   pasteType(req.GetString("what", "NORMAL")),
		}
	} else {
		// A destination that names only its corner is one cell, and Google reads that as
		// "paste the source as it is". An end given as well is what makes a copy repeat
		// itself to fill the rectangle.
		endRow := req.GetInt("to_end_row", toRow+1)
		endColumn := req.GetInt("to_end_column", toColumn+1)

		request.CopyPaste = &google.CopyPasteRequest{
			Source: gridRangeOf(sheetID, bounds),
			Destination: google.GridRange{
				SheetID:          targetID,
				StartRowIndex:    &toRow,
				EndRowIndex:      &endRow,
				StartColumnIndex: &toColumn,
				EndColumnIndex:   &endColumn,
			},
			PasteType:   pasteType(req.GetString("what", "NORMAL")),
			PasteOrient: strings.ToUpper(req.GetString("orientation", "NORMAL")),
		}
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{request}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"from":           sheetTitle,
		"moved":          req.GetBool("move", false),
		"paste_type":     pasteType(req.GetString("what", "NORMAL")),
	})
}

func (r *registry) sheetsPasteText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	title, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	data, err := req.RequireString("data")
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

	request := &google.PasteDataRequest{
		Coordinate: google.GridCoord{
			SheetID:     sheetID,
			RowIndex:    req.GetInt("row", 0),
			ColumnIndex: req.GetInt("column", 0),
		},
		Data: data,
		Type: pasteType(req.GetString("what", "NORMAL")),
	}

	// Delimiter and html are two ways of saying how the text is split, and Google refuses
	// to be told both.
	if req.GetBool("html", false) {
		request.HTML = true
	} else {
		request.Delimiter = req.GetString("delimiter", ",")
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{PasteData: request}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet_title":    title,
		"characters":     utf16Length(data),
	})
}

func (r *registry) sheetsShapeRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, sheetTitle, sheetID, bounds, failure := r.rangeArguments(ctx, req)
	if failure != nil {
		return failure, nil
	}

	area := gridRangeOf(sheetID, bounds)

	request := google.SheetsRequest{}
	switch what := strings.ToLower(optionalString(req, "what")); what {
	case "insert_cells":
		shift := strings.ToUpper(req.GetString("shift", "ROWS"))
		if shift != "ROWS" && shift != "COLUMNS" {
			return toolError(fmt.Errorf("shift is ROWS or COLUMNS, got %q", shift)), nil
		}
		request.InsertRange = &google.InsertRangeRequest{Range: area, ShiftDimension: shift}
	case "randomize":
		request.RandomizeRange = &google.RandomizeRangeRequest{Range: area}
	default:
		return toolError(fmt.Errorf("what is insert_cells or randomize, got %q", what)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{request}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet_title":    sheetTitle,
		"what":           optionalString(req, "what"),
	})
}

func (r *registry) sheetsAppendRows(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	title, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}

	raw, ok := req.GetArguments()["rows"].([]any)
	if !ok || len(raw) == 0 {
		return toolError(fmt.Errorf("rows must be a list of rows, each a list of cell values")), nil
	}

	rows := make([]google.RowData, 0, len(raw))
	for index, item := range raw {
		cells, ok := item.([]any)
		if !ok {
			return toolError(fmt.Errorf("rows[%d] must be a list of cell values, got %T", index, item)), nil
		}

		values := make([]google.CellValue, 0, len(cells))
		for _, cell := range cells {
			values = append(values, google.CellValue{UserEnteredValue: cellValueOf(cell)})
		}
		rows = append(rows, google.RowData{Values: values})
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, title)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		AppendCells: &google.AppendCellsRequest{SheetID: sheetID, Rows: rows, Fields: "userEnteredValue"},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet_title":    title,
		"rows":           len(rows),
	})
}

// cellValueOf turns one JSON value into what a cell holds. A string beginning with "="
// goes in as a formula, which is what a person typing it into the editor would get.
func cellValueOf(value any) *google.ExtendedValue {
	switch typed := value.(type) {
	case string:
		if strings.HasPrefix(typed, "=") {
			return &google.ExtendedValue{FormulaValue: &typed}
		}
		return &google.ExtendedValue{StringValue: &typed}
	case float64:
		return &google.ExtendedValue{NumberValue: &typed}
	case bool:
		return &google.ExtendedValue{BoolValue: &typed}
	case nil:
		return &google.ExtendedValue{}
	default:
		text := fmt.Sprintf("%v", typed)
		return &google.ExtendedValue{StringValue: &text}
	}
}
