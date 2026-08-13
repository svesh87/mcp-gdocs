package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// The colours a dropdown paints its options in are in no API answer, so they are read out
// of the rendering the editors serve. These patterns pull them out of that HTML.
//
// A coloured option is drawn as a span with a rounded corner, so the classes carrying a
// border-radius are what says which spans are options: an ordinary cell paints its fill on
// the td, and picking spans by their inline colours alone would take those too.
var (
	optionClassPattern = regexp.MustCompile(`\.(s\d+)\s*\{[^}]*border-radius[^}]*\}`)
	optionRowPattern   = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	optionCellPattern  = regexp.MustCompile(`(?s)<td\b[^>]*>(.*?)</td>`)
	optionSpanPattern  = regexp.MustCompile(`(?s)<span class="(s\d+)"[^>]*style="([^"]*)"[^>]*>(.*?)</span>`)
	optionTagPattern   = regexp.MustCompile(`<[^>]+>`)
	optionFillPattern  = regexp.MustCompile(`background-color:\s*([^;"]+)`)
	optionTextPattern  = regexp.MustCompile(`(?:^|;)\s*color:\s*([^;"]+)`)
)

// optionLimit is how many distinct option-and-column pairs are reported. A dropdown has a
// handful of options; a workbook where this trips is one where something else is going on,
// and the answer says so rather than quietly ending early.
const optionLimit = 500

// dropdownOption is one option of a dropdown together with the colours it is drawn in.
type dropdownOption struct {
	Column     int    `json:"column"`
	Value      string `json:"value"`
	Background string `json:"background,omitempty"`
	TextColor  string `json:"text_color,omitempty"`
	Cells      int    `json:"cells"`
}

// dropdownTab groups the coloured options found on one tab.
type dropdownTab struct {
	Title   string           `json:"title"`
	Options []dropdownOption `json:"options"`
	Cutoff  bool             `json:"more_than_reported,omitempty"`
}

func (r *registry) registerSheetsDropdown(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_sheets_read_dropdown_colors",
		mcp.WithDescription("Read the colours a dropdown paints its options in — the green \"Done\", the "+
			"red \"Blocked\". No API carries them: a cell holding a coloured option answers black on white "+
			"in effectiveFormat, and DataValidationRule is only condition, inputMessage, showCustomUi and "+
			"strict. So this asks the editors for the sheet as HTML, where the same code that draws the "+
			"sheet writes each option's fill and text colour as CSS, and reports them per column. "+
			"What to do with them is a decision with no good answer — see the note on the limit in "+
			"gdocs_sheets_set_validation. Reading a large workbook this way takes a few seconds, because "+
			"it renders the whole file."),
		mcp.WithString("spreadsheet_id", mcp.Required(), mcp.Description(spreadsheetIDHelp)),
		mcp.WithString("sheet_title", mcp.Description("One tab to report. Without it, every tab.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.sheetsReadDropdownColors)
}

func (r *registry) sheetsReadDropdownColors(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := r.client(ctx)
	if err != nil {
		return nil, err
	}

	spreadsheetID, err := req.RequireString("spreadsheet_id")
	if err != nil {
		return nil, err
	}
	wanted := req.GetString("sheet_title", "")

	archive, err := client.ExportSheetHTML(ctx, spreadsheetID)
	if err != nil {
		return nil, err
	}

	tabs, err := optionsFromArchive(archive, wanted)
	if err != nil {
		return toolError(err), nil
	}

	if wanted != "" && len(tabs) == 0 {
		return toolError(fmt.Errorf("the export holds no tab called %q", wanted)), nil
	}

	return resultJSON(map[string]any{
		"spreadsheet_id": spreadsheetID,
		"tabs":           tabs,
	})
}

// optionsFromArchive reads the zipped HTML export, one file per tab, and reports the
// coloured options on each. A tab with none is reported with an empty list rather than
// dropped: "this tab has none" and "this tab was not looked at" are different answers.
func optionsFromArchive(archive []byte, wanted string) ([]dropdownTab, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("the editors' export is not a zip file: %w", err)
	}

	var tabs []dropdownTab
	for _, file := range reader.File {
		name := file.Name
		if !strings.HasSuffix(name, ".html") || strings.Contains(name, "/") {
			continue
		}

		title := strings.TrimSuffix(name, ".html")
		if wanted != "" && title != wanted {
			continue
		}

		opened, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("reading %s out of the export: %w", name, err)
		}
		content, err := io.ReadAll(opened)
		_ = opened.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s out of the export: %w", name, err)
		}

		options, cutoff := optionsInHTML(string(content))
		tabs = append(tabs, dropdownTab{Title: title, Options: options, Cutoff: cutoff})
	}

	return tabs, nil
}

// optionsInHTML finds every coloured option on one tab and groups them by column and
// value. The same text can be coloured differently in two columns — a status list reused
// with its own palette — so the column is part of the key rather than a detail of the
// first hit.
func optionsInHTML(source string) ([]dropdownOption, bool) {
	rounded := map[string]bool{}
	for _, match := range optionClassPattern.FindAllStringSubmatch(source, -1) {
		rounded[match[1]] = true
	}
	if len(rounded) == 0 {
		return nil, false
	}

	type key struct {
		column int
		value  string
	}
	found := map[key]*dropdownOption{}
	cutoff := false

	for _, row := range optionRowPattern.FindAllStringSubmatch(source, -1) {
		for column, cell := range optionCellPattern.FindAllStringSubmatch(row[1], -1) {
			span := optionSpanPattern.FindStringSubmatch(cell[1])
			if span == nil || !rounded[span[1]] {
				continue
			}

			// The export writes the spaces inside a value as &nbsp;, and a value read
			// with those in it matches nothing: TEXT_EQ built from it would never fire,
			// and the rule would sit on the sheet colouring no cell at all.
			value := html.UnescapeString(optionTagPattern.ReplaceAllString(span[3], ""))
			value = strings.TrimSpace(strings.ReplaceAll(value, " ", " "))
			// An option on an empty cell renders as a zero-width space. It carries the
			// dropdown's default grey and says nothing about any value.
			value = strings.Trim(value, "​")
			if value == "" {
				continue
			}

			at := key{column: column, value: value}
			if found[at] == nil {
				if len(found) >= optionLimit {
					cutoff = true
					continue
				}
				found[at] = &dropdownOption{
					Column:     column,
					Value:      value,
					Background: firstGroup(optionFillPattern, span[2]),
					TextColor:  firstGroup(optionTextPattern, span[2]),
				}
			}
			found[at].Cells++
		}
	}

	options := make([]dropdownOption, 0, len(found))
	for _, option := range found {
		options = append(options, *option)
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Column != options[j].Column {
			return options[i].Column < options[j].Column
		}
		return options[i].Value < options[j].Value
	})

	return options, cutoff
}

func firstGroup(pattern *regexp.Regexp, source string) string {
	match := pattern.FindStringSubmatch(source)
	if match == nil {
		return ""
	}
	return strings.TrimSpace(match[1])
}
