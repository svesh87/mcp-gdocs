package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// narrowedHarness is a registry that may remove things inside a document but nothing whole.
//
// That is the configuration the finer groups exist for: a step that landed wrong leaves a
// shape or a row behind and somebody has to be able to take it out, while a slide or a tab is
// an hour's work or data nobody has any more.
func narrowedHarness(t *testing.T, fake *fakeGoogle, groups ...Group) *harness {
	t.Helper()

	h := newHarness(t, fake)

	enabled := map[Group]bool{}
	for _, group := range allGroups {
		switch group {
		case SlidesDeletePage, SheetsDeleteTab, DocsDeleteTab:
			continue
		}
		enabled[group] = true
	}
	for _, group := range groups {
		enabled[group] = true
	}
	h.registry.opts.Groups = enabled

	return h
}

// deckToPruneFrom is two slides, one with a shape on it. Two, because a deck cannot be taken
// down to nothing and a one-slide fixture would be refused for that reason instead of this
// one — which would leave the rule under test unproven.
const deckToPruneFrom = `{
  "presentationId": "deck",
  "slides": [
    {"objectId": "slide1", "pageElements": [
      {"objectId": "shape1", "shape": {"shapeType": "TEXT_BOX"}}]},
    {"objectId": "slide2", "pageElements": []}
  ],
  "layouts": [],
  "masters": []
}`

// TestRemovalStopsShortOfAWholePage walks every pair of target and group. The pairs are the
// whole of the mechanism: one tool serves both jobs, so the group cannot be worked out from
// its name and has to be checked against what the call actually named.
func TestRemovalStopsShortOfAWholePage(t *testing.T) {
	for _, probe := range []struct {
		name    string
		finer   Group
		fake    func(t *testing.T) *fakeGoogle
		call    func(h *harness) (*mcp.CallToolResult, error)
		refused string
	}{
		{
			name:  "a slide",
			finer: SlidesDeletePage,
			fake: func(t *testing.T) *fakeGoogle {
				return newFakeGoogle(t).
					answer(":batchUpdate", emptyBatchReply).
					answer("/presentations/deck", deckToPruneFrom)
			},
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.slidesDelete(context.Background(), request(map[string]any{
					"presentation_id": "deck", "object_ids": []any{"slide1"},
				}))
			},
			refused: "slides-delete-page",
		},
		{
			name:  "a tab of a workbook",
			finer: SheetsDeleteTab,
			fake: func(t *testing.T) *fakeGoogle {
				return newFakeGoogle(t).
					answer(":batchUpdate", `{"spreadsheetId": "book", "replies": [{}]}`).
					answer("/spreadsheets/book", spreadsheetInfo)
			},
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.sheetsDelete(context.Background(), request(map[string]any{
					"spreadsheet_id": "book", "what": "tab", "sheet_title": "Отделы",
				}))
			},
			refused: "sheets-delete-tab",
		},
		{
			name:  "a tab of a document",
			finer: DocsDeleteTab,
			fake: func(t *testing.T) *fakeGoogle {
				return newFakeGoogle(t).answer(":batchUpdate", `{"documentId": "doc", "replies": [{}]}`)
			},
			call: func(h *harness) (*mcp.CallToolResult, error) {
				return h.registry.docsDelete(context.Background(), request(map[string]any{
					"document_id": "doc", "tab_id": "t.0",
				}))
			},
			refused: "docs-delete-tab",
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			t.Run("refused without the group", func(t *testing.T) {
				h := narrowedHarness(t, probe.fake(t))

				message := h.fail(probe.call(h))
				if !strings.Contains(message, probe.refused) {
					t.Errorf("the refusal has to name the group that is missing, got %s", message)
				}
				// An operator reading a refusal needs to know what the server still can do,
				// or the only safe reading is "removal is broken".
				if !strings.Contains(message, "switched off") {
					t.Errorf("the refusal should say this is a configuration, got %s", message)
				}
			})

			t.Run("allowed with it", func(t *testing.T) {
				h := narrowedHarness(t, probe.fake(t), probe.finer)
				h.ok(probe.call(h))
			})
		})
	}
}

// TestRemovingSomethingSmallerNeedsNoExtraGroup: the finer groups are about what disappears,
// and a shape on a slide disappears whatever they say.
func TestRemovingSomethingSmallerNeedsNoExtraGroup(t *testing.T) {
	h := narrowedHarness(t, newFakeGoogle(t).
		answer(":batchUpdate", emptyBatchReply).
		answer("/presentations/deck", deckToPruneFrom))

	h.ok(h.registry.slidesDelete(context.Background(), request(map[string]any{
		"presentation_id": "deck", "object_ids": []any{"shape1"},
	})))
}

// TestThePageGroupsAreNeverInTheDefaultSet keeps the rule that has held since removal existed
// here at all: a configuration that says nothing removes nothing.
func TestThePageGroupsAreNeverInTheDefaultSet(t *testing.T) {
	for group := range pageGroups {
		finer := pageGroups[group]
		if defaultGroups()[finer] {
			t.Errorf("%s is in the default set, and no removal ever should be", finer)
		}
		if _, described := pageWords[finer]; !described {
			t.Errorf("%s has no words to refuse with, so its refusal would say nothing useful", finer)
		}
	}
}

// TestThePageGroupsAreAskedForByName: they are permissions rather than sets of tools, so
// nothing registers into them — but --tools has to accept them, or they cannot be switched on.
func TestThePageGroupsAreAskedForByName(t *testing.T) {
	enabled, err := ParseGroups("all,slides-delete,slides-delete-page")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if !enabled[SlidesDeletePage] || !enabled[SlidesDelete] {
		t.Error("naming both groups should switch both on")
	}
	if enabled[SheetsDeleteTab] {
		t.Error("one family's finer group should not switch another's on")
	}

	// The window on a family carries its own permissions with it, or /mcp/slides would be a
	// window that cannot do what the server was started to allow.
	if !Narrow(enabled, "slides")[SlidesDeletePage] {
		t.Error("the slides window should keep the slides page permission")
	}
}
