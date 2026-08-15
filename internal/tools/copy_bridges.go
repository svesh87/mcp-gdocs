package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// The bridges between the three kinds of document.
//
// Three families, three shapes of content, and exactly three things they say the same way: a
// table is values with a look per cell, text is paragraphs with a look per run, and a picture
// is an address. Everything outside that common ground — a formula, a conditional format, a
// slide's theme, a document's section — has meaning in one family and none in the others, and
// is named in the answer rather than approximated.
//
// Each tool is named for where the content lands, and reads the family it comes from. That
// makes a window on one family — /mcp/slides — able to read another, which is a deliberate
// widening: the window stops being a boundary on what data is reachable and stays only a
// boundary on what can be changed.

// registerCopyBridges adds them. They live in the copy group of the family they write to.
func (r *registry) registerCopyBridges(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_slides_copy_table_from_sheets",
		mcp.WithDescription("Put a rectangle of a workbook on a slide as a real Slides table: the values "+
			"as they are shown — a formula's result, a number in its cell's format — with each cell's "+
			"font, weight, colour, alignment and fill. Reads the workbook, so a window on slides alone "+
			"still needs the spreadsheet to be readable by the signed-in account. What has no meaning "+
			"on a slide is named in the answer: formulas, rules that colour by content, dropdowns."),
		mcp.WithString("source_spreadsheet_id", mcp.Required(), mcp.Description("Workbook to read.")),
		mcp.WithString("source_sheet_title", mcp.Required(), mcp.Description("Tab the rectangle is on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("One past the last row.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("One past the last column.")),
		mcp.WithString("target_presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("target_page_object_id", mcp.Required(), mcp.Description("Slide to put it on.")),
		mcp.WithNumber("x_emu", mcp.Required(), mcp.Description("Left edge, in EMU.")),
		mcp.WithNumber("y_emu", mcp.Required(), mcp.Description("Top edge, in EMU.")),
		mcp.WithNumber("width_emu", mcp.Required(), mcp.Description("Width, in EMU.")),
		mcp.WithNumber("height_emu", mcp.Required(), mcp.Description("Height, in EMU.")),
	), r.slidesCopyTableFromSheets)

	srv.AddTool(mcp.NewTool("gdocs_slides_copy_text_from_docs",
		mcp.WithDescription("Put a stretch of a document on a slide as a text box: paragraphs with their "+
			"alignment and spacing, runs with their font, size, weight and colour, and lists with their "+
			"depth. Indices are the document's own, as gdocs_docs_read_structure reports them. A "+
			"document's own furniture — tables, section breaks, headers, chips — has no place in a text "+
			"box and is named in the answer instead."),
		mcp.WithString("source_document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("start_index", mcp.Required(), mcp.Description("First index of the stretch.")),
		mcp.WithNumber("end_index", mcp.Required(), mcp.Description("One past its last index.")),
		mcp.WithString("target_presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("target_page_object_id", mcp.Required(), mcp.Description("Slide to put it on.")),
		mcp.WithNumber("x_emu", mcp.Required(), mcp.Description("Left edge, in EMU.")),
		mcp.WithNumber("y_emu", mcp.Required(), mcp.Description("Top edge, in EMU.")),
		mcp.WithNumber("width_emu", mcp.Required(), mcp.Description("Width, in EMU.")),
		mcp.WithNumber("height_emu", mcp.Required(), mcp.Description("Height, in EMU.")),
	), r.slidesCopyTextFromDocs)

	srv.AddTool(mcp.NewTool("gdocs_docs_copy_table_from_sheets",
		mcp.WithDescription("Put a rectangle of a workbook into a document as a real table: the values as "+
			"they are shown, with each cell's weight, colour, alignment and fill. Done in two passes, "+
			"because a table's cells only get indices once the table exists — so the table is made, the "+
			"document is read back, and the cells are filled. What has no meaning in a document is named "+
			"in the answer: formulas, rules that colour by content, dropdowns."),
		mcp.WithString("source_spreadsheet_id", mcp.Required(), mcp.Description("Workbook to read.")),
		mcp.WithString("source_sheet_title", mcp.Required(), mcp.Description("Tab the rectangle is on.")),
		mcp.WithNumber("start_row", mcp.Required(), mcp.Description("First row, counting from 0.")),
		mcp.WithNumber("end_row", mcp.Required(), mcp.Description("One past the last row.")),
		mcp.WithNumber("start_column", mcp.Required(), mcp.Description("First column, from 0.")),
		mcp.WithNumber("end_column", mcp.Required(), mcp.Description("One past the last column.")),
		mcp.WithString("target_document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("target_index", mcp.Description(
			"Where the table goes. Without one it goes at the end of the document.")),
	), r.docsCopyTableFromSheets)

	srv.AddTool(mcp.NewTool("gdocs_docs_copy_slide_image",
		mcp.WithDescription("Put a picture of a slide into a document. A slide is not text and has no "+
			"equivalent in a document, so what crosses is its rendering — which is what a report quoting "+
			"a deck actually wants. The picture is a snapshot: it stops following the slide the moment "+
			"it is taken, and a slide changed afterwards leaves the document showing the old one."),
		mcp.WithString("source_presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("source_page_object_id", mcp.Required(), mcp.Description("Slide to render.")),
		mcp.WithString("target_document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("target_index", mcp.Description(
			"Where it goes. Without one it goes at the end of the document.")),
		mcp.WithString("size", mcp.DefaultString("MEDIUM"), mcp.Description("SMALL, MEDIUM or LARGE.")),
		mcp.WithNumber("width_pt", mcp.Description(
			"Width on the page, in points. The height follows the slide's proportions.")),
	), r.docsCopySlideImage)

	srv.AddTool(mcp.NewTool("gdocs_sheets_copy_table_from_docs",
		mcp.WithDescription("Put a table out of a document, or off a slide, into a workbook as cells. The "+
			"text of every cell lands as its value; a cell that reads as a number becomes one, so the "+
			"column can be summed. What a table has and a range does not — merged cells, per-cell fills, "+
			"borders — is named in the answer rather than approximated with formatting."),
		mcp.WithString("source_document_id", mcp.Description("Document the table is in.")),
		mcp.WithNumber("table_start_index", mcp.Description(
			"Index the table starts at, as gdocs_docs_read_structure reports it. With source_document_id.")),
		mcp.WithString("source_presentation_id", mcp.Description("Deck the table is in, instead of a document.")),
		mcp.WithString("source_object_id", mcp.Description("Table on a slide. With source_presentation_id.")),
		mcp.WithString("target_spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("target_sheet_title", mcp.Required(), mcp.Description("Tab to write into.")),
		mcp.WithNumber("target_row", mcp.DefaultNumber(0), mcp.Description("Row the top-left cell lands on.")),
		mcp.WithNumber("target_column", mcp.DefaultNumber(0), mcp.Description("Column it lands on.")),
	), r.sheetsCopyTableFromDocs)
}

// gridCell is one cell of a workbook rectangle, reduced to what both a slide and a document
// can express.
type gridCell struct {
	text      string
	bold      *bool
	italic    *bool
	underline *bool
	strike    *bool
	font      string
	sizePT    float64
	colour    *google.RGBColor
	fill      *google.RGBColor
	alignment string
}

// readRectangle reads a workbook rectangle as shown, and says what it could not bring.
//
// As shown rather than as typed: a table on a slide or in a document is read by a person, and
// a formula there is a formula nobody can evaluate. This is the opposite of what
// gdocs_sheets_copy_range does, and the difference is the whole reason both exist.
func (r *registry) readRectangle(ctx context.Context, client *google.Client, req mcp.CallToolRequest) ([][]gridCell, []string, error) {
	spreadsheetID, err := requiredString(req, "source_spreadsheet_id")
	if err != nil {
		return nil, nil, err
	}
	sheetTitle, err := requiredString(req, "source_sheet_title")
	if err != nil {
		return nil, nil, err
	}
	bounds, err := gridBounds(req)
	if err != nil {
		return nil, nil, err
	}

	a1 := google.A1Range(sheetTitle, rectangleA1(bounds))
	book, err := client.SpreadsheetGrid(ctx, spreadsheetID, a1)
	if err != nil {
		return nil, nil, err
	}

	sheet := sheetNamed(book, sheetTitle)
	if sheet == nil || len(sheet.Data) == 0 {
		return nil, nil, fmt.Errorf("nothing came back for %s: check the tab name and the rectangle", a1)
	}

	width := bounds["end_column"] - bounds["start_column"]
	rows := make([][]gridCell, 0, bounds["end_row"]-bounds["start_row"])

	for _, row := range sheet.Data[0].RowData {
		cells := make([]gridCell, width)
		for index := range cells {
			if index >= len(row.Values) {
				continue
			}
			cells[index] = describeGridCell(&row.Values[index])
		}
		rows = append(rows, cells)
	}

	// A rectangle read past the last filled row comes back short. Padding keeps the table
	// rectangular, which both targets require and which a reader expects.
	for len(rows) < bounds["end_row"]-bounds["start_row"] {
		rows = append(rows, make([]gridCell, width))
	}

	var lost []string
	if len(sheet.Merges) > 0 {
		lost = append(lost, fmt.Sprintf("%d merged cells, which arrive as separate cells", len(sheet.Merges)))
	}
	if len(sheet.ConditionalFormats) > 0 {
		lost = append(lost, "rules that colour by content: the colours they were painting are here, "+
			"the rule is not, so the table stops reacting to its numbers")
	}
	lost = append(lost, "formulas, which arrive as the values they produced when this was read")

	return rows, lost, nil
}

// describeGridCell reduces one cell to the common ground.
func describeGridCell(cell *google.CellValue) gridCell {
	described := gridCell{text: cell.FormattedValue}

	format := cell.UserEnteredFormat
	if format == nil {
		return described
	}

	described.alignment = format.HorizontalAlignment
	described.fill = format.BackgroundColor

	if text := format.TextFormat; text != nil {
		described.bold, described.italic = text.Bold, text.Italic
		described.underline, described.strike = text.Underline, text.Strikethrough
		described.font = text.FontFamily
		described.sizePT = float64(text.FontSize)
		described.colour = text.ForegroundColor
	}

	return described
}

// slidesAlignment turns a spreadsheet's alignment into the word Slides uses.
func slidesAlignment(value string) string {
	switch value {
	case "LEFT":
		return "START"
	case "CENTER":
		return "CENTER"
	case "RIGHT":
		return "END"
	}

	return ""
}

func (r *registry) slidesCopyTableFromSheets(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	targetID, err := requiredString(req, "target_presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "target_page_object_id")
	if err != nil {
		return toolError(err), nil
	}

	box, err := requiredBox(req)
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	rows, lost, err := r.readRectangle(ctx, client, req)
	if err != nil {
		return toolError(err), nil
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return toolError(fmt.Errorf("the rectangle has no cells")), nil
	}

	objectID := r.objectID("table")
	requests := []google.Request{{
		CreateTable: &google.CreateTableRequest{
			ObjectID: objectID,
			ElementProperties: &google.ElementProperties{
				PageObjectID: pageObjectID,
				Size:         &google.Size{Width: google.EMU(box.width), Height: google.EMU(box.height)},
				Transform: &google.Transform{
					ScaleX: 1, ScaleY: 1, TranslateX: box.x, TranslateY: box.y, Unit: "EMU",
				},
			},
			Rows:    len(rows),
			Columns: len(rows[0]),
		},
	}}

	for rowIndex, row := range rows {
		for columnIndex := range row {
			cell := &row[columnIndex]
			location := &google.CellLocation{RowIndex: rowIndex, ColumnIndex: columnIndex}

			if cell.fill != nil {
				requests = append(requests, google.Request{
					UpdateTableCellProperties: &google.UpdateTableCellPropertiesRequest{
						ObjectID:   objectID,
						TableRange: &google.TableRange{Location: location, RowSpan: 1, ColumnSpan: 1},
						TableCellProperties: &google.TableCellProperties{
							BackgroundFill: &google.TableCellBackgroundFill{
								SolidFill: &google.SolidFill{
									Color: &google.OpaqueColor{RGBColor: cell.fill}, Alpha: 1,
								},
							},
						},
						Fields: "tableCellBackgroundFill",
					},
				})
			}

			// Slides refuses to style a cell with no text in it, so an empty cell is left
			// alone entirely rather than half-written.
			if cell.text == "" {
				continue
			}

			requests = append(requests, google.Request{
				InsertText: &google.InsertTextRequest{
					ObjectID: objectID, CellLocation: location, Text: cell.text, InsertionIndex: 0,
				},
			})

			if style, fields := gridCellStyle(cell); len(fields) > 0 {
				requests = append(requests, google.Request{
					UpdateTextStyle: &google.UpdateTextStyleRequest{
						ObjectID: objectID, CellLocation: location, TextRange: google.AllText(),
						Style: style, Fields: strings.Join(fields, ","),
					},
				})
			}

			if alignment := slidesAlignment(cell.alignment); alignment != "" {
				requests = append(requests, google.Request{
					UpdateParagraphStyle: &google.UpdateParagraphStyleRequest{
						ObjectID: objectID, CellLocation: location, TextRange: google.AllText(),
						Style: &google.ParagraphStyle{Alignment: alignment}, Fields: "alignment",
					},
				})
			}
		}
	}

	if _, err := client.SlidesBatchUpdate(ctx, targetID, requests); err != nil {
		return toolError(err), nil
	}

	return resultJSON(withLosses(map[string]any{
		"target_presentation_id": targetID,
		"target_page_object_id":  pageObjectID,
		"object_id":              objectID,
		"rows":                   len(rows),
		"columns":                len(rows[0]),
		"requests":               len(requests),
	}, lost))
}

// gridCellStyle is a workbook cell's look as a slide's text style.
func gridCellStyle(cell *gridCell) (*google.TextStyle, []string) {
	style := &google.TextStyle{
		Bold: cell.bold, Italic: cell.italic, Underline: cell.underline, Strikethrough: cell.strike,
	}
	if cell.font != "" {
		style.FontFamily = cell.font
	}
	if cell.sizePT > 0 {
		style.FontSize = google.PT(cell.sizePT)
	}
	if cell.colour != nil {
		style.ForegroundColor = &google.OptionalColor{
			OpaqueColor: &google.OpaqueColor{RGBColor: cell.colour},
		}
	}

	return style, style.Fields()
}

func (r *registry) slidesCopyTextFromDocs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID, err := requiredString(req, "source_document_id")
	if err != nil {
		return toolError(err), nil
	}
	targetID, err := requiredString(req, "target_presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "target_page_object_id")
	if err != nil {
		return toolError(err), nil
	}
	box, err := requiredBox(req)
	if err != nil {
		return toolError(err), nil
	}

	start, err := req.RequireInt("start_index")
	if err != nil {
		return toolError(err), nil
	}
	end, err := req.RequireInt("end_index")
	if err != nil {
		return toolError(err), nil
	}
	if end <= start {
		return toolError(fmt.Errorf("the range is empty: end_index is exclusive")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	document, err := client.Document(ctx, sourceID)
	if err != nil {
		return toolError(err), nil
	}
	if document.Body == nil {
		return toolError(fmt.Errorf("the source document has no body")), nil
	}

	// The document's paragraphs are turned into the shape a slide's text box takes: the text
	// of every run, the depth of every list item as tabs, and the styles as ranges over what
	// results. That is the same arithmetic gdocs_slides_copy_slide does, and it has to be —
	// the target is the same kind of object.
	content, lost := docsTextAsSlideContent(document, int64(start), int64(end))
	if content == nil {
		return toolError(fmt.Errorf("nothing in that range is text: %s", strings.Join(lost, "; "))), nil
	}

	objectID := r.objectID("box")
	requests := []google.Request{{
		CreateShape: &google.CreateShapeRequest{
			ObjectID:  objectID,
			ShapeType: "TEXT_BOX",
			ElementProperties: &google.ElementProperties{
				PageObjectID: pageObjectID,
				Size:         &google.Size{Width: google.EMU(box.width), Height: google.EMU(box.height)},
				Transform: &google.Transform{
					ScaleX: 1, ScaleY: 1, TranslateX: box.x, TranslateY: box.y, Unit: "EMU",
				},
			},
		},
	}}
	requests = append(requests, textRequests(objectID, content, nil)...)

	if _, err := client.SlidesBatchUpdate(ctx, targetID, requests); err != nil {
		return toolError(err), nil
	}

	return resultJSON(withLosses(map[string]any{
		"source_document_id":     sourceID,
		"target_presentation_id": targetID,
		"target_page_object_id":  pageObjectID,
		"object_id":              objectID,
		"requests":               len(requests),
	}, lost))
}

// docsTextAsSlideContent turns a document's paragraphs into the text content a slide's box
// takes, and names what a text box has no place for.
func docsTextAsSlideContent(document *google.Document, start, end int64) (*google.TextContent, []string) {
	content := &google.TextContent{}
	var lost []string

	lose := func(what string) {
		for _, already := range lost {
			if already == what {
				return
			}
		}
		lost = append(lost, what)
	}

	for index := range document.Body.Content {
		element := &document.Body.Content[index]
		if !docsTouches(element, start, end) {
			continue
		}

		switch {
		case element.Table != nil:
			lose("a table, which is not text: bring it with gdocs_slides_copy_table_from_sheets " +
				"if the numbers live in a workbook, or rebuild it with gdocs_slides_create_table_with_text")
			continue
		case element.SectionBreak != nil:
			lose("a section break, which a slide has no equivalent of")
			continue
		case element.Paragraph == nil:
			continue
		}

		paragraph := element.Paragraph
		marker := &google.TextElement{ParagraphMarker: &google.ParagraphMarker{
			Style: slideParagraphStyleOf(paragraph.Style),
		}}
		if paragraph.Bullet != nil {
			level := 0
			if paragraph.Bullet.NestingLevel != nil {
				level = *paragraph.Bullet.NestingLevel
			}
			marker.ParagraphMarker.Bullet = &google.Bullet{NestingLevel: &level}
		}

		wrote := false
		runs := make([]google.TextElement, 0, len(paragraph.Elements))
		for item := range paragraph.Elements {
			piece := &paragraph.Elements[item]
			switch {
			case piece.TextRun != nil:
				text := docsClip(piece, piece.TextRun.Content, start, end)
				if text == "" {
					continue
				}
				runs = append(runs, google.TextElement{TextRun: &google.TextRun{
					Content: text, Style: slideTextStyleOf(piece.TextRun.Style),
				}})
				wrote = true
			case piece.InlineObject != nil:
				lose("a picture in the text: a slide's text box holds no pictures, so put it on the " +
					"slide with gdocs_slides_insert_image")
			case piece.Person != nil || piece.RichLink != nil:
				lose("a chip, which is a live object a slide cannot hold; its text is not carried either")
			}
		}

		if !wrote {
			continue
		}

		content.TextElements = append(content.TextElements, *marker)
		content.TextElements = append(content.TextElements, runs...)
	}

	if len(content.TextElements) == 0 {
		return nil, lost
	}

	return content, lost
}

// slideTextStyleOf turns a document's run style into a slide's. The two describe the same
// things and nest them differently, which is the whole of the translation.
func slideTextStyleOf(style *google.DocsTextStyle) *google.TextStyle {
	if style == nil {
		return nil
	}

	converted := &google.TextStyle{
		Bold: style.Bold, Italic: style.Italic,
		Underline: style.Underline, Strikethrough: style.Strikethrough,
		SmallCaps: style.SmallCaps, BaselineOffset: style.BaselineOffset,
		FontSize: style.FontSize, Link: style.Link,
	}
	if style.WeightedFont != nil {
		converted.FontFamily = style.WeightedFont.FontFamily
		converted.WeightedFontFamily = style.WeightedFont
	}
	if colour := docsColourToOpaque(style.ForegroundColor); colour != nil {
		converted.ForegroundColor = colour
	}
	if colour := docsColourToOpaque(style.BackgroundColor); colour != nil {
		converted.BackgroundColor = colour
	}

	return converted
}

// docsColourToOpaque unwraps a document's colour, which nests one level deeper than a
// slide's. An empty object is "no colour" on both sides and stays empty.
func docsColourToOpaque(colour *google.DocsColor) *google.OptionalColor {
	if colour == nil || colour.Color == nil || colour.Color.RGBColor == nil {
		return nil
	}

	return &google.OptionalColor{OpaqueColor: &google.OpaqueColor{RGBColor: colour.Color.RGBColor}}
}

// slideParagraphStyleOf keeps the fields both sides mean the same by. A document's indents
// are left out on purpose: on a slide they fight the list depth, which is the same trap a
// copied slide taught.
func slideParagraphStyleOf(style *google.DocsParagraphStyle) *google.ParagraphStyle {
	if style == nil {
		return nil
	}

	converted := &google.ParagraphStyle{
		Alignment: style.Alignment, Direction: style.Direction,
		SpaceAbove: style.SpaceAbove, SpaceBelow: style.SpaceBelow,
	}
	if style.LineSpacing != nil {
		converted.LineSpacing = *style.LineSpacing
	}

	return converted
}

func (r *registry) docsCopyTableFromSheets(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	targetID, err := requiredString(req, "target_document_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	rows, lost, err := r.readRectangle(ctx, client, req)
	if err != nil {
		return toolError(err), nil
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return toolError(fmt.Errorf("the rectangle has no cells")), nil
	}

	document, err := client.Document(ctx, targetID)
	if err != nil {
		return toolError(err), nil
	}

	at := int64(req.GetInt("target_index", 0))
	if _, given := req.GetArguments()["target_index"]; !given {
		at = docsEndOfBody(document)
	}
	if at < 1 {
		return toolError(fmt.Errorf("target_index is %d: a document's own content starts at 1", at)), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, targetID, []google.DocsRequest{{
		InsertTable: &google.DocsInsertTable{
			Rows: len(rows), Columns: len(rows[0]),
			Location: &google.DocsLocation{Index: at},
		},
	}}); err != nil {
		return toolError(err), nil
	}

	// The second pass, and it cannot be avoided: a table's cells have no indices until the
	// table exists, and those indices are not predictable from the request that made it.
	// Reading the document back is how the cells are found; guessing is how text lands in
	// the wrong ones.
	after, err := client.Document(ctx, targetID)
	if err != nil {
		return toolError(err), nil
	}

	table := docsTableAt(after, at)
	if table == nil {
		return toolError(fmt.Errorf("the table was made but cannot be found again in the document, " +
			"so its cells were left empty: read it with gdocs_docs_read_structure and fill it with " +
			"gdocs_docs_edit_table")), nil
	}

	requests := docsFillTable(table, rows)
	if len(requests) > 0 {
		if _, err := client.DocsBatchUpdate(ctx, targetID, requests); err != nil {
			return toolError(err), nil
		}
	}

	return resultJSON(withLosses(map[string]any{
		"target_document_id": targetID,
		"target_index":       at,
		"rows":               len(rows),
		"columns":            len(rows[0]),
		"requests":           len(requests) + 1,
	}, lost))
}

// docsTableAt finds the table that starts at or just after an index.
func docsTableAt(document *google.Document, at int64) *google.StructuralElement {
	if document.Body == nil {
		return nil
	}

	for index := range document.Body.Content {
		element := &document.Body.Content[index]
		if element.Table == nil || element.StartIndex == nil {
			continue
		}
		if *element.StartIndex >= at {
			return element
		}
	}

	return nil
}

// docsFillTable writes the cells of a table that already exists.
//
// The cells are filled from the last one backwards, because every insertion moves everything
// after it: going forwards would make every index after the first cell wrong by the length of
// what was written into it.
func docsFillTable(table *google.StructuralElement, rows [][]gridCell) []google.DocsRequest {
	var requests []google.DocsRequest

	for rowIndex := len(table.Table.Content) - 1; rowIndex >= 0; rowIndex-- {
		row := &table.Table.Content[rowIndex]
		if rowIndex >= len(rows) {
			continue
		}

		for cellIndex := len(row.Cells) - 1; cellIndex >= 0; cellIndex-- {
			if cellIndex >= len(rows[rowIndex]) {
				continue
			}
			cell := rows[rowIndex][cellIndex]
			at := docsCellStart(&row.Cells[cellIndex])
			if at == 0 {
				continue
			}

			if cell.fill != nil {
				requests = append(requests, google.DocsRequest{
					UpdateTableCellStyle: &google.DocsUpdateTableCellStyle{
						Range: &google.DocsTableRange{
							CellLocation: google.DocsTableCellLocation{
								TableStart:  google.DocsLocation{Index: *table.StartIndex},
								RowIndex:    rowIndex,
								ColumnIndex: cellIndex,
							},
							RowSpan: 1, ColumnSpan: 1,
						},
						Style: &google.DocsTableCellStyle{
							BackgroundColor: &google.DocsColor{
								Color: &google.DocsColorValue{RGBColor: cell.fill},
							},
						},
						Fields: "backgroundColor",
					},
				})
			}

			if cell.text == "" {
				continue
			}

			requests = append(requests, google.DocsRequest{
				InsertText: &google.DocsInsertText{
					Location: &google.DocsLocation{Index: at}, Text: cell.text,
				},
			})

			length := utf16Length(cell.text)
			if style, fields := docsCellTextStyle(&cell); fields != "" {
				requests = append(requests, google.DocsRequest{
					UpdateTextStyle: &google.DocsUpdateTextStyle{
						Range:  google.DocsRange{StartIndex: at, EndIndex: at + length},
						Style:  style,
						Fields: fields,
					},
				})
			}

			if alignment := docsAlignment(cell.alignment); alignment != "" {
				requests = append(requests, google.DocsRequest{
					UpdateParagraph: &google.DocsUpdateParagraph{
						Range:  google.DocsRange{StartIndex: at, EndIndex: at + length},
						Style:  &google.DocsParagraphStyle{Alignment: alignment},
						Fields: "alignment",
					},
				})
			}
		}
	}

	return requests
}

// docsCellStart is where text goes inside an empty cell: after the cell's own paragraph mark.
func docsCellStart(cell *google.DocsTableCell) int64 {
	if cell.StartIndex == nil {
		return 0
	}

	return *cell.StartIndex + 1
}

// docsCellTextStyle is a workbook cell's look as a document's text style.
func docsCellTextStyle(cell *gridCell) (*google.DocsTextStyle, string) {
	style := &google.DocsTextStyle{
		Bold: cell.bold, Italic: cell.italic,
		Underline: cell.underline, Strikethrough: cell.strike,
	}
	if cell.font != "" {
		style.WeightedFont = &google.WeightedFontFamily{FontFamily: cell.font}
	}
	if cell.sizePT > 0 {
		style.FontSize = google.PT(cell.sizePT)
	}
	if cell.colour != nil {
		style.ForegroundColor = &google.DocsColor{Color: &google.DocsColorValue{RGBColor: cell.colour}}
	}

	return style, docsTextStyleFields(style)
}

// docsAlignment turns a spreadsheet's alignment into the word a document uses.
func docsAlignment(value string) string {
	switch value {
	case "LEFT":
		return "START"
	case "CENTER":
		return "CENTER"
	case "RIGHT":
		return "END"
	}

	return ""
}

func (r *registry) docsCopySlideImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID, err := requiredString(req, "source_presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "source_page_object_id")
	if err != nil {
		return toolError(err), nil
	}
	targetID, err := requiredString(req, "target_document_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	thumbnail, err := client.Thumbnail(ctx, sourceID, pageObjectID, "PNG",
		strings.ToUpper(req.GetString("size", "MEDIUM")))
	if err != nil {
		return toolError(err), nil
	}
	if thumbnail.ContentURL == "" {
		return toolError(fmt.Errorf("no rendering came back for that slide")), nil
	}

	document, err := client.Document(ctx, targetID)
	if err != nil {
		return toolError(err), nil
	}

	at := int64(req.GetInt("target_index", 0))
	if _, given := req.GetArguments()["target_index"]; !given {
		at = docsEndOfBody(document)
	}
	if at < 1 {
		return toolError(fmt.Errorf("target_index is %d: a document's own content starts at 1", at)), nil
	}

	insert := &google.DocsInsertInlineImage{
		URI:      thumbnail.ContentURL,
		Location: &google.DocsLocation{Index: at},
	}

	// A width given on its own keeps the slide's proportions, which is what a reader expects
	// of a picture of a slide: a squashed one reads as a mistake rather than as a choice.
	if width := req.GetFloat("width_pt", 0); width > 0 {
		height := width
		if thumbnail.Width > 0 && thumbnail.Height > 0 {
			height = width * float64(thumbnail.Height) / float64(thumbnail.Width)
		}
		insert.Size = &google.DocsSize{Width: google.PT(width), Height: google.PT(height)}
	}

	if _, err := client.DocsBatchUpdate(ctx, targetID, []google.DocsRequest{
		{InsertInlineImage: insert},
	}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"source_presentation_id": sourceID,
		"source_page_object_id":  pageObjectID,
		"target_document_id":     targetID,
		"target_index":           at,
		"rendered":               fmt.Sprintf("%dx%d", thumbnail.Width, thumbnail.Height),
		"note": "this is a snapshot: it stops following the slide the moment it is taken, so a " +
			"slide changed afterwards leaves the document showing the old one",
	})
}

func (r *registry) sheetsCopyTableFromDocs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	targetID, err := requiredString(req, "target_spreadsheet_id")
	if err != nil {
		return toolError(err), nil
	}
	targetTitle, err := requiredString(req, "target_sheet_title")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	rows, lost, err := r.readSourceTable(ctx, client, req)
	if err != nil {
		return toolError(err), nil
	}
	if len(rows) == 0 {
		return toolError(fmt.Errorf("that table has no rows")), nil
	}

	targetRow, targetColumn := req.GetInt("target_row", 0), req.GetInt("target_column", 0)
	if targetRow < 0 || targetColumn < 0 {
		return toolError(fmt.Errorf("target_row and target_column are counted from 0")), nil
	}

	values := make([][]any, 0, len(rows))
	for _, row := range rows {
		line := make([]any, 0, len(row))
		for _, cell := range row {
			line = append(line, cell)
		}
		values = append(values, line)
	}

	// USER_ENTERED rather than RAW: a cell that reads as a number becomes one, and a column of
	// them can be summed. Written raw, every figure out of a document arrives as text and the
	// first sum over it is zero.
	a1 := google.A1Range(targetTitle,
		google.ColumnLetters(targetColumn)+fmt.Sprint(targetRow+1))
	updated, err := client.UpdateValues(ctx, targetID, a1, values, "USER_ENTERED")
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(withLosses(map[string]any{
		"target_spreadsheet_id": targetID,
		"target_sheet_title":    targetTitle,
		"target_range":          updated.UpdatedRange,
		"rows":                  len(values),
		"cells":                 updated.UpdatedCells,
	}, lost))
}

// readSourceTable reads a table out of whichever kind of document holds it.
func (r *registry) readSourceTable(ctx context.Context, client *google.Client, req mcp.CallToolRequest) ([][]string, []string, error) {
	documentID := optionalString(req, "source_document_id")
	presentationID := optionalString(req, "source_presentation_id")

	switch {
	case documentID != "" && presentationID != "":
		return nil, nil, fmt.Errorf("name one source: a document or a presentation, not both")
	case documentID != "":
		return docsTableRows(ctx, client, req, documentID)
	case presentationID != "":
		return slideTableRows(ctx, client, req, presentationID)
	}

	return nil, nil, fmt.Errorf("name where the table is: source_document_id with " +
		"table_start_index, or source_presentation_id with source_object_id")
}

// docsTableRows reads a document's table as plain rows of text.
func docsTableRows(ctx context.Context, client *google.Client, req mcp.CallToolRequest, documentID string) ([][]string, []string, error) {
	at, err := req.RequireInt("table_start_index")
	if err != nil {
		return nil, nil, fmt.Errorf("with source_document_id, table_start_index says which table: %w", err)
	}

	document, err := client.Document(ctx, documentID)
	if err != nil {
		return nil, nil, err
	}

	table := docsTableAt(document, int64(at))
	if table == nil {
		return nil, nil, fmt.Errorf("no table at or after index %d in that document: "+
			"gdocs_docs_read_structure reports where each one starts", at)
	}

	var rows [][]string
	merged := false
	for rowIndex := range table.Table.Content {
		row := &table.Table.Content[rowIndex]
		line := make([]string, 0, len(row.Cells))
		for cellIndex := range row.Cells {
			cell := &row.Cells[cellIndex]
			if cell.Style != nil && (cell.Style.RowSpan > 1 || cell.Style.ColumnSpan > 1) {
				merged = true
			}
			line = append(line, strings.TrimSpace(docsCellText(cell)))
		}
		rows = append(rows, line)
	}

	return rows, tableLosses(merged), nil
}

// docsCellText is everything written in one cell of a document's table.
func docsCellText(cell *google.DocsTableCell) string {
	var text strings.Builder
	for index := range cell.Content {
		paragraph := cell.Content[index].Paragraph
		if paragraph == nil {
			continue
		}
		for item := range paragraph.Elements {
			if run := paragraph.Elements[item].TextRun; run != nil {
				text.WriteString(run.Content)
			}
		}
	}

	return text.String()
}

// slideTableRows reads a table off a slide as plain rows of text.
func slideTableRows(ctx context.Context, client *google.Client, req mcp.CallToolRequest, presentationID string) ([][]string, []string, error) {
	objectID, err := requiredString(req, "source_object_id")
	if err != nil {
		return nil, nil, fmt.Errorf("with source_presentation_id, source_object_id says which table: %w", err)
	}

	presentation, err := client.Presentation(ctx, presentationID, copyMask)
	if err != nil {
		return nil, nil, err
	}

	element := elementNamed(presentation, objectID)
	if element == nil || element.Table == nil {
		return nil, nil, fmt.Errorf("no table called %q on any slide of that presentation: "+
			"gdocs_slides_list reports which elements are tables", objectID)
	}

	var rows [][]string
	merged := false
	for rowIndex := range element.Table.TableRows {
		row := &element.Table.TableRows[rowIndex]
		line := make([]string, element.Table.Columns)
		for cellIndex := range row.TableCells {
			cell := &row.TableCells[cellIndex]
			if cell.RowSpan > 1 || cell.ColumnSpan > 1 {
				merged = true
			}
			if cell.Location == nil || cell.Location.ColumnIndex >= len(line) {
				continue
			}
			line[cell.Location.ColumnIndex] = strings.TrimSpace(slideCellText(cell))
		}
		rows = append(rows, line)
	}

	return rows, tableLosses(merged), nil
}

// slideCellText is everything written in one cell of a slide's table.
func slideCellText(cell *google.TableCell) string {
	if cell.Text == nil {
		return ""
	}

	var text strings.Builder
	for index := range cell.Text.TextElements {
		if run := cell.Text.TextElements[index].TextRun; run != nil {
			text.WriteString(run.Content)
		}
	}

	return text.String()
}

// tableLosses names what a table has and a range of cells does not.
func tableLosses(merged bool) []string {
	lost := []string{
		"the look of each cell — its fill, its borders, the weight of its text — which a table " +
			"carries and a range of values does not; format the range with gdocs_sheets_format_cells " +
			"if it has to match",
	}
	if merged {
		lost = append(lost, "merged cells: their text lands in the first cell they covered and the "+
			"rest arrive empty, because a merge in a table and a merge in a sheet are not the same shape")
	}

	return lost
}

// placedBox is where a bridged element goes, in EMU.
type placedBox struct{ x, y, width, height float64 }

func requiredBox(req mcp.CallToolRequest) (placedBox, error) {
	var placed placedBox
	for _, side := range []struct {
		name   string
		target *float64
	}{
		{"x_emu", &placed.x}, {"y_emu", &placed.y},
		{"width_emu", &placed.width}, {"height_emu", &placed.height},
	} {
		value, err := req.RequireFloat(side.name)
		if err != nil {
			return placedBox{}, err
		}
		*side.target = value
	}

	if placed.width <= 0 || placed.height <= 0 {
		return placedBox{}, fmt.Errorf("width_emu and height_emu are in EMU and have to be positive, "+
			"got %g and %g", placed.width, placed.height)
	}

	return placed, nil
}
