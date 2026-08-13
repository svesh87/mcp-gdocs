package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

const (
	docsRangeHelp = "Indexes count characters from the start of the segment, the way a reading reports " +
		"them, and the end is exclusive. They shift with every edit: take them from a reading made " +
		"after the last one."
	docsSegmentHelp = "Identifier of the header, footer or footnote to work in. Omit for the body itself."
	docsStyleHelp   = "Style object with the same keys a reading answers with. Only the keys present are " +
		"written; everything else keeps what it had."
)

// registerDocsWrite adds the writing half: the tools that put back what a reading
// reports. Between them they cover what Docs v1 can express — with two holes that are the
// API's rather than this server's, and they are named in the tools that meet them.
func (r *registry) registerDocsWrite(srv *server.MCPServer) {
	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_docs_style_text",
		mcp.WithDescription("Set how a stretch of text looks: weight, slant, underline, size, font, colours, "+
			"and the link it carries. "+docsRangeHelp),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("start_index", mcp.Required(), mcp.Description("First character of the stretch.")),
		mcp.WithNumber("end_index", mcp.Required(), mcp.Description("One past its last character.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
		mcp.WithObject("style", mcp.Required(), mcp.Description(docsStyleHelp+
			" Keys: bold, italic, underline, strikethrough, small_caps, baseline_offset, font_size_pt, "+
			"font_family, font_weight, color, background_color, link. Colours are \"#RRGGBB\" or \"none\".")),
	), r.docsStyleText)

	srv.AddTool(mcp.NewTool("gdocs_docs_style_paragraph",
		mcp.WithDescription("Set how the paragraphs a range touches are laid out: their named style, "+
			"alignment, indents, spacing, borders, shading and page-break behaviour. A range that "+
			"touches a paragraph at all restyles the whole of it. "+docsRangeHelp),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("start_index", mcp.Required(), mcp.Description("Start of the range.")),
		mcp.WithNumber("end_index", mcp.Required(), mcp.Description("End of the range, exclusive.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
		mcp.WithObject("style", mcp.Required(), mcp.Description(docsStyleHelp+
			" Keys: named_style (NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1…6), alignment, direction, "+
			"spacing_mode, line_spacing, space_above_pt, space_below_pt, indent_start_pt, "+
			"indent_end_pt, indent_first_line_pt, keep_lines_together, keep_with_next, "+
			"avoid_widow_and_orphan, page_break_before, shading_color, and border_top / border_bottom "+
			"/ border_left / border_right / border_between as objects of color, width_pt, padding_pt, "+
			"dash_style — null takes a border off.")),
	), r.docsStyleParagraph)

	srv.AddTool(mcp.NewTool("gdocs_docs_make_bullets",
		mcp.WithDescription("Turn the paragraphs a range touches into a list. The depth of an item comes "+
			"from the tab characters its text starts with, not from an indent: one tab is the second "+
			"level. The glyphs come from a preset — Docs has no request that sets a glyph directly, so "+
			"a sample's own bullet character can only be matched by picking the preset that uses it. "+
			"Making bullets also rewrites the paragraph's indents, so do it before setting them."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("start_index", mcp.Required(), mcp.Description("Start of the range.")),
		mcp.WithNumber("end_index", mcp.Required(), mcp.Description("End of the range, exclusive.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
		mcp.WithString("preset", mcp.DefaultString("BULLET_DISC_CIRCLE_SQUARE"), mcp.Description(
			"Which glyphs the levels get, e.g. BULLET_DISC_CIRCLE_SQUARE (● ○ ■), "+
				"BULLET_ARROW_DIAMOND_DISC, BULLET_CHECKBOX, NUMBERED_DECIMAL_ALPHA_ROMAN.")),
	), r.docsMakeBullets)

	srv.AddTool(mcp.NewTool("gdocs_docs_insert_table",
		mcp.WithDescription("Put an empty table into a document. Its cells are filled afterwards, by "+
			"writing text at the indexes a fresh reading reports — a table shifts everything after it, "+
			"and its own cells start one index apart from where they look like they should."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("rows", mcp.Required(), mcp.Description("How many rows.")),
		mcp.WithNumber("columns", mcp.Required(), mcp.Description("How many columns.")),
		mcp.WithNumber("index", mcp.Description("Where to put it. Omit to put it at the end of the segment.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
	), r.docsInsertTable)

	srv.AddTool(mcp.NewTool("gdocs_docs_style_table",
		mcp.WithDescription("Give a table its look: cell fills, borders, padding and vertical alignment, "+
			"column widths, row heights, merged cells, and how many rows repeat as a header on each "+
			"page. The table is named by the index its own reading reports as the table's start."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("table_start_index", mcp.Required(), mcp.Description(
			"Index the reading gives as the table element's start.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
		mcp.WithArray("cells", mcp.Description(
			"Rectangles of cells to paint: {\"row\": 0, \"column\": 0, \"row_span\": 1, \"column_span\": 2, "+
				"\"style\": {…}}. The style takes the keys a reading reports: background_color, "+
				"content_alignment, padding_*_pt, border_top/bottom/left/right."),
			mcp.Items(map[string]any{"type": "object"})),
		mcp.WithArray("column_widths", mcp.Description(
			"Column widths in points: {\"columns\": [0, 1], \"width_pt\": 233.5}. A width only applies "+
				"with width_type FIXED_WIDTH, which is what this sends."),
			mcp.Items(map[string]any{"type": "object"})),
		mcp.WithArray("row_heights", mcp.Description(
			"Row heights in points: {\"rows\": [0], \"min_height_pt\": 116.2, \"table_header\": false}. "+
				"The height is a minimum — a row with more text in it grows."),
			mcp.Items(map[string]any{"type": "object"})),
		mcp.WithArray("merge", mcp.Description(
			"Rectangles to merge: {\"row\": 0, \"column\": 0, \"row_span\": 2, \"column_span\": 1}."),
			mcp.Items(map[string]any{"type": "object"})),
		mcp.WithNumber("pin_header_rows", mcp.Description(
			"How many rows from the top repeat on every page.")),
	), r.docsStyleTable)

	srv.AddTool(mcp.NewTool("gdocs_docs_insert_section_break",
		mcp.WithDescription("Start a new section. A section is what carries page setup and its own headers "+
			"and footers, so a document whose second half has a different footer is a document with a "+
			"section break in the middle."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("section_type", mcp.DefaultString("NEXT_PAGE"), mcp.Description(
			"NEXT_PAGE starts the section on a new page; CONTINUOUS keeps it on the same one.")),
		mcp.WithNumber("index", mcp.Description("Where to put it. Omit for the end of the body.")),
	), r.docsInsertSectionBreak)

	srv.AddTool(mcp.NewTool("gdocs_docs_style_section",
		mcp.WithDescription("Set the page setup of the section a range falls in: margins, columns, and "+
			"which header and footer it uses."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("start_index", mcp.Required(), mcp.Description("Any index inside the section.")),
		mcp.WithNumber("end_index", mcp.Required(), mcp.Description("End of the range, exclusive.")),
		mcp.WithObject("style", mcp.Required(), mcp.Description(docsStyleHelp+
			" Keys: column_separator, direction, margin_*_pt, default_header_id, default_footer_id, "+
			"first_page_header_id, first_page_footer_id, even_page_header_id, even_page_footer_id, "+
			"use_first_page_header_footer, flip_page_orientation, page_number_start.")),
	), r.docsStyleSection)

	srv.AddTool(mcp.NewTool("gdocs_docs_add_header_footer",
		mcp.WithDescription("Make a header or a footer and hand back its identifier, which is the "+
			"segment_id the text tools write into. Without a section break index it belongs to the "+
			"document; with one it belongs to that section alone, which is how the halves of a "+
			"document end up with different footers. "+
			"Only the ordinary kind can be made: Docs v1 knows one type, DEFAULT. A document whose "+
			"first page has its own header was made that way in the editor, and the way to that "+
			"effect through the API is a section — the first page as a section of its own, with the "+
			"rest given theirs. There is one per section, and a second attempt is refused rather "+
			"than replacing it."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("kind", mcp.Required(), mcp.Description("header or footer.")),
		mcp.WithNumber("section_break_index", mcp.Description(
			"Index of the section break this belongs to. Omit for the document's own.")),
	), r.docsAddHeaderFooter)

	srv.AddTool(mcp.NewTool("gdocs_docs_style_document",
		mcp.WithDescription("Set the page setup of the whole document: paper size, margins, background, "+
			"and whether the first page has its own header and footer."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithObject("style", mcp.Required(), mcp.Description(docsStyleHelp+
			" Keys: page_size {width_pt, height_pt}, background_color, margin_top_pt, margin_bottom_pt, "+
			"margin_left_pt, margin_right_pt, margin_header_pt, margin_footer_pt, "+
			"use_custom_header_footer_margins, use_first_page_header_footer, "+
			"use_even_page_header_footer, flip_page_orientation, page_number_start, and the header and "+
			"footer identifiers.")),
	), r.docsStyleDocument)

	srv.AddTool(mcp.NewTool("gdocs_docs_style_named",
		mcp.WithDescription("Change what a named style means in this document — what NORMAL_TEXT or "+
			"HEADING_1 look like. This is how a copy gets the sample's body font instead of the new "+
			"document's default, and it applies to every paragraph carrying that style at once."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("named_style", mcp.Required(), mcp.Description(
			"NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1 … HEADING_6.")),
		mcp.WithObject("text_style", mcp.Description("Text style keys, as in style_text.")),
		mcp.WithObject("paragraph_style", mcp.Description("Paragraph style keys, as in style_paragraph.")),
	), r.docsStyleNamed)

	srv.AddTool(mcp.NewTool("gdocs_docs_insert_image",
		mcp.WithDescription("Put a picture into the text, from an address Google can reach — including "+
			"the content_uri a reading reports for a picture in another document, which is how a "+
			"picture is carried over without going through a file. That address is signed and expires, "+
			"so read and insert in one go. The picture lands in the line of text: Docs v1 has no "+
			"request that makes a floating one, and none that changes a picture's margins, border or "+
			"crop afterwards."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("uri", mcp.Required(), mcp.Description("Address of the picture, http or https.")),
		mcp.WithNumber("index", mcp.Description("Where to put it. Omit for the end of the segment.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
		mcp.WithNumber("width_pt", mcp.Description("Width in points. Docs keeps the picture's own ratio, "+
			"so the height may come out a little different from what was asked.")),
		mcp.WithNumber("height_pt", mcp.Description("Height in points.")),
	), r.docsInsertImage)

	srv.AddTool(mcp.NewTool("gdocs_docs_insert_page_break",
		mcp.WithDescription("Start a new page at one place in the text."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("index", mcp.Description("Where to break. Omit for the end of the segment.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
	), r.docsInsertPageBreak)

	srv.AddTool(mcp.NewTool("gdocs_docs_insert_footnote",
		mcp.WithDescription("Put a footnote reference into the text and hand back the identifier of the "+
			"footnote it made, which is the segment_id its text is then written into."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("index", mcp.Description("Where the reference goes. Omit for the end of the body.")),
	), r.docsInsertFootnote)
}

func (r *registry) docsStyleText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, textRange, err := docsRangeArgs(req)
	if err != nil {
		return toolError(err), nil
	}

	object, err := objectArg(req, "style")
	if err != nil {
		return toolError(err), nil
	}

	style, fields, err := docsTextStyleFrom(object)
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		UpdateTextStyle: &google.DocsUpdateTextStyle{Range: textRange, Style: style, Fields: fields},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"start_index": textRange.StartIndex,
		"end_index":   textRange.EndIndex,
		"fields":      fields,
	})
}

func (r *registry) docsStyleParagraph(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, textRange, err := docsRangeArgs(req)
	if err != nil {
		return toolError(err), nil
	}

	object, err := objectArg(req, "style")
	if err != nil {
		return toolError(err), nil
	}

	style, fields, err := docsParagraphStyleFrom(object)
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		UpdateParagraph: &google.DocsUpdateParagraph{Range: textRange, Style: style, Fields: fields},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"start_index": textRange.StartIndex,
		"end_index":   textRange.EndIndex,
		"fields":      fields,
	})
}

func (r *registry) docsMakeBullets(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, textRange, err := docsRangeArgs(req)
	if err != nil {
		return toolError(err), nil
	}

	preset := optionalString(req, "preset")
	if preset == "" {
		preset = "BULLET_DISC_CIRCLE_SQUARE"
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		CreateBullets: &google.DocsCreateBullets{Range: textRange, BulletPreset: preset},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"start_index": textRange.StartIndex,
		"end_index":   textRange.EndIndex,
		"preset":      preset,
	})
}

func (r *registry) docsInsertTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	rows, err := req.RequireInt("rows")
	if err != nil {
		return toolError(err), nil
	}
	columns, err := req.RequireInt("columns")
	if err != nil {
		return toolError(err), nil
	}
	if rows < 1 || columns < 1 {
		return toolError(fmt.Errorf("a table needs at least one row and one column, got %d by %d", rows, columns)), nil
	}

	location, end := docsPlaceArgs(req)

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		InsertTable: &google.DocsInsertTable{
			Rows: rows, Columns: columns, Location: location, EndOfDoc: end,
		},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"rows":        rows,
		"columns":     columns,
	})
}

func (r *registry) docsStyleTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	start, err := req.RequireInt("table_start_index")
	if err != nil {
		return toolError(err), nil
	}

	tableStart := google.DocsLocation{Index: int64(start), SegmentID: optionalString(req, "segment_id")}

	requests, err := docsTableRequests(req, tableStart)
	if err != nil {
		return toolError(err), nil
	}
	if len(requests) == 0 {
		return toolError(fmt.Errorf("nothing to do: give cells, column_widths, row_heights, merge or pin_header_rows")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, requests); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id":       documentID,
		"table_start_index": start,
		"requests":          len(requests),
	})
}

// docsTableRequests builds a table's styling in the order that survives: the merges last,
// because a merge changes which cells exist and every rectangle named before it was named
// against the table as it is now.
func docsTableRequests(req mcp.CallToolRequest, tableStart google.DocsLocation) ([]google.DocsRequest, error) {
	var requests []google.DocsRequest

	cells, err := optionalObjectList(req, "cells")
	if err != nil {
		return nil, err
	}
	for index, cell := range cells {
		styleObject, ok := cell["style"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cells[%d] needs a style object", index)
		}
		style, fields, err := docsCellStyleFrom(styleObject)
		if err != nil {
			return nil, fmt.Errorf("cells[%d]: %w", index, err)
		}
		tableRange, err := docsTableRangeFrom(cell, tableStart, fmt.Sprintf("cells[%d]", index))
		if err != nil {
			return nil, err
		}
		requests = append(requests, google.DocsRequest{
			UpdateTableCellStyle: &google.DocsUpdateTableCellStyle{
				Range: tableRange, Style: style, Fields: fields,
			},
		})
	}

	widths, err := optionalObjectList(req, "column_widths")
	if err != nil {
		return nil, err
	}
	for index, entry := range widths {
		width, ok, err := fieldFloat(entry, "width_pt")
		if err != nil {
			return nil, fmt.Errorf("column_widths[%d]: %w", index, err)
		}
		if !ok {
			return nil, fmt.Errorf("column_widths[%d] needs width_pt", index)
		}
		columns, err := intListField(entry, "columns")
		if err != nil {
			return nil, fmt.Errorf("column_widths[%d]: %w", index, err)
		}
		requests = append(requests, google.DocsRequest{
			UpdateTableColumn: &google.DocsUpdateTableColumn{
				Start:   tableStart,
				Indices: columns,
				// A width with no type is ignored: Docs keeps distributing the columns
				// evenly and the number goes nowhere.
				Properties: &google.DocsTableColumnProperties{Width: points(*width), WidthType: "FIXED_WIDTH"},
				Fields:     "width,widthType",
			},
		})
	}

	heights, err := optionalObjectList(req, "row_heights")
	if err != nil {
		return nil, err
	}
	for index, entry := range heights {
		rows, err := intListField(entry, "rows")
		if err != nil {
			return nil, fmt.Errorf("row_heights[%d]: %w", index, err)
		}

		style := &google.DocsTableRowStyle{}
		mask := &docsStyleFields{}
		if height, ok, err := fieldFloat(entry, "min_height_pt"); err != nil {
			return nil, fmt.Errorf("row_heights[%d]: %w", index, err)
		} else if ok {
			style.MinRowHeight = points(*height)
			mask.add("minRowHeight")
		}
		// A reading reports table_header, and this request refuses it: "Unallowed field:
		// tableHeader". Which rows repeat on each page is settled by pin_header_rows,
		// which pins them from the top rather than marking one row anywhere.
		if _, given := entry["table_header"]; given {
			return nil, fmt.Errorf("row_heights[%d]: table_header cannot be written here — Docs answers "+
				"\"Unallowed field\". Use pin_header_rows to repeat the first rows on every page", index)
		}
		if prevent, ok, err := fieldBool(entry, "prevent_overflow"); err != nil {
			return nil, fmt.Errorf("row_heights[%d]: %w", index, err)
		} else if ok {
			style.PreventOverflow = prevent
			mask.add("preventOverflow")
		}
		if len(mask.fields) == 0 {
			return nil, fmt.Errorf("row_heights[%d] names nothing: give min_height_pt, table_header or prevent_overflow", index)
		}

		requests = append(requests, google.DocsRequest{
			UpdateTableRow: &google.DocsUpdateTableRow{
				Start: tableStart, Indices: rows, Style: style, Fields: mask.mask(),
			},
		})
	}

	merges, err := optionalObjectList(req, "merge")
	if err != nil {
		return nil, err
	}
	for index, entry := range merges {
		tableRange, err := docsTableRangeFrom(entry, tableStart, fmt.Sprintf("merge[%d]", index))
		if err != nil {
			return nil, err
		}
		requests = append(requests, google.DocsRequest{
			MergeTableCells: &google.DocsMergeTableCells{Range: *tableRange},
		})
	}

	if pinned := req.GetInt("pin_header_rows", -1); pinned >= 0 {
		requests = append(requests, google.DocsRequest{
			PinTableHeaderRows: &google.DocsPinTableHeaderRows{Start: tableStart, Count: pinned},
		})
	}

	return requests, nil
}

func docsTableRangeFrom(object map[string]any, tableStart google.DocsLocation, where string) (*google.DocsTableRange, error) {
	row, ok, err := fieldFloat(object, "row")
	if err != nil || !ok {
		return nil, fmt.Errorf("%s needs a row", where)
	}
	column, ok, err := fieldFloat(object, "column")
	if err != nil || !ok {
		return nil, fmt.Errorf("%s needs a column", where)
	}

	rowSpan := 1.0
	if value, ok, err := fieldFloat(object, "row_span"); err != nil {
		return nil, fmt.Errorf("%s: %w", where, err)
	} else if ok {
		rowSpan = *value
	}
	columnSpan := 1.0
	if value, ok, err := fieldFloat(object, "column_span"); err != nil {
		return nil, fmt.Errorf("%s: %w", where, err)
	} else if ok {
		columnSpan = *value
	}

	return &google.DocsTableRange{
		CellLocation: google.DocsTableCellLocation{
			TableStart: tableStart, RowIndex: int(*row), ColumnIndex: int(*column),
		},
		RowSpan:    int(rowSpan),
		ColumnSpan: int(columnSpan),
	}, nil
}

func (r *registry) docsInsertSectionBreak(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	sectionType := optionalString(req, "section_type")
	if sectionType == "" {
		sectionType = "NEXT_PAGE"
	}

	location, end := docsPlaceArgs(req)

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		InsertSectionBreak: &google.DocsInsertSectionBreak{
			SectionType: sectionType, Location: location, EndOfDoc: end,
		},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id":  documentID,
		"section_type": sectionType,
	})
}

func (r *registry) docsStyleSection(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, textRange, err := docsRangeArgs(req)
	if err != nil {
		return toolError(err), nil
	}

	object, err := objectArg(req, "style")
	if err != nil {
		return toolError(err), nil
	}

	style, fields, err := docsSectionStyleFrom(object)
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		UpdateSectionStyle: &google.DocsUpdateSectionStyle{Range: textRange, Style: style, Fields: fields},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"fields":      fields,
	})
}

func (r *registry) docsAddHeaderFooter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	kind, err := requiredString(req, "kind")
	if err != nil {
		return toolError(err), nil
	}

	// The enum has two values, one of which is "unspecified": DEFAULT is the whole of what
	// can be made. Asked for FIRST_PAGE, Google does not refuse the name — it fails to
	// read the field at all and answers "Header type not specified", which is a puzzle
	// worth not handing on.
	if headerType := optionalString(req, "type"); headerType != "" && headerType != "DEFAULT" {
		return toolError(fmt.Errorf("Docs can only make a DEFAULT %s, not %q. A header or footer that "+
			"only the first page shows is made in the editor; through the API the same effect is a "+
			"section break after the first page, with a %s of its own on each side",
			kind, headerType, kind)), nil
	}

	const headerType = "DEFAULT"

	var breakLocation *google.DocsLocation
	if index := req.GetInt("section_break_index", -1); index >= 0 {
		breakLocation = &google.DocsLocation{Index: int64(index)}
	}

	request := google.DocsRequest{}
	switch kind {
	case "header":
		request.CreateHeader = &google.DocsCreateHeaderFooter{Type: headerType, SectionBreakLocation: breakLocation}
	case "footer":
		request.CreateFooter = &google.DocsCreateHeaderFooter{Type: headerType, SectionBreakLocation: breakLocation}
	default:
		return toolError(fmt.Errorf("kind must be header or footer, got %q", kind)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{request})
	if err != nil {
		return toolError(err), nil
	}

	segment := ""
	for _, reply := range response.Replies {
		if reply.CreateHeader != nil {
			segment = reply.CreateHeader.HeaderID
		}
		if reply.CreateFooter != nil {
			segment = reply.CreateFooter.FooterID
		}
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"kind":        kind,
		"type":        headerType,
		"segment_id":  segment,
	})
}

func (r *registry) docsStyleDocument(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	object, err := objectArg(req, "style")
	if err != nil {
		return toolError(err), nil
	}

	style, fields, err := docsDocumentStyleFrom(object)
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		UpdateDocumentStyle: &google.DocsUpdateDocumentStyle{Style: style, Fields: fields},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"fields":      fields,
	})
}

func (r *registry) docsStyleNamed(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	namedStyle, err := requiredString(req, "named_style")
	if err != nil {
		return toolError(err), nil
	}

	style := &google.DocsNamedStyle{NamedStyleType: namedStyle}
	mask := &docsStyleFields{}

	// The type has to be in the mask as well as in the style. Left out of it, Google does
	// not read the field at all and answers "Named style type is required" about a request
	// that plainly carries one.
	mask.add("namedStyleType")

	if object, ok := req.GetArguments()["text_style"].(map[string]any); ok {
		text, fields, err := docsTextStyleFrom(object)
		if err != nil {
			return toolError(err), nil
		}
		style.TextStyle = text
		for _, field := range splitFields(fields) {
			mask.add("textStyle." + field)
		}
	}

	if object, ok := req.GetArguments()["paragraph_style"].(map[string]any); ok {
		// A reading of a named style reports named_style inside its paragraph style too,
		// and handing that back is refused: "Unallowed field: paragraphStyle.namedStyleType".
		// Which style is being changed is settled by the named_style argument.
		if _, ok := object["named_style"]; ok {
			return toolError(fmt.Errorf("paragraph_style must not carry named_style: the style being " +
				"changed is the one named in the named_style argument, and Docs refuses it inside")), nil
		}

		paragraph, fields, err := docsParagraphStyleFrom(object)
		if err != nil {
			return toolError(err), nil
		}
		style.ParagraphStyle = paragraph
		for _, field := range splitFields(fields) {
			mask.add("paragraphStyle." + field)
		}
	}

	if len(mask.fields) == 1 {
		return toolError(fmt.Errorf("give a text_style, a paragraph_style, or both")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		UpdateNamedStyle: &google.DocsUpdateNamedStyle{Style: style, Fields: mask.mask()},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"named_style": namedStyle,
		"fields":      mask.mask(),
	})
}

func (r *registry) docsInsertImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	uri, err := requiredString(req, "uri")
	if err != nil {
		return toolError(err), nil
	}

	location, end := docsPlaceArgs(req)

	var size *google.DocsSize
	width := req.GetFloat("width_pt", 0)
	height := req.GetFloat("height_pt", 0)
	if width > 0 || height > 0 {
		size = &google.DocsSize{}
		if width > 0 {
			size.Width = points(width)
		}
		if height > 0 {
			size.Height = points(height)
		}
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		InsertInlineImage: &google.DocsInsertInlineImage{
			URI: uri, Location: location, EndOfDoc: end, Size: size,
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	objectID := ""
	for _, reply := range response.Replies {
		if reply.InsertInlineImage != nil {
			objectID = reply.InsertInlineImage.ObjectID
		}
	}

	return resultJSON(map[string]any{
		"document_id":      documentID,
		"inline_object_id": objectID,
	})
}

func (r *registry) docsInsertPageBreak(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	location, end := docsPlaceArgs(req)

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		InsertPageBreak: &google.DocsInsertPageBreak{Location: location, EndOfDoc: end},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"document_id": documentID})
}

func (r *registry) docsInsertFootnote(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	location, end := docsPlaceArgs(req)

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		CreateFootnote: &google.DocsCreateFootnote{Location: location, EndOfDoc: end},
	}})
	if err != nil {
		return toolError(err), nil
	}

	segment := ""
	for _, reply := range response.Replies {
		if reply.CreateFootnote != nil {
			segment = reply.CreateFootnote.FootnoteID
		}
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"segment_id":  segment,
	})
}

// docsRangeArgs reads the document and the stretch of it a call works on.
func docsRangeArgs(req mcp.CallToolRequest) (string, google.DocsRange, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return "", google.DocsRange{}, err
	}

	start, err := req.RequireInt("start_index")
	if err != nil {
		return "", google.DocsRange{}, err
	}
	end, err := req.RequireInt("end_index")
	if err != nil {
		return "", google.DocsRange{}, err
	}

	// The body's own text starts at 1; a header, a footer or a footnote starts at 0. Both
	// refusals below are cheaper than the round trip that would earn them from Google.
	segment := optionalString(req, "segment_id")
	if start < 1 && segment == "" {
		return "", google.DocsRange{}, fmt.Errorf("start_index %d is before the first character: a document's own text starts at 1", start)
	}
	if start < 0 {
		return "", google.DocsRange{}, fmt.Errorf("start_index %d is before the start of the segment", start)
	}
	if end <= start {
		return "", google.DocsRange{}, fmt.Errorf("end_index %d must be past start_index %d", end, start)
	}

	return documentID, google.DocsRange{
		StartIndex: int64(start),
		EndIndex:   int64(end),
		SegmentID:  segment,
	}, nil
}

// docsPlaceArgs turns "index" and "segment_id" into the pair of fields every inserting
// request has: a place, or the end of a segment when no place was named.
func docsPlaceArgs(req mcp.CallToolRequest) (*google.DocsLocation, *google.DocsSegmentEnd) {
	segment := optionalString(req, "segment_id")

	// Presence decides, not the value: index 0 is a real place in a header or a footer,
	// where the text starts at 0 rather than at 1.
	if _, given := req.GetArguments()["index"]; given {
		return &google.DocsLocation{Index: int64(req.GetInt("index", 0)), SegmentID: segment}, nil
	}

	return nil, &google.DocsSegmentEnd{SegmentID: segment}
}

// objectArg reads an argument that has to be an object.
func objectArg(req mcp.CallToolRequest, name string) (map[string]any, error) {
	raw, ok := req.GetArguments()[name]
	if !ok {
		return nil, fmt.Errorf("%s is required", name)
	}

	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object, got %T", name, raw)
	}

	return object, nil
}

// optionalObjectList reads a list of objects that may be absent altogether.
func optionalObjectList(req mcp.CallToolRequest, name string) ([]map[string]any, error) {
	if _, ok := req.GetArguments()[name]; !ok {
		return nil, nil
	}

	return objectList(req, name)
}

// intListField reads a list of indexes out of a decoded object.
func intListField(object map[string]any, name string) ([]int, error) {
	raw, ok := object[name]
	if !ok {
		return nil, fmt.Errorf("%s is required", name)
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list of numbers, got %T", name, raw)
	}

	values := make([]int, 0, len(list))
	for index, item := range list {
		switch value := item.(type) {
		case float64:
			values = append(values, int(value))
		case int:
			values = append(values, value)
		default:
			return nil, fmt.Errorf("%s[%d] must be a number, got %T", name, index, item)
		}
	}

	return values, nil
}

// splitFields takes a mask apart again, so a named style can prefix each of its fields.
func splitFields(mask string) []string {
	if mask == "" {
		return nil
	}

	return strings.Split(mask, ",")
}
