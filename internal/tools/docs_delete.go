package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerDocsDelete adds removal inside a document, on the same terms as the one that
// already exists for presentations: what a document contains can be taken out, the
// document itself cannot be touched.
//
// The reason it exists at all is that building is not one-shot. A step that lands wrong
// leaves a paragraph, a table or a picture behind, and without this the only way back is
// a person opening the document in a browser. The reason it stops at the document's edge
// is unchanged: a file, a folder or a drive removed by an agent is an incident nobody
// can undo.
func (r *registry) registerDocsDelete(srv *server.MCPServer) {
	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_docs_delete",
		mcp.WithDescription("Remove something inside a document: a stretch of its content, a row or a "+
			"column of one of its tables, a header or a footer, a floating object, or the bullets of "+
			"a list. Exactly one of those per call. It reaches nothing outside the document — no "+
			"files, no folders, no other documents. Indexes come from a reading made after the last "+
			"edit: a stale one deletes the wrong text, and there is no undo on this side. Deleting a "+
			"range takes its paragraph marks with it, so removing a whole paragraph means removing "+
			"the newline that ends it too."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("start_index", mcp.Description("First character to remove.")),
		mcp.WithNumber("end_index", mcp.Description("One past the last character to remove.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
		mcp.WithNumber("table_start_index", mcp.Description(
			"Index of the table to take a row or a column out of.")),
		mcp.WithNumber("row", mcp.Description("Row to remove, or the row of the cell naming the column.")),
		mcp.WithNumber("column", mcp.Description("Column to remove, or the column of the cell naming the row.")),
		mcp.WithString("what", mcp.Description(
			"With a table: row or column. With a range: bullets to take the list glyphs off the "+
				"paragraphs and leave the text.")),
		mcp.WithString("header_id", mcp.Description("Header segment to remove, with everything in it.")),
		mcp.WithString("footer_id", mcp.Description("Footer segment to remove, with everything in it.")),
		mcp.WithString("positioned_object_id", mcp.Description("Floating object to remove.")),
		mcp.WithDestructiveHintAnnotation(true),
	), r.docsDelete)
}

func (r *registry) docsDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	request, described, err := docsDeleteRequest(req)
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{*request}); err != nil {
		return toolError(err), nil
	}

	described["document_id"] = documentID

	return resultJSON(described)
}

// docsDeleteRequest works out which single thing a call names, and refuses anything that
// names two or none. One call, one removal: a tool that can be talked into removing more
// than was asked for is the tool that eventually does.
func docsDeleteRequest(req mcp.CallToolRequest) (*google.DocsRequest, map[string]any, error) {
	segment := optionalString(req, "segment_id")
	what := optionalString(req, "what")

	headerID := optionalString(req, "header_id")
	footerID := optionalString(req, "footer_id")
	objectID := optionalString(req, "positioned_object_id")
	tableStart := req.GetInt("table_start_index", 0)
	start := req.GetInt("start_index", 0)
	end := req.GetInt("end_index", 0)

	named := 0
	for _, given := range []bool{headerID != "", footerID != "", objectID != "", tableStart > 0, start > 0 || end > 0} {
		if given {
			named++
		}
	}
	switch {
	case named == 0:
		return nil, nil, fmt.Errorf("name one thing to remove: a range with start_index and end_index, " +
			"a table with table_start_index and what=row or what=column, a header_id, a footer_id, " +
			"or a positioned_object_id")
	case named > 1:
		return nil, nil, fmt.Errorf("this call names more than one thing to remove; make one call each, " +
			"so what goes is what was meant")
	}

	switch {
	case headerID != "":
		return &google.DocsRequest{DeleteHeader: &google.DocsDeleteHeader{HeaderID: headerID}},
			map[string]any{"removed": "header", "header_id": headerID}, nil

	case footerID != "":
		return &google.DocsRequest{DeleteFooter: &google.DocsDeleteFooter{FooterID: footerID}},
			map[string]any{"removed": "footer", "footer_id": footerID}, nil

	case objectID != "":
		return &google.DocsRequest{DeletePositioned: &google.DocsDeletePositioned{ObjectID: objectID}},
			map[string]any{"removed": "positioned_object", "positioned_object_id": objectID}, nil

	case tableStart > 0:
		cell := google.DocsTableCellLocation{
			TableStart:  google.DocsLocation{Index: int64(tableStart), SegmentID: segment},
			RowIndex:    req.GetInt("row", 0),
			ColumnIndex: req.GetInt("column", 0),
		}
		switch what {
		case "row":
			return &google.DocsRequest{DeleteTableRow: &google.DocsDeleteTableRow{CellLocation: cell}},
				map[string]any{"removed": "table_row", "row": cell.RowIndex}, nil
		case "column":
			return &google.DocsRequest{DeleteTableColumn: &google.DocsDeleteTableColumn{CellLocation: cell}},
				map[string]any{"removed": "table_column", "column": cell.ColumnIndex}, nil
		default:
			return nil, nil, fmt.Errorf("with a table say what to remove: what=row or what=column, got %q", what)
		}

	default:
		if start < 1 && segment == "" {
			return nil, nil, fmt.Errorf("start_index %d is before the first character: a document's own text starts at 1", start)
		}
		if end <= start {
			return nil, nil, fmt.Errorf("end_index %d must be past start_index %d", end, start)
		}

		textRange := google.DocsRange{StartIndex: int64(start), EndIndex: int64(end), SegmentID: segment}

		if what == "bullets" {
			return &google.DocsRequest{DeleteBullets: &google.DocsDeleteBullets{Range: textRange}},
				map[string]any{"removed": "bullets", "start_index": start, "end_index": end}, nil
		}

		return &google.DocsRequest{DeleteContent: &google.DocsDeleteContent{Range: textRange}},
			map[string]any{"removed": "content", "start_index": start, "end_index": end}, nil
	}
}
