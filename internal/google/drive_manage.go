package google

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// This file is the half of Drive that is about the file rather than its contents: where it
// sits, what it is called, who can see it, what was said about it, and what it looked like
// before the last edit.
//
// None of it deletes anything. The strongest thing here puts a file in the bin, which
// Drive keeps for thirty days and the owner can undo — and there is deliberately no code
// for emptying that bin or for removing a file outright.

// CreateFolder makes a folder. A folder in Drive is a file with a particular MIME type,
// which is why this is the same call as creating anything else.
func (c *Client) CreateFolder(ctx context.Context, name, parentFolderID string) (*File, error) {
	query := url.Values{}
	query.Set("fields", fileFields)
	query.Set("supportsAllDrives", "true")

	body := map[string]any{"name": name, "mimeType": MimeFolder}
	if parentFolderID != "" {
		body["parents"] = []string{parentFolderID}
	}

	var out File
	if err := c.call(ctx, http.MethodPost, endpoint(c.driveBase, "/files", query), body, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// RenameFile gives a file another name. The name is not the identity: links keep working,
// and nothing inside the file changes.
func (c *Client) RenameFile(ctx context.Context, fileID, name string) (*File, error) {
	query := url.Values{}
	query.Set("fields", fileFields)
	query.Set("supportsAllDrives", "true")

	var out File
	if err := c.call(ctx, http.MethodPatch,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID), query),
		map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// SetTrashed puts a file in the bin or takes it back out.
//
// This is as far as removal goes on this side: a trashed file is recoverable by its owner
// for thirty days, keeps its identifier and comes back whole. Emptying the bin and
// deleting outright are not implemented anywhere in this package.
func (c *Client) SetTrashed(ctx context.Context, fileID string, trashed bool) (*File, error) {
	query := url.Values{}
	query.Set("fields", fileFields+",trashed")
	query.Set("supportsAllDrives", "true")

	var out File
	if err := c.call(ctx, http.MethodPatch,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID), query),
		map[string]any{"trashed": trashed}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// Permission is one entry of who can reach a file.
type Permission struct {
	ID           string `json:"id,omitempty"`
	Type         string `json:"type,omitempty"`
	Role         string `json:"role,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	Domain       string `json:"domain,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	Deleted      bool   `json:"deleted,omitempty"`
}

// PermissionList is what a listing of them comes back as.
type PermissionList struct {
	Permissions []Permission `json:"permissions"`
}

// Permissions reports who can reach a file. Reading this is not sharing: it is what to
// check before sending a link, and before assuming a document is private.
func (c *Client) Permissions(ctx context.Context, fileID string) ([]Permission, error) {
	query := url.Values{}
	query.Set("fields", "permissions(id,type,role,emailAddress,domain,displayName,deleted)")
	query.Set("supportsAllDrives", "true")
	query.Set("pageSize", "100")

	var out PermissionList
	if err := c.call(ctx, http.MethodGet,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/permissions", query), nil, &out); err != nil {
		return nil, err
	}

	return out.Permissions, nil
}

// SharePermission is a grant about to be made.
type SharePermission struct {
	Type         string `json:"type"`
	Role         string `json:"role"`
	EmailAddress string `json:"emailAddress,omitempty"`
	Domain       string `json:"domain,omitempty"`
}

// Share gives someone access to a file, or opens it to anyone with the link.
//
// This is the one thing in this server that makes something visible outside the account,
// which is why it lives in a group of its own and is never in the default set.
func (c *Client) Share(ctx context.Context, fileID string, permission SharePermission, notify bool) (*Permission, error) {
	query := url.Values{}
	query.Set("fields", "id,type,role,emailAddress,domain")
	query.Set("supportsAllDrives", "true")
	query.Set("sendNotificationEmail", strconv.FormatBool(notify))

	var out Permission
	if err := c.call(ctx, http.MethodPost,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/permissions", query), permission, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// Unshare takes one grant back. The file is untouched; only the access goes.
func (c *Client) Unshare(ctx context.Context, fileID, permissionID string) error {
	query := url.Values{}
	query.Set("supportsAllDrives", "true")

	return c.call(ctx, http.MethodDelete,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/permissions/"+url.PathEscape(permissionID), query),
		nil, nil)
}

// Comment is one remark on a file, with its replies.
type Comment struct {
	ID           string      `json:"id,omitempty"`
	Content      string      `json:"content,omitempty"`
	Author       *Owner      `json:"author,omitempty"`
	CreatedTime  string      `json:"createdTime,omitempty"`
	ModifiedTime string      `json:"modifiedTime,omitempty"`
	Resolved     bool        `json:"resolved,omitempty"`
	QuotedText   *QuotedText `json:"quotedFileContent,omitempty"`
	Anchor       string      `json:"anchor,omitempty"`
	Replies      []Reply     `json:"replies,omitempty"`
	Deleted      bool        `json:"deleted,omitempty"`
}

// QuotedText is the piece of the document a comment hangs on.
type QuotedText struct {
	MimeType string `json:"mimeType,omitempty"`
	Value    string `json:"value,omitempty"`
}

// Reply is one answer under a comment.
type Reply struct {
	ID           string `json:"id,omitempty"`
	Content      string `json:"content,omitempty"`
	Action       string `json:"action,omitempty"`
	Author       *Owner `json:"author,omitempty"`
	CreatedTime  string `json:"createdTime,omitempty"`
	ModifiedTime string `json:"modifiedTime,omitempty"`
	Deleted      bool   `json:"deleted,omitempty"`
}

// CommentList is a page of comments.
type CommentList struct {
	Comments      []Comment `json:"comments"`
	NextPageToken string    `json:"nextPageToken,omitempty"`
}

const commentFields = "comments(id,content,author(displayName,emailAddress),createdTime,modifiedTime," +
	"resolved,anchor,quotedFileContent,deleted,replies(id,content,action,author(displayName,emailAddress)," +
	"createdTime,modifiedTime,deleted)),nextPageToken"

// Comments reads the remarks on a file.
//
// This is the half of a review an API caller could not see before: a person leaves
// comments in the editor, and without this the agent asked to act on them has nothing to
// read. Resolved ones are included, because "already handled" is an answer too.
func (c *Client) Comments(ctx context.Context, fileID string, includeDeleted bool, pageToken string) (*CommentList, error) {
	query := url.Values{}
	query.Set("fields", commentFields)
	query.Set("pageSize", "100")
	query.Set("includeDeleted", strconv.FormatBool(includeDeleted))
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}

	var out CommentList
	if err := c.call(ctx, http.MethodGet,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/comments", query), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// AddComment leaves a remark on a file.
func (c *Client) AddComment(ctx context.Context, fileID, content string) (*Comment, error) {
	query := url.Values{}
	query.Set("fields", "id,content,createdTime")

	var out Comment
	if err := c.call(ctx, http.MethodPost,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/comments", query),
		map[string]any{"content": content}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// ReplyToComment answers one, and can resolve or reopen it in the same breath.
//
// The action is part of the reply rather than a separate call because that is how Drive
// models it: a thread is closed by somebody saying something, not by a flag.
func (c *Client) ReplyToComment(ctx context.Context, fileID, commentID, content, action string) (*Reply, error) {
	query := url.Values{}
	query.Set("fields", "id,content,action,createdTime")

	body := map[string]any{"content": content}
	if action != "" {
		body["action"] = action
	}

	var out Reply
	if err := c.call(ctx, http.MethodPost,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/comments/"+url.PathEscape(commentID)+"/replies", query),
		body, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// Revision is one saved state of a file.
type Revision struct {
	ID             string `json:"id,omitempty"`
	ModifiedTime   string `json:"modifiedTime,omitempty"`
	KeepForever    bool   `json:"keepForever,omitempty"`
	LastModifyUser *Owner `json:"lastModifyingUser,omitempty"`
	Size           string `json:"size,omitempty"`
}

// RevisionList is what a listing of them comes back as.
type RevisionList struct {
	Revisions     []Revision `json:"revisions"`
	NextPageToken string     `json:"nextPageToken,omitempty"`
}

// Revisions lists the saved states of a file. For a Google editor file these are the
// versions the editor's own history shows.
func (c *Client) Revisions(ctx context.Context, fileID string, pageToken string) (*RevisionList, error) {
	query := url.Values{}
	query.Set("fields", "revisions(id,modifiedTime,keepForever,size,lastModifyingUser(displayName,emailAddress)),nextPageToken")
	query.Set("pageSize", "200")
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}

	var out RevisionList
	if err := c.call(ctx, http.MethodGet,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/revisions", query), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// KeepRevision marks a revision so Drive stops pruning it.
//
// Drive thins out old revisions on its own, and the one worth going back to is exactly the
// one worth keeping. There is no request that restores a revision of a Google editor file
// through the API — the editor does that — so the honest thing this server can do is make
// sure the revision survives long enough for a person to restore it.
func (c *Client) KeepRevision(ctx context.Context, fileID, revisionID string, keep bool) (*Revision, error) {
	query := url.Values{}
	query.Set("fields", "id,modifiedTime,keepForever")

	var out Revision
	if err := c.call(ctx, http.MethodPatch,
		endpoint(c.driveBase, "/files/"+url.PathEscape(fileID)+"/revisions/"+url.PathEscape(revisionID), query),
		map[string]any{"keepForever": keep}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
