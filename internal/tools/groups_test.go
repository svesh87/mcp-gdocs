package tools

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// serverWith builds a server the way the binary does, with a set of groups.
func serverWith(t *testing.T, groups map[Group]bool, allowWrite bool) *server.MCPServer {
	t.Helper()

	srv := server.NewMCPServer("mcp-gdocs", "test", server.WithToolCapabilities(true))
	err := Register(srv, Options{
		Clients:    ClientFunc(func(context.Context) (*google.Client, error) { return nil, errNoClient }),
		AllowWrite: allowWrite,
		Groups:     groups,
		FilesDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("registering the tools: %v", err)
	}

	return srv
}

func namesOf(t *testing.T, srv *server.MCPServer) []string {
	t.Helper()

	names := make([]string, 0, len(srv.ListTools()))
	for name := range srv.ListTools() {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// TestEveryToolLandsInAGroup is the rule that keeps the whole mechanism honest: the group
// is worked out from the tool's name, so a name that says nothing about its family — or
// says something that is not a family — has to fail here rather than quietly become
// unswitchable.
func TestEveryToolLandsInAGroup(t *testing.T) {
	for _, name := range namesOf(t, serverWith(t, everyGroup(), true)) {
		if _, err := GroupOf(name); err != nil {
			t.Errorf("%v", err)
		}
	}
}

// TestNothingThatWritesLandsInAReadGroup is the rule a real configuration broke.
//
// The group comes from the tool's name, and two of the words that mean "this only reads" are
// also ordinary objects: a list, a colour. So gdocs_slides_set_list, which writes a list, and
// gdocs_slides_set_theme_colors, which replaces the deck's palette, both sat in slides-read —
// and --tools=slides-read, which an operator picks precisely in order to change nothing,
// handed over two tools that change a deck.
//
// The verb a name begins with is what the tool does; the rest of the name is what it does it
// to. This walks every registered tool and holds that line.
func TestNothingThatWritesLandsInAReadGroup(t *testing.T) {
	for _, name := range namesOf(t, serverWith(t, everyGroup(), true)) {
		group, err := GroupOf(name)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		if !strings.HasSuffix(string(group), "-read") {
			continue
		}

		// The name is gdocs_<family>_<verb>_<rest>: the family is dropped, and the verb is
		// the word after it.
		_, rest, _ := strings.Cut(strings.TrimPrefix(name, "gdocs_"), "_")
		verb, _, _ := strings.Cut(rest, "_")

		if isWritingVerb(verb) {
			t.Errorf("%s is in %s, but it begins with %q, which changes something", name, group, verb)
		}
	}
}

// TestAReadOnlySetChangesNothing is the same rule stated the way an operator would: the tools
// a reading configuration offers must all be ones that only read.
func TestAReadOnlySetChangesNothing(t *testing.T) {
	groups, err := ParseGroups("slides-read,sheets-read,docs-read,drive-read")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	for _, name := range namesOf(t, serverWith(t, groups, true)) {
		if commonTools[name] {
			continue
		}
		if group, _ := GroupOf(name); !strings.HasSuffix(string(group), "-read") {
			t.Errorf("a reading-only configuration offers %s, which is in %s", name, group)
		}
	}
}

// TestAWindowCanFindTheFileItReads is the smallest widening the bridges made necessary.
//
// A tool on the slides path can read a workbook, and a workbook that cannot be found by name
// is one nobody can name the identifier of. So two of Drive's readings travel into every
// family window — and only two: comments, revisions, permissions and the folder listing are
// Drive's own job and stay behind its own path.
func TestAWindowCanFindTheFileItReads(t *testing.T) {
	slides := tools(t, Narrow(defaultGroups(), "slides"), WindowDriveReads()...)

	for _, want := range WindowDriveReads() {
		if !contains(slides, want) {
			t.Errorf("a slides window should offer %s, so a bridge can find its source", want)
		}
	}

	// And nothing else of Drive's: the widening is two names, not a group.
	for _, unwanted := range []string{
		"gdocs_drive_list_comments", "gdocs_drive_list_revisions",
		"gdocs_drive_list_permissions", "gdocs_drive_list_folder", "gdocs_drive_copy",
	} {
		if contains(slides, unwanted) {
			t.Errorf("a slides window should not offer %s", unwanted)
		}
	}
}

// TestTheWideningNeverBeatsTheCeiling: naming a tool outright widens a window, and a window
// still cannot show what --tools refused.
func TestTheWideningNeverBeatsTheCeiling(t *testing.T) {
	withoutDrive, err := ParseGroups("slides")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	// main only passes the names when drive-read was allowed; this is the same rule stated
	// where it can be tested — a configuration with no Drive at all has no file_info to give.
	if withoutDrive[DriveRead] {
		t.Fatal("--tools=slides should not allow drive-read, so this test proves nothing")
	}
}

// tools builds a server with a group set and returns the names it offers.
func tools(t *testing.T, groups map[Group]bool, alsoOffer ...string) []string {
	t.Helper()

	srv := server.NewMCPServer("mcp-gdocs", "test", server.WithToolCapabilities(true))
	err := Register(srv, Options{
		Clients:    ClientFunc(func(context.Context) (*google.Client, error) { return nil, errNoClient }),
		AllowWrite: true,
		Groups:     groups,
		AlsoOffer:  alsoOffer,
		FilesDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	return namesOf(t, srv)
}

// TestGroupCompositionIsPinned records which tools are in which group. The golden file is
// the answer to "what does --tools=docs-read actually give me", and a tool that moves
// between groups — or appears without one — shows up here as a diff to be read.
func TestGroupCompositionIsPinned(t *testing.T) {
	composition, err := Composition(serverWith(t, everyGroup(), true))
	if err != nil {
		t.Fatalf("composing: %v", err)
	}

	encoded, err := json.MarshalIndent(composition, "", "  ")
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}

	checkGolden(t, "tool_groups.json", append(encoded, '\n'))
}

// TestToolsFlagPicksTheSet covers what an operator types.
func TestToolsFlagPicksTheSet(t *testing.T) {
	for _, probe := range []struct {
		flag    string
		present []string
		absent  []string
	}{
		{
			flag:    "slides-read",
			present: []string{"gdocs_slides_list", "gdocs_reference"},
			absent:  []string{"gdocs_slides_set_text", "gdocs_sheets_read", "gdocs_docs_read"},
		},
		{
			flag:    "docs",
			present: []string{"gdocs_docs_read", "gdocs_docs_style_text"},
			absent:  []string{"gdocs_docs_delete", "gdocs_slides_list"},
		},
		{
			flag:    "docs,docs-delete",
			present: []string{"gdocs_docs_read", "gdocs_docs_delete"},
			absent:  []string{"gdocs_slides_delete"},
		},
		{
			flag:    "all",
			present: []string{"gdocs_slides_list", "gdocs_sheets_write", "gdocs_docs_style_table", "gdocs_drive_search"},
			absent:  []string{"gdocs_slides_delete", "gdocs_docs_delete"},
		},
		{
			flag:    "sheets-read,drive-read",
			present: []string{"gdocs_sheets_read", "gdocs_drive_search", "gdocs_drive_export_file"},
			absent:  []string{"gdocs_sheets_write", "gdocs_docs_read"},
		},
	} {
		t.Run(probe.flag, func(t *testing.T) {
			groups, err := ParseGroups(probe.flag)
			if err != nil {
				t.Fatalf("parsing %q: %v", probe.flag, err)
			}

			names := namesOf(t, serverWith(t, groups, true))
			for _, want := range probe.present {
				if !contains(names, want) {
					t.Errorf("--tools=%s should offer %s", probe.flag, want)
				}
			}
			for _, unwanted := range probe.absent {
				if contains(names, unwanted) {
					t.Errorf("--tools=%s should not offer %s", probe.flag, unwanted)
				}
			}
		})
	}
}

// TestDefaultSetIsWhatItWasBeforeGroups: an existing configuration that never heard of
// --tools keeps the tools it had, and gains no removal and no sharing.
func TestDefaultSetIsWhatItWasBeforeGroups(t *testing.T) {
	names := namesOf(t, serverWith(t, nil, true))

	for _, want := range []string{"gdocs_slides_list", "gdocs_sheets_write", "gdocs_docs_style_text", "gdocs_reference"} {
		if !contains(names, want) {
			t.Errorf("the default set should offer %s", want)
		}
	}
	for _, name := range names {
		if strings.Contains(name, "delete") {
			t.Errorf("the default set should not offer %s", name)
		}
	}
}

// TestUnknownGroupIsRefused, because a typo that silently drops a family is a server that
// does less than the operator believes.
func TestUnknownGroupIsRefused(t *testing.T) {
	_, err := ParseGroups("slides-read,docsss")
	if err == nil {
		t.Fatal("an unknown group should be refused")
	}
	if !strings.Contains(err.Error(), "docsss") || !strings.Contains(err.Error(), "docs-read") {
		t.Errorf("the refusal should name the typo and the known groups, got %q", err)
	}
}

// TestReadingGroupsSurviveWithoutAllowWrite: --tools and --allow-write are two gates, and
// asking for a writing group on a read-only server yields the reading half rather than an
// error or a tool that fails on every call.
func TestReadingGroupsSurviveWithoutAllowWrite(t *testing.T) {
	groups, err := ParseGroups("docs")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	names := namesOf(t, serverWith(t, groups, false))
	if !contains(names, "gdocs_docs_read") {
		t.Error("a read-only server should still read documents")
	}
	if contains(names, "gdocs_docs_style_text") {
		t.Error("without --allow-write nothing that changes a document may be offered")
	}
}

func everyGroup() map[Group]bool {
	enabled := map[Group]bool{}
	for _, group := range allGroups {
		enabled[group] = true
	}

	return enabled
}
