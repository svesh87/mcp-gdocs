package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// exportLimit caps how much an export hands back in one answer. A document exported as
// text is fine; a spreadsheet exported as a workbook is megabytes of binary nobody wants
// in a conversation.
const exportLimit = 4 << 20

func (r *registry) registerDrive(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_drive_search",
		mcp.WithDescription("Find files on Drive, shared drives included. "+
			"The query is Drive's own: name contains 'budget', mimeType = "+
			"'application/vnd.google-apps.presentation', 'FOLDER_ID' in parents, and so on, joined with "+
			"and. Without a query it lists recently modified files."),
		mcp.WithString("query", mcp.Description("Drive query, e.g. name contains 'report' and trashed = false.")),
		mcp.WithString("kind", mcp.Description(
			"Narrow to one kind without writing a query: spreadsheet, presentation, document or folder.")),
		mcp.WithNumber("limit", mcp.DefaultNumber(25), mcp.Description("How many files to return, at most 100.")),
		mcp.WithString("page_token", mcp.Description("Token from a previous answer, to read the next page.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.driveSearch)

	srv.AddTool(mcp.NewTool("gdocs_drive_file_info",
		mcp.WithDescription("Describe one file on Drive: its name, kind, owners, folders and address."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description("File identifier.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.driveFileInfo)

	srv.AddTool(mcp.NewTool("gdocs_drive_export",
		mcp.WithDescription("Export a Google editor file into another format and return its content. "+
			"Text formats come back as text, everything else as base64. Use it to read a document as "+
			"plain text or to hand a spreadsheet over as CSV."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description("File identifier.")),
		mcp.WithString("mime_type", mcp.Required(), mcp.Description(
			"Format to export to, e.g. text/plain, text/csv, application/pdf.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.driveExport)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_drive_copy",
		mcp.WithDescription("Copy a file on Drive. For a presentation this is how a deck is started from a "+
			"template; gdocs_slides_copy_presentation is the same thing under a name that says so."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description("File to copy.")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name for the copy.")),
		mcp.WithString("parent_folder_id", mcp.Description("Folder to put it in. Without one it lands beside the original.")),
	), r.driveCopy)
}

// kindMimeTypes turns a plain word into the MIME type Drive knows it by.
var kindMimeTypes = map[string]string{
	"spreadsheet":  google.MimeSpreadsheet,
	"presentation": google.MimePresentation,
	"document":     google.MimeDocument,
	"folder":       google.MimeFolder,
}

func (r *registry) driveSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := optionalString(req, "query")

	if kind := strings.ToLower(optionalString(req, "kind")); kind != "" {
		mimeType, ok := kindMimeTypes[kind]
		if !ok {
			return toolError(fmt.Errorf("kind %q is not one this server knows: "+
				"use spreadsheet, presentation, document or folder", kind)), nil
		}

		clause := "mimeType = '" + mimeType + "'"
		if query == "" {
			query = clause
		} else {
			query = "(" + query + ") and " + clause
		}
	}

	limit := req.GetInt("limit", 25)
	if limit < 1 || limit > 100 {
		return toolError(fmt.Errorf("limit %d is outside 1..100", limit)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	// Newest first when nothing else was asked for: a search for a template usually
	// wants the one somebody touched last, not the one created first.
	files, err := client.SearchFiles(ctx, google.SearchOptions{
		Query:     query,
		PageSize:  limit,
		PageToken: optionalString(req, "page_token"),
		OrderBy:   "modifiedTime desc",
	})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"query":           query,
		"files":           describeFiles(files.Files),
		"next_page_token": files.NextPageToken,
	})
}

func (r *registry) driveFileInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	file, err := client.FileMetadata(ctx, fileID)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(describeFile(*file))
}

func (r *registry) driveExport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}
	mimeType, err := requiredString(req, "mime_type")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	content, contentType, err := client.ExportFile(ctx, fileID, mimeType)
	if err != nil {
		return toolError(err), nil
	}

	if len(content) > exportLimit {
		return toolError(fmt.Errorf("the export is %d bytes, over the %d byte limit: "+
			"export a smaller part of it, or a lighter format", len(content), exportLimit)), nil
	}

	payload := map[string]any{
		"file_id":      fileID,
		"mime_type":    mimeType,
		"content_type": contentType,
		"bytes":        len(content),
	}

	// Text goes back as text, anything else as base64. Deciding by whether the bytes are
	// valid UTF-8 rather than by the MIME type keeps a text/csv export readable and a
	// PDF from arriving as mojibake.
	if utf8.Valid(content) && !strings.HasPrefix(mimeType, "application/pdf") {
		payload["text"] = string(content)
	} else {
		payload["content_base64"] = base64.StdEncoding.EncodeToString(content)
	}

	return resultJSON(payload)
}

func (r *registry) driveCopy(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}
	name, err := requiredString(req, "name")
	if err != nil {
		return toolError(err), nil
	}

	var parents []string
	if folder := optionalString(req, "parent_folder_id"); folder != "" {
		parents = []string{folder}
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	file, err := client.CopyFile(ctx, fileID, name, parents)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(describeFile(*file))
}

// describeFile renders a file the way a caller reads it.
func describeFile(file google.File) map[string]any {
	described := map[string]any{
		"file_id":       file.ID,
		"name":          file.Name,
		"mime_type":     file.MimeType,
		"modified_time": file.ModifiedTime,
		"web_view_link": file.WebViewLink,
	}

	if len(file.Parents) > 0 {
		described["parents"] = file.Parents
	}
	if file.Size != "" {
		described["size"] = file.Size
	}
	if len(file.Owners) > 0 {
		owners := make([]string, 0, len(file.Owners))
		for _, owner := range file.Owners {
			owners = append(owners, owner.DisplayName)
		}
		described["owners"] = owners
	}

	return described
}

func describeFiles(files []google.File) []map[string]any {
	described := make([]map[string]any, 0, len(files))
	for _, file := range files {
		described = append(described, describeFile(file))
	}
	return described
}
