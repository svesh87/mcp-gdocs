package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registeredTools is the names a server offers with the given settings.
func registeredTools(t *testing.T, allowWrite bool) []string {
	t.Helper()

	mcpServer := server.NewMCPServer("mcp-gdocs", "test", server.WithToolCapabilities(true))

	err := Register(mcpServer, Options{
		Clients:    ClientFunc(func(context.Context) (*google.Client, error) { return nil, errNoClient }),
		AllowWrite: allowWrite,
	})
	if err != nil {
		t.Fatalf("registering the tools: %v", err)
	}

	answer := mcpServer.HandleMessage(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))

	response, ok := answer.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("listing the tools failed: %+v", answer)
	}

	listing, ok := response.Result.(mcp.ListToolsResult)
	if !ok {
		t.Fatalf("the listing came back as %T", response.Result)
	}

	names := make([]string, 0, len(listing.Tools))
	for _, tool := range listing.Tools {
		names = append(names, tool.Name)
	}

	return names
}

// deletionTools is every tool this server is allowed to offer that removes anything, and
// the list is the rule: removal stops at the edge of the file it works in.
//
// Inside a presentation or a document, removal is ordinary editing — a copied deck has to
// come down to the slides that apply, a step that landed wrong leaves a paragraph or a
// table behind, and without a way back the only repair is a person in a browser. Outside
// one, nothing: no file, no folder, no drive. That is the line, and a name added here
// without moving it is a name that fails the tests below.
var deletionTools = []string{
	"gdocs_slides_delete",
	"gdocs_docs_delete",
}

// TestDeletionStopsInsideTheFile keeps that line where it is.
func TestDeletionStopsInsideTheFile(t *testing.T) {
	for _, allowWrite := range []bool{false, true} {
		for _, name := range registeredTools(t, allowWrite) {
			if contains(deletionTools, name) {
				if !allowWrite {
					t.Errorf("%s changes a document and must not exist without --allow-write", name)
				}
				continue
			}

			for _, forbidden := range []string{"delete", "remove", "trash", "clear", "destroy"} {
				if strings.Contains(name, forbidden) {
					t.Errorf("with allow_write=%v the server offers %q, which is a deletion tool "+
						"and is not in the list that says which ones may exist", allowWrite, name)
				}
			}
		}
	}
}

// TestNoDeletionOfFilesThemselves is the half of the rule that has not moved: Drive and
// the file tools have no removal in them at all, whatever happens inside a document.
func TestNoDeletionOfFilesThemselves(t *testing.T) {
	for _, name := range registeredTools(t, true) {
		if !strings.HasPrefix(name, "gdocs_slides_") && !strings.HasPrefix(name, "gdocs_docs_") {
			for _, forbidden := range []string{"delete", "remove", "trash", "destroy"} {
				if strings.Contains(name, forbidden) {
					t.Errorf("%s deletes something outside a document or a presentation", name)
				}
			}
		}
	}
}

// TestNoRawBatchUpdateTool guards the other rule: a caller cannot hand this server an
// arbitrary batch of API requests. Assembled batches are exactly what puts text boxes at
// invented coordinates and leaves a deck looking broken.
func TestNoRawBatchUpdateTool(t *testing.T) {
	for _, name := range registeredTools(t, true) {
		if strings.Contains(name, "batch") || strings.Contains(name, "raw") || strings.Contains(name, "request") {
			t.Errorf("the server offers %q, which looks like a way to send arbitrary requests", name)
		}
	}
}

func TestReadingToolsAreAlwaysOffered(t *testing.T) {
	readOnly := registeredTools(t, false)

	for _, want := range []string{
		"gdocs_slides_inspect_text_structure",
		"gdocs_slides_inspect_title_style",
		"gdocs_slides_inspect_page",
		"gdocs_slides_read_theme",
		"gdocs_slides_read_table",
		"gdocs_slides_list",
		"gdocs_slides_list_layouts",
		"gdocs_slides_export_thumbnail",
		"gdocs_sheets_info",
		"gdocs_sheets_read",
		"gdocs_docs_read",
		"gdocs_drive_search",
		"gdocs_drive_file_info",
		"gdocs_drive_export",
	} {
		if !contains(readOnly, want) {
			t.Errorf("a read-only server should still offer %s, it offers %v", want, readOnly)
		}
	}
}

func TestWritingToolsNeedAllowWrite(t *testing.T) {
	readOnly := registeredTools(t, false)
	writing := registeredTools(t, true)

	for _, name := range []string{
		"gdocs_slides_set_list",
		"gdocs_slides_hide",
		"gdocs_slides_create_table_with_text",
		"gdocs_slides_copy_presentation",
		"gdocs_slides_add_slide",
		"gdocs_slides_set_text",
		"gdocs_slides_set_page_background",
		"gdocs_slides_set_paragraph_style",
		"gdocs_slides_set_speaker_notes",
		"gdocs_slides_set_theme_colors",
		"gdocs_slides_style_layout",
		"gdocs_slides_style_shape",
		"gdocs_slides_style_image",
		"gdocs_slides_create_shape",
		"gdocs_slides_create_line",
		"gdocs_slides_order_elements",
		"gdocs_slides_group",
		"gdocs_slides_duplicate",
		"gdocs_sheets_write",
		"gdocs_sheets_append",
		"gdocs_sheets_create",
		"gdocs_sheets_add_tab",
		"gdocs_sheets_format_cells",
		"gdocs_docs_create",
		"gdocs_docs_append",
		"gdocs_docs_insert_text",
		"gdocs_docs_replace_text",
		"gdocs_drive_copy",
	} {
		if contains(readOnly, name) {
			t.Errorf("%s changes documents and should not be offered without --allow-write", name)
		}
		if !contains(writing, name) {
			t.Errorf("%s should be offered with --allow-write, the server offers %v", name, writing)
		}
	}
}

// TestToolNamesAreNamespaced keeps the names one family, so a client that runs several
// servers can tell whose tool is whose.
func TestToolNamesAreNamespaced(t *testing.T) {
	for _, name := range registeredTools(t, true) {
		if !strings.HasPrefix(name, "gdocs_") {
			t.Errorf("%s is not part of this server's family of names", name)
		}
	}
}

func TestGeneratedObjectIDsAreUsable(t *testing.T) {
	first := newObjectID("table")
	second := newObjectID("table")

	if first == second {
		t.Error("two generated identifiers should differ")
	}
	// Slides wants at least five characters of letters, digits, underscores or hyphens.
	if len(first) < 5 {
		t.Errorf("the identifier %q is too short for Slides", first)
	}
	for _, r := range first {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-", r) {
			t.Errorf("the identifier %q carries %q, which Slides does not accept", first, r)
		}
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
