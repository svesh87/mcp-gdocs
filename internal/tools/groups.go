package tools

import (
	"fmt"
	"sort"
	"strings"
)

// A group is what a caller switches on and off: a family of documents crossed with what
// the tool does to them.
//
// The crossing is the point. "Give the agent slides" and "let it change nothing" are two
// different questions, and a server that answers only the first ends up either useless or
// too powerful. A configuration names the pieces it wants — slides-read,docs-write — and
// gets exactly those tool names, which is also the only way to keep an agent's context
// from filling with a hundred descriptions it will never call.
type Group string

// The families, crossed with what a tool does.
const (
	SlidesRead   Group = "slides-read"
	SlidesWrite  Group = "slides-write"
	SlidesCopy   Group = "slides-copy"
	SlidesDelete Group = "slides-delete"

	SheetsRead   Group = "sheets-read"
	SheetsWrite  Group = "sheets-write"
	SheetsCopy   Group = "sheets-copy"
	SheetsDelete Group = "sheets-delete"

	DocsRead   Group = "docs-read"
	DocsWrite  Group = "docs-write"
	DocsCopy   Group = "docs-copy"
	DocsDelete Group = "docs-delete"

	DriveRead   Group = "drive-read"
	DriveWrite  Group = "drive-write"
	DriveDelete Group = "drive-delete"

	// DriveShare stands apart from drive-write on purpose: everything else in this server
	// changes what is inside the account's own files, while this changes who outside the
	// account can see one. It is never included by "all".
	DriveShare Group = "drive-share"

	// Common is the handful of tools that belong to no family and are always offered:
	// the reference of Google's own enum values, which every other tool's arguments are
	// spelled in. A set without it is a set whose values have to be guessed.
	Common Group = "common"
)

// commonTools are the names in the Common group, listed rather than derived: a name that
// belongs to no family is either one of these or a mistake, and a mistake should fail.
var commonTools = map[string]bool{
	"gdocs_reference": true,
}

// allGroups is every group there is, in the order they are reported.
var allGroups = []Group{
	Common,
	SlidesRead, SlidesWrite, SlidesCopy, SlidesDelete,
	SheetsRead, SheetsWrite, SheetsCopy, SheetsDelete,
	DocsRead, DocsWrite, DocsCopy, DocsDelete,
	DriveRead, DriveWrite, DriveDelete, DriveShare,
}

// fileCopyTools are the tools whose names say "copy" and whose subject is a file rather
// than the content of one.
//
// The line the copy groups draw is between carrying content from one document into
// another and duplicating a file on Drive. The first is what an operator may want to
// forbid — "work here, but do not drag things in from elsewhere" — while the second is
// how every deck starts, from a copy of a template, and switching that off by accident
// would break the main way this server is used. There is no drive-copy group for the same
// reason: a file copy belongs to Drive's ordinary writing.
var fileCopyTools = map[string]bool{
	"gdocs_drive_copy":               true,
	"gdocs_slides_copy_presentation": true,
}

// families maps a family name to the groups it stands for. Naming a family is shorthand
// for reading and writing it — never for deleting, which is always spelled out, and never
// for carrying content in from another document, which an operator who names a family
// this precisely is likely to have an opinion about.
var families = map[string][]Group{
	"slides": {SlidesRead, SlidesWrite},
	"sheets": {SheetsRead, SheetsWrite},
	"docs":   {DocsRead, DocsWrite},
	"drive":  {DriveRead, DriveWrite},
}

// readingVerbs are the words that make a tool a reading one. The name of a tool decides
// its group, so the names have to say what they do — which is why the file tools are
// called gdocs_drive_export_file and gdocs_slides_export_images rather than anything
// shorter.
// "download" is here for the same reason as "export": both take a file out of Drive and
// leave Drive as it was. What they write to is the server's own files directory, which is
// what --files-dir is for, and neither changes anything an operator could lose.
var readingVerbs = []string{
	"read", "list", "inspect", "info", "search", "export", "download", "thumbnail",
	"colors", "structure",
}

// GroupOf works out which group a tool belongs to from its name.
//
// Deriving it rather than declaring it at every registration keeps one truth instead of
// two: a tool named gdocs_sheets_delete cannot end up in sheets-write by an oversight. The
// exact composition is pinned by a test, so a name that lands in the wrong group fails the
// build rather than the operator's expectations.
func GroupOf(name string) (Group, error) {
	if commonTools[name] {
		return Common, nil
	}

	trimmed := strings.TrimPrefix(name, "gdocs_")

	family, rest, found := strings.Cut(trimmed, "_")
	if !found {
		return "", fmt.Errorf("the tool name %q says no family", name)
	}

	// Sharing is one tool's worth of a group, and its name is the whole of the rule.
	if family == "drive" && (strings.HasPrefix(rest, "share") || strings.HasPrefix(rest, "unshare")) {
		return DriveShare, nil
	}

	class := "write"
	switch {
	case strings.Contains(rest, "delete"):
		class = "delete"
	case strings.Contains(rest, "copy") && !fileCopyTools[name]:
		class = "copy"
	default:
		for _, verb := range readingVerbs {
			if strings.Contains(rest, verb) {
				class = "read"
				break
			}
		}
	}

	group := Group(family + "-" + class)
	for _, known := range allGroups {
		if known == group {
			return group, nil
		}
	}

	return "", fmt.Errorf("the tool name %q lands in %q, which is not a group", name, group)
}

// defaultGroups is the set a server offers when nothing was asked for: everything except
// removal and sharing, copying between documents included. A configuration that says
// nothing gets a server that can do the work, and the two things it does not get are the
// two whose damage reaches outside the file being edited.
func defaultGroups() map[Group]bool {
	enabled := map[Group]bool{}
	for _, group := range allGroups {
		switch group {
		case SlidesDelete, SheetsDelete, DocsDelete, DriveDelete, DriveShare:
			continue
		}
		enabled[group] = true
	}

	return enabled
}

// ParseGroups reads the value of --tools.
//
// Names add up: "all,docs-delete" is everything ordinary plus one kind of removal.
// "slides" is the family, meaning its read and write halves. An unknown name is refused
// rather than ignored — a typo that silently drops a family is a server that quietly does
// less than the operator thinks it does.
func ParseGroups(value string) (map[Group]bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultGroups(), nil
	}

	known := map[string]Group{}
	for _, group := range allGroups {
		known[string(group)] = group
	}

	enabled := map[Group]bool{}
	for _, piece := range strings.Split(value, ",") {
		name := strings.ToLower(strings.TrimSpace(piece))
		if name == "" {
			continue
		}

		switch {
		case name == "all":
			for group := range defaultGroups() {
				enabled[group] = true
			}
		case families[name] != nil:
			for _, group := range families[name] {
				enabled[group] = true
			}
		case known[name] != "":
			enabled[known[name]] = true
		default:
			return nil, fmt.Errorf("unknown tool group %q; the groups are %s; the families %s stand "+
				"for their read and write halves; all is everything except removal and sharing",
				name, strings.Join(groupNames(), ", "), strings.Join(familyNames(), ", "))
		}
	}

	if len(enabled) == 0 {
		return nil, fmt.Errorf("no tool groups named")
	}

	// The reference comes with every set: its whole job is to say which values the other
	// tools' arguments accept, and a set without it is a set that has to be guessed at.
	enabled[Common] = true

	return enabled, nil
}

// GroupNames lists every group, for a message to an operator.
func GroupNames() []string { return groupNames() }

// Families lists the families a set can be narrowed to, which is also the list of
// sub-paths the HTTP transport serves.
func Families() []string { return familyNames() }

// Narrow keeps only the groups of one family, and always the common ones.
//
// This is what makes /mcp/slides a window on the same process rather than a second
// configuration: the family cannot widen what --tools allowed, only cut it down.
func Narrow(enabled map[Group]bool, family string) map[Group]bool {
	wanted := map[Group]bool{}
	for _, group := range families[family] {
		wanted[group] = true
	}
	// The family shorthand covers reading and writing; removal and sharing of that same
	// family belong to it too, when the configuration allowed them at all.
	for _, group := range allGroups {
		if strings.HasPrefix(string(group), family+"-") {
			wanted[group] = true
		}
	}

	narrowed := map[Group]bool{Common: true}
	for group := range wanted {
		if enabled[group] {
			narrowed[group] = true
		}
	}

	return narrowed
}

func groupNames() []string {
	names := make([]string, 0, len(allGroups))
	for _, group := range allGroups {
		names = append(names, string(group))
	}

	return names
}

func familyNames() []string {
	names := make([]string, 0, len(families))
	for name := range families {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
