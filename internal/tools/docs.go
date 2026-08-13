package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

const documentIDHelp = "Document identifier, the part of its address between /d/ and /edit."

func (r *registry) registerDocs(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_docs_read",
		mcp.WithDescription("Read a document as text. Tables come out row by row with tabs between cells. "+
			"The character count that comes back is what the writing tools address positions by."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.docsRead)

	r.registerDocsRead(srv)
	r.registerDocsWrite(srv)
	r.registerDocsDelete(srv)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_docs_create",
		mcp.WithDescription("Create an empty document with a title."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Name of the new document.")),
	), r.docsCreate)

	srv.AddTool(mcp.NewTool("gdocs_docs_append",
		mcp.WithDescription("Add text to the end of a document, or to the end of one of its headers, "+
			"footers or footnotes."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to add. Newlines make paragraphs.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
	), r.docsAppend)

	srv.AddTool(mcp.NewTool("gdocs_docs_insert_text",
		mcp.WithDescription("Insert text at one place in a document, counted in characters from its start. "+
			"Read the document first: the indexes shift with every edit, and one taken from an older "+
			"reading lands somewhere else."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("index", mcp.Required(), mcp.Description("Where to insert, counting characters from 1.")),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to insert.")),
		mcp.WithString("segment_id", mcp.Description(docsSegmentHelp)),
	), r.docsInsertText)

	srv.AddTool(mcp.NewTool("gdocs_docs_replace_text",
		mcp.WithDescription("Replace every occurrence of a string in a document. "+
			"This is the safe way to fill a template in: it needs no indexes and cannot land in the "+
			"wrong place."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithString("find", mcp.Required(), mcp.Description("Text to look for, e.g. {{name}}.")),
		mcp.WithString("replace", mcp.Required(), mcp.Description("Text to put in its place.")),
		mcp.WithBoolean("match_case", mcp.DefaultBool(true), mcp.Description("Match upper and lower case exactly.")),
		mcp.WithIdempotentHintAnnotation(false),
	), r.docsReplaceText)
}

func (r *registry) docsRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	text := document.PlainText()

	return resultJSON(map[string]any{
		"document_id": document.DocumentID,
		"title":       document.Title,
		"characters":  utf16Length(text),
		"text":        text,
	})
}

func (r *registry) docsCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := requiredString(req, "title")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	document, err := client.CreateDocument(ctx, title)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": document.DocumentID,
		"title":       document.Title,
	})
}

func (r *registry) docsAppend(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}
	text, err := req.RequireString("text")
	if err != nil {
		return toolError(err), nil
	}
	if text == "" {
		return toolError(fmt.Errorf("text is empty")), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	// endOfSegmentLocation with no segment identifier means the end of the body, which
	// spares the caller a read just to find out where the document ends. With one it is
	// the end of that header or footer instead.
	segment := optionalString(req, "segment_id")

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		InsertText: &google.DocsInsertText{EndOfDoc: &google.DocsSegmentEnd{SegmentID: segment}, Text: text},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"characters":  utf16Length(text),
	})
}

func (r *registry) docsInsertText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}
	text, err := req.RequireString("text")
	if err != nil {
		return toolError(err), nil
	}
	index, err := req.RequireInt("index")
	if err != nil {
		return toolError(err), nil
	}

	// The body starts at 1 — index 0 there is the document itself rather than a place in
	// it, and Docs refuses it with an error about a segment. A header, a footer or a
	// footnote starts at 0 instead, and a freshly made one is a single newline: index 1
	// is already past its end.
	segment := optionalString(req, "segment_id")
	if index < 1 && segment == "" {
		return toolError(fmt.Errorf("index %d is before the first character: a document's own text starts at 1", index)), nil
	}
	if index < 0 {
		return toolError(fmt.Errorf("index %d is before the start of the segment", index)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		InsertText: &google.DocsInsertText{
			Location: &google.DocsLocation{Index: int64(index), SegmentID: segment},
			Text:     text,
		},
	}}); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"index":       index,
		"characters":  utf16Length(text),
	})
}

func (r *registry) docsReplaceText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}
	find, err := req.RequireString("find")
	if err != nil {
		return toolError(err), nil
	}
	if strings.TrimSpace(find) == "" {
		return toolError(fmt.Errorf("find is empty: replacing nothing would match everywhere")), nil
	}
	replace, err := req.RequireString("replace")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.DocsBatchUpdate(ctx, documentID, []google.DocsRequest{{
		ReplaceAllText: &google.DocsReplaceAllText{
			ContainsText: google.DocsSubstringMatch{Text: find, MatchCase: req.GetBool("match_case", true)},
			ReplaceText:  replace,
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	occurrences := 0
	for _, reply := range response.Replies {
		if reply.ReplaceAllText != nil {
			occurrences += reply.ReplaceAllText.OccurrencesChanged
		}
	}

	return resultJSON(map[string]any{
		"document_id": documentID,
		"find":        find,
		"occurrences": occurrences,
	})
}
