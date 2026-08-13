package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
)

// MIME types of the Google editors. A folder is a file too, as far as Drive is
// concerned, which is why it needs naming here at all.
const (
	MimeFolder       = "application/vnd.google-apps.folder"
	MimeSpreadsheet  = "application/vnd.google-apps.spreadsheet"
	MimePresentation = "application/vnd.google-apps.presentation"
	MimeDocument     = "application/vnd.google-apps.document"
)

// fileFields is what a listing carries. Enough to decide what a file is and to open it,
// and nothing that would make a listing of a hundred files unreadable.
const fileFields = "id,name,mimeType,modifiedTime,createdTime,size,parents,webViewLink,owners(displayName,emailAddress)"

// File is a Drive item.
type File struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	MimeType     string   `json:"mimeType,omitempty"`
	ModifiedTime string   `json:"modifiedTime,omitempty"`
	CreatedTime  string   `json:"createdTime,omitempty"`
	Size         string   `json:"size,omitempty"`
	Parents      []string `json:"parents,omitempty"`
	WebViewLink  string   `json:"webViewLink,omitempty"`
	Owners       []Owner  `json:"owners,omitempty"`
}

// Owner is who owns a file.
type Owner struct {
	DisplayName  string `json:"displayName,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
}

// UploadFile sends bytes to Drive as a new file, optionally converting them into a Google
// editor format.
//
// The upload is multipart: the metadata and the content in one request. Naming a Google
// MIME type as the target is what turns a .pptx into a real presentation rather than an
// attachment sitting on Drive — Drive converts on the way in, and the result is editable
// by every other tool here.
func (c *Client) UploadFile(ctx context.Context, name, mimeType, targetMimeType string,
	parentFolderID string, content []byte) (*File, error) {
	metadata := map[string]any{"name": name}
	if parentFolderID != "" {
		metadata["parents"] = []string{parentFolderID}
	}
	if targetMimeType != "" {
		metadata["mimeType"] = targetMimeType
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encoding the file metadata: %w", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	metaPart, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {"application/json; charset=UTF-8"}})
	if err != nil {
		return nil, fmt.Errorf("building the upload: %w", err)
	}
	if _, err := metaPart.Write(encoded); err != nil {
		return nil, fmt.Errorf("building the upload: %w", err)
	}

	filePart, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {mimeType}})
	if err != nil {
		return nil, fmt.Errorf("building the upload: %w", err)
	}
	if _, err := filePart.Write(content); err != nil {
		return nil, fmt.Errorf("building the upload: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("building the upload: %w", err)
	}

	query := url.Values{}
	query.Set("uploadType", "multipart")
	query.Set("supportsAllDrives", "true")
	query.Set("fields", fileFields)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint(c.uploadBase, "/files", query), bytes.NewReader(body.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("building the upload: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/related; boundary="+writer.Boundary())

	raw, err := c.send(req)
	if err != nil {
		return nil, err
	}

	var out File
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding the answer from the upload: %w", err)
	}

	return &out, nil
}

// FileList is a page of a listing.
type FileList struct {
	Files         []File `json:"files"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

// SearchOptions narrows a search.
type SearchOptions struct {
	// Query is the Drive query, e.g. name contains 'budget'. Passed through as written:
	// the query language is Drive's, and translating it here would only hide half of it.
	Query     string
	PageSize  int
	PageToken string
	OrderBy   string
}

// SearchFiles finds files anywhere the signed-in account can see them, shared drives
// included — a template deck usually lives on one.
func (c *Client) SearchFiles(ctx context.Context, opts SearchOptions) (*FileList, error) {
	query := url.Values{}
	if opts.Query != "" {
		query.Set("q", opts.Query)
	}
	query.Set("fields", "nextPageToken,files("+fileFields+")")
	query.Set("supportsAllDrives", "true")
	query.Set("includeItemsFromAllDrives", "true")
	query.Set("corpora", "allDrives")
	if opts.PageSize > 0 {
		query.Set("pageSize", strconv.Itoa(opts.PageSize))
	}
	if opts.PageToken != "" {
		query.Set("pageToken", opts.PageToken)
	}
	if opts.OrderBy != "" {
		query.Set("orderBy", opts.OrderBy)
	}

	var out FileList
	if err := c.call(ctx, http.MethodGet, endpoint(c.driveBase, "/files", query), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// FileMetadata reads one file.
func (c *Client) FileMetadata(ctx context.Context, fileID string) (*File, error) {
	query := url.Values{}
	query.Set("fields", fileFields)
	query.Set("supportsAllDrives", "true")

	var out File
	if err := c.call(ctx, http.MethodGet,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID), query), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// CopyFileRequest is a copy about to be made.
type CopyFileRequest struct {
	Name    string   `json:"name,omitempty"`
	Parents []string `json:"parents,omitempty"`
}

// CopyFile copies a file, which is how a deck is started from a template: the copy keeps
// the master, the layouts and the fonts, and that is the whole reason decks made this
// way come out looking right.
func (c *Client) CopyFile(ctx context.Context, fileID, name string, parents []string) (*File, error) {
	query := url.Values{}
	query.Set("fields", fileFields)
	query.Set("supportsAllDrives", "true")

	var out File
	if err := c.call(ctx, http.MethodPost,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/copy", query),
		CopyFileRequest{Name: name, Parents: parents}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// MoveFile puts a file in a folder.
//
// Moving is adding a parent and dropping the old one — Drive has no move of its own, and
// a file with two parents is a file that shows up in two places. Nothing is deleted here:
// the file itself is untouched.
func (c *Client) MoveFile(ctx context.Context, fileID, addParent, removeParent string) (*File, error) {
	query := url.Values{}
	query.Set("fields", fileFields)
	query.Set("supportsAllDrives", "true")
	if addParent != "" {
		query.Set("addParents", addParent)
	}
	if removeParent != "" {
		query.Set("removeParents", removeParent)
	}

	var out File
	if err := c.call(ctx, http.MethodPatch,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID), query), struct{}{}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// ExportFile renders a Google editor file into another format and returns the bytes.
//
// Drive's export refuses anything over ten megabytes with "This file is too large to be
// exported", and a deck with two dozen pictures passes that easily — which is exactly the
// deck worth exporting, because a PPTX is where the things the Slides API does not report
// become visible: text insets, a rounded rectangle's corner radius. The editors serve the
// same conversion from their own address without that ceiling, so a refusal on size is
// retried there rather than handed back to the caller.
func (c *Client) ExportFile(ctx context.Context, fileID, mimeType string) ([]byte, string, error) {
	query := url.Values{}
	query.Set("mimeType", mimeType)
	query.Set("supportsAllDrives", "true")

	content, contentType, err := c.download(ctx,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/export", query))
	if err == nil {
		return content, contentType, nil
	}

	var failure *Error
	if !errors.As(err, &failure) || !strings.Contains(failure.Message, "too large to be exported") {
		return nil, "", err
	}

	address := c.editorExport(fileID, mimeType)
	if address == "" {
		return nil, "", err
	}

	content, contentType, retryErr := c.download(ctx, address)
	if retryErr != nil {
		// The original refusal is the one that explains the situation; the retry only
		// says that the way around did not work either.
		return nil, "", fmt.Errorf("%w (the editors' own export was tried too: %v)", err, retryErr)
	}

	return content, contentType, nil
}

// editorExport is where a Google editor serves a conversion of its own document, without
// the size ceiling Drive's export has. It is empty for formats or kinds not served there.
func (c *Client) editorExport(fileID, mimeType string) string {
	format, ok := editorExportFormats[mimeType]
	if !ok {
		return ""
	}

	kind, ok := editorExportKinds[mimeType]
	if !ok {
		return ""
	}

	return c.editorsBase + "/" + kind + "/d/" + url.PathEscape(fileID) +
		"/export?format=" + format
}

// ExportSheetHTML asks the editors for a spreadsheet as HTML, zipped, one file per tab.
//
// This is the only place a dropdown's colours can be read. The API does not carry them:
// a chip's fill and text colour are in neither the cell's effectiveFormat (which answers
// black on white for a coloured chip) nor its DataValidationRule, whose whole schema is
// condition, inputMessage, showCustomUi and strict. Drive's export refuses the format
// outright — "The requested conversion is not supported" — while the editors serve it,
// rendered by the same code that draws the sheet, with the colours as inline CSS.
func (c *Client) ExportSheetHTML(ctx context.Context, spreadsheetID string) ([]byte, error) {
	address := c.editorsBase + "/spreadsheets/d/" + url.PathEscape(spreadsheetID) +
		"/export?format=zip"

	content, _, err := c.download(ctx, address)
	if err != nil {
		return nil, err
	}

	return content, nil
}

// editorExportFormats maps a MIME type onto the short name the editors' export takes.
var editorExportFormats = map[string]string{
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": "pptx",
	"application/vnd.oasis.opendocument.presentation":                           "odp",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   "docx",
	"application/vnd.oasis.opendocument.text":                                   "odt",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         "xlsx",
	"application/vnd.oasis.opendocument.spreadsheet":                            "ods",
	// PDF is deliberately absent: every editor serves it, and which one to ask cannot be
	// told from the format alone.
}

// editorExportKinds says which editor serves which format: the path differs per editor,
// and a presentation asked for at the documents' address answers with a redirect to a
// sign-in page rather than a file.
var editorExportKinds = map[string]string{
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": "presentation",
	"application/vnd.oasis.opendocument.presentation":                           "presentation",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   "document",
	"application/vnd.oasis.opendocument.text":                                   "document",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         "spreadsheets",
	"application/vnd.oasis.opendocument.spreadsheet":                            "spreadsheets",
}
