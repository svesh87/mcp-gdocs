package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

const spreadsheetIDHelp = "Spreadsheet identifier, the part of its address between /d/ and /edit."

const valueInputHelp = "USER_ENTERED reads what is written the way typing it would — formulas become " +
	"formulas, 01.02 becomes a date. RAW stores it as text. Default: USER_ENTERED."

func (r *registry) registerSheets(srv *server.MCPServer) {
	r.registerSheetsLayout(srv)
	r.registerSheetsStructure(srv)
	r.registerSheetsData(srv)
	r.registerSheetsDropdown(srv)
	r.registerSheetsDelete(srv)
	r.registerSheetsMove(srv)
	r.registerSheetsObjects(srv)

	srv.AddTool(mcp.NewTool("gdocs_sheets_info",
		mcp.WithDescription("Describe a spreadsheet: its title, its tabs, their identifiers and sizes. "+
			"Read this before addressing a range — a tab renamed since somebody wrote the range down is "+
			"the usual reason a read comes back empty."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.sheetsInfo)

	srv.AddTool(mcp.NewTool("gdocs_sheets_read",
		mcp.WithDescription("Read cells from a spreadsheet. Name a tab to read all of it, or an A1 range "+
			"like 'Sheet1'!A1:D50 to read part. Empty trailing cells are not padded: a row comes back as "+
			"long as it actually is."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("range", mcp.Description("A1 range, e.g. 'Sheet1'!A1:D50. Without it the whole of sheet_title is read.")),
		mcp.WithString("sheet_title", mcp.Description("Tab to read whole. Ignored when range is given.")),
		mcp.WithString("value_render", mcp.DefaultString("FORMATTED_VALUE"), mcp.Description(
			"FORMATTED_VALUE for what a person sees, UNFORMATTED_VALUE for the underlying numbers, "+
				"FORMULA for the formulas themselves.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.sheetsRead)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_sheets_write",
		mcp.WithDescription("Write a rectangle of cells over whatever is in that range. "+
			"The range and the data have to line up: this replaces cells, it does not insert rows."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("range", mcp.Required(), mcp.Description("A1 range to write into, e.g. 'Sheet1'!A1.")),
		mcp.WithArray("values", mcp.Required(), mcp.Description("Rows as a list of lists of cell values.")),
		mcp.WithString("value_input", mcp.DefaultString("USER_ENTERED"), mcp.Description(valueInputHelp)),
		mcp.WithDestructiveHintAnnotation(true),
	), r.sheetsWrite)

	srv.AddTool(mcp.NewTool("gdocs_sheets_append",
		mcp.WithDescription("Add rows after the last row that has anything in it. "+
			"Rows are inserted rather than written over what sits below, so a table further down the "+
			"tab stays where it is."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("range", mcp.Required(), mcp.Description("Range naming the table to append to, e.g. 'Sheet1'!A:D.")),
		mcp.WithArray("values", mcp.Required(), mcp.Description("Rows as a list of lists of cell values.")),
		mcp.WithString("value_input", mcp.DefaultString("USER_ENTERED"), mcp.Description(valueInputHelp)),
	), r.sheetsAppend)

	srv.AddTool(mcp.NewTool("gdocs_sheets_create",
		mcp.WithDescription("Create a spreadsheet with the named tabs, optionally in a particular folder — "+
			"which is how a new workbook ends up beside the one it copies rather than loose in My Drive. "+
			"The locale and the size of each tab are settable here and only here: a tab arrives 1000 by "+
			"26, and cutting it down afterwards would delete rows."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Name of the new spreadsheet.")),
		mcp.WithArray("sheet_titles", mcp.WithStringItems(), mcp.Description(
			"Tabs to create. Without any, Google makes one called Sheet1.")),
		mcp.WithArray("sheets", mcp.Description(
			"Tabs with their sizes, instead of sheet_titles: "+
				"{\"title\": \"Q1\", \"rows\": 993, \"columns\": 31, \"frozen_rows\": 1}."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":          map[string]any{"type": "string"},
					"rows":           map[string]any{"type": "integer"},
					"columns":        map[string]any{"type": "integer"},
					"frozen_rows":    map[string]any{"type": "integer"},
					"frozen_columns": map[string]any{"type": "integer"},
				},
				"required": []string{"title"},
			})),
		mcp.WithString("locale", mcp.Description(
			"Locale, e.g. ru_RU. It decides how a typed number or date is read, so a copy made under a "+
				"different one stores different values from the same text.")),
		mcp.WithString("time_zone", mcp.Description("Time zone, e.g. Europe/Moscow.")),
		mcp.WithString("parent_folder_id", mcp.Description(
			"Folder to put it in. Without one it lands in My Drive.")),
	), r.sheetsCreate)

	srv.AddTool(mcp.NewTool("gdocs_sheets_add_tab",
		mcp.WithDescription("Add a tab to an existing spreadsheet, with its size if the default 1000 by 26 "+
			"is not what it should be."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("title", mcp.Required(), mcp.Description("Name for the new tab.")),
		mcp.WithNumber("rows", mcp.Description("How many rows it has. Default 1000.")),
		mcp.WithNumber("columns", mcp.Description("How many columns it has. Default 26.")),
		mcp.WithNumber("frozen_rows", mcp.Description("How many rows stay visible while the rest scrolls.")),
		mcp.WithNumber("frozen_columns", mcp.Description("How many columns stay visible while the rest scrolls.")),
	), r.sheetsAddTab)

	srv.AddTool(mcp.NewTool("gdocs_sheets_format_cells",
		mcp.WithDescription("Format a rectangle of cells: font family, size and weight, italic, underline "+
			"and strikethrough, text and background colour, both alignments, wrapping, a number format, "+
			"a link and a note. Rows and columns are counted from 0, and the end of each is exclusive. "+
			"This changes how cells look, never what is in them."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the range is on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
		mcp.WithBoolean("bold", mcp.Description("Bold the text.")),
		mcp.WithBoolean("italic", mcp.Description("Italicise the text.")),
		mcp.WithBoolean("underline", mcp.Description("Underline the text.")),
		mcp.WithBoolean("strikethrough", mcp.Description("Strike the text through.")),
		mcp.WithNumber("font_size", mcp.Description("Font size in points.")),
		mcp.WithString("font_family", mcp.Description(
			"Font family, e.g. Arial or Roboto. A sample that names one on every cell means the tab's "+
				"default is something else, and leaving it out is visible.")),
		mcp.WithString("horizontal_alignment", mcp.Description("LEFT, CENTER or RIGHT.")),
		mcp.WithString("vertical_alignment", mcp.Description(
			"TOP, MIDDLE or BOTTOM. Sheets defaults to BOTTOM, so a tall row whose sample sits MIDDLE "+
				"looks wrong in a way nothing else explains.")),
		mcp.WithString("wrap", mcp.Description(
			"WRAP to grow the row and keep the text inside, OVERFLOW_CELL to let it run over an empty "+
				"neighbour, CLIP to cut it off.")),
		mcp.WithString("background_color", mcp.Description(
			"Cell fill, as #RRGGBB — the way the reading reports one — or {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}.")),
		mcp.WithString("text_color", mcp.Description("Text colour, as #RRGGBB or an object.")),
		mcp.WithNumber("rotation_angle", mcp.Description(
			"Turn the text, -90 to 90 degrees. Positive is upwards. This is how a narrow column of "+
				"headings is made readable, and it changes the row's height.")),
		mcp.WithBoolean("vertical_text", mcp.Description(
			"Stack the letters top to bottom instead of turning them. An alternative to rotation_angle, "+
				"not a companion.")),
		mcp.WithObject("padding", mcp.Description(
			"Room between the cell's edges and its text, in pixels: {\"top\": 2, \"right\": 3, "+
				"\"bottom\": 2, \"left\": 3}. Sheets' own default is exactly that.")),
		mcp.WithString("number_format", mcp.Description(
			"Number pattern, e.g. #,##0.00 or dd.mm.yyyy.")),
		mcp.WithString("number_type", mcp.DefaultString("NUMBER"), mcp.Description(
			"What kind of format the pattern is: NUMBER, PERCENT, CURRENCY, DATE, TIME, DATE_TIME, "+
				"SCIENTIFIC or TEXT. The pattern alone does not say — 0% stored as NUMBER shows the same "+
				"digits as 0% stored as PERCENT, and the interface reports the cell as something else.")),
		mcp.WithString("link", mcp.Description(
			"Address the cells point at. This is a cell's own link, so the text stays text — writing "+
				"=HYPERLINK() instead would replace the value with a formula.")),
		mcp.WithString("link_display", mcp.Description(
			"LINKED to draw a linked cell as a link, PLAIN_TEXT to leave it looking like text. This "+
				"belongs to the cell rather than to the link, and a sample carries it on cells whose "+
				"link has since been taken away.")),
		mcp.WithString("note", mcp.Description(
			"Note to hang on every cell of the range — the small comment that shows on hover.")),
	), r.sheetsFormatCells)

	srv.AddTool(mcp.NewTool("gdocs_sheets_set_validation",
		mcp.WithDescription("Put a dropdown on a rectangle of cells: the list of values a cell will take, "+
			"or a range to take the list from. This is what makes a copied table behave like its sample "+
			"rather than merely look like it — without it a status column is free text."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab the range is on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("Row to stop before.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, counting from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("Column to stop before.")),
		mcp.WithArray("values", mcp.WithStringItems(), mcp.Description(
			"The list to choose from, for ONE_OF_LIST. For ONE_OF_RANGE give one entry, the range as a "+
				"formula: =Лист1!A2:A9.")),
		mcp.WithString("type", mcp.DefaultString("ONE_OF_LIST"), mcp.Description(
			"ONE_OF_LIST for a list of values, ONE_OF_RANGE for a list read out of a range.")),
		mcp.WithBoolean("strict", mcp.Description(
			"Refuse anything not on the list. Without it Sheets warns and keeps what was typed.")),
		mcp.WithBoolean("show_dropdown", mcp.DefaultBool(true), mcp.Description(
			"Show the arrow in the cell. Off means the rule still applies but nothing hints at it.")),
		mcp.WithString("input_message", mcp.Description("Hint shown when the cell is selected.")),
	), r.sheetsSetValidation)
}

func (r *registry) sheetsInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	spreadsheet, err := client.Spreadsheet(ctx, spreadsheetID)
	if err != nil {
		return toolError(err), nil
	}

	type tab struct {
		SheetID       int    `json:"sheet_id"`
		Title         string `json:"title"`
		Index         int    `json:"index"`
		Hidden        bool   `json:"hidden,omitempty"`
		Rows          int    `json:"rows,omitempty"`
		Columns       int    `json:"columns,omitempty"`
		FrozenRows    int    `json:"frozen_rows,omitempty"`
		FrozenColumns int    `json:"frozen_columns,omitempty"`
		TabColor      string `json:"tab_color,omitempty"`
		Merges        int    `json:"merges,omitempty"`
		// What follows is counted rather than described: none of it can be written by this
		// server, and a tab that has it is a tab a copy will differ from. A count is what
		// turns that into something that can be named in a report instead of found later.
		ConditionalFormats int  `json:"conditional_formats,omitempty"`
		BandedRanges       int  `json:"banded_ranges,omitempty"`
		ProtectedRanges    int  `json:"protected_ranges,omitempty"`
		Charts             int  `json:"charts,omitempty"`
		Slicers            int  `json:"slicers,omitempty"`
		Tables             int  `json:"tables,omitempty"`
		FilterViews        int  `json:"filter_views,omitempty"`
		BasicFilter        bool `json:"basic_filter,omitempty"`
		RowGroups          int  `json:"row_groups,omitempty"`
		ColumnGroups       int  `json:"column_groups,omitempty"`
	}

	tabs := make([]tab, 0, len(spreadsheet.Sheets))
	for _, sheet := range spreadsheet.Sheets {
		entry := tab{
			SheetID:            sheet.Properties.ID(),
			Title:              sheet.Properties.Title,
			Hidden:             sheet.Properties.Hidden,
			TabColor:           describeColor(sheet.Properties.TabColor),
			Merges:             len(sheet.Merges),
			ConditionalFormats: len(sheet.ConditionalFormats),
			BandedRanges:       len(sheet.BandedRanges),
			ProtectedRanges:    len(sheet.ProtectedRanges),
			Charts:             len(sheet.Charts),
			Slicers:            len(sheet.Slicers),
			Tables:             len(sheet.Tables),
			FilterViews:        len(sheet.FilterViews),
			BasicFilter:        sheet.BasicFilter != nil,
			RowGroups:          len(sheet.RowGroups),
			ColumnGroups:       len(sheet.ColumnGroups),
		}
		if sheet.Properties.Index != nil {
			entry.Index = *sheet.Properties.Index
		}
		if sheet.Properties.GridProps != nil {
			entry.Rows = sheet.Properties.GridProps.RowCount
			entry.Columns = sheet.Properties.GridProps.ColumnCount
			entry.FrozenRows = sheet.Properties.GridProps.FrozenRowCount
			entry.FrozenColumns = sheet.Properties.GridProps.FrozenColumnCount
		}
		tabs = append(tabs, entry)
	}

	payload := map[string]any{
		"spreadsheet_id": spreadsheet.SpreadsheetID,
		"title":          spreadsheet.Properties.Title,
		"locale":         spreadsheet.Properties.Locale,
		"time_zone":      spreadsheet.Properties.TimeZone,
		"url":            spreadsheet.SpreadsheetURL,
		"sheets":         tabs,
	}

	// Named ranges belong to the workbook rather than to a tab, and a dropdown that reads
	// its list out of one points at the name: a copy without them has empty dropdowns.
	if len(spreadsheet.NamedRanges) > 0 {
		type named struct {
			Name        string `json:"name"`
			SheetID     int    `json:"sheet_id"`
			StartRow    int    `json:"start_row"`
			EndRow      int    `json:"end_row"`
			StartColumn int    `json:"start_column"`
			EndColumn   int    `json:"end_column"`
		}

		ranges := make([]named, 0, len(spreadsheet.NamedRanges))
		for _, one := range spreadsheet.NamedRanges {
			entry := named{Name: one.Name, SheetID: one.Range.SheetID}
			for _, edge := range []struct {
				from *int
				to   *int
			}{
				{one.Range.StartRowIndex, &entry.StartRow},
				{one.Range.EndRowIndex, &entry.EndRow},
				{one.Range.StartColumnIndex, &entry.StartColumn},
				{one.Range.EndColumnIndex, &entry.EndColumn},
			} {
				if edge.from != nil {
					*edge.to = *edge.from
				}
			}
			ranges = append(ranges, entry)
		}
		payload["named_ranges"] = ranges
	}

	return resultJSON(payload)
}

func (r *registry) sheetsRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}

	a1Range := optionalString(req, "range")
	if a1Range == "" {
		sheetTitle := optionalString(req, "sheet_title")
		if sheetTitle == "" {
			return toolError(fmt.Errorf("name what to read: a range, or a sheet_title to read a whole tab")), nil
		}
		a1Range = google.A1Range(sheetTitle, "")
	}

	render := strings.ToUpper(req.GetString("value_render", "FORMATTED_VALUE"))
	switch render {
	case "FORMATTED_VALUE", "UNFORMATTED_VALUE", "FORMULA":
	default:
		return toolError(fmt.Errorf("value_render %q is not one Sheets knows: "+
			"use FORMATTED_VALUE, UNFORMATTED_VALUE or FORMULA", render)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	values, err := client.Values(ctx, spreadsheetID, a1Range, "ROWS", render)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          values.Range,
		"rows":           len(values.Values),
		"values":         values.Values,
	})
}

func (r *registry) sheetsWrite(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	a1Range, err := requiredString(req, "range")
	if err != nil {
		return toolError(err), nil
	}

	values, err := parseValues(req, "values")
	if err != nil {
		return toolError(err), nil
	}

	valueInput, err := valueInputArg(req)
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	updated, err := client.UpdateValues(ctx, spreadsheetID, a1Range, values, valueInput)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"updated_range":  updated.UpdatedRange,
		"updated_rows":   updated.UpdatedRows,
		"updated_cells":  updated.UpdatedCells,
	})
}

func (r *registry) sheetsAppend(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	a1Range, err := requiredString(req, "range")
	if err != nil {
		return toolError(err), nil
	}

	values, err := parseValues(req, "values")
	if err != nil {
		return toolError(err), nil
	}

	valueInput, err := valueInputArg(req)
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	appended, err := client.AppendValues(ctx, spreadsheetID, a1Range, values, valueInput)
	if err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{
		"spreadsheet_id": spreadsheetID,
		"table_range":    appended.TableRange,
	}
	if appended.Updates != nil {
		payload["updated_range"] = appended.Updates.UpdatedRange
		payload["updated_rows"] = appended.Updates.UpdatedRows
		payload["updated_cells"] = appended.Updates.UpdatedCells
	}

	return resultJSON(payload)
}

func (r *registry) sheetsCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := requiredString(req, "title")
	if err != nil {
		return toolError(err), nil
	}

	body := google.CreateSpreadsheetRequest{Properties: google.SpreadsheetProperties{
		Title:    title,
		Locale:   optionalString(req, "locale"),
		TimeZone: optionalString(req, "time_zone"),
	}}

	titles := req.GetStringSlice("sheet_titles", nil)
	_, described := req.GetArguments()["sheets"]

	if described && len(titles) > 0 {
		return toolError(fmt.Errorf("name the tabs once: either sheet_titles or sheets, not both")), nil
	}

	switch {
	case described:
		entries, err := objectList(req, "sheets")
		if err != nil {
			return toolError(err), nil
		}
		for index, entry := range entries {
			name, _ := entry["title"].(string)
			if name == "" {
				return toolError(fmt.Errorf("sheets[%d].title is missing", index)), nil
			}

			properties := google.SheetProperties{Title: name}
			grid := &google.GridProps{}
			for _, field := range []struct {
				name   string
				target *int
			}{
				{"rows", &grid.RowCount},
				{"columns", &grid.ColumnCount},
				{"frozen_rows", &grid.FrozenRowCount},
				{"frozen_columns", &grid.FrozenColumnCount},
			} {
				value, ok := intField(entry, field.name)
				if !ok {
					continue
				}
				if value < 0 {
					return toolError(fmt.Errorf("sheets[%d].%s is %d", index, field.name, value)), nil
				}
				*field.target = value
			}
			if *grid != (google.GridProps{}) {
				properties.GridProps = grid
			}

			body.Sheets = append(body.Sheets, google.Sheet{Properties: properties})
		}
	default:
		for _, name := range titles {
			body.Sheets = append(body.Sheets, google.Sheet{Properties: google.SheetProperties{Title: name}})
		}
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	spreadsheet, err := client.CreateSpreadsheet(ctx, body)
	if err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{
		"spreadsheet_id": spreadsheet.SpreadsheetID,
		"title":          spreadsheet.Properties.Title,
		"url":            spreadsheet.SpreadsheetURL,
		"sheets":         spreadsheet.SheetTitles(),
	}

	// Sheets always creates in My Drive, so a folder is a second step. A failure to move
	// is reported without failing the call: the workbook exists either way, and saying it
	// does not would be worse than saying where it is.
	if folder := optionalString(req, "parent_folder_id"); folder != "" {
		file, err := client.MoveFile(ctx, spreadsheet.SpreadsheetID, folder, "root")
		if err != nil {
			payload["moved"] = false
			payload["move_error"] = err.Error()
		} else {
			payload["moved"] = true
			payload["parents"] = file.Parents
		}
	}

	return resultJSON(payload)
}

func (r *registry) sheetsAddTab(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	title, err := requiredString(req, "title")
	if err != nil {
		return toolError(err), nil
	}

	properties := google.SheetProperties{Title: title}
	grid := &google.GridProps{}
	for _, field := range []struct {
		name   string
		target *int
	}{
		{"rows", &grid.RowCount},
		{"columns", &grid.ColumnCount},
		{"frozen_rows", &grid.FrozenRowCount},
		{"frozen_columns", &grid.FrozenColumnCount},
	} {
		if _, ok := req.GetArguments()[field.name]; !ok {
			continue
		}
		value := req.GetInt(field.name, 0)
		if value < 0 {
			return toolError(fmt.Errorf("%s is %d", field.name, value)), nil
		}
		*field.target = value
	}
	if *grid != (google.GridProps{}) {
		properties.GridProps = grid
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		AddSheet: &google.AddSheetRequest{Properties: properties},
	}})
	if err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{"spreadsheet_id": spreadsheetID, "title": title}
	for _, reply := range response.Replies {
		if reply.AddSheet != nil {
			payload["sheet_id"] = reply.AddSheet.Properties.ID()
			payload["index"] = reply.AddSheet.Properties.Index
		}
	}

	return resultJSON(payload)
}

func (r *registry) sheetsFormatCells(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	spreadsheet, err := client.Spreadsheet(ctx, spreadsheetID)
	if err != nil {
		return toolError(err), nil
	}

	sheetID, ok := spreadsheet.SheetIDByTitle(sheetTitle)
	if !ok {
		return toolError(fmt.Errorf("no tab called %q in this spreadsheet: it has %s",
			sheetTitle, strings.Join(spreadsheet.SheetTitles(), ", "))), nil
	}

	format, fields, err := parseCellFormat(req)
	if err != nil {
		return toolError(err), nil
	}

	cell := google.CellData{UserEnteredFormat: format}
	// The note is not part of the format: it sits beside it on the cell, and it needs its
	// own name in the mask or it is silently dropped.
	if note := optionalString(req, "note"); note != "" {
		cell.Note = note
		fields = append(fields, "note")
	}

	if len(fields) == 0 {
		return toolError(fmt.Errorf("nothing to change: name at least one of bold, italic, underline, " +
			"strikethrough, font_size, font_family, horizontal_alignment, vertical_alignment, wrap, " +
			"background_color, text_color, number_format, link or note")), nil
	}

	startRow, endRow := bounds["start_row"], bounds["end_row"]
	startColumn, endColumn := bounds["start_column"], bounds["end_column"]

	response, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		RepeatCell: &google.RepeatCellRequest{
			Range: google.GridRange{
				SheetID:          sheetID,
				StartRowIndex:    &startRow,
				EndRowIndex:      &endRow,
				StartColumnIndex: &startColumn,
				EndColumnIndex:   &endColumn,
			},
			Cell:   cell,
			Fields: strings.Join(fields, ","),
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet_title":    sheetTitle,
		"sheet_id":       sheetID,
		"fields":         strings.Join(fields, ","),
		"replies":        len(response.Replies),
	})
}

// sheetColor reads a colour written either the way this server reports one — "#EFEFEF" —
// or the way the API takes it. A reading that answers in hex and a writing that refuses hex
// is a trap the caller falls into once per session.
func sheetColor(req mcp.CallToolRequest, name string) (*google.RGBColor, error) {
	raw, ok := req.GetArguments()[name]
	if !ok || raw == nil {
		return nil, nil
	}

	if text, ok := raw.(string); ok {
		return hexColor(text, name)
	}

	return parseColor(req, name)
}

// colorField is sheetColor for a colour inside a list of objects.
func colorField(object map[string]any, name string) (*google.RGBColor, error) {
	raw, ok := object[name]
	if !ok || raw == nil {
		return nil, nil
	}

	if text, ok := raw.(string); ok {
		return hexColor(text, name)
	}

	nested, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be #RRGGBB or an object with red, green and blue, got %T", name, raw)
	}

	colour := &google.RGBColor{}
	for _, component := range []struct {
		name   string
		target *float64
	}{
		{"red", &colour.Red},
		{"green", &colour.Green},
		{"blue", &colour.Blue},
	} {
		value, present := nested[component.name]
		if !present {
			continue
		}
		number, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("%s.%s must be a number between 0 and 1, got %T",
				name, component.name, value)
		}
		if number < 0 || number > 1 {
			return nil, fmt.Errorf("%s.%s is %v: colours run from 0 to 1", name, component.name, number)
		}
		*component.target = number
	}

	return colour, nil
}

// hexColor turns "#EFEFEF" into the three numbers the API takes.
func hexColor(text, name string) (*google.RGBColor, error) {
	digits := strings.TrimPrefix(strings.TrimSpace(text), "#")
	if len(digits) != 6 {
		return nil, fmt.Errorf("%s is %q: a colour is #RRGGBB, or an object with red, green and blue",
			name, text)
	}

	var channels [3]float64
	for index := 0; index < 3; index++ {
		var value int
		if _, err := fmt.Sscanf(digits[index*2:index*2+2], "%02x", &value); err != nil {
			return nil, fmt.Errorf("%s is %q: %s is not a pair of hex digits", name, text,
				digits[index*2:index*2+2])
		}
		channels[index] = float64(value) / 255
	}

	return &google.RGBColor{Red: channels[0], Green: channels[1], Blue: channels[2]}, nil
}

// intList reads a list of whole numbers written as an argument.
func intList(req mcp.CallToolRequest, name string) ([]int, error) {
	raw, ok := req.GetArguments()[name]
	if !ok {
		return nil, nil
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list of numbers, got %T", name, raw)
	}

	numbers := make([]int, 0, len(list))
	for index, item := range list {
		number, ok := item.(float64)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a number, got %T", name, index, item)
		}
		if number < 0 {
			return nil, fmt.Errorf("%s[%d] is %v: rows and columns are counted from 0", name, index, number)
		}
		numbers = append(numbers, int(number))
	}

	return numbers, nil
}

// gridBounds reads the four numbers that name a rectangle, and says what is wrong with
// them in the terms the caller wrote them in.
func gridBounds(req mcp.CallToolRequest) (map[string]int, error) {
	bounds := map[string]int{}
	for _, name := range []string{"start_row", "end_row", "start_column", "end_column"} {
		value, err := req.RequireInt(name)
		if err != nil {
			return nil, err
		}
		if value < 0 {
			return nil, fmt.Errorf("%s is %d: rows and columns are counted from 0", name, value)
		}
		bounds[name] = value
	}

	if bounds["end_row"] <= bounds["start_row"] || bounds["end_column"] <= bounds["start_column"] {
		return nil, fmt.Errorf("the range is empty: end_row and end_column are exclusive, " +
			"so a single cell at row 0 column 0 is start 0/0 and end 1/1")
	}

	return bounds, nil
}

// sheetsSetValidation puts a dropdown on a rectangle.
func (r *registry) sheetsSetValidation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	kind := strings.ToUpper(req.GetString("type", "ONE_OF_LIST"))
	switch kind {
	case "ONE_OF_LIST", "ONE_OF_RANGE":
	default:
		return toolError(fmt.Errorf("type %q is not one this tool sets: use ONE_OF_LIST or ONE_OF_RANGE", kind)), nil
	}

	values := req.GetStringSlice("values", nil)
	if len(values) == 0 {
		return toolError(fmt.Errorf("values is empty: a ONE_OF_LIST rule needs the list, " +
			"a ONE_OF_RANGE rule the range as a single formula like =Sheet1!A2:A9")), nil
	}
	if kind == "ONE_OF_RANGE" && len(values) != 1 {
		return toolError(fmt.Errorf("a ONE_OF_RANGE rule takes exactly one value, the range as a "+
			"formula; got %d", len(values))), nil
	}

	condition := &google.BooleanCondition{Type: kind}
	for _, value := range values {
		condition.Values = append(condition.Values, google.ConditionValue{UserEnteredValue: value})
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	spreadsheet, err := client.Spreadsheet(ctx, spreadsheetID)
	if err != nil {
		return toolError(err), nil
	}

	sheetID, ok := spreadsheet.SheetIDByTitle(sheetTitle)
	if !ok {
		return toolError(fmt.Errorf("no tab called %q in this spreadsheet: it has %s",
			sheetTitle, strings.Join(spreadsheet.SheetTitles(), ", "))), nil
	}

	startRow, endRow := bounds["start_row"], bounds["end_row"]
	startColumn, endColumn := bounds["start_column"], bounds["end_column"]

	rule := &google.DataValidationRule{
		Condition:    condition,
		Strict:       req.GetBool("strict", false),
		ShowCustomUI: req.GetBool("show_dropdown", true),
		InputMessage: optionalString(req, "input_message"),
	}

	response, err := client.SheetsBatchUpdate(ctx, spreadsheetID, []google.SheetsRequest{{
		SetValidation: &google.SetDataValidationRequest{
			Range: google.GridRange{
				SheetID:          sheetID,
				StartRowIndex:    &startRow,
				EndRowIndex:      &endRow,
				StartColumnIndex: &startColumn,
				EndColumnIndex:   &endColumn,
			},
			Rule: rule,
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet_title":    sheetTitle,
		"sheet_id":       sheetID,
		"type":           kind,
		"values":         len(values),
		"replies":        len(response.Replies),
	})
}

// parseCellFormat reads the formatting arguments and the mask that goes with them.
func parseCellFormat(req mcp.CallToolRequest) (*google.CellFormat, []string, error) {
	format := &google.CellFormat{}
	var fields []string

	text := &google.SheetsText{}
	textFields := []string{}

	// Each of these is read as "was it named", not "is it true": a sample that sets bold
	// to false is asking for plain text, and skipping the field would leave whatever the
	// range had before.
	for _, style := range []struct {
		argument, field string
		target          **bool
	}{
		{"bold", "bold", &text.Bold},
		{"italic", "italic", &text.Italic},
		{"underline", "underline", &text.Underline},
		{"strikethrough", "strikethrough", &text.Strikethrough},
	} {
		if _, ok := req.GetArguments()[style.argument]; ok {
			value := req.GetBool(style.argument, false)
			*style.target = &value
			textFields = append(textFields, style.field)
		}
	}

	if size := req.GetInt("font_size", 0); size > 0 {
		text.FontSize = size
		textFields = append(textFields, "fontSize")
	}

	if family := optionalString(req, "font_family"); family != "" {
		text.FontFamily = family
		textFields = append(textFields, "fontFamily")
	}

	if link := optionalString(req, "link"); link != "" {
		text.Link = &google.CellLink{URI: link}
		textFields = append(textFields, "link")
	}

	colour, err := sheetColor(req, "text_color")
	if err != nil {
		return nil, nil, err
	}
	if colour != nil {
		text.ForegroundColor = colour
		textFields = append(textFields, "foregroundColor")
	}

	if len(textFields) > 0 {
		format.TextFormat = text
		fields = append(fields, "userEnteredFormat.textFormat("+strings.Join(textFields, ",")+")")
	}

	if alignment := strings.ToUpper(optionalString(req, "horizontal_alignment")); alignment != "" {
		switch alignment {
		case "LEFT", "CENTER", "RIGHT":
		default:
			return nil, nil, fmt.Errorf("horizontal_alignment %q is not one Sheets knows: use LEFT, CENTER or RIGHT", alignment)
		}
		format.HorizontalAlignment = alignment
		fields = append(fields, "userEnteredFormat.horizontalAlignment")
	}

	if alignment := strings.ToUpper(optionalString(req, "vertical_alignment")); alignment != "" {
		switch alignment {
		case "TOP", "MIDDLE", "BOTTOM":
		default:
			return nil, nil, fmt.Errorf("vertical_alignment %q is not one Sheets knows: use TOP, MIDDLE or BOTTOM", alignment)
		}
		format.VerticalAlignment = alignment
		fields = append(fields, "userEnteredFormat.verticalAlignment")
	}

	if wrap := strings.ToUpper(optionalString(req, "wrap")); wrap != "" {
		switch wrap {
		case "WRAP", "OVERFLOW_CELL", "CLIP":
		default:
			return nil, nil, fmt.Errorf("wrap %q is not one Sheets knows: use WRAP, OVERFLOW_CELL or CLIP", wrap)
		}
		format.WrapStrategy = wrap
		fields = append(fields, "userEnteredFormat.wrapStrategy")
	}

	if display := strings.ToUpper(optionalString(req, "link_display")); display != "" {
		switch display {
		case "LINKED", "PLAIN_TEXT":
		default:
			return nil, nil, fmt.Errorf("link_display %q is not one Sheets knows: use LINKED or PLAIN_TEXT", display)
		}
		format.HyperlinkDisplayType = display
		fields = append(fields, "userEnteredFormat.hyperlinkDisplayType")
	}

	background, err := sheetColor(req, "background_color")
	if err != nil {
		return nil, nil, err
	}
	if background != nil {
		format.BackgroundColor = background
		fields = append(fields, "userEnteredFormat.backgroundColor")
	}

	arguments := req.GetArguments()
	_, hasAngle := arguments["rotation_angle"]
	_, hasVertical := arguments["vertical_text"]

	if hasAngle && hasVertical {
		return nil, nil, fmt.Errorf("rotation_angle and vertical_text are alternatives: Sheets stores " +
			"one turn per cell, either an angle or letters stacked downwards")
	}
	if hasAngle {
		angle := req.GetInt("rotation_angle", 0)
		if angle < -90 || angle > 90 {
			return nil, nil, fmt.Errorf("rotation_angle is %d: Sheets turns text between -90 and 90 degrees", angle)
		}
		format.TextRotation = &google.TextRotation{Angle: &angle}
		fields = append(fields, "userEnteredFormat.textRotation")
	}
	if hasVertical {
		vertical := req.GetBool("vertical_text", false)
		format.TextRotation = &google.TextRotation{Vertical: &vertical}
		fields = append(fields, "userEnteredFormat.textRotation")
	}

	if raw, ok := arguments["padding"]; ok {
		object, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("padding must be an object with top, right, bottom and left, got %T", raw)
		}
		padding := &google.Padding{}
		for _, side := range []struct {
			name   string
			target *int
		}{
			{"top", &padding.Top},
			{"right", &padding.Right},
			{"bottom", &padding.Bottom},
			{"left", &padding.Left},
		} {
			value, ok := intField(object, side.name)
			if !ok {
				continue
			}
			if value < 0 {
				return nil, nil, fmt.Errorf("padding.%s is %d: padding is measured in pixels", side.name, value)
			}
			*side.target = value
		}
		format.Padding = padding
		fields = append(fields, "userEnteredFormat.padding")
	}

	if pattern := optionalString(req, "number_format"); pattern != "" {
		kind := strings.ToUpper(req.GetString("number_type", "NUMBER"))
		switch kind {
		case "NUMBER", "PERCENT", "CURRENCY", "DATE", "TIME", "DATE_TIME", "SCIENTIFIC", "TEXT":
		default:
			return nil, nil, fmt.Errorf("number_type %q is not one Sheets knows: use NUMBER, PERCENT, "+
				"CURRENCY, DATE, TIME, DATE_TIME, SCIENTIFIC or TEXT", kind)
		}
		format.NumberFormat = &google.NumberFormat{Type: kind, Pattern: pattern}
		fields = append(fields, "userEnteredFormat.numberFormat")
	}

	return format, fields, nil
}

// parseValues reads a rectangle of cell values. Rows may differ in length: Sheets
// accepts a short row as "the rest of it is empty", and forcing them equal here would
// reject something the API is fine with.
func parseValues(req mcp.CallToolRequest, name string) ([][]any, error) {
	raw, ok := req.GetArguments()[name]
	if !ok {
		return nil, fmt.Errorf("%s is required: rows as a list of lists", name)
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list of lists, got %T", name, raw)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}

	values := make([][]any, 0, len(list))
	for index, rawRow := range list {
		row, ok := rawRow.([]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a list of cells, got %T", name, index, rawRow)
		}
		values = append(values, row)
	}

	return values, nil
}

func valueInputArg(req mcp.CallToolRequest) (string, error) {
	option := strings.ToUpper(req.GetString("value_input", "USER_ENTERED"))
	switch option {
	case "USER_ENTERED", "RAW":
		return option, nil
	default:
		return "", fmt.Errorf("value_input %q is not one Sheets knows: use USER_ENTERED or RAW", option)
	}
}
