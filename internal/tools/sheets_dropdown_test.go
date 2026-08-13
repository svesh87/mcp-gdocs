package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

// exportedTab is a tab the way the editors' HTML export writes one: the classes that draw
// a coloured dropdown option declared in the stylesheet by their rounded corner, the
// colours inline on each span. The last cell of the third row is a span that is not an
// option — an ordinary cell with a fill has one too, and taking it would invent a dropdown
// value that does not exist.
const exportedTab = `<html><head><style>
.s1{background-color:#ffffff;color:#000000;font-family:Arial;}
.s2{overflow:hidden;vertical-align:top;display:inline-block;border-radius:8px;}
</style></head><body><table>
<tr><th id="R0"><div>1</div></th>
<td class="s1"><span class="s2" style="background-color: #d4edbc; color: #11734b; width: 207.0px; ">Обеспечение стабильной работы&nbsp;продукта</span></td>
<td class="s1" dir="ltr"><span class="s2" style="background-color: #ffcfc9; color: #b10202; width: 74.0px; ">Высокое</span></td></tr>
<tr><th id="R1"><div>2</div></th>
<td class="s1"><span class="s2" style="background-color: #d4edbc; color: #11734b; width: 207.0px; ">Обеспечение стабильной работы&nbsp;продукта</span></td>
<td class="s1" dir="ltr"><span class="s2" style="background-color: #bfe1f6; color: #0a53a8; width: 74.0px; ">Среднее</span></td></tr>
<tr><th id="R2"><div>3</div></th>
<td class="s1"><span class="s2" style="background-color: #e8eaed; color: #000000; ">&#8203;</span></td>
<td class="s1"><span class="s1" style="background-color: #ff0000; color: #ffffff; ">не список</span></td></tr>
</table></body></html>`

// exportedPlainTab has no coloured options at all: no class carries a rounded corner.
const exportedPlainTab = `<html><head><style>
.s1{background-color:#ffffff;color:#000000;}
</style></head><body><table><tr><td class="s1">просто текст</td></tr></table></body></html>`

// zipped builds the archive the editors serve: one HTML file per tab, plus the resources
// directory they put pictures in, which carries nothing to read.
func zipped(t *testing.T, files map[string]string) string {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("building the zip: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("building the zip: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing the zip: %v", err)
	}

	return buf.String()
}

// TestDropdownColorsComeOffTheRendering: the colour of a dropdown option is in no API
// answer, so the tool reads the editors' own rendering. What it has to get right is which
// spans are options and which column each one sits in — the same word can be green in one
// column and grey in the next.
func TestDropdownColorsComeOffTheRendering(t *testing.T) {
	archive := zipped(t, map[string]string{
		"DevOps.html":            exportedTab,
		"заметки.html":           exportedPlainTab,
		"resources/sheet001.png": "not html",
	})
	h := newHarness(t, newFakeGoogle(t).answer("/editors/spreadsheets/d/book/export", archive))

	answer := h.ok(h.registry.sheetsReadDropdownColors(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
	})))

	for _, want := range []string{
		`"title": "DevOps"`,
		`"column": 0`,
		// The export writes the spaces inside a value as &nbsp;; a value carrying those
		// would match no cell when a rule is built from it.
		`"value": "Обеспечение стабильной работы продукта"`,
		`"background": "#d4edbc"`,
		`"text_color": "#11734b"`,
		`"cells": 2`,
		`"value": "Высокое"`,
		`"background": "#ffcfc9"`,
		`"value": "Среднее"`,
		`"title": "заметки"`,
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer is missing %s:\n%s", want, answer)
		}
	}

	// A span that is not an option, and an option on an empty cell, are both nothing to
	// report.
	for _, unwanted := range []string{`"не список"`, `"#ff0000"`, `"value": ""`} {
		if strings.Contains(answer, unwanted) {
			t.Errorf("the answer should not carry %s:\n%s", unwanted, answer)
		}
	}
}

// TestDropdownColorsTakeOneTab: a workbook of three tabs renders slowly, and a caller
// after one column should not have to read all of it.
func TestDropdownColorsTakeOneTab(t *testing.T) {
	archive := zipped(t, map[string]string{
		"DevOps.html":  exportedTab,
		"заметки.html": exportedPlainTab,
	})
	h := newHarness(t, newFakeGoogle(t).answer("/editors/spreadsheets/d/book/export", archive))

	answer := h.ok(h.registry.sheetsReadDropdownColors(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"sheet_title":    "DevOps",
	})))

	if strings.Contains(answer, "заметки") {
		t.Errorf("only the tab asked for should be reported:\n%s", answer)
	}

	refusal := h.fail(h.registry.sheetsReadDropdownColors(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
		"sheet_title":    "Нет такой",
	})))
	if !strings.Contains(refusal, "Нет такой") {
		t.Errorf("the refusal should name the tab: %s", refusal)
	}
}

// TestDropdownColorsRefuseSomethingThatIsNotAZip: the editors answer a sign-in page rather
// than a file when the export is not available, and "unexpected EOF" would send whoever
// reads it looking in the wrong place.
func TestDropdownColorsRefuseSomethingThatIsNotAZip(t *testing.T) {
	h := newHarness(t, newFakeGoogle(t).answer("/editors/spreadsheets", "<html>sign in</html>"))

	refusal := h.fail(h.registry.sheetsReadDropdownColors(context.Background(), request(map[string]any{
		"spreadsheet_id": "book",
	})))
	if !strings.Contains(refusal, "not a zip") {
		t.Errorf("the refusal should say what arrived instead: %s", refusal)
	}
}
