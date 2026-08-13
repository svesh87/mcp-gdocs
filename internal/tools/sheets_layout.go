package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerSheetsLayout adds the tools that read a workbook's look and reproduce it: the
// half of "make one like this" that has nothing to do with the values.
func (r *registry) registerSheetsLayout(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_sheets_read_format",
		mcp.WithDescription("Read how a range looks, not just what is in it: per-cell font, size, weight, "+
			"colours, both alignments, wrapping, number format, links and notes, plus the column widths, "+
			"the row heights, the merges and the dropdowns of the tab. Rows and columns are reported as "+
			"the sheet's own indexes, so a range that starts partway down still says where it is. "+
			"This is what to read off a sample workbook before building one like it."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("range", mcp.Required(), mcp.Description(
			"A1 range to read, e.g. 'Лист1'!A1:F10. Keep it small: this returns a description per cell.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.sheetsReadFormat)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_sheets_set_layout",
		mcp.WithDescription("Set the shape of a tab rather than its contents: column widths and row heights "+
			"in pixels, how many rows stay frozen at the top, merged cells, and how big the grid is. "+
			"Widths and heights are what make a copied table actually look like its sample — the same "+
			"values in default-sized rows look nothing alike. The grid can only grow here: shrinking it "+
			"deletes rows, which this server does not do — ask for the size when the tab is created."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Required(), mcp.Description("Tab to shape.")),
		mcp.WithArray("column_widths", mcp.Description(
			"Column widths in pixels, as a list of objects: {\"column\": 0, \"pixels\": 220}. "+
				"Columns count from 0."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"column": map[string]any{"type": "integer"},
					"pixels": map[string]any{"type": "integer"},
				},
				"required": []string{"column", "pixels"},
			})),
		mcp.WithArray("row_heights", mcp.Description(
			"Row heights in pixels, as a list of objects: {\"row\": 0, \"pixels\": 68}, or a run at once: "+
				"{\"row\": 1, \"through_row\": 19, \"pixels\": 50}. through_row is inclusive; rows count from 0."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"row":         map[string]any{"type": "integer"},
					"through_row": map[string]any{"type": "integer"},
					"pixels":      map[string]any{"type": "integer"},
				},
				"required": []string{"row", "pixels"},
			})),
		mcp.WithNumber("rows", mcp.Description(
			"How many rows the grid has. Only more than it has now: fewer would delete rows.")),
		mcp.WithNumber("columns", mcp.Description(
			"How many columns the grid has. Only more than it has now.")),
		mcp.WithNumber("frozen_rows", mcp.Description("How many rows stay visible while the rest scrolls.")),
		mcp.WithNumber("frozen_columns", mcp.Description("How many columns stay visible while the rest scrolls.")),
		mcp.WithString("tab_color", mcp.Description("Colour of the tab's own label, as #RRGGBB or an object.")),
		mcp.WithArray("hide_rows", mcp.Description(
			"Rows to hide or show: {\"row\": 5, \"through_row\": 9, \"hidden\": true}. Hiding is not "+
				"deleting — the rows stay and come back with hidden false."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"row":         map[string]any{"type": "integer"},
					"through_row": map[string]any{"type": "integer"},
					"hidden":      map[string]any{"type": "boolean"},
				},
				"required": []string{"row"},
			})),
		mcp.WithArray("hide_columns", mcp.Description(
			"Columns to hide or show: {\"column\": 12, \"through_column\": 30, \"hidden\": true}."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"column":         map[string]any{"type": "integer"},
					"through_column": map[string]any{"type": "integer"},
					"hidden":         map[string]any{"type": "boolean"},
				},
				"required": []string{"column"},
			})),
		mcp.WithArray("auto_resize_columns", mcp.Description(
			"Columns to fit to their contents, as [first, last] pairs or single indexes. This is the "+
				"opposite of copying a sample's widths — use one or the other."),
			mcp.Items(map[string]any{"type": "integer"})),
		mcp.WithArray("unmerge", mcp.Description(
			"Rectangles to take apart, in the same four numbers as merge. The cells come back; nothing "+
				"in them is removed."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"start_row":    map[string]any{"type": "integer"},
					"end_row":      map[string]any{"type": "integer"},
					"start_column": map[string]any{"type": "integer"},
					"end_column":   map[string]any{"type": "integer"},
				},
				"required": []string{"start_row", "end_row", "start_column", "end_column"},
			})),
		mcp.WithArray("merge", mcp.Description(
			"Rectangles to merge, as a list of objects: "+
				"{\"start_row\": 0, \"end_row\": 1, \"start_column\": 0, \"end_column\": 3}. "+
				"The end of each is exclusive."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"start_row":    map[string]any{"type": "integer"},
					"end_row":      map[string]any{"type": "integer"},
					"start_column": map[string]any{"type": "integer"},
					"end_column":   map[string]any{"type": "integer"},
				},
				"required": []string{"start_row", "end_row", "start_column", "end_column"},
			})),
		mcp.WithIdempotentHintAnnotation(true),
	), r.sheetsSetLayout)
}

// sheetsReadFormat describes a range the way it would have to be rebuilt.
func (r *registry) sheetsReadFormat(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spreadsheetID, err := requiredString(req, "spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	a1Range, err := requiredString(req, "range")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	spreadsheet, err := client.SpreadsheetGrid(ctx, spreadsheetID, a1Range)
	if err != nil {
		return toolError(err), nil
	}

	if len(spreadsheet.Sheets) == 0 || len(spreadsheet.Sheets[0].Data) == 0 {
		return toolError(fmt.Errorf("nothing came back for %s: check the tab name and the range", a1Range)), nil
	}

	sheet := spreadsheet.Sheets[0]
	data := sheet.Data[0]

	var cells []describedCell

	for rowIndex, row := range data.RowData {
		for columnIndex, value := range row.Values {
			// The indexes are the sheet's own, not the rectangle's. A reading of C5:F20
			// that reported its first cell as row 0 would be written back over C1.
			at := describedCore{Row: data.StartRow + rowIndex, Column: data.StartColumn + columnIndex}
			described := at
			described.Value = value.FormattedValue
			described.Note = value.Note
			described.Link = value.Hyperlink

			if format := value.UserEnteredFormat; format != nil {
				described.Alignment = format.HorizontalAlignment
				described.VerticalAlignment = format.VerticalAlignment
				described.Wrap = format.WrapStrategy
				described.LinkDisplay = format.HyperlinkDisplayType
				described.Background = describeColor(format.BackgroundColor)

				if text := format.TextFormat; text != nil {
					described.FontFamily = text.FontFamily
					described.FontSize = text.FontSize
					described.Bold = text.Bold != nil && *text.Bold
					described.Italic = text.Italic != nil && *text.Italic
					described.Underline = text.Underline != nil && *text.Underline
					described.Strikethrough = text.Strikethrough != nil && *text.Strikethrough
					described.TextColor = describeColor(text.ForegroundColor)
					if text.Link != nil && text.Link.URI != "" {
						described.Link = text.Link.URI
					}
				}
				if number := format.NumberFormat; number != nil {
					described.NumberType = number.Type
					described.Pattern = number.Pattern
				}
				if rotation := format.TextRotation; rotation != nil {
					described.RotationAngle = rotation.Angle
					described.VerticalText = rotation.Vertical != nil && *rotation.Vertical
				}
				if padding := format.Padding; padding != nil {
					described.Padding = &describedPadding{Top: padding.Top, Right: padding.Right,
						Bottom: padding.Bottom, Left: padding.Left}
				}
				described.Borders = describeBorders(format.Borders)
			}

			runs := describeRuns(value.TextFormatRuns)

			// A cell with nothing in it and nothing on it is noise in the answer.
			if described != at || len(runs) > 0 {
				cells = append(cells, describedCell{describedCore: described, Runs: runs})
			}
		}
	}

	widths := make([]int, 0, len(data.ColumnMetadata))
	hiddenColumns := []int{}
	for index, column := range data.ColumnMetadata {
		widths = append(widths, column.PixelSize)
		if column.HiddenByUser != nil && *column.HiddenByUser {
			hiddenColumns = append(hiddenColumns, data.StartColumn+index)
		}
	}

	heights := make([]int, 0, len(data.RowMetadata))
	hiddenRows := []int{}
	for index, row := range data.RowMetadata {
		heights = append(heights, row.PixelSize)
		if row.HiddenByUser != nil && *row.HiddenByUser {
			hiddenRows = append(hiddenRows, data.StartRow+index)
		}
	}

	payload := map[string]any{
		"spreadsheet_id":       spreadsheetID,
		"range":                a1Range,
		"sheet_title":          sheet.Properties.Title,
		"sheet_id":             sheet.Properties.ID(),
		"first_row":            data.StartRow,
		"first_column":         data.StartColumn,
		"column_widths_pixels": widths,
		"row_heights_pixels":   heights,
		"cells":                cells,
	}

	if validations := describeValidations(data); len(validations) > 0 {
		payload["validations"] = validations
	}
	if len(hiddenColumns) > 0 {
		payload["hidden_columns"] = hiddenColumns
	}
	if len(hiddenRows) > 0 {
		payload["hidden_rows"] = hiddenRows
	}
	if merges := describeMerges(sheet.Merges); len(merges) > 0 {
		payload["merges"] = merges
	}
	if rules := describeConditionalFormats(sheet.ConditionalFormats); len(rules) > 0 {
		payload["conditional_formats"] = rules
	}
	if bandings := describeBandings(sheet.BandedRanges); len(bandings) > 0 {
		payload["bandings"] = bandings
	}
	if protections := describeProtections(sheet.ProtectedRanges); len(protections) > 0 {
		payload["protected_ranges"] = protections
	}
	if filter := sheet.BasicFilter; filter != nil {
		payload["basic_filter"] = describeFilter(filter.Range, "", filter.SortSpecs, filter.FilterSpecs)
	}
	if len(sheet.FilterViews) > 0 {
		views := make([]describedFilter, 0, len(sheet.FilterViews))
		for _, view := range sheet.FilterViews {
			views = append(views, describeFilter(view.Range, view.Title, view.SortSpecs, view.FilterSpecs))
		}
		payload["filter_views"] = views
	}
	if groups := describeGroups(sheet.RowGroups); len(groups) > 0 {
		payload["row_groups"] = groups
	}
	if groups := describeGroups(sheet.ColumnGroups); len(groups) > 0 {
		payload["column_groups"] = groups
	}

	if sheet.Properties.GridProps != nil {
		payload["frozen_rows"] = sheet.Properties.GridProps.FrozenRowCount
		payload["frozen_columns"] = sheet.Properties.GridProps.FrozenColumnCount
		payload["rows"] = sheet.Properties.GridProps.RowCount
		payload["columns"] = sheet.Properties.GridProps.ColumnCount
	}

	return resultJSON(payload)
}

// describedCell is one cell the way it would have to be rebuilt: its comparable half,
// which is what says whether the cell is worth reporting at all, plus the runs, which are
// a list and cannot be compared.
type describedCell struct {
	describedCore
	Runs []describedRun `json:"runs,omitempty"`
}

// describedRun is a stretch of a cell's text that looks different from the rest.
type describedRun struct {
	Start         int    `json:"start"`
	Bold          bool   `json:"bold,omitempty"`
	Italic        bool   `json:"italic,omitempty"`
	Underline     bool   `json:"underline,omitempty"`
	Strikethrough bool   `json:"strikethrough,omitempty"`
	FontFamily    string `json:"font_family,omitempty"`
	FontSize      int    `json:"font_size,omitempty"`
	TextColor     string `json:"text_color,omitempty"`
	Link          string `json:"link,omitempty"`
}

// describedBorder is one edge of a cell.
type describedBorder struct {
	Style string `json:"style,omitempty"`
	Color string `json:"color,omitempty"`
	Width int    `json:"width,omitempty"`
}

// describedBorders is the four edges as they were read.
type describedBorders struct {
	Top    *describedBorder `json:"top,omitempty"`
	Bottom *describedBorder `json:"bottom,omitempty"`
	Left   *describedBorder `json:"left,omitempty"`
	Right  *describedBorder `json:"right,omitempty"`
}

// describedPadding is the room between a cell's edges and its text, in pixels.
type describedPadding struct {
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
	Left   int `json:"left"`
}

type describedCore struct {
	Row               int    `json:"row"`
	Column            int    `json:"column"`
	Value             string `json:"value,omitempty"`
	Bold              bool   `json:"bold,omitempty"`
	Italic            bool   `json:"italic,omitempty"`
	Underline         bool   `json:"underline,omitempty"`
	Strikethrough     bool   `json:"strikethrough,omitempty"`
	FontSize          int    `json:"font_size,omitempty"`
	FontFamily        string `json:"font_family,omitempty"`
	Alignment         string `json:"alignment,omitempty"`
	VerticalAlignment string `json:"vertical_alignment,omitempty"`
	Wrap              string `json:"wrap,omitempty"`
	Background        string `json:"background,omitempty"`
	TextColor         string `json:"text_color,omitempty"`
	NumberType        string `json:"number_type,omitempty"`
	Pattern           string `json:"number_pattern,omitempty"`
	Link              string `json:"link,omitempty"`
	LinkDisplay       string `json:"link_display,omitempty"`
	Note              string `json:"note,omitempty"`

	RotationAngle *int              `json:"rotation_angle,omitempty"`
	VerticalText  bool              `json:"vertical_text,omitempty"`
	Padding       *describedPadding `json:"padding,omitempty"`
	Borders       *describedBorders `json:"borders,omitempty"`
}

// describedRange is a rectangle in the numbers the writing tools take.
type describedRange struct {
	StartRow    int `json:"start_row"`
	EndRow      int `json:"end_row"`
	StartColumn int `json:"start_column"`
	EndColumn   int `json:"end_column"`
}

// describedValidation is one rule over one rectangle, shaped so it can be handed straight
// to gdocs_sheets_set_validation.
type describedValidation struct {
	describedRange
	Type         string   `json:"type"`
	Values       []string `json:"values,omitempty"`
	Strict       bool     `json:"strict,omitempty"`
	ShowDropdown bool     `json:"show_dropdown,omitempty"`
	InputMessage string   `json:"input_message,omitempty"`
}

// describeValidations gathers the per-cell rules back into the rectangles they were set
// as. Reporting them cell by cell would be two hundred copies of the same dropdown, and
// nothing that could be written back in one call.
func describeValidations(data google.GridData) []describedValidation {
	type block struct {
		rule                   *google.DataValidationRule
		startRow, endRow       int
		startColumn, endColumn int
	}

	key := func(rule *google.DataValidationRule) string {
		encoded, err := json.Marshal(rule)
		if err != nil {
			return ""
		}
		return string(encoded)
	}

	// First down each column: a rule is nearly always set on a run of rows.
	runs := map[string][]block{}
	var order []string

	columns := 0
	for _, row := range data.RowData {
		if len(row.Values) > columns {
			columns = len(row.Values)
		}
	}

	for column := 0; column < columns; column++ {
		var current *block
		var currentKey string

		flush := func() {
			if current != nil {
				if _, seen := runs[currentKey]; !seen {
					order = append(order, currentKey)
				}
				runs[currentKey] = append(runs[currentKey], *current)
				current = nil
			}
		}

		for rowIndex, row := range data.RowData {
			var rule *google.DataValidationRule
			if column < len(row.Values) {
				rule = row.Values[column].DataValidation
			}
			if rule == nil {
				flush()
				continue
			}

			identity := key(rule)
			if current != nil && identity == currentKey && current.endRow == data.StartRow+rowIndex {
				current.endRow++
				continue
			}
			flush()
			currentKey = identity
			current = &block{
				rule:        rule,
				startRow:    data.StartRow + rowIndex,
				endRow:      data.StartRow + rowIndex + 1,
				startColumn: data.StartColumn + column,
				endColumn:   data.StartColumn + column + 1,
			}
		}
		flush()
	}

	var described []describedValidation
	for _, identity := range order {
		blocks := runs[identity]
		// Then sideways: neighbouring columns carrying the same rule over the same rows
		// are one rectangle, which is how a person set them.
		merged := make([]block, 0, len(blocks))
		for _, candidate := range blocks {
			if last := len(merged) - 1; last >= 0 &&
				merged[last].endColumn == candidate.startColumn &&
				merged[last].startRow == candidate.startRow &&
				merged[last].endRow == candidate.endRow {
				merged[last].endColumn = candidate.endColumn
				continue
			}
			merged = append(merged, candidate)
		}

		for _, one := range merged {
			entry := describedValidation{
				describedRange: describedRange{
					StartRow:    one.startRow,
					EndRow:      one.endRow,
					StartColumn: one.startColumn,
					EndColumn:   one.endColumn,
				},
				Strict:       one.rule.Strict,
				ShowDropdown: one.rule.ShowCustomUI,
				InputMessage: one.rule.InputMessage,
			}
			if condition := one.rule.Condition; condition != nil {
				entry.Type = condition.Type
				for _, value := range condition.Values {
					entry.Values = append(entry.Values, value.UserEnteredValue)
				}
			}
			described = append(described, entry)
		}
	}

	return described
}

// describeRuns reports the stretches of a cell whose text changes style partway through.
// Each run holds until the next one starts, so the offsets are all that is needed.
func describeRuns(runs []google.TextFormatRun) []describedRun {
	var described []describedRun
	for _, run := range runs {
		entry := describedRun{Start: run.StartIndex}
		if style := run.Format; style != nil {
			entry.Bold = style.Bold != nil && *style.Bold
			entry.Italic = style.Italic != nil && *style.Italic
			entry.Underline = style.Underline != nil && *style.Underline
			entry.Strikethrough = style.Strikethrough != nil && *style.Strikethrough
			entry.FontFamily = style.FontFamily
			entry.FontSize = style.FontSize
			entry.TextColor = describeColor(style.ForegroundColor)
			if style.Link != nil {
				entry.Link = style.Link.URI
			}
		}
		described = append(described, entry)
	}
	return described
}

// describeBorders reports the edges a cell carries, leaving out the ones it does not.
func describeBorders(borders *google.Borders) *describedBorders {
	if borders == nil {
		return nil
	}

	described := &describedBorders{}
	for _, side := range []struct {
		from *google.Border
		to   **describedBorder
	}{
		{borders.Top, &described.Top},
		{borders.Bottom, &described.Bottom},
		{borders.Left, &described.Left},
		{borders.Right, &described.Right},
	} {
		if side.from == nil {
			continue
		}
		*side.to = &describedBorder{
			Style: side.from.Style,
			Width: side.from.Width,
			Color: describeColor(side.from.Color),
		}
	}

	if *described == (describedBorders{}) {
		return nil
	}

	return described
}

// describedRule is one conditional format, in the numbers the writing tool takes.
type describedRule struct {
	Ranges     []describedRange `json:"ranges"`
	Condition  string           `json:"condition,omitempty"`
	Values     []string         `json:"values,omitempty"`
	Background string           `json:"background_color,omitempty"`
	TextColor  string           `json:"text_color,omitempty"`
	Bold       bool             `json:"bold,omitempty"`
	Italic     bool             `json:"italic,omitempty"`
	Gradient   []describedPoint `json:"gradient,omitempty"`
}

// describedPoint is one stop of a colour scale.
type describedPoint struct {
	Type  string `json:"type"`
	Color string `json:"color,omitempty"`
	Value string `json:"value,omitempty"`
}

// describeConditionalFormats reports the rules that colour cells by what is in them. They
// are tried in the order given and the first match wins, which is why the order is kept.
func describeConditionalFormats(rules []google.ConditionalFormat) []describedRule {
	var described []describedRule

	for _, rule := range rules {
		entry := describedRule{Ranges: describeMerges(rule.Ranges)}

		if boolean := rule.BooleanRule; boolean != nil {
			if condition := boolean.Condition; condition != nil {
				entry.Condition = condition.Type
				for _, value := range condition.Values {
					entry.Values = append(entry.Values, value.UserEnteredValue)
				}
			}
			if format := boolean.Format; format != nil {
				entry.Background = describeColor(format.BackgroundColor)
				if text := format.TextFormat; text != nil {
					entry.TextColor = describeColor(text.ForegroundColor)
					entry.Bold = text.Bold != nil && *text.Bold
					entry.Italic = text.Italic != nil && *text.Italic
				}
			}
		}

		if gradient := rule.GradientRule; gradient != nil {
			for _, point := range []*google.InterpolationPoint{
				gradient.MinPoint, gradient.MidPoint, gradient.MaxPoint,
			} {
				if point == nil {
					continue
				}
				entry.Gradient = append(entry.Gradient, describedPoint{
					Type: point.Type, Color: describeColor(point.Color), Value: point.Value})
			}
		}

		described = append(described, entry)
	}

	return described
}

// describedBanding is the alternating fill of a range.
type describedBanding struct {
	describedRange
	Direction  string `json:"direction"`
	Header     string `json:"header_color,omitempty"`
	FirstBand  string `json:"first_band_color,omitempty"`
	SecondBand string `json:"second_band_color,omitempty"`
	Footer     string `json:"footer_color,omitempty"`
}

func describeBandings(bandings []google.BandedRange) []describedBanding {
	var described []describedBanding
	for _, banding := range bandings {
		entry := describedBanding{
			describedRange: describeMerges([]google.GridRange{banding.Range})[0],
			Direction:      "ROWS",
		}
		properties := banding.RowProperties
		if properties == nil {
			properties = banding.ColumnProperties
			entry.Direction = "COLUMNS"
		}
		if properties != nil {
			entry.Header = describeColor(properties.HeaderColor)
			entry.FirstBand = describeColor(properties.FirstBandColor)
			entry.SecondBand = describeColor(properties.SecondBandColor)
			entry.Footer = describeColor(properties.FooterColor)
		}
		described = append(described, entry)
	}
	return described
}

// describedGroup is a run of rows or columns that folds up.
type describedGroup struct {
	Start     int  `json:"start"`
	End       int  `json:"end"`
	Depth     int  `json:"depth,omitempty"`
	Collapsed bool `json:"collapsed,omitempty"`
}

func describeGroups(groups []google.DimensionGroup) []describedGroup {
	var described []describedGroup
	for _, group := range groups {
		entry := describedGroup{Depth: group.Depth, Collapsed: group.Collapsed}
		if group.Range.StartIndex != nil {
			entry.Start = *group.Range.StartIndex
		}
		if group.Range.EndIndex != nil {
			entry.End = *group.Range.EndIndex
		}
		described = append(described, entry)
	}
	return described
}

// describedFilter is what a tab's filter hides and how it sorts.
type describedFilter struct {
	Range  *describedRange   `json:"range,omitempty"`
	Title  string            `json:"title,omitempty"`
	Hidden []describedHidden `json:"hide,omitempty"`
	SortBy []describedSort   `json:"sort,omitempty"`
}

type describedHidden struct {
	Column int      `json:"column"`
	Values []string `json:"values,omitempty"`
}

type describedSort struct {
	Column int    `json:"column"`
	Order  string `json:"order,omitempty"`
}

func describeFilter(span *google.GridRange, title string, sorts []google.SortSpec, filters []google.FilterSpec) describedFilter {
	described := describedFilter{Title: title}
	if span != nil {
		rectangle := describeMerges([]google.GridRange{*span})[0]
		described.Range = &rectangle
	}
	for _, spec := range filters {
		entry := describedHidden{Column: spec.ColumnIndex}
		if spec.FilterCriteria != nil {
			entry.Values = spec.FilterCriteria.HiddenValues
		}
		described.Hidden = append(described.Hidden, entry)
	}
	for _, spec := range sorts {
		described.SortBy = append(described.SortBy, describedSort{
			Column: spec.DimensionIndex, Order: spec.SortOrder})
	}
	return described
}

// describedProtection is a range somebody kept from being changed.
type describedProtection struct {
	Range       *describedRange `json:"range,omitempty"`
	WholeTab    bool            `json:"whole_tab,omitempty"`
	Description string          `json:"description,omitempty"`
	WarningOnly bool            `json:"warning_only,omitempty"`
	Editors     []string        `json:"editors,omitempty"`
}

func describeProtections(ranges []google.ProtectedRange) []describedProtection {
	var described []describedProtection
	for _, protected := range ranges {
		entry := describedProtection{
			Description: protected.Description,
			WarningOnly: protected.WarningOnly,
		}
		if span := protected.Range; span != nil {
			if span.StartRowIndex == nil && span.StartColumnIndex == nil &&
				span.EndRowIndex == nil && span.EndColumnIndex == nil {
				entry.WholeTab = true
			} else {
				rectangle := describeMerges([]google.GridRange{*span})[0]
				entry.Range = &rectangle
			}
		}
		if protected.Editors != nil {
			entry.Editors = protected.Editors.Users
		}
		described = append(described, entry)
	}
	return described
}

// describeMerges reports merged rectangles in the same numbers set_layout takes.
func describeMerges(merges []google.GridRange) []describedRange {
	var described []describedRange
	for _, merge := range merges {
		entry := describedRange{}
		if merge.StartRowIndex != nil {
			entry.StartRow = *merge.StartRowIndex
		}
		if merge.EndRowIndex != nil {
			entry.EndRow = *merge.EndRowIndex
		}
		if merge.StartColumnIndex != nil {
			entry.StartColumn = *merge.StartColumnIndex
		}
		if merge.EndColumnIndex != nil {
			entry.EndColumn = *merge.EndColumnIndex
		}
		described = append(described, entry)
	}
	return described
}

// describeColor renders a colour as hex, which is how a person reads one back.
//
// White is reported like any other. It looks like noise — Sheets answers white for the
// background of every cell in a workbook — but that is the effective format, and what is
// read here is what the author entered. A white entered over a coloured block, or white
// letters on a dark heading, is a decision, and dropping it paints the block through and
// turns the letters black.
func describeColor(colour *google.RGBColor) string {
	if colour == nil {
		return ""
	}

	return fmt.Sprintf("#%02X%02X%02X",
		int(colour.Red*255+0.5), int(colour.Green*255+0.5), int(colour.Blue*255+0.5))
}

// sheetsSetLayout gives a tab the shape its sample has.
func (r *registry) sheetsSetLayout(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	spreadsheet, err := client.Spreadsheet(ctx, spreadsheetID)
	if err != nil {
		return toolError(err), nil
	}

	sheet, ok := spreadsheet.SheetByTitle(sheetTitle)
	if !ok {
		return toolError(fmt.Errorf("no tab called %q in this spreadsheet: it has %s",
			sheetTitle, strings.Join(spreadsheet.SheetTitles(), ", "))), nil
	}
	sheetID := sheet.Properties.ID()

	var requests []google.SheetsRequest
	arguments := req.GetArguments()

	if _, ok := arguments["column_widths"]; ok {
		widths, err := objectList(req, "column_widths")
		if err != nil {
			return toolError(err), nil
		}

		for index, entry := range widths {
			column, ok := intField(entry, "column")
			if !ok || column < 0 {
				return toolError(fmt.Errorf("column_widths[%d].column is missing or negative", index)), nil
			}
			pixels, ok := intField(entry, "pixels")
			if !ok || pixels <= 0 {
				return toolError(fmt.Errorf("column_widths[%d].pixels is missing or not positive", index)), nil
			}

			start, end := column, column+1
			requests = append(requests, google.SheetsRequest{
				UpdateDimension: &google.UpdateDimensionRequest{
					Range: google.DimensionRange{
						SheetID:    sheetID,
						Dimension:  "COLUMNS",
						StartIndex: &start,
						EndIndex:   &end,
					},
					Properties: google.DimensionProperties{PixelSize: pixels},
					Fields:     "pixelSize",
				},
			})
		}
	}

	if _, ok := arguments["row_heights"]; ok {
		heights, err := objectList(req, "row_heights")
		if err != nil {
			return toolError(err), nil
		}

		for index, entry := range heights {
			row, ok := intField(entry, "row")
			if !ok || row < 0 {
				return toolError(fmt.Errorf("row_heights[%d].row is missing or negative", index)), nil
			}
			pixels, ok := intField(entry, "pixels")
			if !ok || pixels <= 0 {
				return toolError(fmt.Errorf("row_heights[%d].pixels is missing or not positive", index)), nil
			}

			last := row
			if through, ok := intField(entry, "through_row"); ok {
				if through < row {
					return toolError(fmt.Errorf("row_heights[%d].through_row is %d, before row %d",
						index, through, row)), nil
				}
				last = through
			}

			start, end := row, last+1
			requests = append(requests, google.SheetsRequest{
				UpdateDimension: &google.UpdateDimensionRequest{
					Range: google.DimensionRange{
						SheetID:    sheetID,
						Dimension:  "ROWS",
						StartIndex: &start,
						EndIndex:   &end,
					},
					Properties: google.DimensionProperties{PixelSize: pixels},
					Fields:     "pixelSize",
				},
			})
		}
	}

	// Hiding and showing are the same request with a different answer, which is why one
	// argument carries both: a row hidden by mistake comes back with hidden false.
	for _, kind := range []struct {
		argument, dimension, index, through string
	}{
		{"hide_rows", "ROWS", "row", "through_row"},
		{"hide_columns", "COLUMNS", "column", "through_column"},
	} {
		if _, ok := arguments[kind.argument]; !ok {
			continue
		}

		entries, err := objectList(req, kind.argument)
		if err != nil {
			return toolError(err), nil
		}

		for position, entry := range entries {
			first, ok := intField(entry, kind.index)
			if !ok || first < 0 {
				return toolError(fmt.Errorf("%s[%d].%s is missing or negative",
					kind.argument, position, kind.index)), nil
			}

			last := first
			if through, ok := intField(entry, kind.through); ok {
				if through < first {
					return toolError(fmt.Errorf("%s[%d].%s is %d, before %d",
						kind.argument, position, kind.through, through, first)), nil
				}
				last = through
			}

			hidden := true
			if raw, ok := entry["hidden"].(bool); ok {
				hidden = raw
			}

			start, end := first, last+1
			requests = append(requests, google.SheetsRequest{
				UpdateDimension: &google.UpdateDimensionRequest{
					Range: google.DimensionRange{SheetID: sheetID, Dimension: kind.dimension,
						StartIndex: &start, EndIndex: &end},
					Properties: google.DimensionProperties{HiddenByUser: &hidden},
					Fields:     "hiddenByUser",
				},
			})
		}
	}

	if _, ok := arguments["auto_resize_columns"]; ok {
		columns, err := intList(req, "auto_resize_columns")
		if err != nil {
			return toolError(err), nil
		}
		if len(columns) == 0 || len(columns) > 2 {
			return toolError(fmt.Errorf("auto_resize_columns takes one column or a first and a last, "+
				"got %d numbers", len(columns))), nil
		}

		start := columns[0]
		end := start + 1
		if len(columns) == 2 {
			if columns[1] < start {
				return toolError(fmt.Errorf("auto_resize_columns ends at %d, before %d", columns[1], start)), nil
			}
			end = columns[1] + 1
		}

		requests = append(requests, google.SheetsRequest{
			AutoResize: &google.AutoResizeRequest{Dimensions: google.DimensionRange{
				SheetID: sheetID, Dimension: "COLUMNS", StartIndex: &start, EndIndex: &end}},
		})
	}

	_, hasRows := arguments["rows"]
	_, hasColumns := arguments["columns"]
	_, hasFrozenRows := arguments["frozen_rows"]
	_, hasFrozenColumns := arguments["frozen_columns"]

	tabColour, err := sheetColor(req, "tab_color")
	if err != nil {
		return toolError(err), nil
	}

	if hasRows || hasColumns || hasFrozenRows || hasFrozenColumns || tabColour != nil {
		grid := &google.GridProps{}
		var fields []string

		current := sheet.Properties.GridProps
		if current == nil {
			current = &google.GridProps{}
		}

		if hasRows {
			count := req.GetInt("rows", 0)
			if count < current.RowCount {
				return toolError(fmt.Errorf("the tab has %d rows and rows is %d: making a grid smaller "+
					"deletes the rows that fall off it, which this server does not do — ask for the size "+
					"when the tab is created", current.RowCount, count)), nil
			}
			grid.RowCount = count
			fields = append(fields, "gridProperties.rowCount")
		}
		if hasColumns {
			count := req.GetInt("columns", 0)
			if count < current.ColumnCount {
				return toolError(fmt.Errorf("the tab has %d columns and columns is %d: making a grid "+
					"smaller deletes the columns that fall off it, which this server does not do — ask "+
					"for the size when the tab is created", current.ColumnCount, count)), nil
			}
			grid.ColumnCount = count
			fields = append(fields, "gridProperties.columnCount")
		}

		if hasFrozenRows {
			count := req.GetInt("frozen_rows", 0)
			if count < 0 {
				return toolError(fmt.Errorf("frozen_rows %d is negative", count)), nil
			}
			grid.FrozenRowCount = count
			fields = append(fields, "gridProperties.frozenRowCount")
		}
		if hasFrozenColumns {
			count := req.GetInt("frozen_columns", 0)
			if count < 0 {
				return toolError(fmt.Errorf("frozen_columns %d is negative", count)), nil
			}
			grid.FrozenColumnCount = count
			fields = append(fields, "gridProperties.frozenColumnCount")
		}

		properties := google.SheetProperties{SheetID: google.SheetIDOf(sheetID)}
		if len(fields) > 0 {
			properties.GridProps = grid
		}
		if tabColour != nil {
			properties.TabColor = tabColour
			fields = append(fields, "tabColor")
		}

		requests = append(requests, google.SheetsRequest{
			UpdateSheet: &google.UpdateSheetRequest{
				Properties: properties,
				Fields:     strings.Join(fields, ","),
			},
		})
	}

	for _, kind := range []string{"merge", "unmerge"} {
		if _, ok := arguments[kind]; !ok {
			continue
		}

		merges, err := objectList(req, kind)
		if err != nil {
			return toolError(err), nil
		}

		for index, entry := range merges {
			bounds := map[string]int{}
			for _, name := range []string{"start_row", "end_row", "start_column", "end_column"} {
				value, ok := intField(entry, name)
				if !ok || value < 0 {
					return toolError(fmt.Errorf("%s[%d].%s is missing or negative", kind, index, name)), nil
				}
				bounds[name] = value
			}

			if bounds["end_row"] <= bounds["start_row"] || bounds["end_column"] <= bounds["start_column"] {
				return toolError(fmt.Errorf("%s[%d] is empty: the ends are exclusive", kind, index)), nil
			}

			startRow, endRow := bounds["start_row"], bounds["end_row"]
			startColumn, endColumn := bounds["start_column"], bounds["end_column"]
			span := google.GridRange{
				SheetID:          sheetID,
				StartRowIndex:    &startRow,
				EndRowIndex:      &endRow,
				StartColumnIndex: &startColumn,
				EndColumnIndex:   &endColumn,
			}

			if kind == "unmerge" {
				requests = append(requests, google.SheetsRequest{
					UnmergeCells: &google.UnmergeCellsRequest{Range: span},
				})
				continue
			}

			requests = append(requests, google.SheetsRequest{
				MergeCells: &google.MergeCellsRequest{Range: span, MergeType: "MERGE_ALL"},
			})
		}
	}

	if len(requests) == 0 {
		return toolError(fmt.Errorf("nothing to do: name column_widths, row_heights, rows, columns, " +
			"frozen_rows, frozen_columns, hide_rows, hide_columns, auto_resize_columns, tab_color " +
			"or merge")), nil
	}

	response, err := client.SheetsBatchUpdate(ctx, spreadsheetID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet_title":    sheetTitle,
		"sheet_id":       sheetID,
		"changes":        len(requests),
		"replies":        len(response.Replies),
	})
}
