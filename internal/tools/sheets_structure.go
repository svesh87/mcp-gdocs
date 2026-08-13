package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerSheetsStructure adds everything about a workbook that is not a cell's own look:
// borders, rules that colour by content, banding, filters, protection, names, groups, and
// the ways rows and columns are added or moved.
//
// They are here rather than in sheets.go because they share one shape — a rectangle and a
// description of what to do with it — and because together they are what makes a copy of a
// real workbook behave like the original rather than merely resemble it.
func (r *registry) registerSheetsStructure(srv *server.MCPServer) {
	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_sheets_set_borders",
		mcp.WithDescription("Draw the edges of a rectangle and the lines inside it. Each side takes a "+
			"style and a colour; NONE takes a line away, which removes a border and never a cell. "+
			"Borders are what a table of numbers is read by, and a copy without them looks like a dump."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the range is on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
		mcp.WithString("style", mcp.Description(
			"Style for every side named: SOLID, SOLID_MEDIUM, SOLID_THICK, DASHED, DOTTED, DOUBLE or NONE.")),
		mcp.WithString("color", mcp.Description("Colour of the lines, as #RRGGBB or {red, green, blue}.")),
		mcp.WithArray("sides", mcp.WithStringItems(), mcp.Description(
			"Which sides to draw: top, bottom, left, right, inner_horizontal, inner_vertical, or all. "+
				"Default: all.")),
	), r.sheetsSetBorders)

	srv.AddTool(mcp.NewTool("gdocs_sheets_set_conditional_format",
		mcp.WithDescription("Add a rule that colours cells by what is in them — the red-amber-green a "+
			"status column is read by. Either a condition with a format, or a colour scale across the "+
			"range. Rules are tried in order and the first match wins, so index decides which one shows."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the range is on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
		mcp.WithString("condition", mcp.Description(
			"What makes the rule fire: TEXT_EQ, TEXT_CONTAINS, TEXT_STARTS_WITH, TEXT_ENDS_WITH, "+
				"NUMBER_GREATER, NUMBER_GREATER_THAN_EQ, NUMBER_LESS, NUMBER_LESS_THAN_EQ, NUMBER_EQ, "+
				"NUMBER_BETWEEN, DATE_BEFORE, DATE_AFTER, BLANK, NOT_BLANK or CUSTOM_FORMULA.")),
		mcp.WithArray("values", mcp.WithStringItems(), mcp.Description(
			"What the condition compares against: the text, the number, or the formula for "+
				"CUSTOM_FORMULA. BLANK and NOT_BLANK take none; NUMBER_BETWEEN takes two.")),
		mcp.WithString("background_color", mcp.Description("Fill for a matching cell, #RRGGBB or an object.")),
		mcp.WithString("text_color", mcp.Description("Text colour for a matching cell.")),
		mcp.WithBoolean("bold", mcp.Description("Bold a matching cell.")),
		mcp.WithBoolean("italic", mcp.Description("Italicise a matching cell.")),
		mcp.WithBoolean("strikethrough", mcp.Description("Strike a matching cell through.")),
		mcp.WithArray("gradient", mcp.Description(
			"A colour scale instead of a condition, as two or three points: "+
				"[{\"type\": \"MIN\", \"color\": \"#FFFFFF\"}, {\"type\": \"MAX\", \"color\": \"#57BB8A\"}]. "+
				"type is MIN, MAX, NUMBER, PERCENT or PERCENTILE; NUMBER and the two per-cent kinds take "+
				"a value."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":  map[string]any{"type": "string"},
					"color": map[string]any{"type": "string"},
					"value": map[string]any{"type": "string"},
				},
				"required": []string{"type", "color"},
			})),
		mcp.WithNumber("index", mcp.Description(
			"Where in the tab's list of rules this one goes. Default: last, which means an earlier "+
				"rule over the same cells wins.")),
		mcp.WithNumber("replace_index", mcp.Description(
			"Replace the rule already at this index instead of adding one.")),
	), r.sheetsSetConditionalFormat)

	srv.AddTool(mcp.NewTool("gdocs_sheets_set_banding",
		mcp.WithDescription("Paint a range in alternating stripes, with their own colours for the header "+
			"and the footer. This is a property of the range, not of its cells: rows inserted into it "+
			"keep the stripes, which is why a sample's banding cannot be reproduced by colouring cells."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the range is on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
		mcp.WithString("first_band_color", mcp.Required(), mcp.Description("Colour of the odd stripes.")),
		mcp.WithString("second_band_color", mcp.Required(), mcp.Description("Colour of the even stripes.")),
		mcp.WithString("header_color", mcp.Description("Colour of the first row of the range.")),
		mcp.WithString("footer_color", mcp.Description("Colour of the last row of the range.")),
		mcp.WithString("direction", mcp.DefaultString("ROWS"), mcp.Description(
			"ROWS for stripes across, COLUMNS for stripes down.")),
	), r.sheetsSetBanding)

	srv.AddTool(mcp.NewTool("gdocs_sheets_set_filter",
		mcp.WithDescription("Put the tab's filter on a range, with what each column hides and how the "+
			"range is sorted. A sample with a filter is a sample somebody works in daily; a copy without "+
			"one is read-only in practice."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to filter.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
		mcp.WithArray("hide", mcp.Description(
			"Values hidden per column: [{\"column\": 10, \"values\": [\"Не начали\"]}]. Columns are the "+
				"sheet's own numbers, not the range's."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"column": map[string]any{"type": "integer"},
					"values": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"column", "values"},
			})),
		mcp.WithArray("sort", mcp.Description(
			"How the range is sorted: [{\"column\": 3, \"order\": \"ASCENDING\"}]."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"column": map[string]any{"type": "integer"},
					"order":  map[string]any{"type": "string"},
				},
				"required": []string{"column"},
			})),
	), r.sheetsSetFilter)

	srv.AddTool(mcp.NewTool("gdocs_sheets_protect_range",
		mcp.WithDescription("Keep a range from being changed — a warning on edit, or only named people. "+
			"A whole tab is protected by naming no rectangle. This adds protection; it never removes any."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to protect.")),
		mcp.WithNumber("start_row", mcp.Description("First row, counting from 0. Leave the four out to protect the tab.")),
		mcp.WithNumber("end_row", mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Description("Column to stop before.")),
		mcp.WithString("description", mcp.Description("Why it is protected — shown to whoever bumps into it.")),
		mcp.WithBoolean("warning_only", mcp.DefaultBool(true), mcp.Description(
			"Warn and let the edit through. Off means only the named editors may change it.")),
		mcp.WithArray("editors", mcp.WithStringItems(), mcp.Description(
			"Addresses allowed to edit when warning_only is off.")),
	), r.sheetsProtectRange)

	srv.AddTool(mcp.NewTool("gdocs_sheets_add_named_range",
		mcp.WithDescription("Give a range a name, so formulas and dropdowns can say what they mean "+
			"instead of where they point. A sample's ONE_OF_RANGE dropdown usually points at one."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the range is on.")),
		mcp.WithString("name", mcp.Required(), mcp.Description("The name. Letters, digits and underscores.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
	), r.sheetsAddNamedRange)

	srv.AddTool(mcp.NewTool("gdocs_sheets_group_dimensions",
		mcp.WithDescription("Fold a run of rows or columns into a group with a handle in the margin. "+
			"Grouping is not hiding: the rows stay, and the reader decides."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to group on.")),
		mcp.WithString("dimension", mcp.DefaultString("ROWS"), mcp.Description("ROWS or COLUMNS.")),
		mcp.WithNumber("start", mcp.Required(), mcp.Description("First row or column, counting from 0.")),
		mcp.WithNumber("end", mcp.Required(), mcp.Description("Row or column to stop before.")),
	), r.sheetsGroupDimensions)

	srv.AddTool(mcp.NewTool("gdocs_sheets_insert_dimensions",
		mcp.WithDescription("Make room: add rows or columns in the middle of a tab, or at its end. "+
			"Everything below or to the right moves along and nothing is lost — this is the way a grid "+
			"grows, and the only one this server has."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to grow.")),
		mcp.WithString("dimension", mcp.DefaultString("ROWS"), mcp.Description("ROWS or COLUMNS.")),
		mcp.WithNumber("at", mcp.Description(
			"Where they appear, counting from 0. Without it they are added at the end of the tab.")),
		mcp.WithNumber("count", mcp.Required(), mcp.Description("How many to add.")),
		mcp.WithBoolean("inherit_from_before", mcp.DefaultBool(true), mcp.Description(
			"Take the formatting of the row above, rather than the one below.")),
	), r.sheetsInsertDimensions)

	srv.AddTool(mcp.NewTool("gdocs_sheets_move_dimensions",
		mcp.WithDescription("Move a run of rows or columns somewhere else on the same tab. Nothing is "+
			"removed: what was there shifts to make room."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to move on.")),
		mcp.WithString("dimension", mcp.DefaultString("ROWS"), mcp.Description("ROWS or COLUMNS.")),
		mcp.WithNumber("start", mcp.Required(), mcp.Description("First row or column to move, from 0.")),
		mcp.WithNumber("end", mcp.Required(), mcp.Description("Row or column to stop before.")),
		mcp.WithNumber("to", mcp.Required(), mcp.Description(
			"Index they land before, in the numbering as it is now.")),
	), r.sheetsMoveDimensions)

	srv.AddTool(mcp.NewTool("gdocs_sheets_sort_range",
		mcp.WithDescription("Sort a rectangle by one or more of its columns. The rectangle must not "+
			"include the heading row, or the heading is sorted with the data."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the range is on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
		mcp.WithArray("by", mcp.Required(), mcp.Description(
			"Columns to sort by, in order: [{\"column\": 3, \"order\": \"ASCENDING\"}]. Columns are the "+
				"sheet's own numbers."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"column": map[string]any{"type": "integer"},
					"order":  map[string]any{"type": "string"},
				},
				"required": []string{"column"},
			})),
	), r.sheetsSortRange)

	srv.AddTool(mcp.NewTool("gdocs_sheets_duplicate_tab",
		mcp.WithDescription("Copy a whole tab inside the same workbook. This is the only way to carry "+
			"everything a tab has — charts, banding, protection, the colours of its dropdowns — because "+
			"no pair of reading and writing tools reaches all of it."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to copy.")),
		mcp.WithString("new_title", mcp.Description("Name for the copy. Without one Google names it.")),
		mcp.WithNumber("index", mcp.Description("Where the copy goes, counting from 0.")),
	), r.sheetsDuplicateTab)

	srv.AddTool(mcp.NewTool("gdocs_sheets_update_properties",
		mcp.WithDescription("Change the workbook's own settings after it exists: its title, its locale, "+
			"its time zone. The locale decides how typed text is read, so changing it changes what the "+
			"next write stores, not what is already there."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("title", mcp.Description("New name for the workbook.")),
		mcp.WithString("locale", mcp.Description("Locale, e.g. ru_RU.")),
		mcp.WithString("time_zone", mcp.Description("Time zone, e.g. Europe/Moscow.")),
	), r.sheetsUpdateProperties)

	srv.AddTool(mcp.NewTool("gdocs_sheets_set_text_runs",
		mcp.WithDescription("Style parts of one cell's text differently — a bold lead-in and a plain "+
			"remainder in the same cell. Each run starts at a character offset and holds until the next "+
			"begins. Offsets are counted the way the reading reports them."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the cell is on.")),
		mcp.WithNumber("row", mcp.Required(), mcp.Description("Row, counting from 0.")),
		mcp.WithNumber("column", mcp.Required(), mcp.Description("Column, counting from 0.")),
		mcp.WithString("text", mcp.Required(), mcp.Description(
			"The cell's whole text. It is written along with the runs, because a run without the text "+
				"it belongs to lands on whatever happens to be there.")),
		mcp.WithArray("runs", mcp.Required(), mcp.Description(
			"Runs, each starting at a character: [{\"start\": 0, \"bold\": true}, {\"start\": 6, "+
				"\"bold\": false, \"text_color\": \"#666666\"}]. A run may set bold, italic, underline, "+
				"strikethrough, font_family, font_size, text_color and link."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"start":         map[string]any{"type": "integer"},
					"bold":          map[string]any{"type": "boolean"},
					"italic":        map[string]any{"type": "boolean"},
					"underline":     map[string]any{"type": "boolean"},
					"strikethrough": map[string]any{"type": "boolean"},
					"font_family":   map[string]any{"type": "string"},
					"font_size":     map[string]any{"type": "integer"},
					"text_color":    map[string]any{"type": "string"},
					"link":          map[string]any{"type": "string"},
				},
				"required": []string{"start"},
			})),
	), r.sheetsSetTextRuns)
}

// sheetIDOf resolves a tab's name to the number the batch requests address it by, and says
// which tabs there are when the name is wrong.
func (r *registry) sheetIDOf(ctx context.Context, client *google.Client, spreadsheetID, title string) (int, error) {
	spreadsheet, err := client.Spreadsheet(ctx, spreadsheetID)
	if err != nil {
		return 0, err
	}

	sheetID, ok := spreadsheet.SheetIDByTitle(title)
	if !ok {
		return 0, fmt.Errorf("no tab called %q in this spreadsheet: it has %s",
			title, strings.Join(spreadsheet.SheetTitles(), ", "))
	}

	return sheetID, nil
}

// gridRangeOf builds the rectangle the writing requests take.
func gridRangeOf(sheetID int, bounds map[string]int) google.GridRange {
	startRow, endRow := bounds["start_row"], bounds["end_row"]
	startColumn, endColumn := bounds["start_column"], bounds["end_column"]
	return google.GridRange{
		SheetID:          sheetID,
		StartRowIndex:    &startRow,
		EndRowIndex:      &endRow,
		StartColumnIndex: &startColumn,
		EndColumnIndex:   &endColumn,
	}
}

// borderStyles is what Sheets will draw. NONE is here on purpose: taking a line off is not
// deleting anything, and without it a border could only ever be added.
var borderStyles = map[string]bool{
	"SOLID": true, "SOLID_MEDIUM": true, "SOLID_THICK": true,
	"DASHED": true, "DOTTED": true, "DOUBLE": true, "NONE": true,
}

func (r *registry) sheetsSetBorders(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	bounds, err := gridBounds(req)
	if err != nil {
		return toolError(err), nil
	}

	style := strings.ToUpper(req.GetString("style", "SOLID"))
	if !borderStyles[style] {
		return toolError(fmt.Errorf("style %q is not one Sheets draws: use SOLID, SOLID_MEDIUM, "+
			"SOLID_THICK, DASHED, DOTTED, DOUBLE or NONE", style)), nil
	}

	colour, err := sheetColor(req, "color")
	if err != nil {
		return toolError(err), nil
	}

	sides := req.GetStringSlice("sides", nil)
	if len(sides) == 0 {
		sides = []string{"all"}
	}

	border := &google.Border{Style: style, Color: colour}
	request := &google.UpdateBordersRequest{}
	for _, side := range sides {
		switch strings.ToLower(strings.TrimSpace(side)) {
		case "all":
			request.Top, request.Bottom = border, border
			request.Left, request.Right = border, border
			request.InnerHorizontal, request.InnerVertical = border, border
		case "top":
			request.Top = border
		case "bottom":
			request.Bottom = border
		case "left":
			request.Left = border
		case "right":
			request.Right = border
		case "inner_horizontal":
			request.InnerHorizontal = border
		case "inner_vertical":
			request.InnerVertical = border
		default:
			return toolError(fmt.Errorf("sides carries %q: use top, bottom, left, right, "+
				"inner_horizontal, inner_vertical or all", side)), nil
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

	request.Range = gridRangeOf(sheetID, bounds)

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID,
		[]google.SheetsRequest{{UpdateBorders: request}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"style": style, "sides": sides,
	})
}

// conditionTypes are the tests a conditional format may apply. The list is the useful
// subset of what the API knows: everything here can be written from a description a person
// would give, and nothing here needs a second range to point at.
var conditionTypes = map[string]int{
	"TEXT_EQ": 1, "TEXT_CONTAINS": 1, "TEXT_NOT_CONTAINS": 1,
	"TEXT_STARTS_WITH": 1, "TEXT_ENDS_WITH": 1, "TEXT_NOT_EQ": 1,
	"NUMBER_GREATER": 1, "NUMBER_GREATER_THAN_EQ": 1, "NUMBER_LESS": 1,
	"NUMBER_LESS_THAN_EQ": 1, "NUMBER_EQ": 1, "NUMBER_NOT_EQ": 1,
	"NUMBER_BETWEEN": 2, "NUMBER_NOT_BETWEEN": 2,
	"DATE_BEFORE": 1, "DATE_AFTER": 1, "DATE_EQ": 1,
	"BLANK": 0, "NOT_BLANK": 0, "CUSTOM_FORMULA": 1,
}

func (r *registry) sheetsSetConditionalFormat(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	bounds, err := gridBounds(req)
	if err != nil {
		return toolError(err), nil
	}

	rule := google.ConditionalFormat{}
	arguments := req.GetArguments()

	_, hasGradient := arguments["gradient"]
	condition := strings.ToUpper(optionalString(req, "condition"))

	switch {
	case hasGradient && condition != "":
		return toolError(fmt.Errorf("a rule is either a condition or a gradient, not both")), nil

	case hasGradient:
		points, err := objectList(req, "gradient")
		if err != nil {
			return toolError(err), nil
		}
		if len(points) < 2 || len(points) > 3 {
			return toolError(fmt.Errorf("a gradient takes two points or three, got %d", len(points))), nil
		}

		gradient := &google.GradientRule{}
		for index, point := range points {
			kind := strings.ToUpper(stringField(point, "type"))
			switch kind {
			case "MIN", "MAX", "NUMBER", "PERCENT", "PERCENTILE":
			default:
				return toolError(fmt.Errorf("gradient[%d].type is %q: use MIN, MAX, NUMBER, PERCENT "+
					"or PERCENTILE", index, kind)), nil
			}

			colour, err := colorField(point, "color")
			if err != nil {
				return toolError(fmt.Errorf("gradient[%d]: %w", index, err)), nil
			}
			if colour == nil {
				return toolError(fmt.Errorf("gradient[%d] has no colour", index)), nil
			}

			at := &google.InterpolationPoint{Type: kind, Color: colour, Value: stringField(point, "value")}
			if kind != "MIN" && kind != "MAX" && at.Value == "" {
				return toolError(fmt.Errorf("gradient[%d] is a %s point and needs a value", index, kind)), nil
			}

			switch {
			case index == 0:
				gradient.MinPoint = at
			case index == len(points)-1:
				gradient.MaxPoint = at
			default:
				gradient.MidPoint = at
			}
		}
		rule.GradientRule = gradient

	case condition != "":
		wanted, known := conditionTypes[condition]
		if !known {
			return toolError(fmt.Errorf("condition %q is not one this tool writes: use one of the text, "+
				"number or date tests, BLANK, NOT_BLANK or CUSTOM_FORMULA", condition)), nil
		}

		values := req.GetStringSlice("values", nil)
		if len(values) != wanted {
			return toolError(fmt.Errorf("%s takes %d value(s), got %d", condition, wanted, len(values))), nil
		}

		test := &google.BooleanCondition{Type: condition}
		for _, value := range values {
			test.Values = append(test.Values, google.ConditionValue{UserEnteredValue: value})
		}

		format, _, err := conditionalFormatOf(req)
		if err != nil {
			return toolError(err), nil
		}
		if format == nil {
			return toolError(fmt.Errorf("nothing would change on a matching cell: name background_color, " +
				"text_color, bold, italic or strikethrough")), nil
		}

		rule.BooleanRule = &google.BooleanRule{Condition: test, Format: format}

	default:
		return toolError(fmt.Errorf("name what the rule does: a condition with a format, or a gradient")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	rule.Ranges = []google.GridRange{gridRangeOf(sheetID, bounds)}

	var request google.SheetsRequest
	if replace, ok := arguments["replace_index"]; ok {
		_ = replace
		index := req.GetInt("replace_index", 0)
		request = google.SheetsRequest{UpdateConditional: &google.UpdateConditionalFormatRequest{
			Rule: &rule, Index: index, SheetID: google.SheetIDOf(sheetID)}}
	} else {
		add := &google.AddConditionalFormatRequest{Rule: rule}
		if _, ok := arguments["index"]; ok {
			index := req.GetInt("index", 0)
			add.Index = &index
		}
		request = google.SheetsRequest{AddConditional: add}
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{request}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"kind": map[bool]string{true: "gradient", false: "condition"}[rule.GradientRule != nil],
	})
}

// conditionalFormatOf reads the look a matching cell takes.
func conditionalFormatOf(req mcp.CallToolRequest) (*google.CellFormat, []string, error) {
	format := &google.CellFormat{}
	text := &google.SheetsText{}
	var named []string

	for _, style := range []struct {
		argument string
		target   **bool
	}{
		{"bold", &text.Bold},
		{"italic", &text.Italic},
		{"strikethrough", &text.Strikethrough},
	} {
		if _, ok := req.GetArguments()[style.argument]; ok {
			value := req.GetBool(style.argument, false)
			*style.target = &value
			named = append(named, style.argument)
		}
	}

	colour, err := sheetColor(req, "text_color")
	if err != nil {
		return nil, nil, err
	}
	if colour != nil {
		text.ForegroundColor = colour
		named = append(named, "text_color")
	}

	if len(named) > 0 {
		format.TextFormat = text
	}

	background, err := sheetColor(req, "background_color")
	if err != nil {
		return nil, nil, err
	}
	if background != nil {
		format.BackgroundColor = background
		named = append(named, "background_color")
	}

	if len(named) == 0 {
		return nil, nil, nil
	}

	return format, named, nil
}

func (r *registry) sheetsSetBanding(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	bounds, err := gridBounds(req)
	if err != nil {
		return toolError(err), nil
	}

	properties := &google.BandingProperties{}
	for _, field := range []struct {
		argument string
		target   **google.RGBColor
		required bool
	}{
		{"first_band_color", &properties.FirstBandColor, true},
		{"second_band_color", &properties.SecondBandColor, true},
		{"header_color", &properties.HeaderColor, false},
		{"footer_color", &properties.FooterColor, false},
	} {
		colour, err := sheetColor(req, field.argument)
		if err != nil {
			return toolError(err), nil
		}
		if colour == nil && field.required {
			return toolError(fmt.Errorf("%s is required: a banding needs both stripe colours", field.argument)), nil
		}
		*field.target = colour
	}

	direction := strings.ToUpper(req.GetString("direction", "ROWS"))
	if direction != "ROWS" && direction != "COLUMNS" {
		return toolError(fmt.Errorf("direction %q is neither ROWS nor COLUMNS", direction)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	banded := google.BandedRange{Range: gridRangeOf(sheetID, bounds)}
	if direction == "ROWS" {
		banded.RowProperties = properties
	} else {
		banded.ColumnProperties = properties
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		AddBanding: &google.AddBandingRequest{BandedRange: banded},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"direction": direction,
	})
}

func (r *registry) sheetsSetFilter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	bounds, err := gridBounds(req)
	if err != nil {
		return toolError(err), nil
	}

	filter := google.BasicFilter{}

	if _, ok := req.GetArguments()["hide"]; ok {
		entries, err := objectList(req, "hide")
		if err != nil {
			return toolError(err), nil
		}
		for index, entry := range entries {
			column, ok := intField(entry, "column")
			if !ok || column < 0 {
				return toolError(fmt.Errorf("hide[%d].column is missing or negative", index)), nil
			}
			values, err := stringListField(entry, "values")
			if err != nil {
				return toolError(fmt.Errorf("hide[%d]: %w", index, err)), nil
			}
			filter.FilterSpecs = append(filter.FilterSpecs, google.FilterSpec{
				ColumnIndex:    column,
				FilterCriteria: &google.FilterCriteria{HiddenValues: values},
			})
		}
	}

	sorts, err := sortSpecsOf(req, "sort")
	if err != nil {
		return toolError(err), nil
	}
	filter.SortSpecs = sorts

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	span := gridRangeOf(sheetID, bounds)
	filter.Range = &span

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		SetBasicFilter: &google.SetBasicFilterRequest{Filter: filter},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"columns_filtered": len(filter.FilterSpecs), "sorted_by": len(filter.SortSpecs),
	})
}

// sortSpecsOf reads a list of columns to sort by.
func sortSpecsOf(req mcp.CallToolRequest, name string) ([]google.SortSpec, error) {
	if _, ok := req.GetArguments()[name]; !ok {
		return nil, nil
	}

	entries, err := objectList(req, name)
	if err != nil {
		return nil, err
	}

	var specs []google.SortSpec
	for index, entry := range entries {
		column, ok := intField(entry, "column")
		if !ok || column < 0 {
			return nil, fmt.Errorf("%s[%d].column is missing or negative", name, index)
		}

		order := strings.ToUpper(stringField(entry, "order"))
		if order == "" {
			order = "ASCENDING"
		}
		if order != "ASCENDING" && order != "DESCENDING" {
			return nil, fmt.Errorf("%s[%d].order is %q: use ASCENDING or DESCENDING", name, index, order)
		}

		specs = append(specs, google.SortSpec{DimensionIndex: column, SortOrder: order})
	}

	return specs, nil
}

func (r *registry) sheetsProtectRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}

	arguments := req.GetArguments()
	_, named := arguments["start_row"]

	var bounds map[string]int
	if named {
		bounds, err = gridBounds(req)
		if err != nil {
			return toolError(err), nil
		}
	}

	warningOnly := req.GetBool("warning_only", true)
	editors := req.GetStringSlice("editors", nil)
	if !warningOnly && len(editors) == 0 {
		return toolError(fmt.Errorf("warning_only is off and no editors are named: that would leave the " +
			"range editable by whoever owns the file and nobody else")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	protected := google.ProtectedRange{
		Description: optionalString(req, "description"),
		WarningOnly: warningOnly,
	}
	if named {
		span := gridRangeOf(sheetID, bounds)
		protected.Range = &span
	} else {
		protected.Range = &google.GridRange{SheetID: sheetID}
	}
	if !warningOnly {
		protected.Editors = &google.Editors{Users: editors}
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		AddProtected: &google.AddProtectedRangeRequest{ProtectedRange: protected},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"whole_tab": !named, "warning_only": warningOnly,
	})
}

func (r *registry) sheetsAddNamedRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		AddNamedRange: &google.AddNamedRangeRequest{NamedRange: google.NamedRange{
			Name: name, Range: gridRangeOf(sheetID, bounds)}},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "name": name,
	})
}

// dimensionOf reads ROWS or COLUMNS, which four of these tools take.
func dimensionOf(req mcp.CallToolRequest) (string, error) {
	dimension := strings.ToUpper(req.GetString("dimension", "ROWS"))
	if dimension != "ROWS" && dimension != "COLUMNS" {
		return "", fmt.Errorf("dimension %q is neither ROWS nor COLUMNS", dimension)
	}
	return dimension, nil
}

func (r *registry) sheetsGroupDimensions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		return toolError(fmt.Errorf("the run is empty: end is exclusive, so one row at 3 is start 3 end 4")), nil
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
		AddDimensionGroup: &google.AddDimensionGroupRequest{Range: google.DimensionRange{
			SheetID: sheetID, Dimension: dimension, StartIndex: &start, EndIndex: &end}},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"dimension": dimension, "start": start, "end": end,
	})
}

func (r *registry) sheetsInsertDimensions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	count, err := req.RequireInt("count")
	if err != nil {
		return toolError(err), nil
	}
	if count <= 0 {
		return toolError(fmt.Errorf("count is %d: this adds rows and columns, it never takes any away", count)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	var request google.SheetsRequest
	at, inTheMiddle := req.GetArguments()["at"]

	if inTheMiddle {
		_ = at
		index := req.GetInt("at", 0)
		if index < 0 {
			return toolError(fmt.Errorf("at is %d: rows and columns are counted from 0", index)), nil
		}
		end := index + count
		inherit := req.GetBool("inherit_from_before", true)
		request = google.SheetsRequest{InsertDimension: &google.InsertDimensionRequest{
			Range: google.DimensionRange{SheetID: sheetID, Dimension: dimension,
				StartIndex: &index, EndIndex: &end},
			InheritFromBefore: &inherit,
		}}
	} else {
		request = google.SheetsRequest{AppendDimension: &google.AppendDimensionRequest{
			SheetID: sheetID, Dimension: dimension, Length: count}}
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{request}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"dimension": dimension, "added": count, "at_the_end": !inTheMiddle,
	})
}

func (r *registry) sheetsMoveDimensions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	destination, err := req.RequireInt("to")
	if err != nil {
		return toolError(err), nil
	}
	if start < 0 || end <= start || destination < 0 {
		return toolError(fmt.Errorf("the run is empty or lands nowhere: end is exclusive and all three " +
			"are counted from 0")), nil
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
		MoveDimension: &google.MoveDimensionRequest{
			Source: google.DimensionRange{SheetID: sheetID, Dimension: dimension,
				StartIndex: &start, EndIndex: &end},
			DestinationIndex: destination,
		},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"dimension": dimension, "moved": end - start, "to": destination,
	})
}

func (r *registry) sheetsSortRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	bounds, err := gridBounds(req)
	if err != nil {
		return toolError(err), nil
	}

	specs, err := sortSpecsOf(req, "by")
	if err != nil {
		return toolError(err), nil
	}
	if len(specs) == 0 {
		return toolError(fmt.Errorf("by is empty: name at least one column to sort on")), nil
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
		SortRange: &google.SortRangeRequest{Range: gridRangeOf(sheetID, bounds), SortSpecs: specs},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"sorted_by": len(specs),
	})
}

func (r *registry) sheetsDuplicateTab(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	request := &google.DuplicateSheetRequest{
		SourceSheetID: sheetID,
		NewSheetName:  optionalString(req, "new_title"),
	}
	if _, ok := req.GetArguments()["index"]; ok {
		index := req.GetInt("index", 0)
		if index < 0 {
			return toolError(fmt.Errorf("index is %d: tabs are counted from 0", index)), nil
		}
		request.InsertSheetIndex = &index
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID,
		[]google.SheetsRequest{{DuplicateSheet: request}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "copied": sheetTitle, "new_title": request.NewSheetName,
	})
}

func (r *registry) sheetsUpdateProperties(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}

	properties := google.SpreadsheetProperties{}
	var fields []string

	if title := optionalString(req, "title"); title != "" {
		properties.Title = title
		fields = append(fields, "title")
	}
	if locale := optionalString(req, "locale"); locale != "" {
		properties.Locale = locale
		fields = append(fields, "locale")
	}
	if zone := optionalString(req, "time_zone"); zone != "" {
		properties.TimeZone = zone
		fields = append(fields, "timeZone")
	}

	if len(fields) == 0 {
		return toolError(fmt.Errorf("nothing to change: name title, locale or time_zone")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		UpdateSpreadsheet: &google.UpdateSpreadsheetPropsRequest{
			Properties: properties, Fields: strings.Join(fields, ",")},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"spreadsheet_id": spreadsheetID, "changed": fields})
}

func (r *registry) sheetsSetTextRuns(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	sheetTitle, err := requiredString(req, "sheet_title")
	if err != nil {
		return toolError(err), nil
	}
	text, err := requiredString(req, "text")
	if err != nil {
		return toolError(err), nil
	}

	row, err := req.RequireInt("row")
	if err != nil {
		return toolError(err), nil
	}
	column, err := req.RequireInt("column")
	if err != nil {
		return toolError(err), nil
	}
	if row < 0 || column < 0 {
		return toolError(fmt.Errorf("row %d column %d: both are counted from 0", row, column)), nil
	}

	entries, err := objectList(req, "runs")
	if err != nil {
		return toolError(err), nil
	}
	if len(entries) == 0 {
		return toolError(fmt.Errorf("runs is empty: without runs this is an ordinary write")), nil
	}

	length := len([]rune(text))
	var runs []google.TextFormatRun
	previous := -1

	for index, entry := range entries {
		start, ok := intField(entry, "start")
		if !ok || start < 0 {
			return toolError(fmt.Errorf("runs[%d].start is missing or negative", index)), nil
		}
		if start <= previous {
			return toolError(fmt.Errorf("runs[%d] starts at %d, not after the run before it: "+
				"runs go in order and each one holds until the next begins", index, start)), nil
		}
		if start >= length {
			return toolError(fmt.Errorf("runs[%d] starts at %d and the text is %d characters long",
				index, start, length)), nil
		}
		previous = start

		style, err := runStyleOf(entry, index)
		if err != nil {
			return toolError(err), nil
		}
		runs = append(runs, google.TextFormatRun{StartIndex: start, Format: style})
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}
	sheetID, err := r.sheetIDOf(ctx, client, spreadsheetID, sheetTitle)
	if err != nil {
		return toolError(err), nil
	}

	value := text
	if _, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		UpdateCells: &google.UpdateCellsRequest{
			Start: &google.GridCoord{SheetID: sheetID, RowIndex: row, ColumnIndex: column},
			Rows: []google.RowData{{Values: []google.CellValue{{
				UserEnteredValue: &google.ExtendedValue{StringValue: &value},
				TextFormatRuns:   runs,
			}}}},
			Fields: "userEnteredValue,textFormatRuns",
		},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID, "sheet_title": sheetTitle, "sheet_id": sheetID,
		"row": row, "column": column, "runs": len(runs),
	})
}

// runStyleOf reads one run's look out of the object a caller wrote it as.
func runStyleOf(entry map[string]any, index int) (*google.SheetsText, error) {
	style := &google.SheetsText{}

	for _, field := range []struct {
		name   string
		target **bool
	}{
		{"bold", &style.Bold},
		{"italic", &style.Italic},
		{"underline", &style.Underline},
		{"strikethrough", &style.Strikethrough},
	} {
		raw, ok := entry[field.name]
		if !ok {
			continue
		}
		value, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("runs[%d].%s must be true or false, got %T", index, field.name, raw)
		}
		*field.target = &value
	}

	style.FontFamily = stringField(entry, "font_family")
	if size, ok := intField(entry, "font_size"); ok {
		style.FontSize = size
	}

	colour, err := colorField(entry, "text_color")
	if err != nil {
		return nil, fmt.Errorf("runs[%d]: %w", index, err)
	}
	style.ForegroundColor = colour

	if link := stringField(entry, "link"); link != "" {
		style.Link = &google.CellLink{URI: link}
	}

	return style, nil
}
