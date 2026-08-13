package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerSheetsData adds the things a workbook does to its own contents — charts, tables,
// and the operations a person reaches for from the Data menu.
func (r *registry) registerSheetsData(srv *server.MCPServer) {
	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_sheets_add_chart",
		mcp.WithDescription("Draw a chart from a range: column, bar, line, area, stepped area, scatter or "+
			"pie. It floats over a tab at a cell you name, or lands on a tab of its own. The chart reads "+
			"the cells rather than a copy of them, so it follows the numbers as they change."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the data is on.")),
		mcp.WithString("type", mcp.DefaultString("COLUMN"), mcp.Description(
			"COLUMN, BAR, LINE, AREA, STEPPED_AREA, SCATTER or PIE.")),
		mcp.WithString("title", mcp.Description("Title over the chart.")),
		mcp.WithString("subtitle", mcp.Description("Line under the title.")),
		mcp.WithNumber("labels_column", mcp.Required(), mcp.Description(
			"Column holding what runs along the bottom — the names of the bars, or the slices of a pie.")),
		mcp.WithArray("value_columns", mcp.Required(), mcp.Description(
			"Columns holding the numbers, one series each. A pie takes exactly one."),
			mcp.Items(map[string]any{"type": "integer"})),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description(
			"First row of the data, counting from 0. Include the heading row and set header_rows to 1 "+
				"to have the headings name the series.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("header_rows", mcp.DefaultNumber(1), mcp.Description(
			"How many rows at the top are headings rather than data.")),
		mcp.WithString("legend", mcp.DefaultString("RIGHT_LEGEND"), mcp.Description(
			"BOTTOM_LEGEND, LEFT_LEGEND, RIGHT_LEGEND, TOP_LEGEND, NO_LEGEND or LABELED_LEGEND.")),
		mcp.WithString("stacked", mcp.Description(
			"STACKED to pile the series on each other, PERCENT_STACKED to make each column add to 100%.")),
		mcp.WithString("axis_title", mcp.Description("Title under the bottom axis.")),
		mcp.WithString("value_axis_title", mcp.Description("Title beside the left axis.")),
		mcp.WithNumber("pie_hole", mcp.Description(
			"Hole in the middle of a pie, 0 to 1. Anything above 0 makes it a doughnut.")),
		mcp.WithBoolean("own_tab", mcp.Description("Put the chart on a tab of its own instead of over this one.")),
		mcp.WithNumber("anchor_row", mcp.Description("Row the floating chart's top-left corner sits on.")),
		mcp.WithNumber("anchor_column", mcp.Description("Column the floating chart's top-left corner sits on.")),
		mcp.WithNumber("width_pixels", mcp.Description("How wide the floating chart is.")),
		mcp.WithNumber("height_pixels", mcp.Description("How tall the floating chart is.")),
	), r.sheetsAddChart)

	srv.AddTool(mcp.NewTool("gdocs_sheets_add_table",
		mcp.WithDescription("Turn a rectangle into one of Sheets' tables: a named block whose columns have "+
			"types of their own. A DROPDOWN column is the modern chip-style list, and the table's own "+
			"banding follows rows added to it — neither is reachable by formatting cells."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the rectangle is on.")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name of the table.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description(
			"First row, counting from 0. This is the heading row: a table takes its column names from it.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
		mcp.WithArray("columns", mcp.Description(
			"What each column holds: {\"column\": 0, \"name\": \"Статус\", \"type\": \"DROPDOWN\", "+
				"\"values\": [\"Всё ок\", \"Пауза\"]}. type is TEXT, DOUBLE, CURRENCY, PERCENT, DATE, "+
				"TIME, DATE_TIME, BOOLEAN or DROPDOWN; column is the sheet's own number."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"column": map[string]any{"type": "integer"},
					"name":   map[string]any{"type": "string"},
					"type":   map[string]any{"type": "string"},
					"values": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"column"},
			})),
		mcp.WithString("header_color", mcp.Description("Colour of the heading row, as #RRGGBB.")),
		mcp.WithString("first_band_color", mcp.Description("Colour of the odd rows.")),
		mcp.WithString("second_band_color", mcp.Description("Colour of the even rows.")),
	), r.sheetsAddTable)

	srv.AddTool(mcp.NewTool("gdocs_sheets_find_replace",
		mcp.WithDescription("Replace text across a range, a tab or the whole workbook, optionally by "+
			"regular expression. This changes values: name a range when only part of a tab is meant."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("find", mcp.Required(), mcp.Description("What to look for.")),
		mcp.WithString("replacement", mcp.Required(), mcp.Description("What to put in its place.")),
		mcp.WithString("sheet_title", mcp.Description("Tab to work on. Without it, and without all_sheets, nothing runs.")),
		mcp.WithBoolean("all_sheets", mcp.Description("Every tab of the workbook.")),
		mcp.WithNumber("start_row", mcp.Description("Narrow it to a rectangle: first row, from 0.")),
		mcp.WithNumber("end_row", mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Description("First column, from 0.")),
		mcp.WithNumber("end_column", mcp.Description("Column to stop before.")),
		mcp.WithBoolean("match_case", mcp.Description("Tell upper case from lower.")),
		mcp.WithBoolean("match_entire_cell", mcp.Description("Only replace when the whole cell is the text.")),
		mcp.WithBoolean("regex", mcp.Description("Read what to find as a regular expression.")),
		mcp.WithBoolean("include_formulas", mcp.Description("Look inside formulas as well as values.")),
	), r.sheetsFindReplace)

	srv.AddTool(mcp.NewTool("gdocs_sheets_trim_whitespace",
		mcp.WithDescription("Take the spaces off both ends of every cell in a range, and squeeze the runs "+
			"inside. A column pasted from elsewhere sorts and groups wrongly until this is done."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the range is on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
	), r.sheetsTrimWhitespace)

	srv.AddTool(mcp.NewTool("gdocs_sheets_split_column",
		mcp.WithDescription("Split one column into several by a separator. What is to the right of it is "+
			"written over, so make room first with gdocs_sheets_insert_dimensions."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the column is on.")),
		mcp.WithNumber("column", mcp.Required(), mcp.Description("Column to split, counting from 0.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithString("separator", mcp.DefaultString("COMMA"), mcp.Description(
			"COMMA, SEMICOLON, PERIOD, SPACE, AUTODETECT, or CUSTOM with a separator of your own.")),
		mcp.WithString("custom_separator", mcp.Description("The separator itself, when separator is CUSTOM.")),
	), r.sheetsSplitColumn)

	srv.AddTool(mcp.NewTool("gdocs_sheets_auto_fill",
		mcp.WithDescription("Continue a series the way dragging the corner of a cell does: dates, numbers, "+
			"weekdays, and formulas whose references move along."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to fill on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row of the example, from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column of the example, from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
		mcp.WithString("direction", mcp.DefaultString("ROWS"), mcp.Description(
			"ROWS to carry on downwards, COLUMNS to carry on to the right.")),
		mcp.WithNumber("length", mcp.Required(), mcp.Description(
			"How many rows or columns to fill after the example. Negative fills upwards or leftwards.")),
	), r.sheetsAutoFill)

	srv.AddTool(mcp.NewTool("gdocs_sheets_collapse_group",
		mcp.WithDescription("Fold a group of rows or columns up, or open it. The rows stay either way — "+
			"this is what the reader sees, not what the tab holds."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the group is on.")),
		mcp.WithString("dimension", mcp.DefaultString("ROWS"), mcp.Description("ROWS or COLUMNS.")),
		mcp.WithNumber("start", mcp.Required(), mcp.Description("First row or column of the group, from 0.")),
		mcp.WithNumber("end", mcp.Required(), mcp.Description("Row or column to stop before.")),
		mcp.WithNumber("depth", mcp.DefaultNumber(1), mcp.Description(
			"Which nested group is meant, counting outwards from 1.")),
		mcp.WithBoolean("collapsed", mcp.DefaultBool(true), mcp.Description("Folded up, or open.")),
	), r.sheetsCollapseGroup)
}

// chartTypes are the kinds this tool draws. Everything here reads a rectangle and needs
// nothing else; the histograms, candlesticks and org charts of the API need a shape of
// their own and are not here.
var chartTypes = map[string]bool{
	"COLUMN": true, "BAR": true, "LINE": true, "AREA": true,
	"STEPPED_AREA": true, "SCATTER": true,
}

func (r *registry) sheetsAddChart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}

	kind := strings.ToUpper(req.GetString("type", "COLUMN"))
	if kind != "PIE" && !chartTypes[kind] {
		return toolError(fmt.Errorf("type %q is not one this tool draws: use COLUMN, BAR, LINE, AREA, "+
			"STEPPED_AREA, SCATTER or PIE", kind)), nil
	}

	startRow, err := req.RequireInt("start_row")
	if err != nil {
		return toolError(err), nil
	}
	endRow, err := req.RequireInt("end_row")
	if err != nil {
		return toolError(err), nil
	}
	if startRow < 0 || endRow <= startRow {
		return toolError(fmt.Errorf("the data is empty: end_row is exclusive and both count from 0")), nil
	}

	labels, err := req.RequireInt("labels_column")
	if err != nil {
		return toolError(err), nil
	}
	values, err := intList(req, "value_columns")
	if err != nil {
		return toolError(err), nil
	}
	if len(values) == 0 {
		return toolError(fmt.Errorf("value_columns is empty: a chart needs at least one column of numbers")), nil
	}
	if kind == "PIE" && len(values) != 1 {
		return toolError(fmt.Errorf("a pie draws one column of numbers, got %d", len(values))), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	column := func(index int) google.ChartData {
		start, end := index, index+1
		return google.ChartData{SourceRange: &google.ChartSourceRange{Sources: []google.GridRange{{
			SheetID:          sheetID,
			StartRowIndex:    &startRow,
			EndRowIndex:      &endRow,
			StartColumnIndex: &start,
			EndColumnIndex:   &end,
		}}}}
	}

	spec := google.ChartSpec{
		Title:    optionalString(req, "title"),
		Subtitle: optionalString(req, "subtitle"),
	}
	legend := strings.ToUpper(req.GetString("legend", "RIGHT_LEGEND"))

	if kind == "PIE" {
		pie := &google.PieChartSpec{
			Domain:         column(labels),
			Series:         column(values[0]),
			LegendPosition: legend,
		}
		if hole, ok := req.GetArguments()["pie_hole"]; ok {
			_ = hole
			size := req.GetFloat("pie_hole", 0)
			if size < 0 || size >= 1 {
				return toolError(fmt.Errorf("pie_hole is %v: it runs from 0 to just under 1", size)), nil
			}
			pie.PieHole = size
		}
		spec.PieChart = pie
	} else {
		basic := &google.BasicChartSpec{
			ChartType:      kind,
			LegendPosition: legend,
			HeaderCount:    req.GetInt("header_rows", 1),
			Domains:        []google.BasicChartDomain{{Domain: column(labels)}},
		}
		if stacked := strings.ToUpper(optionalString(req, "stacked")); stacked != "" {
			if stacked != "STACKED" && stacked != "PERCENT_STACKED" {
				return toolError(fmt.Errorf("stacked is %q: use STACKED or PERCENT_STACKED", stacked)), nil
			}
			basic.StackedType = stacked
		}
		for _, index := range values {
			basic.Series = append(basic.Series, google.BasicChartSeries{
				Series: column(index), TargetAxis: "LEFT_AXIS"})
		}
		for _, axis := range []struct {
			position, argument string
		}{
			{"BOTTOM_AXIS", "axis_title"},
			{"LEFT_AXIS", "value_axis_title"},
		} {
			if title := optionalString(req, axis.argument); title != "" {
				basic.Axis = append(basic.Axis, google.BasicChartAxis{
					Position: axis.position, Title: title})
			}
		}
		spec.BasicChart = basic
	}

	position := google.EmbeddedObjectPosition{}
	if req.GetBool("own_tab", false) {
		position.NewSheet = true
	} else {
		overlay := &google.OverlayPosition{AnchorCell: google.GridCoord{
			SheetID:     sheetID,
			RowIndex:    req.GetInt("anchor_row", endRow+1),
			ColumnIndex: req.GetInt("anchor_column", 0),
		}}
		overlay.WidthPixels = req.GetInt("width_pixels", 600)
		overlay.HeightPixels = req.GetInt("height_pixels", 371)
		position.OverlayPosition = overlay
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		AddChart: &google.AddChartRequest{Chart: google.EmbeddedChart{Spec: spec, Position: position}},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"type": kind, "series": len(values), "own_tab": position.NewSheet,
	})
}

// tableColumnTypes are what a table's column may hold. The chip kinds that point at people,
// files or places are left out: each needs a source this server does not reach.
var tableColumnTypes = map[string]bool{
	"TEXT": true, "DOUBLE": true, "CURRENCY": true, "PERCENT": true,
	"DATE": true, "TIME": true, "DATE_TIME": true, "BOOLEAN": true, "DROPDOWN": true,
}

func (r *registry) sheetsAddTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	name, err := requiredString(req, "name")
	if err != nil {
		return toolError(err), nil
	}
	bounds, err := gridBounds(req)
	if err != nil {
		return toolError(err), nil
	}

	var columns []google.SheetsTableColumn
	if _, ok := req.GetArguments()["columns"]; ok {
		entries, err := objectList(req, "columns")
		if err != nil {
			return toolError(err), nil
		}

		for index, entry := range entries {
			at, ok := intField(entry, "column")
			if !ok || at < 0 {
				return toolError(fmt.Errorf("columns[%d].column is missing or negative", index)), nil
			}

			properties := google.SheetsTableColumn{
				ColumnIndex: at,
				ColumnName:  stringField(entry, "name"),
			}

			kind := strings.ToUpper(stringField(entry, "type"))
			if kind != "" {
				if !tableColumnTypes[kind] {
					return toolError(fmt.Errorf("columns[%d].type is %q: use TEXT, DOUBLE, CURRENCY, "+
						"PERCENT, DATE, TIME, DATE_TIME, BOOLEAN or DROPDOWN", index, kind)), nil
				}
				properties.ColumnType = kind
			}

			list, err := stringListField(entry, "values")
			if err != nil {
				return toolError(fmt.Errorf("columns[%d]: %w", index, err)), nil
			}
			if len(list) > 0 {
				if properties.ColumnType != "DROPDOWN" {
					return toolError(fmt.Errorf("columns[%d] has values but is a %s column: a list of "+
						"values belongs to a DROPDOWN", index, properties.ColumnType)), nil
				}
				condition := &google.BooleanCondition{Type: "ONE_OF_LIST"}
				for _, value := range list {
					condition.Values = append(condition.Values, google.ConditionValue{UserEnteredValue: value})
				}
				properties.DataValidationRule = &google.TableColumnValidation{Condition: condition}
			}

			columns = append(columns, properties)
		}
	}

	var rows *google.TableRowsProperties

	header, err := sheetColor(req, "header_color")
	if err != nil {
		return toolError(err), nil
	}
	first, err := sheetColor(req, "first_band_color")
	if err != nil {
		return toolError(err), nil
	}
	second, err := sheetColor(req, "second_band_color")
	if err != nil {
		return toolError(err), nil
	}
	if header != nil || first != nil || second != nil {
		rows = &google.TableRowsProperties{}
		if header != nil {
			rows.HeaderColorStyle = &google.ColorStyle{RGBColor: header}
		}
		if first != nil {
			rows.FirstBandColorStyle = &google.ColorStyle{RGBColor: first}
		}
		if second != nil {
			rows.SecondBandColorStyle = &google.ColorStyle{RGBColor: second}
		}
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		AddTable: &google.AddTableRequest{Table: google.SheetsTable{
			Name:             name,
			Range:            gridRangeOf(sheetID, bounds),
			ColumnProperties: columns,
			RowsProperties:   rows,
		}},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"name": name, "columns": len(columns),
	})
}

func (r *registry) sheetsFindReplace(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	find, err := requiredString(req, "find")
	if err != nil {
		return toolError(err), nil
	}

	replacement := optionalString(req, "replacement")
	sheetTitle := optionalString(req, "sheet_title")
	allSheets := req.GetBool("all_sheets", false)

	if sheetTitle == "" && !allSheets {
		return toolError(fmt.Errorf("name a sheet_title, or say all_sheets: a replacement with neither " +
			"would have nowhere to run")), nil
	}
	if sheetTitle != "" && allSheets {
		return toolError(fmt.Errorf("sheet_title and all_sheets are alternatives")), nil
	}

	request := &google.FindReplaceRequest{
		Find:            find,
		Replacement:     replacement,
		AllSheets:       allSheets,
		MatchCase:       req.GetBool("match_case", false),
		MatchEntireCell: req.GetBool("match_entire_cell", false),
		SearchByRegex:   req.GetBool("regex", false),
		IncludeFormulas: req.GetBool("include_formulas", false),
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if sheetTitle != "" {
		sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
		if err != nil {
			return toolError(err), nil
		}

		if _, narrowed := req.GetArguments()["start_row"]; narrowed {
			bounds, err := gridBounds(req)
			if err != nil {
				return toolError(err), nil
			}
			span := gridRangeOf(sheetID, bounds)
			request.Range = &span
		} else {
			request.SheetID = google.SheetIDOf(sheetID)
		}
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID,
		[]google.SheetsRequest{{FindReplace: request}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "find": find, "replacement": replacement,
		"scope": map[bool]string{true: "the whole workbook", false: sheetTitle}[allSheets],
	})
}

func (r *registry) sheetsTrimWhitespace(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, sheetTitle, sheetID, bounds, failure := r.rangeArguments(ctx, req)
	if failure != nil {
		return failure, nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		TrimWhitespace: &google.TrimWhitespaceRequest{Range: gridRangeOf(sheetID, bounds)},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
	})
}

// separators are the ways a column can be split.
var separators = map[string]bool{
	"COMMA": true, "SEMICOLON": true, "PERIOD": true, "SPACE": true,
	"AUTODETECT": true, "CUSTOM": true,
}

func (r *registry) sheetsSplitColumn(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}

	column, err := req.RequireInt("column")
	if err != nil {
		return toolError(err), nil
	}
	startRow, err := req.RequireInt("start_row")
	if err != nil {
		return toolError(err), nil
	}
	endRow, err := req.RequireInt("end_row")
	if err != nil {
		return toolError(err), nil
	}
	if column < 0 || startRow < 0 || endRow <= startRow {
		return toolError(fmt.Errorf("the column is empty: end_row is exclusive and all three count from 0")), nil
	}

	separator := strings.ToUpper(req.GetString("separator", "COMMA"))
	if !separators[separator] {
		return toolError(fmt.Errorf("separator %q is not one Sheets splits on: use COMMA, SEMICOLON, "+
			"PERIOD, SPACE, AUTODETECT or CUSTOM", separator)), nil
	}

	custom := optionalString(req, "custom_separator")
	if separator == "CUSTOM" && custom == "" {
		return toolError(fmt.Errorf("separator is CUSTOM and custom_separator is empty")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	endColumn := column + 1
	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		TextToColumns: &google.TextToColumnsRequest{
			Source: google.GridRange{SheetID: sheetID,
				StartRowIndex: &startRow, EndRowIndex: &endRow,
				StartColumnIndex: &column, EndColumnIndex: &endColumn},
			DelimiterType: separator,
			Delimiter:     custom,
		},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"column": column, "separator": separator,
	})
}

func (r *registry) sheetsAutoFill(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, sheetTitle, sheetID, bounds, failure := r.rangeArguments(ctx, req)
	if failure != nil {
		return failure, nil
	}

	dimension, err := dimensionOf(req)
	if err != nil {
		return toolError(err), nil
	}

	length, err := req.RequireInt("length")
	if err != nil {
		return toolError(err), nil
	}
	if length == 0 {
		return toolError(fmt.Errorf("length is 0: nothing would be filled")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		AutoFill: &google.AutoFillRequest{SourceAndDestination: &google.SourceAndDestination{
			Source: gridRangeOf(sheetID, bounds), Dimension: dimension, FillLength: length}},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"dimension": dimension, "length": length,
	})
}

func (r *registry) sheetsCollapseGroup(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	dimension, err := dimensionOf(req)
	if err != nil {
		return toolError(err), nil
	}

	start, err := req.RequireInt("start")
	if err != nil {
		return toolError(err), nil
	}
	end, err := req.RequireInt("end")
	if err != nil {
		return toolError(err), nil
	}
	if start < 0 || end <= start {
		return toolError(fmt.Errorf("the run is empty: end is exclusive")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		UpdateGroup: &google.UpdateDimensionGroupRequest{
			DimensionGroup: google.DimensionGroup{
				Range: google.DimensionRange{SheetID: sheetID, Dimension: dimension,
					StartIndex: &start, EndIndex: &end},
				Depth:     req.GetInt("depth", 1),
				Collapsed: req.GetBool("collapsed", true),
			},
			Fields: "collapsed",
		},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"dimension": dimension, "collapsed": req.GetBool("collapsed", true),
	})
}

// rangeArguments reads the four arguments every rectangle tool takes and resolves the tab,
// because five of them do exactly this and nothing else before their one request.
func (r *registry) rangeArguments(ctx context.Context, req mcp.CallToolRequest) (
	string, string, int, map[string]int, *mcp.CallToolResult) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return "", "", 0, nil, toolError(err)
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return "", "", 0, nil, toolError(err)
	}
	bounds, err := gridBounds(req)
	if err != nil {
		return "", "", 0, nil, toolError(err)
	}

	client, err := r.client(ctx)
	if err != nil {
		return "", "", 0, nil, toolError(err)
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return "", "", 0, nil, toolError(err)
	}

	return spreadsheetID, sheetTitle, sheetID, bounds, nil
}
