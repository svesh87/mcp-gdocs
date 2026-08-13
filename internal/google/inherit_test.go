package google

import (
	"context"
	"strings"
	"testing"
)

// deckWithInheritance is a slide whose look is spread across three levels, which is what
// every real slide looks like: the run sets a size, the layout supplies the font, and the
// master supplies the colour.
func deckWithInheritance() *Presentation {
	size := PT(20)
	bold := true

	return &Presentation{
		Slides: []Page{{
			ObjectID: "slide1",
			PageElements: []PageElement{{
				ObjectID: "body1",
				Shape: &Shape{
					Placeholder: &Placeholder{Type: "BODY", ParentObjectID: "layout_body"},
					Text: &TextContent{TextElements: []TextElement{
						{ParagraphMarker: &ParagraphMarker{}},
						{TextRun: &TextRun{Content: "Что сделали\n", Style: &TextStyle{FontSize: size, Bold: &bold}}},
					}},
				},
			}},
		}},
		Layouts: []Page{{
			ObjectID: "layout1",
			PageElements: []PageElement{{
				ObjectID: "layout_body",
				Shape: &Shape{
					Placeholder: &Placeholder{Type: "BODY", ParentObjectID: "master_body"},
					Text: &TextContent{TextElements: []TextElement{
						{TextRun: &TextRun{Content: "Текст\n", Style: &TextStyle{FontFamily: "Rubik", FontSize: PT(18)}}},
					}},
				},
			}},
		}},
		Masters: []Page{{
			ObjectID: "master1",
			PageElements: []PageElement{{
				ObjectID: "master_body",
				Shape: &Shape{
					Placeholder: &Placeholder{Type: "BODY"},
					Text: &TextContent{TextElements: []TextElement{
						{TextRun: &TextRun{Content: "Текст\n", Style: &TextStyle{
							ForegroundColor: &OptionalColor{OpaqueColor: &OpaqueColor{
								RGBColor: &RGBColor{Red: 0.2, Green: 0.2, Blue: 0.2}}},
						}}},
					}},
				},
			}},
		}},
	}
}

// TestEffectiveTextStyleMergesThreeLevels is the answer to "how big is this text really".
// Reading the run alone returns almost nothing on a real slide: the size that shows is
// the run's, then whatever the layout adds, then the master's.
func TestEffectiveTextStyleMergesThreeLevels(t *testing.T) {
	presentation := deckWithInheritance()

	style, origin := presentation.EffectiveTextStyle("", "body1")

	if style.FontSize == nil || style.FontSize.Magnitude != 20 {
		t.Errorf("the size should be the one the run sets, got %+v", style.FontSize)
	}
	// The layout's 18 pt must not win over the run's 20: what a level sets stays set, and
	// the level above only fills what is still missing.
	if origin["fontSize"] != "text" {
		t.Errorf("the size comes from the text, the reading says %q", origin["fontSize"])
	}
	if style.FontFamily != "Rubik" || origin["fontFamily"] != "layout" {
		t.Errorf("the font should come from the layout, got %q from %q", style.FontFamily, origin["fontFamily"])
	}
	if style.ForegroundColor == nil || origin["foregroundColor"] != "master" {
		t.Errorf("the colour should come from the master, the reading says %q", origin["foregroundColor"])
	}
}

func TestEffectiveTextStyleOfSomethingThatIsNotThere(t *testing.T) {
	presentation := deckWithInheritance()

	style, origin := presentation.EffectiveTextStyle("", "nothing")

	if !style.IsEmpty() || len(origin) != 0 {
		t.Errorf("an element that is not on any slide has no style, got %+v", style)
	}
}

// TestEffectiveTextStyleStopsWhereInheritanceDoes keeps the walk from inventing a parent:
// a text box that is not a placeholder inherits nothing, however much the layout sets.
func TestEffectiveTextStyleStopsWhereInheritanceDoes(t *testing.T) {
	presentation := deckWithInheritance()
	presentation.Slides[0].PageElements[0].Shape.Placeholder = nil

	style, origin := presentation.EffectiveTextStyle("", "body1")

	if style.FontFamily != "" {
		t.Errorf("a plain text box should not pick up the layout's font, got %q", style.FontFamily)
	}
	if origin["fontSize"] != "text" {
		t.Errorf("its own size should still be reported, got %q", origin["fontSize"])
	}
}

// TestDimensionsConvert is the bug this pair of methods exists for: Slides answers in
// whichever unit a value was stored in. A paragraph indent comes back in points, a
// position in EMU, and a magnitude read without its unit turned an indent of 36 points
// into 36 EMU — the same number, twelve thousand times smaller, and an indent that
// vanished from the slide.
func TestDimensionsConvert(t *testing.T) {
	points := &Dimension{Magnitude: 36, Unit: "PT"}
	if got := points.InEMU(); got != 36*EMUPerPoint {
		t.Errorf("36 pt is %g EMU, got %g", float64(36*EMUPerPoint), got)
	}
	if got := points.InPoints(); got != 36 {
		t.Errorf("36 pt stays 36 pt, got %g", got)
	}

	emu := EMU(457200)
	if got := emu.InPoints(); got != 36 {
		t.Errorf("457200 EMU is 36 pt, got %g", got)
	}
	if got := emu.InEMU(); got != 457200 {
		t.Errorf("457200 EMU stays itself, got %g", got)
	}

	// A dimension that is not there converts to nothing rather than crashing: most style
	// fields are absent on most paragraphs.
	var missing *Dimension
	if missing.InEMU() != 0 || missing.InPoints() != 0 {
		t.Error("an absent dimension is zero in both units")
	}
}

func TestRangesAndDimensions(t *testing.T) {
	if all := AllText(); all.Type != "ALL" || all.StartIndex != nil {
		t.Errorf("the whole of a text object is a range with no bounds, got %+v", all)
	}

	fixed := FixedRange(2, 7)
	if fixed.Type != "FIXED_RANGE" || *fixed.StartIndex != 2 || *fixed.EndIndex != 7 {
		t.Errorf("a fixed range came out as %+v", fixed)
	}

	// EMU stays EMU: converting to points and back is where rounding turns into visibly
	// shifted layout.
	if emu := EMU(914400); emu.Unit != "EMU" || emu.Magnitude != 914400 {
		t.Errorf("a dimension in EMU came out as %+v", emu)
	}
}

func TestSpreadsheetGridAsksForTheFormatting(t *testing.T) {
	f := &fake{body: `{"spreadsheetId": "book", "sheets": [{"properties": {"sheetId": 0, "title": "Лист 1"}}]}`}
	client := newClient(t, f)

	if _, err := client.SpreadsheetGrid(context.Background(), "book", "'Лист 1'!A1:B2"); err != nil {
		t.Fatalf("reading the grid: %v", err)
	}

	for _, want := range []string{"includeGridData=true", "ranges=", "userEnteredFormat"} {
		if !strings.Contains(f.query, want) {
			t.Errorf("the grid read should ask for %s, got %s", want, f.query)
		}
	}
}

// TestUploadFileConverts pins the request shape Drive needs to turn an uploaded .pptx
// into an editable presentation: metadata and content in one multipart body, with the
// Google MIME type as the target.
func TestUploadFileConverts(t *testing.T) {
	f := &fake{body: `{"id": "new1", "name": "Отчёт", "mimeType": "` + MimePresentation + `"}`}
	client := newClient(t, f)

	file, err := client.UploadFile(context.Background(), "Отчёт",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		MimePresentation, "folder1", []byte("PK pretend"))
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	if file.ID != "new1" {
		t.Errorf("the upload came back as %+v", file)
	}

	if !strings.Contains(f.query, "uploadType=multipart") {
		t.Errorf("the upload should be multipart, got %s", f.query)
	}

	body := string(f.request)
	for _, want := range []string{MimePresentation, "folder1", "PK pretend"} {
		if !strings.Contains(body, want) {
			t.Errorf("the body should carry %q, got %s", want, body)
		}
	}
}

func TestUploadFileWithoutConversion(t *testing.T) {
	f := &fake{body: `{"id": "new2", "name": "deck.pptx"}`}
	client := newClient(t, f)

	if _, err := client.UploadFile(context.Background(), "deck.pptx", "application/octet-stream",
		"", "", []byte("PK")); err != nil {
		t.Fatalf("uploading: %v", err)
	}

	if strings.Contains(string(f.request), "google-apps") {
		t.Errorf("an unconverted upload should ask for no Google type, got %s", f.request)
	}
}

func TestMoveFileNamesBothParents(t *testing.T) {
	f := &fake{body: `{"id": "file1", "name": "Отчёт"}`}
	client := newClient(t, f)

	if _, err := client.MoveFile(context.Background(), "file1", "folder_new", "folder_old"); err != nil {
		t.Fatalf("moving the file: %v", err)
	}

	if f.method != "PATCH" {
		t.Errorf("a move is a PATCH, got %s", f.method)
	}
	for _, want := range []string{"addParents=folder_new", "removeParents=folder_old", "supportsAllDrives=true"} {
		if !strings.Contains(f.query, want) {
			t.Errorf("the move should carry %s, got %s", want, f.query)
		}
	}
}
