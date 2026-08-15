package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

const fileIDHelp = "File identifier, the part of its address between /d/ and /edit."

// registerDriveManage adds the tools about a file rather than its contents: where it sits,
// what it is called, who can see it, what was said about it, and what it looked like
// before.
//
// Sharing lives in its own group. Everything else in this server changes what is inside
// the account's own files; this one changes who outside the account can see one, and that
// difference is worth a switch of its own rather than a line in a description.
func (r *registry) registerDriveManage(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_drive_list_folder",
		mcp.WithDescription("List what is in a folder. This is the plain question a search query has to be "+
			"written for otherwise, and it answers with the same fields as a search."),
		mcp.WithString("folder_id", mcp.Required(), mcp.Description(
			"Folder identifier. The part of a folder's address after /folders/.")),
		mcp.WithString("kind", mcp.Description(
			"Narrow to one kind: presentation, spreadsheet, document, folder.")),
		mcp.WithNumber("limit", mcp.DefaultNumber(50), mcp.Description("How many to return, at most 100.")),
		mcp.WithString("page_token", mcp.Description("Token from a previous answer, to read the next page.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.driveListFolder)

	srv.AddTool(mcp.NewTool("gdocs_drive_list_permissions",
		mcp.WithDescription("Report who can reach a file: people, groups, domains, and whether it is open "+
			"to anyone with the link. Reading this is not sharing — it is what to check before sending "+
			"a link, and before assuming a document is private."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.driveListPermissions)

	srv.AddTool(mcp.NewTool("gdocs_drive_list_comments",
		mcp.WithDescription("Read the comments on a file, with their replies, the text each one hangs on, "+
			"and whether it is resolved. This is the half of a review that lives nowhere in the document "+
			"itself: a person leaves remarks in the editor, and without this there is nothing to act on."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithBoolean("include_deleted", mcp.DefaultBool(false), mcp.Description(
			"Include comments that were removed.")),
		mcp.WithString("page_token", mcp.Description("Token from a previous answer.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.driveListComments)

	srv.AddTool(mcp.NewTool("gdocs_drive_list_revisions",
		mcp.WithDescription("List the saved versions of a file: when, by whom, and which ones Drive has "+
			"been told to keep. gdocs_drive_restore_revision goes back to one, exactly for an ordinary "+
			"file and through a conversion for a Google one; gdocs_drive_keep_revision stops Drive "+
			"pruning the version worth going back to."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithString("page_token", mcp.Description("Token from a previous answer.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.driveListRevisions)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_drive_create_folder",
		mcp.WithDescription("Make a folder. Without a parent it lands in My Drive; with one it lands inside "+
			"that folder, which is also how a folder is made on a shared drive."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name for the folder.")),
		mcp.WithString("parent_folder_id", mcp.Description("Folder to make it in.")),
	), r.driveCreateFolder)

	srv.AddTool(mcp.NewTool("gdocs_drive_rename",
		mcp.WithDescription("Give a file or a folder another name. Links keep working and nothing inside "+
			"changes: in Drive a name is a label, not an identity."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithString("name", mcp.Required(), mcp.Description("New name.")),
	), r.driveRename)

	srv.AddTool(mcp.NewTool("gdocs_drive_move",
		mcp.WithDescription("Put a file in another folder. Drive has no move of its own — this adds the new "+
			"parent and drops the old one, and a file with two parents shows up in two places. Nothing "+
			"is deleted; read the file's folders first if you are unsure which one to drop."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithString("to_folder_id", mcp.Required(), mcp.Description("Folder to put it in.")),
		mcp.WithString("from_folder_id", mcp.Description(
			"Folder to take it out of. Without one the file keeps its old folder as well.")),
	), r.driveMove)

	srv.AddTool(mcp.NewTool("gdocs_drive_add_comment",
		mcp.WithDescription("Leave a comment on a file. It appears in the editor as a remark from the "+
			"signed-in person, which is what makes it a reply somebody will actually see."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithString("content", mcp.Required(), mcp.Description("What the comment says.")),
	), r.driveAddComment)

	srv.AddTool(mcp.NewTool("gdocs_drive_reply_comment",
		mcp.WithDescription("Answer a comment, and resolve or reopen it in the same breath. A thread is "+
			"closed by somebody saying something, which is how Drive models it — there is no flag."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithString("comment_id", mcp.Required(), mcp.Description(
			"Comment to answer, as reported by gdocs_drive_list_comments.")),
		mcp.WithString("content", mcp.Required(), mcp.Description("What the reply says.")),
		mcp.WithString("action", mcp.Description("resolve to close the thread, reopen to bring it back.")),
	), r.driveReplyComment)

	srv.AddTool(mcp.NewTool("gdocs_drive_keep_revision",
		mcp.WithDescription("Tell Drive to keep a version rather than prune it. Worth doing before a "+
			"risky edit: Drive thins out old versions on its own, and for a Google file the browser's "+
			"own history is the only way back that loses nothing — gdocs_drive_restore_revision has to "+
			"go through a conversion."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithString("revision_id", mcp.Required(), mcp.Description(
			"Revision, as reported by gdocs_drive_list_revisions.")),
		mcp.WithBoolean("keep", mcp.DefaultBool(true), mcp.Description("Keep it, or stop keeping it.")),
	), r.driveKeepRevision)

	srv.AddTool(mcp.NewTool("gdocs_drive_restore_revision",
		mcp.WithDescription("Put a file back to what it was at an earlier version, keeping its identifier "+
			"— and so every link to it, every slide showing it and every permission on it. Drive has no "+
			"request that does this, so it is done by fetching that version's content and writing it "+
			"over the file. What that costs depends on the file. An ordinary one — a PDF, a picture, an "+
			"archive — is stored as bytes and comes back exactly. A Google document, workbook or deck "+
			"is not stored as bytes at all: the only way out is a conversion, so the restore goes "+
			"through DOCX, XLSX or PPTX and loses whatever that format cannot carry. The answer lists "+
			"what that is. Restoring also makes a new version rather than undoing the ones since, so "+
			"nothing is lost twice."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithString("revision_id", mcp.Required(), mcp.Description(
			"Version to go back to, as gdocs_drive_list_revisions reports it.")),
		mcp.WithBoolean("confirm_conversion", mcp.Description(
			"Required for a Google document, workbook or deck, and refused for anything else. It is "+
				"the acknowledgement that the restore goes through a conversion and comes back changed: "+
				"for a deck that usually means the theme, the speaker notes' formatting and any linked "+
				"chart. Without it the tool reads the version and refuses, naming the format it would "+
				"have used.")),
		mcp.WithDestructiveHintAnnotation(true),
	), r.driveRestoreRevision)

	srv.AddTool(mcp.NewTool("gdocs_drive_delete_to_trash",
		mcp.WithDescription("Put a file in the bin, or take it back out. This is as far as removal goes: a "+
			"file in the bin keeps its identifier, comes back whole, and is the owner's to restore for "+
			"thirty days. There is no tool here that empties the bin or deletes a file outright, and "+
			"there will not be."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithBoolean("restore", mcp.DefaultBool(false), mcp.Description(
			"Take it back out of the bin instead of putting it in.")),
		mcp.WithDestructiveHintAnnotation(true),
	), r.driveTrash)

	srv.AddTool(mcp.NewTool("gdocs_drive_share",
		mcp.WithDescription("Give someone access to a file, or open it to anyone with the link. "+
			"This is the only thing this server does that makes something visible outside the account, "+
			"which is why it is in a group of its own and is never offered unless --tools names "+
			"drive-share. Say who and as what; a link opened to anyone stays open until it is closed "+
			"again with gdocs_drive_unshare."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithString("type", mcp.Required(), mcp.Description(
			"user, group, domain, or anyone — anyone means everybody who has the link.")),
		mcp.WithString("role", mcp.DefaultString("reader"), mcp.Description(
			"reader, commenter or writer.")),
		mcp.WithString("email", mcp.Description("Address, for type user or group.")),
		mcp.WithString("domain", mcp.Description("Domain, for type domain.")),
		mcp.WithBoolean("notify", mcp.DefaultBool(false), mcp.Description(
			"Send Google's notification email. Off by default: a silent grant is the one that does not "+
				"surprise anybody's inbox.")),
	), r.driveShare)

	srv.AddTool(mcp.NewTool("gdocs_drive_unshare",
		mcp.WithDescription("Take back one grant of access. The file is untouched; only the access goes. "+
			"Use the identifier gdocs_drive_list_permissions reports."),
		mcp.WithString("file_id", mcp.Required(), mcp.Description(fileIDHelp)),
		mcp.WithString("permission_id", mcp.Required(), mcp.Description("Grant to take back.")),
		mcp.WithDestructiveHintAnnotation(true),
	), r.driveUnshare)
}

func (r *registry) driveListFolder(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	folderID, err := requiredString(req, "folder_id")
	if err != nil {
		return toolError(err), nil
	}

	query := fmt.Sprintf("%q in parents and trashed = false", folderID)
	if kind := strings.ToLower(optionalString(req, "kind")); kind != "" {
		mimeType, ok := kindMimeTypes[kind]
		if !ok {
			return toolError(fmt.Errorf("kind %q is not one this server knows: "+
				"use spreadsheet, presentation, document or folder", kind)), nil
		}
		query += fmt.Sprintf(" and mimeType = %q", mimeType)
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	listing, err := client.SearchFiles(ctx, google.SearchOptions{
		Query:     query,
		PageSize:  req.GetInt("limit", 50),
		PageToken: optionalString(req, "page_token"),
	})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"folder_id":       folderID,
		"files":           describeFiles(listing.Files),
		"next_page_token": listing.NextPageToken,
	})
}

func (r *registry) driveListPermissions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	permissions, err := client.Permissions(ctx, fileID)
	if err != nil {
		return toolError(err), nil
	}

	described := make([]map[string]any, 0, len(permissions))
	openToAnyone := false
	for _, permission := range permissions {
		if permission.Type == "anyone" {
			openToAnyone = true
		}
		entry := map[string]any{"id": permission.ID, "type": permission.Type, "role": permission.Role}
		putString(entry, "email", permission.EmailAddress)
		putString(entry, "domain", permission.Domain)
		putString(entry, "name", permission.DisplayName)
		described = append(described, entry)
	}

	return resultJSON(map[string]any{
		"file_id":                  fileID,
		"permissions":              described,
		"open_to_anyone_with_link": openToAnyone,
	})
}

func (r *registry) driveListComments(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	listing, err := client.Comments(ctx, fileID, req.GetBool("include_deleted", false),
		optionalString(req, "page_token"))
	if err != nil {
		return toolError(err), nil
	}

	comments := make([]map[string]any, 0, len(listing.Comments))
	open := 0
	for _, comment := range listing.Comments {
		if !comment.Resolved && !comment.Deleted {
			open++
		}

		entry := map[string]any{
			"id":       comment.ID,
			"content":  comment.Content,
			"resolved": comment.Resolved,
			"created":  comment.CreatedTime,
		}
		if comment.Author != nil {
			entry["author"] = comment.Author.DisplayName
		}
		if comment.QuotedText != nil && comment.QuotedText.Value != "" {
			entry["about"] = comment.QuotedText.Value
		}
		if len(comment.Replies) > 0 {
			replies := make([]map[string]any, 0, len(comment.Replies))
			for _, reply := range comment.Replies {
				described := map[string]any{"content": reply.Content}
				putString(described, "action", reply.Action)
				if reply.Author != nil {
					described["author"] = reply.Author.DisplayName
				}
				replies = append(replies, described)
			}
			entry["replies"] = replies
		}

		comments = append(comments, entry)
	}

	return resultJSON(map[string]any{
		"file_id":         fileID,
		"comments":        comments,
		"open":            open,
		"next_page_token": listing.NextPageToken,
	})
}

func (r *registry) driveListRevisions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	listing, err := client.Revisions(ctx, fileID, optionalString(req, "page_token"))
	if err != nil {
		return toolError(err), nil
	}

	revisions := make([]map[string]any, 0, len(listing.Revisions))
	for _, revision := range listing.Revisions {
		entry := map[string]any{
			"id":           revision.ID,
			"modified":     revision.ModifiedTime,
			"kept_forever": revision.KeepForever,
		}
		if revision.LastModifyUser != nil {
			entry["by"] = revision.LastModifyUser.DisplayName
		}
		revisions = append(revisions, entry)
	}

	return resultJSON(map[string]any{
		"file_id":         fileID,
		"revisions":       revisions,
		"next_page_token": listing.NextPageToken,
		"note": "restoring a version of a Google editor file is done in the editor; the API can only " +
			"keep one from being pruned",
	})
}

func (r *registry) driveCreateFolder(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := requiredString(req, "name")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	folder, err := client.CreateFolder(ctx, name, optionalString(req, "parent_folder_id"))
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"folder_id": folder.ID,
		"name":      folder.Name,
		"link":      folder.WebViewLink,
	})
}

func (r *registry) driveRename(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
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

	file, err := client.RenameFile(ctx, fileID, name)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"file_id": file.ID, "name": file.Name})
}

func (r *registry) driveMove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}
	target, err := requiredString(req, "to_folder_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	file, err := client.MoveFile(ctx, fileID, target, optionalString(req, "from_folder_id"))
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"file_id": file.ID,
		"name":    file.Name,
		"folders": file.Parents,
	})
}

func (r *registry) driveAddComment(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}
	content, err := requiredString(req, "content")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	comment, err := client.AddComment(ctx, fileID, content)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"file_id": fileID, "comment_id": comment.ID})
}

func (r *registry) driveReplyComment(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}
	commentID, err := requiredString(req, "comment_id")
	if err != nil {
		return toolError(err), nil
	}
	content, err := requiredString(req, "content")
	if err != nil {
		return toolError(err), nil
	}

	action := strings.ToLower(optionalString(req, "action"))
	if action != "" && action != "resolve" && action != "reopen" {
		return toolError(fmt.Errorf("action is resolve or reopen, got %q", action)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	reply, err := client.ReplyToComment(ctx, fileID, commentID, content, action)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"file_id":    fileID,
		"comment_id": commentID,
		"reply_id":   reply.ID,
		"action":     reply.Action,
	})
}

// restoreFormats are the format a restore takes each kind of Google file back through, and
// what that format is known to drop.
//
// The list is short and specific on purpose. "Some formatting may be lost" is not something a
// caller can act on; "the theme, and every chart's link to its workbook" is — it says what to
// check afterwards and what to put back by hand.
var restoreFormats = map[string]struct {
	mimeType string
	loses    []string
}{
	google.MimeDocument: {
		mimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		loses: []string{"comments and suggestions", "chips pointing at people or files",
			"drawings made in the editor", "named ranges"},
	},
	google.MimeSpreadsheet: {
		mimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		loses: []string{"comments and notes attached by others", "the look of dropdowns",
			"charts' links to their ranges where the ranges moved", "developer metadata"},
	},
	google.MimePresentation: {
		mimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		loses: []string{"the theme's own identity, which comes back as ordinary layouts",
			"every chart's link to its workbook, which becomes a picture",
			"comments and suggestions", "linked slides"},
	},
}

func (r *registry) driveRestoreRevision(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}
	revisionID, err := requiredString(req, "revision_id")
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

	format, isEditorFile := restoreFormats[file.MimeType]
	confirmed := req.GetBool("confirm_conversion", false)

	switch {
	case isEditorFile && !confirmed:
		return toolError(fmt.Errorf("%q is a Google %s, which is not stored as bytes: putting it back "+
			"means exporting that version as %s and writing it over the file, and it comes back without "+
			"%s. Nothing has been changed. Pass confirm_conversion to go ahead, or open the version "+
			"history in the browser, where the editor restores it without a conversion",
			file.Name, kindWord(file.MimeType), shortFormat(format.mimeType),
			strings.Join(format.loses, ", "))), nil

	case !isEditorFile && confirmed:
		return toolError(fmt.Errorf("%q is not a Google document, workbook or deck, so no conversion "+
			"happens and confirm_conversion has nothing to confirm: its bytes go back exactly as they "+
			"were. Call again without it", file.Name)), nil
	}

	preferred := []string{format.mimeType}
	if !isEditorFile {
		preferred = nil
	}

	content, usedFormat, err := client.RevisionContent(ctx, fileID, revisionID, preferred)
	if err != nil {
		return toolError(err), nil
	}
	if len(content) == 0 {
		return toolError(fmt.Errorf("that version came back empty, so nothing was written: " +
			"check the revision identifier against gdocs_drive_list_revisions")), nil
	}

	if usedFormat == "" {
		usedFormat = file.MimeType
	}

	restored, err := client.ReplaceFileContent(ctx, fileID, usedFormat, content)
	if err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{
		"file_id":     fileID,
		"name":        restored.Name,
		"revision_id": revisionID,
		"bytes":       len(content),
		"note": "the file keeps its identifier, so every link to it and every permission on it " +
			"survives; the restore is itself a new version, so the ones it went back past are still " +
			"in the history",
	}

	if isEditorFile {
		payload["through"] = shortFormat(usedFormat)
		payload["not_restored"] = format.loses
	} else {
		payload["exact"] = true
	}

	return resultJSON(payload)
}

// kindWord names a Google file the way a person would.
func kindWord(mimeType string) string {
	switch mimeType {
	case google.MimeDocument:
		return "document"
	case google.MimeSpreadsheet:
		return "workbook"
	case google.MimePresentation:
		return "presentation"
	}

	return "file"
}

// shortFormat is the last word of a MIME type, which is the one a person recognises.
func shortFormat(mimeType string) string {
	switch mimeType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "DOCX"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "XLSX"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "PPTX"
	}

	return mimeType
}

func (r *registry) driveKeepRevision(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}
	revisionID, err := requiredString(req, "revision_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	revision, err := client.KeepRevision(ctx, fileID, revisionID, req.GetBool("keep", true))
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"file_id":      fileID,
		"revision_id":  revision.ID,
		"kept_forever": revision.KeepForever,
	})
}

func (r *registry) driveTrash(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	restore := req.GetBool("restore", false)

	file, err := client.SetTrashed(ctx, fileID, !restore)
	if err != nil {
		return toolError(err), nil
	}

	state := "in the bin"
	if restore {
		state = "restored"
	}

	return resultJSON(map[string]any{
		"file_id": file.ID,
		"name":    file.Name,
		"state":   state,
	})
}

func (r *registry) driveShare(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}

	kind := strings.ToLower(strings.TrimSpace(req.GetString("type", "")))
	role := strings.ToLower(req.GetString("role", "reader"))
	if role == "" {
		role = "reader"
	}

	permission := google.SharePermission{Type: kind, Role: role}

	switch kind {
	case "user", "group":
		email, err := requiredString(req, "email")
		if err != nil {
			return toolError(fmt.Errorf("sharing with a %s needs an email address", kind)), nil
		}
		permission.EmailAddress = email
	case "domain":
		domain, err := requiredString(req, "domain")
		if err != nil {
			return toolError(fmt.Errorf("sharing with a domain needs the domain")), nil
		}
		permission.Domain = domain
	case "anyone":
		// Nothing else to name — and that is exactly why this one deserves a sentence
		// back saying what was opened.
	default:
		return toolError(fmt.Errorf("type is user, group, domain or anyone, got %q", kind)), nil
	}

	switch role {
	case "reader", "commenter", "writer":
	default:
		return toolError(fmt.Errorf("role is reader, commenter or writer, got %q", role)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	granted, err := client.Share(ctx, fileID, permission, req.GetBool("notify", false))
	if err != nil {
		return toolError(err), nil
	}

	answer := map[string]any{
		"file_id":       fileID,
		"permission_id": granted.ID,
		"type":          granted.Type,
		"role":          granted.Role,
	}
	if kind == "anyone" {
		answer["warning"] = "this file is now readable by everybody who has the link, until " +
			"gdocs_drive_unshare takes the grant back"
	}

	return resultJSON(answer)
}

func (r *registry) driveUnshare(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, err := requiredString(req, "file_id")
	if err != nil {
		return toolError(err), nil
	}
	permissionID, err := requiredString(req, "permission_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	if err := client.Unshare(ctx, fileID, permissionID); err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{"file_id": fileID, "permission_id": permissionID, "state": "taken back"})
}
