package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// exportFormats are the formats a deck, a document or a spreadsheet is asked for by name
// rather than by MIME type.
//
// A caller that has to spell out
// "application/vnd.openxmlformats-officedocument.presentationml.presentation" gets it
// wrong, and the error Drive returns for a wrong type says only "invalid argument".
var exportFormats = map[string]string{
	"pdf":  "application/pdf",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"odp":  "application/vnd.oasis.opendocument.presentation",
	"txt":  "text/plain",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"odt":  "application/vnd.oasis.opendocument.text",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"ods":  "application/vnd.oasis.opendocument.spreadsheet",
	"csv":  "text/csv",
	"html": "text/html",
	"rtf":  "application/rtf",
	"epub": "application/epub+zip",
}

// importTargets are what an uploaded file is converted into, by the extension it arrives
// with. A file whose extension is not here is uploaded as it is, which is the right
// answer for a picture or an archive.
var importTargets = map[string]string{
	".pptx": google.MimePresentation,
	".ppt":  google.MimePresentation,
	".odp":  google.MimePresentation,
	".docx": google.MimeDocument,
	".doc":  google.MimeDocument,
	".odt":  google.MimeDocument,
	".rtf":  google.MimeDocument,
	".txt":  google.MimeDocument,
	".xlsx": google.MimeSpreadsheet,
	".xls":  google.MimeSpreadsheet,
	".ods":  google.MimeSpreadsheet,
	".csv":  google.MimeSpreadsheet,
}

// uploadTypes are the content types a file is sent with, by extension. Drive uses this to
// decide how to read the bytes it is converting.
var uploadTypes = map[string]string{
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".ppt":  "application/vnd.ms-powerpoint",
	".odp":  "application/vnd.oasis.opendocument.presentation",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".doc":  "application/msword",
	".odt":  "application/vnd.oasis.opendocument.text",
	".rtf":  "application/rtf",
	".txt":  "text/plain",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xls":  "application/vnd.ms-excel",
	".ods":  "application/vnd.oasis.opendocument.spreadsheet",
	".csv":  "text/csv",
	".pdf":  "application/pdf",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".svg":  "image/svg+xml",
}

// registerFiles adds the tools that move whole files between Google and the disk.
//
// They exist only when the server was started with a directory to use. Without one there
// is nowhere a file may be written that is not somebody else's, and the tools are absent
// rather than failing at call time.
func (r *registry) registerFiles(srv *server.MCPServer) {
	if r.opts.FilesDir == "" {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_drive_export_file",
		mcp.WithDescription("Export a presentation, document or spreadsheet to a file on disk: pdf, pptx, docx, "+
			"xlsx, csv, txt and the rest. gdocs_drive_export hands the content back in the answer, which is "+
			"fine for a page of text and useless for a deck — a PDF arrives as base64 and blows past the "+
			"size limit. This writes it into the server's files directory instead and reports the path."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description("File to export.")),
		mcp.WithString("format", mcp.Required(), mcp.Description(
			"pdf, pptx, odp, docx, odt, xlsx, ods, csv, txt, html, rtf or epub.")),
		mcp.WithString("save_as", mcp.Required(), mcp.Description(
			"File name inside the server's files directory. Subdirectories are allowed; anything leading "+
				"outside it is refused.")),
	), r.exportFile)

	srv.AddTool(mcp.NewTool("gdocs_slides_export_images",
		mcp.WithDescription("Render slides to picture files on disk — the whole deck by default, or the ones "+
			"named. This is how a rebuilt deck gets checked against its sample: gdocs_slides_export_thumbnail "+
			"renders one slide and hands back an address that expires, and comparing twenty-two of those by "+
			"hand is where slides get missed."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("name_prefix", mcp.Required(), mcp.Description(
			"Prefix for the file names inside the files directory; each slide gets its number and .png "+
				"appended, e.g. \"sample/slide\" makes sample/slide_00.png and so on.")),
		mcp.WithArray("page_object_ids", mcp.WithStringItems(), mcp.Description(
			"Slides to render. Without any, the whole deck in its own order.")),
		mcp.WithString("size", mcp.Description("SMALL, MEDIUM or LARGE. Default MEDIUM.")),
	), r.exportSlideImages)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_drive_import_file",
		mcp.WithDescription("Upload a file from disk to Drive, converting it into a Google editor format when "+
			"it is one: a .pptx becomes a real presentation every other tool here can edit, a .xlsx becomes "+
			"a spreadsheet. Anything else is uploaded as it is. "+
			"This is the way back in for a deck built elsewhere."),
		mcp.WithString("path", mcp.Required(), mcp.Description(
			"File to upload, named inside the server's files directory.")),
		mcp.WithString("name", mcp.Description("Name for the file on Drive. Without one, the file's own name.")),
		mcp.WithString("parent_folder_id", mcp.Description("Folder to put it in. Without one it lands in My Drive.")),
		mcp.WithBoolean("keep_original_format", mcp.Description(
			"Upload without converting, so a .pptx stays a .pptx attachment rather than becoming a "+
				"Google presentation.")),
	), r.importFile)
}

// exportFile writes an export into the server's files directory.
func (r *registry) exportFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}
	format, err := requiredString(req, "format")
	if err != nil {
		return toolError(err), nil
	}
	saveAs, err := requiredString(req, "save_as")
	if err != nil {
		return toolError(err), nil
	}

	mimeType, ok := exportFormats[strings.ToLower(format)]
	if !ok {
		names := make([]string, 0, len(exportFormats))
		for name := range exportFormats {
			names = append(names, name)
		}
		return toolError(fmt.Errorf("format %q is not one this server knows: %s",
			format, strings.Join(sortedStrings(names), ", "))), nil
	}

	path, err := r.filePath(saveAs)
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

	if err := writeFile(path, content); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"file_id":      fileID,
		"format":       strings.ToLower(format),
		"mime_type":    mimeType,
		"content_type": contentType,
		"path":         path,
		"bytes":        len(content),
	})
}

// exportSlideImages renders slides into picture files.
func (r *registry) exportSlideImages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	prefix, err := requiredString(req, "name_prefix")
	if err != nil {
		return toolError(err), nil
	}

	size := strings.ToUpper(optionalString(req, "size"))
	switch size {
	case "":
		size = "MEDIUM"
	case "SMALL", "MEDIUM", "LARGE":
	default:
		return toolError(fmt.Errorf("size %q is not SMALL, MEDIUM or LARGE", size)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	pages := req.GetStringSlice("page_object_ids", nil)
	if len(pages) == 0 {
		presentation, err := client.Presentation(ctx, presentationID, "slides(objectId)")
		if err != nil {
			return toolError(err), nil
		}
		for _, slide := range presentation.Slides {
			pages = append(pages, slide.ObjectID)
		}
	}
	if len(pages) == 0 {
		return toolError(fmt.Errorf("this presentation has no slides to render")), nil
	}

	type rendered struct {
		Index        int    `json:"index"`
		PageObjectID string `json:"page_object_id"`
		Path         string `json:"path"`
		Bytes        int    `json:"bytes"`
	}

	files := make([]rendered, 0, len(pages))

	for index, page := range pages {
		// Two digits keep the files in slide order in every listing, which is the whole
		// point of rendering them side by side.
		path, err := r.filePath(fmt.Sprintf("%s_%02d.png", prefix, index))
		if err != nil {
			return toolError(err), nil
		}

		thumbnail, err := client.Thumbnail(ctx, presentationID, page, "PNG", size)
		if err != nil {
			return toolError(fmt.Errorf("rendering %s: %w", page, err)), nil
		}

		content, err := fetch(ctx, thumbnail.ContentURL)
		if err != nil {
			return toolError(fmt.Errorf("fetching the render of %s: %w", page, err)), nil
		}
		if err := writeFile(path, content); err != nil {
			return toolError(err), nil
		}

		files = append(files, rendered{Index: index, PageObjectID: page, Path: path, Bytes: len(content)})
	}

	return resultJSON(map[string]any{
		"presentation_id": presentationID,
		"size":            size,
		"slides":          len(files),
		"files":           files,
	})
}

// importFile uploads a file from disk to Drive.
func (r *registry) importFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := requiredString(req, "path")
	if err != nil {
		return toolError(err), nil
	}

	path, err := r.filePath(name)
	if err != nil {
		return toolError(err), nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return toolError(fmt.Errorf("reading %s: %w", path, err)), nil
	}

	extension := strings.ToLower(filepath.Ext(path))

	uploadType := uploadTypes[extension]
	if uploadType == "" {
		uploadType = "application/octet-stream"
	}

	target := ""
	if !req.GetBool("keep_original_format", false) {
		target = importTargets[extension]
	}

	driveName := optionalString(req, "name")
	if driveName == "" {
		driveName = strings.TrimSuffix(filepath.Base(path), extension)
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	file, err := client.UploadFile(ctx, driveName, uploadType, target,
		optionalString(req, "parent_folder_id"), content)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"file_id":       file.ID,
		"name":          file.Name,
		"mime_type":     file.MimeType,
		"converted":     target != "",
		"web_view_link": file.WebViewLink,
		"bytes":         len(content),
	})
}

// writeFile puts bytes on disk, making the directories the name asks for.
//
// The file is 0600 and the directories 0700: an export is somebody's document, and the
// directory it lands in may be shared with whatever else runs on that machine.
func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("making the directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// fetch downloads what a rendered slide's address points at.
func fetch(ctx context.Context, address string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("the render came back as %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// sortedStrings orders names for an error message, so the same wrong format is answered
// the same way every time.
func sortedStrings(values []string) []string {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}

	return values
}
