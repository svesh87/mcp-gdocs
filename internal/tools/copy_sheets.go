package tools

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerSheetsCopy adds the two ways content comes into a workbook from another one.
//
// They are different in kind, and the difference is worth knowing before choosing. Copying
// a tab is a request Google itself performs: one call, and everything arrives, including
// the parts this server cannot write. Copying a rectangle is this server reading the source
// and rebuilding it, so what arrives is what both ends can express — the cells and their
// look — and everything else is named in the answer rather than lost quietly.
func (r *registry) registerSheetsCopy(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_sheets_copy_sheet",
		mcp.WithDescription("Copy a whole tab into another workbook. This is the one request in any of "+
			"Google's document APIs that carries content between documents, so it brings across what "+
			"nothing else here can: charts, conditional formatting, banding, protections, filters. The "+
			"copy arrives named \"Copy of <title>\" unless new_title says otherwise. Formulas that "+
			"pointed at other tabs of the source workbook come across pointing at tabs of the same name "+
			"in the destination, and break if there are none — worth a look afterwards."),
		mcp.WithString("source_spreadsheet_id", mcp.Required(), mcp.Description("Workbook to copy from.")),
		mcp.WithString("source_sheet_title", mcp.Required(), mcp.Description("Tab to copy.")),
		mcp.WithString("target_spreadsheet_id", mcp.Required(), mcp.Description("Workbook to copy into.")),
		mcp.WithString("new_title", mcp.Description(
			"Name for the copy. Without it Google's own \"Copy of …\" stands.")),
	), r.sheetsCopySheet)

	srv.AddTool(mcp.NewTool("gdocs_sheets_copy_range",
		mcp.WithDescription("Copy a rectangle of cells from one workbook into another: what was typed "+
			"— formulas included, not the numbers they produce today — with the cell's format, its note, "+
			"its dropdown, the styling that changes partway through it, and the merges inside the "+
			"rectangle. Rules that paint by content, banding and protections are not carried and are "+
			"named in the answer instead. For a whole tab use gdocs_sheets_copy_sheet, which brings "+
			"those too. Rows and columns are counted from 0, end exclusive."),
		mcp.WithString("source_spreadsheet_id", mcp.Required(), mcp.Description("Workbook to copy from.")),
		mcp.WithString("source_sheet_title", mcp.Required(), mcp.Description("Tab to copy from.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row of the rectangle.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("One past its last row.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column of the rectangle.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("One past its last column.")),
		mcp.WithString("target_spreadsheet_id", mcp.Required(), mcp.Description("Workbook to copy into.")),
		mcp.WithString("target_sheet_title", mcp.Required(), mcp.Description("Tab to copy into.")),
		mcp.WithNumber("target_row", mcp.DefaultNumber(0), mcp.Description(
			"Row the rectangle's top-left cell lands on.")),
		mcp.WithNumber("target_column", mcp.DefaultNumber(0), mcp.Description(
			"Column it lands on.")),
	), r.sheetsCopyRange)
}

func (r *registry) sheetsCopySheet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID, err := requiredString(req, "source_spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sourceTitle, err := requiredString(req, "source_sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	targetID, err := requiredString(req, "target_spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	sheetID, err := r.sheetIDOf(ctx, client, sourceID, sourceTitle)
	if err != nil {
		return toolError(err), nil
	}

	copied, err := client.CopySheetTo(ctx, sourceID, sheetID, targetID)
	if err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{
		"source_spreadsheet_id": sourceID,
		"source_sheet_title":    sourceTitle,
		"target_spreadsheet_id": targetID,
		"sheet_id":              copied.ID(),
		"title":                 copied.Title,
	}

	// Renaming is a second request on purpose: the copy exists either way, and a rename
	// that fails should leave a tab called "Copy of …" rather than nothing at all.
	if newTitle := optionalString(req, "new_title"); newTitle != "" {
		if _, err := client.SheetsBatchUpdate(ctx, targetID, []google.SheetsRequest{{
			UpdateSheet: &google.UpdateSheetRequest{
				Properties: google.SheetProperties{SheetID: intPointer(copied.ID()), Title: newTitle},
				Fields:     "title",
			},
		}}); err != nil {
			payload["title"] = copied.Title
			payload["rename_failed"] = err.Error()
			return resultJSON(payload)
		}
		payload["title"] = newTitle
	}

	return resultJSON(payload)
}

func (r *registry) sheetsCopyRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID, err := requiredString(req, "source_spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sourceTitle, err := requiredString(req, "source_sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	targetID, err := requiredString(req, "target_spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	targetTitle, err := requiredString(req, "target_sheet_title")
	if err != nil {
		return toolError(err), nil
	}

	bounds, err := gridBounds(req)
	if err != nil {
		return toolError(err), nil
	}

	targetRow, targetColumn := req.GetInt("target_row", 0), req.GetInt("target_column", 0)
	if targetRow < 0 || targetColumn < 0 {
		return toolError(fmt.Errorf("target_row and target_column are counted from 0, got %d and %d",
			targetRow, targetColumn)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	a1 := google.A1Range(sourceTitle, rectangleA1(bounds))
	source, err := client.SpreadsheetCopyGrid(ctx, sourceID, a1)
	if err != nil {
		return toolError(err), nil
	}

	sheet := sheetNamed(source, sourceTitle)
	if sheet == nil || len(sheet.Data) == 0 {
		return toolError(fmt.Errorf("nothing came back for %s: check the tab name and the rectangle", a1)), nil
	}

	targetSheetID, err := r.sheetIDOf(ctx, client, targetID, targetTitle)
	if err != nil {
		return toolError(err), nil
	}

	rows := sheet.Data[0].RowData
	requests := []google.SheetsRequest{{
		UpdateCells: &google.UpdateCellsRequest{
			Start: &google.GridCoord{SheetID: targetSheetID, RowIndex: targetRow, ColumnIndex: targetColumn},
			Rows:  rows,
			// The mask is the whole of what this tool claims to carry. A field left out of
			// it is a field the target keeps from whatever was there before, which for a
			// copy reads as the copy having silently kept somebody else's formatting.
			Fields: "userEnteredValue,userEnteredFormat,note,dataValidation,textFormatRuns",
		},
	}}

	// A merge is part of what the rectangle looks like, so it travels — shifted by the
	// distance between the two corners, and only if it lies inside what was copied. A merge
	// half in and half out of the rectangle has no meaning at the far end.
	merged := 0
	shiftRow := targetRow - bounds["start_row"]
	shiftColumn := targetColumn - bounds["start_column"]
	for _, merge := range sheet.Merges {
		if !within(merge, bounds) {
			continue
		}
		requests = append(requests, google.SheetsRequest{
			MergeCells: &google.MergeCellsRequest{
				Range: google.GridRange{
					SheetID:          targetSheetID,
					StartRowIndex:    shifted(merge.StartRowIndex, shiftRow),
					EndRowIndex:      shifted(merge.EndRowIndex, shiftRow),
					StartColumnIndex: shifted(merge.StartColumnIndex, shiftColumn),
					EndColumnIndex:   shifted(merge.EndColumnIndex, shiftColumn),
				},
				MergeType: "MERGE_ALL",
			},
		})
		merged++
	}

	if _, err := client.SheetsBatchUpdate(ctx, targetID, requests); err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{
		"source_spreadsheet_id": sourceID,
		"source_range":          a1,
		"target_spreadsheet_id": targetID,
		"target_sheet_title":    targetTitle,
		"target_row":            targetRow,
		"target_column":         targetColumn,
		"rows":                  len(rows),
		"merges":                merged,
	}

	if left := notCarried(sheet, bounds); len(left) > 0 {
		payload["not_carried"] = left
		payload["note"] = "these belong to the tab rather than to the cells and this tool does not " +
			"write them; gdocs_sheets_copy_sheet brings a whole tab across with them, or rebuild them " +
			"with gdocs_sheets_set_conditional_format, set_banding and protect_range"
	}

	return resultJSON(payload)
}

// notCarried names the rules that touch the copied rectangle and do not travel with it.
//
// Naming them is the point of the tool answering at all: a rectangle whose colours come from
// a rule that paints by content arrives grey, and a caller told nothing reads that as the
// copy having worked.
func notCarried(sheet *google.Sheet, bounds map[string]int) []string {
	var left []string

	for _, rule := range sheet.ConditionalFormats {
		for _, area := range rule.Ranges {
			if within(area, bounds) || overlaps(area, bounds) {
				left = append(left, "conditional formatting")
				break
			}
		}
		if len(left) > 0 {
			break
		}
	}

	for _, banding := range sheet.BandedRanges {
		if overlaps(banding.Range, bounds) {
			left = append(left, "banding")
			break
		}
	}

	for _, protection := range sheet.ProtectedRanges {
		if protection.Range != nil && overlaps(*protection.Range, bounds) {
			left = append(left, "a protected range")
			break
		}
	}

	return left
}

// rectangleA1 spells a rectangle counted from zero the way the values endpoints want it.
func rectangleA1(bounds map[string]int) string {
	return google.ColumnLetters(bounds["start_column"]) + strconv.Itoa(bounds["start_row"]+1) +
		":" + google.ColumnLetters(bounds["end_column"]-1) + strconv.Itoa(bounds["end_row"])
}

// sheetNamed finds one tab of a reading by its title.
func sheetNamed(spreadsheet *google.Spreadsheet, title string) *google.Sheet {
	for index, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == title {
			return &spreadsheet.Sheets[index]
		}
	}

	return nil
}

// within says whether a range sits entirely inside the copied rectangle.
func within(area google.GridRange, bounds map[string]int) bool {
	edges := [][2]int{
		{value(area.StartRowIndex, 0), bounds["start_row"]},
		{value(area.StartColumnIndex, 0), bounds["start_column"]},
	}
	for _, edge := range edges {
		if edge[0] < edge[1] {
			return false
		}
	}

	return value(area.EndRowIndex, bounds["end_row"]) <= bounds["end_row"] &&
		value(area.EndColumnIndex, bounds["end_column"]) <= bounds["end_column"]
}

// overlaps says whether a range touches the copied rectangle at all. An unset edge means
// "to the end of the tab", which touches everything below or to the right of it.
func overlaps(area google.GridRange, bounds map[string]int) bool {
	return value(area.StartRowIndex, 0) < bounds["end_row"] &&
		value(area.EndRowIndex, bounds["end_row"]) > bounds["start_row"] &&
		value(area.StartColumnIndex, 0) < bounds["end_column"] &&
		value(area.EndColumnIndex, bounds["end_column"]) > bounds["start_column"]
}

func value(pointer *int, fallback int) int {
	if pointer == nil {
		return fallback
	}
	return *pointer
}

func shifted(pointer *int, by int) *int {
	if pointer == nil {
		return nil
	}
	moved := *pointer + by

	return &moved
}

func intPointer(value int) *int { return &value }
