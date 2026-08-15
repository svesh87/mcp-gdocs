package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerDocsExtra adds the rest of what Docs v1 can do to a document: tabs, named
// ranges, the three kinds of chip, replacing a picture in place, and the two table
// operations that change a table's shape rather than its look.
//
// Named ranges deserve their line here. Every other way of addressing a document counts
// characters, and every insertion moves those numbers — a name does not. A template
// filled through named ranges survives being edited between the reading and the writing,
// which is the difference between a fill that works once and one that keeps working.
func (r *registry) registerDocsExtra(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_docs_list_named_ranges",
		mcp.WithDescription("List the named ranges of a document: the names, their identifiers, and where "+
			"each one currently is. A name is the one way of pointing at a place in a document that "+
			"survives editing — the indexes around it move, the name does not."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.docsListNamedRanges)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_docs_add_named_range",
		mcp.WithDescription("Give a stretch of the document a name, so later edits can find it without "+
			"counting characters. Several ranges may share a name; they are then filled together."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name for the range.")),
		mcp.WithNumber("start_index", mcp.Required(), mcp.Description("First character.")),
		mcp.WithNumber("end_index", mcp.Required(), mcp.Description("One past the last character.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
	), r.docsAddNamedRange)

	srv.AddTool(mcp.NewTool("gdocs_docs_fill_named_range",
		mcp.WithDescription("Replace what a named range holds with new text. This is how a template is "+
			"filled safely: no indexes are involved, so nothing lands in the wrong place however much "+
			"the document has changed since it was read."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("name", mcp.Description("Name of the range, or give range_id instead.")),
		mcp.WithString("range_id", mcp.Description("Identifier of one range, when several share a name.")),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to put there.")),
	), r.docsFillNamedRange)

	srv.AddTool(mcp.NewTool("gdocs_docs_add_tab",
		mcp.WithDescription("Add a tab to a document. Tabs are the pages in the editor's left rail: one "+
			"document, several independent bodies. A tab can sit under another one, which is how a "+
			"document grows a table of contents that is not text."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("title", mcp.Description("Name of the tab.")),
		mcp.WithString("parent_tab_id", mcp.Description("Tab to nest this one under.")),
		mcp.WithNumber("index", mcp.Description("Where among its siblings it goes, counting from 0.")),
		mcp.WithString("icon_emoji", mcp.Description("One emoji shown beside the tab's name.")),
	), r.docsAddTab)

	srv.AddTool(mcp.NewTool("gdocs_docs_update_tab",
		mcp.WithDescription("Rename a tab, move it among its siblings, or change its icon."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("tab_id", mcp.Required(), mcp.Description("Tab to change.")),
		mcp.WithString("title", mcp.Description("New name.")),
		mcp.WithNumber("index", mcp.Description("New position among its siblings.")),
		mcp.WithString("icon_emoji", mcp.Description(
			"New icon. An empty string takes the icon off; leaving the argument out keeps it.")),
	), r.docsUpdateTab)

	srv.AddTool(mcp.NewTool("gdocs_docs_insert_chip",
		mcp.WithDescription("Put a smart chip into the text: a person, another Google file, or a date. "+
			"A chip is not styled text — it stays live, so a person chip shows the same card it would "+
			"if somebody typed @ in the editor."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("kind", mcp.Required(), mcp.Description("person, file or date.")),
		mcp.WithNumber("index", mcp.Description("Where it goes. Omit for the end of the segment.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
		mcp.WithString("email", mcp.Description("For kind=person: the address the chip stands for.")),
		mcp.WithString("name", mcp.Description("For kind=person: the name to show.")),
		mcp.WithString("uri", mcp.Description("For kind=file: the address of the Google file.")),
		mcp.WithString("title", mcp.Description("For kind=file: the title to show.")),
		mcp.WithString("timestamp", mcp.Description(
			"For kind=date: an RFC 3339 time, e.g. 2026-08-13T00:00:00Z.")),
		mcp.WithString("date_format", mcp.Description("For kind=date: how the date is written, e.g. dd.MM.yyyy.")),
	), r.docsInsertChip)

	srv.AddTool(mcp.NewTool("gdocs_docs_replace_image",
		mcp.WithDescription("Swap a picture's content while it keeps its place, its size and everything "+
			"around it. This is the only way to change a picture that is already in a document: its "+
			"margins, border and crop cannot be written at all."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("image_object_id", mcp.Required(), mcp.Description(
			"Picture to replace, as reported by gdocs_docs_read_structure.")),
		mcp.WithString("uri", mcp.Required(), mcp.Description("Address of the new picture.")),
		mcp.WithString("method", mcp.DefaultString("CENTER_CROP"), mcp.Description(
			"CENTER_CROP fills the old picture's frame and crops the overflow.")),
	), r.docsReplaceImage)

	srv.AddTool(mcp.NewTool("gdocs_docs_edit_table",
		mcp.WithDescription("Change a table's shape rather than its look: add a row or a column beside a "+
			"cell, or take a merged rectangle apart again. The size a table was created with is not "+
			"final — this is how it grows."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("table_start_index", mcp.Required(), mcp.Description(
			"Index the reading gives as the table element's start.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
		mcp.WithNumber("row", mcp.Required(), mcp.Description("Row of the cell to work beside.")),
		mcp.WithNumber("column", mcp.Required(), mcp.Description("Column of that cell.")),
		mcp.WithString("what", mcp.Required(), mcp.Description(
			"insert_row, insert_column or unmerge.")),
		mcp.WithBoolean("after", mcp.DefaultBool(true), mcp.Description(
			"Put the new row below, or the new column to the right. False puts it before.")),
		mcp.WithNumber("row_span", mcp.Description("For unmerge: how many rows the merged rectangle covers.")),
		mcp.WithNumber("column_span", mcp.Description("For unmerge: how many columns it covers.")),
	), r.docsEditTable)
}

func (r *registry) docsListNamedRanges(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	document, err := client.Document(ctx, documentID)
	if err != nil {
		return toolError(err), nil
	}

	ranges := make([]map[string]any, 0, len(document.NamedRanges))
	for name, group := range document.NamedRanges {
		for _, named := range group.NamedRanges {
			for _, textRange := range named.Ranges {
				ranges = append(ranges, map[string]any{
					"name":        name,
					"range_id":    named.NamedRangeID,
					"start_index": textRange.StartIndex,
					"end_index":   textRange.EndIndex,
				})
			}
		}
	}

	return resultJSON(map[string]any{"document_id": documentID, "named_ranges": ranges})
}

func (r *registry) docsAddNamedRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, textRange, err := docsRangeArgs(req)
	if err != nil {
		return toolError(err), nil
	}

	name, err := requiredString(req, "name")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		CreateNamedRange: &google.DocsCreateNamedRange{Name: name, Range: textRange},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"name":        name,
		"start_index": textRange.StartIndex,
		"end_index":   textRange.EndIndex,
	})
}

func (r *registry) docsFillNamedRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	name := optionalString(req, "name")
	rangeID := optionalString(req, "range_id")
	if name == "" && rangeID == "" {
		return toolError(fmt.Errorf("name the range: give name, or range_id when several share a name")), nil
	}

	text, err := req.RequireString("text")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		ReplaceNamedRange: &google.DocsReplaceNamedRange{Name: name, NamedRangeID: rangeID, Text: text},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"name":        name,
		"range_id":    rangeID,
		"characters":  utf16Length(text),
	})
}

func (r *registry) docsAddTab(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	properties := &google.DocsTabProperties{
		Title:       optionalString(req, "title"),
		ParentTabID: optionalString(req, "parent_tab_id"),
		IconEmoji:   optionalString(req, "icon_emoji"),
	}
	if _, given := req.GetArguments()["index"]; given {
		index := req.GetInt("index", 0)
		properties.Index = &index
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		AddTab: &google.DocsAddTab{Properties: properties},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"document_id": documentID, "title": properties.Title})
}

func (r *registry) docsUpdateTab(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	tabID, err := requiredString(req, "tab_id")
	if err != nil {
		return toolError(err), nil
	}

	properties := &google.DocsTabProperties{TabID: tabID}
	mask := &docsStyleFields{}

	if title, given := givenString(req, "title"); given {
		if title == "" {
			return toolError(fmt.Errorf("title is empty, and a tab has to have a name")), nil
		}
		properties.Title = title
		mask.add("title")
	}
	// The emoji is the opposite case: an empty one takes the icon off, which is the only
	// way back from an icon put on by mistake.
	if emoji, given := givenString(req, "icon_emoji"); given {
		properties.IconEmoji = emoji
		mask.add("iconEmoji")
	}
	if _, given := req.GetArguments()["index"]; given {
		index := req.GetInt("index", 0)
		properties.Index = &index
		mask.add("index")
	}

	if len(mask.fields) == 0 {
		return toolError(fmt.Errorf("nothing to change: give title, index or icon_emoji")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		UpdateTab: &google.DocsUpdateTab{Properties: properties, Fields: mask.mask()},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"document_id": documentID, "tab_id": tabID, "fields": mask.mask()})
}

func (r *registry) docsInsertChip(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	kind := strings.ToLower(optionalString(req, "kind"))
	location, end := docsPlaceArgs(req)

	request := google.DocsRequest{}
	switch kind {
	case "person":
		email, err := requiredString(req, "email")
		if err != nil {
			return toolError(fmt.Errorf("a person chip needs an email address")), nil
		}
		request.InsertPerson = &google.DocsInsertPerson{
			Location: location, EndOfDoc: end,
			Properties: &google.DocsPersonProperties{Email: email, Name: optionalString(req, "name")},
		}
	case "file":
		uri, err := requiredString(req, "uri")
		if err != nil {
			return toolError(fmt.Errorf("a file chip needs the address of the file")), nil
		}
		request.InsertRichLink = &google.DocsInsertRichLink{
			Location: location, EndOfDoc: end,
			Properties: &google.DocsRichLinkProperties{URI: uri, Title: optionalString(req, "title")},
		}
	case "date":
		properties := &google.DocsDateProperties{
			Timestamp:  optionalString(req, "timestamp"),
			DateFormat: optionalString(req, "date_format"),
		}
		if properties.Timestamp == "" {
			return toolError(fmt.Errorf("a date chip needs a timestamp, e.g. 2026-08-13T00:00:00Z")), nil
		}
		request.InsertDate = &google.DocsInsertDate{Location: location, EndOfDoc: end, Properties: properties}
	default:
		return toolError(fmt.Errorf("kind is person, file or date, got %q", kind)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{request}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"document_id": documentID, "kind": kind})
}

func (r *registry) docsReplaceImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "image_object_id")
	if err != nil {
		return toolError(err), nil
	}
	uri, err := requiredString(req, "uri")
	if err != nil {
		return toolError(err), nil
	}

	method := optionalString(req, "method")
	if method == "" {
		method = "CENTER_CROP"
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		ReplaceImage: &google.DocsReplaceImage{ImageObjectID: objectID, URI: uri, Method: method},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"document_id": documentID, "image_object_id": objectID})
}

func (r *registry) docsEditTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	start, err := req.RequireInt("table_start_index")
	if err != nil {
		return toolError(err), nil
	}

	cell := google.DocsTableCellLocation{
		TableStart:  google.DocsLocation{Index: int64(start), SegmentID: optionalString(req, "segment_id")},
		RowIndex:    req.GetInt("row", 0),
		ColumnIndex: req.GetInt("column", 0),
	}

	after := req.GetBool("after", true)

	request := google.DocsRequest{}
	switch what := strings.ToLower(optionalString(req, "what")); what {
	case "insert_row":
		request.InsertTableRow = &google.DocsInsertTableRow{CellLocation: cell, InsertBelow: after}
	case "insert_column":
		request.InsertTableColumn = &google.DocsInsertTableColumn{CellLocation: cell, InsertRight: after}
	case "unmerge":
		rowSpan := req.GetInt("row_span", 1)
		columnSpan := req.GetInt("column_span", 1)
		request.UnmergeTableCells = &google.DocsUnmergeTableCells{
			Range: google.DocsTableRange{CellLocation: cell, RowSpan: rowSpan, ColumnSpan: columnSpan},
		}
	default:
		return toolError(fmt.Errorf("what is insert_row, insert_column or unmerge, got %q", what)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{request}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id":       documentID,
		"table_start_index": start,
		"what":              optionalString(req, "what"),
	})
}
