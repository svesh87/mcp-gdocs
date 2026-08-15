// Package tools exposes Google Slides, Sheets, Docs and Drive as MCP tools.
//
// The tools are narrow on purpose. There is no generic "send this batch to the API"
// tool anywhere in here: a batch assembled by a caller is exactly what puts text boxes
// at arbitrary coordinates, replaces a real table with a scattering of shapes and
// leaves a deck looking broken. Each tool below builds its own requests, in the order
// that keeps a template's own styling in charge.
//
// Removal stops at the bin. Inside a presentation, a document or a workbook, removing
// something is ordinary editing, so those tools exist — each in a group of its own, none
// of them in the default set. A file goes as far as the bin, which its owner can undo.
// Emptying that bin, deleting a file outright and removing a folder have no code here at
// all: an agent that tidied up somebody's shared drive is an incident, and the cheapest
// way to not have that incident is to have nothing that could cause it.
//
// Carrying content in from another document is its own class of tool, in its own groups.
// Google gives exactly one request for it — copying a sheet into another workbook — so
// everything else here reads the source and rebuilds it in the target, in one pass,
// naming in the answer whatever it could not carry.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// Clients hands out an authenticated client, or says why it cannot.
//
// It is an interface and it is consulted per call rather than once at startup: a server
// started in a container may have nobody signed in yet, and it has to run anyway so the
// sign-in page can be opened against it.
type Clients interface {
	Google(ctx context.Context) (*google.Client, error)
}

// ClientFunc adapts a function to Clients.
type ClientFunc func(ctx context.Context) (*google.Client, error)

// Google calls the function.
func (f ClientFunc) Google(ctx context.Context) (*google.Client, error) { return f(ctx) }

// Options is what the tools need to work.
type Options struct {
	Clients Clients
	// AllowWrite gates every tool that changes anything. Reading is always offered;
	// writing is a decision the operator makes when starting the server.
	AllowWrite bool
	// NewObjectID names things this server creates in a presentation. It is settable so
	// a test can pin the identifiers and compare whole request bodies against a golden
	// file; left empty it produces random ones.
	NewObjectID func(prefix string) string
	// FilesDir is the one directory exports may be written to and imports read from.
	// Empty means the file tools are not offered at all: a server that can write anywhere
	// is a server that can overwrite anything the account it runs as owns.
	FilesDir string
	// Groups is the set of tool groups this server offers, as --tools asked for. Nil
	// means the default set: everything except removal and sharing, which is what the
	// server offered before groups existed.
	Groups map[Group]bool
}

type registry struct{ opts Options }

// objectID names something about to be created.
func (r *registry) objectID(prefix string) string {
	if r.opts.NewObjectID != nil {
		return r.opts.NewObjectID(prefix)
	}
	return newObjectID(prefix)
}

// Register adds the tools this configuration allows.
//
// Everything is registered first and the groups that were not asked for are taken back
// out, rather than each registration consulting the set. That keeps the decision in one
// place — a tool cannot be added in a way that forgets about --tools — and the removal is
// real: a tool taken out here is not listed and cannot be called.
func Register(srv *server.MCPServer, opts Options) error {
	if opts.Clients == nil {
		return errors.New("a client provider is required")
	}

	r := &registry{opts: opts}

	r.registerSlides(srv)
	r.registerSheets(srv)
	r.registerDocs(srv)
	r.registerDrive(srv)
	r.registerDriveManage(srv)
	r.registerFiles(srv)

	return keepGroups(srv, opts.Groups)
}

// keepGroups removes every registered tool whose group was not asked for.
func keepGroups(srv *server.MCPServer, groups map[Group]bool) error {
	if groups == nil {
		groups = defaultGroups()
	}

	var drop []string
	for name := range srv.ListTools() {
		group, err := GroupOf(name)
		if err != nil {
			return err
		}
		if !groups[group] {
			drop = append(drop, name)
		}
	}

	if len(drop) > 0 {
		sort.Strings(drop)
		srv.DeleteTools(drop...)
	}

	return nil
}

// Composition reports which tools a server offers, by group. It is what the tests pin the
// groups against, and what an operator is shown when a name is missing.
func Composition(srv *server.MCPServer) (map[Group][]string, error) {
	out := map[Group][]string{}

	for name := range srv.ListTools() {
		group, err := GroupOf(name)
		if err != nil {
			return nil, err
		}
		out[group] = append(out[group], name)
	}

	for _, names := range out {
		sort.Strings(names)
	}

	return out, nil
}

// client is the per-call way to reach Google.
func (r *registry) client(ctx context.Context) (*google.Client, error) {
	return r.opts.Clients.Google(ctx)
}

// filePath turns a caller's file name into a path inside the one directory this server
// may touch, and refuses anything that would leave it.
//
// The check is on the cleaned path rather than on the name, because "..%2f" and a symlink
// spelled as a plain name both look harmless one character at a time. A server that lets
// a caller name an absolute path is a server that writes over its own token file.
func (r *registry) filePath(name string) (string, error) {
	if r.opts.FilesDir == "" {
		return "", fmt.Errorf("this server was started without --files-dir, so it cannot read or write files")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("the file name is empty")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("%q is an absolute path: name a file inside the server's files directory, "+
			"not a place on its disk", name)
	}

	base := filepath.Clean(r.opts.FilesDir)
	full := filepath.Clean(filepath.Join(base, name))

	if full != base && !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return "", fmt.Errorf("%q leads outside the server's files directory", name)
	}

	return full, nil
}

func toolError(err error) *mcp.CallToolResult {
	return mcp.NewToolResultError(err.Error())
}

// requiredString reads an argument that has to be there and cannot be blank.
func requiredString(req mcp.CallToolRequest, name string) (string, error) {
	value, err := req.RequireString(name)
	if err != nil {
		return "", err
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is empty", name)
	}

	return value, nil
}

// optionalString reads an argument that may be absent.
func optionalString(req mcp.CallToolRequest, name string) string {
	return strings.TrimSpace(req.GetString(name, ""))
}

// objectList reads a list of objects out of the arguments, naming the argument in every
// error: a caller that got the shape wrong needs to know which one.
func objectList(req mcp.CallToolRequest, name string) ([]map[string]any, error) {
	raw, ok := req.GetArguments()[name]
	if !ok {
		return nil, fmt.Errorf("%s is required", name)
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list, got %T", name, raw)
	}

	objects := make([]map[string]any, 0, len(list))
	for index, item := range list {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object, got %T", name, index, item)
		}
		objects = append(objects, object)
	}

	return objects, nil
}

// stringField reads a string out of a decoded object.
func stringField(object map[string]any, name string) string {
	value, _ := object[name].(string)
	return strings.TrimSpace(value)
}

// stringListField reads a list of strings out of a decoded object.
func stringListField(object map[string]any, name string) ([]string, error) {
	raw, ok := object[name]
	if !ok {
		return nil, nil
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list of strings, got %T", name, raw)
	}

	values := make([]string, 0, len(list))
	for index, item := range list {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a string, got %T", name, index, item)
		}
		values = append(values, text)
	}

	return values, nil
}

// utf16Length counts text the way the Google APIs index it.
//
// Both Slides and Docs count in UTF-16 code units, not bytes and not runes. For English
// the three agree and the difference never shows; for Russian a byte count is twice too
// large, and a text range built from it lands in the middle of nothing and the API
// refuses the whole batch.
func utf16Length(text string) int64 {
	return int64(len(utf16.Encode([]rune(text))))
}

// firstLineEnd is where the title line of a text box ends, counted the way the API
// counts.
func firstLineEnd(text string) int64 {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return utf16Length(text[:index])
	}

	return utf16Length(text)
}

// resultJSON renders a payload for the caller. Both forms are filled in: the text one
// for clients that show it to a person, the structured one for clients that parse it.
func resultJSON(payload any) (*mcp.CallToolResult, error) {
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return toolError(fmt.Errorf("encoding the answer: %w", err)), nil
	}

	result := mcp.NewToolResultText(string(encoded))
	result.StructuredContent = payload

	return result, nil
}
