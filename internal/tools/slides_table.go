package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// tableSpec is a table about to be created, after the arguments have been checked.
type tableSpec struct {
	PageObjectID     string
	ObjectID         string
	Rows             [][]string
	Columns          int
	X, Y             float64
	Width, Height    float64
	FontSize         float64
	HeaderRow        bool
	HeaderFontSize   float64
	HeaderBold       bool
	ColumnWidths     []float64
	FontFamily       string
	ForegroundColor  *google.RGBColor
	ColumnAlignments []string
	HeaderAlignments []string
}

// slidesCreateTableWithText creates a native table and fills it in.
func (r *registry) slidesCreateTableWithText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	spec, err := r.parseTableSpec(req)
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, tableRequests(spec))
	if err != nil {
		return toolError(err), nil
	}

	objectID := spec.ObjectID
	for _, reply := range response.Replies {
		if reply.CreateTable != nil && reply.CreateTable.ObjectID != "" {
			objectID = reply.CreateTable.ObjectID
		}
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"page_object_id":  spec.PageObjectID,
		"object_id":       objectID,
		"rows":            len(spec.Rows),
		"columns":         spec.Columns,
		"replies":         len(response.Replies),
	})
}

// parseTableSpec turns the arguments into a checked table.
//
// Every per-column list is checked against the width of the table here, because Slides
// answers a mismatched one with an error about request 7 of a batch nobody wrote by
// hand.
func (r *registry) parseTableSpec(req mcp.CallToolRequest) (tableSpec, error) {
	pageObjectID, err := requiredString(req, "page_object_id")
	if err != nil {
		return tableSpec{}, err
	}

	rows, columns, err := parseTableRows(req)
	if err != nil {
		return tableSpec{}, err
	}

	spec := tableSpec{
		PageObjectID:   pageObjectID,
		ObjectID:       optionalString(req, "object_id"),
		Rows:           rows,
		Columns:        columns,
		FontSize:       req.GetFloat("font_size", 12),
		HeaderRow:      req.GetBool("header_row", true),
		HeaderFontSize: req.GetFloat("header_font_size", 0),
		HeaderBold:     req.GetBool("header_bold", true),
		FontFamily:     optionalString(req, "font_family"),
	}

	if spec.ObjectID == "" {
		spec.ObjectID = r.objectID("table")
	}

	for _, dimension := range []struct {
		name   string
		target *float64
	}{
		{"x", &spec.X},
		{"y", &spec.Y},
		{"width", &spec.Width},
		{"height", &spec.Height},
	} {
		value, err := req.RequireFloat(dimension.name)
		if err != nil {
			return tableSpec{}, err
		}
		*dimension.target = value
	}

	if spec.Width <= 0 || spec.Height <= 0 {
		return tableSpec{}, fmt.Errorf("width and height are in EMU and have to be positive, got %g and %g",
			spec.Width, spec.Height)
	}

	if widths := req.GetFloatSlice("column_widths", nil); len(widths) > 0 {
		if len(widths) != columns {
			return tableSpec{}, fmt.Errorf("column_widths has %d entries, but the table has %d columns",
				len(widths), columns)
		}
		for index, width := range widths {
			if width <= 0 {
				return tableSpec{}, fmt.Errorf("column_widths[%d] is %g: widths are in EMU and have to be positive",
					index, width)
			}
		}
		spec.ColumnWidths = widths
	}

	for _, alignments := range []struct {
		name   string
		target *[]string
	}{
		{"column_alignments", &spec.ColumnAlignments},
		{"header_alignments", &spec.HeaderAlignments},
	} {
		values := req.GetStringSlice(alignments.name, nil)
		if len(values) == 0 {
			continue
		}
		if len(values) != columns {
			return tableSpec{}, fmt.Errorf("%s has %d entries, but the table has %d columns",
				alignments.name, len(values), columns)
		}
		for index, value := range values {
			switch value {
			case "START", "CENTER", "END", "JUSTIFIED":
			default:
				return tableSpec{}, fmt.Errorf("%s[%d] is %q: use START, CENTER, END or JUSTIFIED",
					alignments.name, index, value)
			}
		}
		*alignments.target = values
	}

	colour, err := parseColor(req, "foreground_color")
	if err != nil {
		return tableSpec{}, err
	}
	spec.ForegroundColor = colour

	return spec, nil
}

// parseTableRows reads the cells and insists the table is rectangular.
func parseTableRows(req mcp.CallToolRequest) ([][]string, int, error) {
	raw, ok := req.GetArguments()["rows"]
	if !ok {
		return nil, 0, fmt.Errorf("rows is required: a list of lists of cell values")
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, 0, fmt.Errorf("rows must be a list of lists, got %T", raw)
	}
	if len(list) == 0 {
		return nil, 0, fmt.Errorf("rows is empty")
	}

	rows := make([][]string, 0, len(list))
	columns := 0

	for rowIndex, rawRow := range list {
		cells, ok := rawRow.([]any)
		if !ok {
			return nil, 0, fmt.Errorf("rows[%d] must be a list of cells, got %T", rowIndex, rawRow)
		}

		if rowIndex == 0 {
			columns = len(cells)
			if columns == 0 {
				return nil, 0, fmt.Errorf("rows[0] has no cells")
			}
		}
		if len(cells) != columns {
			return nil, 0, fmt.Errorf("rows[%d] has %d cells, but rows[0] has %d: the table has to be rectangular",
				rowIndex, len(cells), columns)
		}

		row := make([]string, 0, columns)
		for columnIndex, cell := range cells {
			text, err := cellText(cell)
			if err != nil {
				return nil, 0, fmt.Errorf("rows[%d][%d]: %w", rowIndex, columnIndex, err)
			}
			row = append(row, text)
		}

		rows = append(rows, row)
	}

	return rows, columns, nil
}

// cellText renders one cell value. Numbers keep the shortest form that reads back the
// same, so 1 stays "1" rather than becoming "1.000000".
func cellText(cell any) (string, error) {
	switch value := cell.(type) {
	case nil:
		return "", nil
	case string:
		return value, nil
	case bool:
		return strconv.FormatBool(value), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(value), nil
	default:
		return "", fmt.Errorf("a cell has to be text, a number, true/false or null, got %T", cell)
	}
}

// parseColor reads an optional RGB colour with components from 0 to 1.
func parseColor(req mcp.CallToolRequest, name string) (*google.RGBColor, error) {
	raw, ok := req.GetArguments()[name]
	if !ok || raw == nil {
		return nil, nil
	}

	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object with red, green and blue, got %T", name, raw)
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
		value, present := object[component.name]
		if !present {
			continue
		}

		number, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("%s.%s must be a number from 0 to 1, got %T", name, component.name, value)
		}
		if number < 0 || number > 1 {
			return nil, fmt.Errorf("%s.%s is %g: colour components run from 0 to 1, not 0 to 255",
				name, component.name, number)
		}

		*component.target = number
	}

	return colour, nil
}

// tableRequests builds the batch that creates a table and fills it in.
//
// One table, then its column widths, then cell by cell: text, style, alignment. The
// cells are filled in row order so a failure part-way through leaves something a person
// can recognise.
func tableRequests(spec tableSpec) []google.Request {
	requests := []google.Request{{
		CreateTable: &google.CreateTableRequest{
			ObjectID: spec.ObjectID,
			ElementProperties: &google.ElementProperties{
				PageObjectID: spec.PageObjectID,
				Size: &google.Size{
					Width:  google.EMU(spec.Width),
					Height: google.EMU(spec.Height),
				},
				Transform: &google.Transform{
					ScaleX:     1,
					ScaleY:     1,
					TranslateX: spec.X,
					TranslateY: spec.Y,
					Unit:       "EMU",
				},
			},
			Rows:    len(spec.Rows),
			Columns: spec.Columns,
		},
	}}

	for index, width := range spec.ColumnWidths {
		requests = append(requests, google.Request{
			UpdateTableColumnProperties: &google.UpdateTableColumnPropertiesRequest{
				ObjectID:              spec.ObjectID,
				ColumnIndices:         []int{index},
				TableColumnProperties: &google.TableColumnProperties{ColumnWidth: google.EMU(width)},
				Fields:                "columnWidth",
			},
		})
	}

	for rowIndex, row := range spec.Rows {
		header := rowIndex == 0 && spec.HeaderRow

		for columnIndex, text := range row {
			location := &google.CellLocation{RowIndex: rowIndex, ColumnIndex: columnIndex}

			// An empty cell gets nothing at all. Slides refuses updateTextStyle over a
			// cell with no text — "the object has no text" — and empty cells are normal:
			// they are what a table looks like where cells are about to be merged.
			if text == "" {
				continue
			}

			requests = append(requests, google.Request{
				InsertText: &google.InsertTextRequest{
					ObjectID:       spec.ObjectID,
					CellLocation:   location,
					Text:           text,
					InsertionIndex: 0,
				},
			})

			if style, fields := cellStyle(spec, header); len(fields) > 0 {
				requests = append(requests, google.Request{
					UpdateTextStyle: &google.UpdateTextStyleRequest{
						ObjectID:     spec.ObjectID,
						CellLocation: location,
						TextRange:    google.AllText(),
						Style:        style,
						Fields:       strings.Join(fields, ","),
					},
				})
			}

			if alignment := cellAlignment(spec, header, columnIndex); alignment != "" {
				requests = append(requests, google.Request{
					UpdateParagraphStyle: &google.UpdateParagraphStyleRequest{
						ObjectID:     spec.ObjectID,
						CellLocation: location,
						TextRange:    google.AllText(),
						Style:        &google.ParagraphStyle{Alignment: alignment},
						Fields:       "alignment",
					},
				})
			}
		}
	}

	return requests
}

// cellStyle is the style one cell gets, and the mask that says which parts of it apply.
func cellStyle(spec tableSpec, header bool) (*google.TextStyle, []string) {
	style := &google.TextStyle{}
	var fields []string

	size := spec.FontSize
	if header && spec.HeaderFontSize > 0 {
		size = spec.HeaderFontSize
	}
	if size > 0 {
		style.FontSize = google.PT(size)
		fields = append(fields, "fontSize")
	}

	if spec.FontFamily != "" {
		style.FontFamily = spec.FontFamily
		fields = append(fields, "fontFamily")
	}

	if spec.ForegroundColor != nil {
		style.ForegroundColor = &google.OptionalColor{
			OpaqueColor: &google.OpaqueColor{RGBColor: spec.ForegroundColor},
		}
		fields = append(fields, "foregroundColor")
	}

	if header {
		bold := spec.HeaderBold
		style.Bold = &bold
		fields = append(fields, "bold")
	}

	return style, fields
}

// cellAlignment is the alignment of one cell, header overriding body.
func cellAlignment(spec tableSpec, header bool, columnIndex int) string {
	if header && len(spec.HeaderAlignments) > columnIndex {
		return spec.HeaderAlignments[columnIndex]
	}
	if len(spec.ColumnAlignments) > columnIndex {
		return spec.ColumnAlignments[columnIndex]
	}
	return ""
}
