package google

import (
	"context"
	"net/http"
	"net/url"
)

// Slides types.
//
// Only the fields this server sends or reads are declared. A request type that is not
// here is one this server deliberately does not offer — there is no generic passthrough
// to presentations.batchUpdate, because arbitrary batches are exactly what makes a deck
// come out crooked.

// Dimension is a magnitude with its unit. EMU everywhere the API allows it: converting
// to points and back is where rounding turns into visibly shifted layout.
type Dimension struct {
	Magnitude float64 `json:"magnitude"`
	Unit      string  `json:"unit"`
}

// EMUPerPoint is how many English Metric Units make a point.
//
// It matters on the way in as much as on the way out: Slides answers with whichever unit
// the value was stored in, and a paragraph indent usually comes back in points while a
// position comes back in EMU. Reading a magnitude without its unit is how an indent of 36
// points became 36 EMU — the same number, twelve thousand times smaller, and an indent
// that visibly disappeared.
const EMUPerPoint = 12700

// EMU builds a dimension in English Metric Units, the unit Slides positions things in.
func EMU(magnitude float64) *Dimension { return &Dimension{Magnitude: magnitude, Unit: "EMU"} }

// InEMU is a dimension's magnitude converted to EMU, whatever unit it arrived in.
func (d *Dimension) InEMU() float64 {
	if d == nil {
		return 0
	}
	if d.Unit == "PT" {
		return d.Magnitude * EMUPerPoint
	}

	return d.Magnitude
}

// InPoints is a dimension's magnitude converted to points, whatever unit it arrived in.
func (d *Dimension) InPoints() float64 {
	if d == nil {
		return 0
	}
	if d.Unit == "EMU" {
		return d.Magnitude / EMUPerPoint
	}

	return d.Magnitude
}

// PT builds a dimension in points, which is what font sizes are measured in.
func PT(magnitude float64) *Dimension { return &Dimension{Magnitude: magnitude, Unit: "PT"} }

// RGBColor is a colour with components from 0 to 1.
type RGBColor struct {
	Red   float64 `json:"red,omitempty"`
	Green float64 `json:"green,omitempty"`
	Blue  float64 `json:"blue,omitempty"`
}

// OpaqueColor is either a literal colour or a name from the theme.
type OpaqueColor struct {
	RGBColor   *RGBColor `json:"rgbColor,omitempty"`
	ThemeColor string    `json:"themeColor,omitempty"`
}

// OptionalColor is a colour that may be absent, which in Slides means "inherited".
type OptionalColor struct {
	OpaqueColor *OpaqueColor `json:"opaqueColor,omitempty"`
}

// WeightedFontFamily is a font with its weight. It travels with fontFamily when a style
// is copied: dropping it is how a bold heading turns regular.
type WeightedFontFamily struct {
	FontFamily string `json:"fontFamily,omitempty"`
	Weight     int    `json:"weight,omitempty"`
}

// Link is where a text run points.
type Link struct {
	URL string `json:"url,omitempty"`
}

// TextStyle is the style of a run of text.
//
// The set of fields is fixed on purpose and matches what the style-reading requests ask
// for: copying a style means copying these and nothing else, so what is copied is the
// same set every time rather than whatever the API happened to return.
type TextStyle struct {
	Bold               *bool               `json:"bold,omitempty"`
	Italic             *bool               `json:"italic,omitempty"`
	Underline          *bool               `json:"underline,omitempty"`
	Strikethrough      *bool               `json:"strikethrough,omitempty"`
	SmallCaps          *bool               `json:"smallCaps,omitempty"`
	BaselineOffset     string              `json:"baselineOffset,omitempty"`
	FontFamily         string              `json:"fontFamily,omitempty"`
	FontSize           *Dimension          `json:"fontSize,omitempty"`
	WeightedFontFamily *WeightedFontFamily `json:"weightedFontFamily,omitempty"`
	ForegroundColor    *OptionalColor      `json:"foregroundColor,omitempty"`
	BackgroundColor    *OptionalColor      `json:"backgroundColor,omitempty"`
	Link               *Link               `json:"link,omitempty"`
}

// TextStyleFields is the field mask asking for every style field this server knows,
// which is what a style-copying request has to read before it can copy anything.
const TextStyleFields = "fontFamily,fontSize,bold,italic,underline,strikethrough,smallCaps," +
	"baselineOffset,foregroundColor,backgroundColor,weightedFontFamily"

// Fields lists the style fields that are set, in declaration order.
//
// This is the mask that goes with the style in an update: Slides changes exactly the
// named fields and leaves the rest inherited from the template.
func (s TextStyle) Fields() []string {
	var fields []string

	for _, field := range []struct {
		name  string
		isSet bool
	}{
		{"bold", s.Bold != nil},
		{"italic", s.Italic != nil},
		{"underline", s.Underline != nil},
		{"strikethrough", s.Strikethrough != nil},
		{"smallCaps", s.SmallCaps != nil},
		{"baselineOffset", s.BaselineOffset != ""},
		{"fontFamily", s.FontFamily != ""},
		{"fontSize", s.FontSize != nil},
		{"weightedFontFamily", s.WeightedFontFamily != nil},
		{"foregroundColor", s.ForegroundColor != nil},
		{"backgroundColor", s.BackgroundColor != nil},
		{"link", s.Link != nil},
	} {
		if field.isSet {
			fields = append(fields, field.name)
		}
	}

	return fields
}

// IsEmpty says whether the style carries nothing at all.
func (s TextStyle) IsEmpty() bool { return len(s.Fields()) == 0 }

// ParagraphStyle is the style of a paragraph.
type ParagraphStyle struct {
	Alignment       string     `json:"alignment,omitempty"`
	IndentStart     *Dimension `json:"indentStart,omitempty"`
	IndentEnd       *Dimension `json:"indentEnd,omitempty"`
	IndentFirstLine *Dimension `json:"indentFirstLine,omitempty"`
	LineSpacing     float64    `json:"lineSpacing,omitempty"`
	SpaceAbove      *Dimension `json:"spaceAbove,omitempty"`
	SpaceBelow      *Dimension `json:"spaceBelow,omitempty"`
	Direction       string     `json:"direction,omitempty"`
	// SpacingMode decides whether the space above a paragraph survives next to the space
	// below the one before it. The default collapses them inside lists, so a heading given
	// ten points of space above sits where it would with none, and every paragraph after
	// it is off by that much.
	SpacingMode string `json:"spacingMode,omitempty"`
}

// Range names a stretch of text inside a shape or a cell.
type Range struct {
	Type       string `json:"type"`
	StartIndex *int64 `json:"startIndex,omitempty"`
	EndIndex   *int64 `json:"endIndex,omitempty"`
}

// AllText is the whole of a text object.
func AllText() *Range { return &Range{Type: "ALL"} }

// FixedRange is a half-open stretch of text.
func FixedRange(start, end int64) *Range {
	return &Range{Type: "FIXED_RANGE", StartIndex: &start, EndIndex: &end}
}

// CellLocation names one cell of a table.
type CellLocation struct {
	RowIndex    int `json:"rowIndex"`
	ColumnIndex int `json:"columnIndex"`
}

// Size is width and height.
type Size struct {
	Width  *Dimension `json:"width,omitempty"`
	Height *Dimension `json:"height,omitempty"`
}

// Transform places an element on a page.
type Transform struct {
	ScaleX     float64 `json:"scaleX,omitempty"`
	ScaleY     float64 `json:"scaleY,omitempty"`
	ShearX     float64 `json:"shearX,omitempty"`
	ShearY     float64 `json:"shearY,omitempty"`
	TranslateX float64 `json:"translateX,omitempty"`
	TranslateY float64 `json:"translateY,omitempty"`
	Unit       string  `json:"unit,omitempty"`
}

// ElementProperties says which page an element goes on and where.
type ElementProperties struct {
	PageObjectID string     `json:"pageObjectId"`
	Size         *Size      `json:"size,omitempty"`
	Transform    *Transform `json:"transform,omitempty"`
}

// Request is one operation in a batch. Exactly one field is set.
type Request struct {
	DeleteParagraphBullets      *DeleteParagraphBulletsRequest      `json:"deleteParagraphBullets,omitempty"`
	DeleteText                  *DeleteTextRequest                  `json:"deleteText,omitempty"`
	InsertText                  *InsertTextRequest                  `json:"insertText,omitempty"`
	CreateParagraphBullets      *CreateParagraphBulletsRequest      `json:"createParagraphBullets,omitempty"`
	UpdateParagraphStyle        *UpdateParagraphStyleRequest        `json:"updateParagraphStyle,omitempty"`
	UpdateTextStyle             *UpdateTextStyleRequest             `json:"updateTextStyle,omitempty"`
	CreateTable                 *CreateTableRequest                 `json:"createTable,omitempty"`
	UpdateTableColumnProperties *UpdateTableColumnPropertiesRequest `json:"updateTableColumnProperties,omitempty"`
	CreateSlide                 *CreateSlideRequest                 `json:"createSlide,omitempty"`
	CreateShape                 *CreateShapeRequest                 `json:"createShape,omitempty"`
	CreateImage                 *CreateImageRequest                 `json:"createImage,omitempty"`
	UpdatePageElementTransform  *UpdatePageElementTransformRequest  `json:"updatePageElementTransform,omitempty"`
	DeleteObject                *DeleteObjectRequest                `json:"deleteObject,omitempty"`
	UpdateSlidesPosition        *UpdateSlidesPositionRequest        `json:"updateSlidesPosition,omitempty"`
	MergeTableCells             *MergeTableCellsRequest             `json:"mergeTableCells,omitempty"`
	UpdateTableCellProperties   *UpdateTableCellPropertiesRequest   `json:"updateTableCellProperties,omitempty"`
	UpdateTableRowProperties    *UpdateTableRowPropertiesRequest    `json:"updateTableRowProperties,omitempty"`
	UpdatePageProperties        *UpdatePagePropertiesRequest        `json:"updatePageProperties,omitempty"`
	UpdateSlideProperties       *UpdateSlidePropertiesRequest       `json:"updateSlideProperties,omitempty"`
	UpdateShapeProperties       *UpdateShapePropertiesRequest       `json:"updateShapeProperties,omitempty"`
	UpdateImageProperties       *UpdateImagePropertiesRequest       `json:"updateImageProperties,omitempty"`
	UpdatePageElementsZOrder    *UpdatePageElementsZOrderRequest    `json:"updatePageElementsZOrder,omitempty"`
	GroupObjects                *GroupObjectsRequest                `json:"groupObjects,omitempty"`
	UngroupObjects              *UngroupObjectsRequest              `json:"ungroupObjects,omitempty"`
	CreateLine                  *CreateLineRequest                  `json:"createLine,omitempty"`
	UpdateLineProperties        *UpdateLinePropertiesRequest        `json:"updateLineProperties,omitempty"`
	DuplicateObject             *DuplicateObjectRequest             `json:"duplicateObject,omitempty"`
	InsertTableRows             *InsertTableRowsRequest             `json:"insertTableRows,omitempty"`
	InsertTableColumns          *InsertTableColumnsRequest          `json:"insertTableColumns,omitempty"`
	UnmergeTableCells           *UnmergeTableCellsRequest           `json:"unmergeTableCells,omitempty"`
	UpdateTableBorder           *UpdateTableBorderRequest           `json:"updateTableBorderProperties,omitempty"`
	ReplaceAllText              *ReplaceAllTextRequest              `json:"replaceAllText,omitempty"`
	ReplaceImage                *ReplaceImageRequest                `json:"replaceImage,omitempty"`
	ReplaceShapesWithImage      *ReplaceShapesWithImageRequest      `json:"replaceAllShapesWithImage,omitempty"`
	ReplaceShapesWithChart      *ReplaceShapesWithChartRequest      `json:"replaceAllShapesWithSheetsChart,omitempty"`
	UpdateAltText               *UpdateAltTextRequest               `json:"updatePageElementAltText,omitempty"`
	RerouteLine                 *RerouteLineRequest                 `json:"rerouteLine,omitempty"`
	UpdateLineCategory          *UpdateLineCategoryRequest          `json:"updateLineCategory,omitempty"`
	CreateSheetsChart           *CreateSheetsChartRequest           `json:"createSheetsChart,omitempty"`
	RefreshSheetsChart          *RefreshSheetsChartRequest          `json:"refreshSheetsChart,omitempty"`
	CreateVideo                 *CreateVideoRequest                 `json:"createVideo,omitempty"`
	UpdateVideoProperties       *UpdateVideoPropertiesRequest       `json:"updateVideoProperties,omitempty"`
}

// InsertTableRowsRequest grows a table by rows, beside a cell that says where.
type InsertTableRowsRequest struct {
	TableObjectID string        `json:"tableObjectId"`
	CellLocation  *CellLocation `json:"cellLocation,omitempty"`
	InsertBelow   bool          `json:"insertBelow"`
	Number        int           `json:"number,omitempty"`
}

// InsertTableColumnsRequest grows a table by columns.
type InsertTableColumnsRequest struct {
	TableObjectID string        `json:"tableObjectId"`
	CellLocation  *CellLocation `json:"cellLocation,omitempty"`
	InsertRight   bool          `json:"insertRight"`
	Number        int           `json:"number,omitempty"`
}

// UnmergeTableCellsRequest takes a merged rectangle apart again.
type UnmergeTableCellsRequest struct {
	ObjectID   string      `json:"objectId"`
	TableRange *TableRange `json:"tableRange"`
}

// TableBorderFill is what a table's border line is filled with.
type TableBorderFill struct {
	SolidFill *SolidFill `json:"solidFill,omitempty"`
}

// TableBorderProperties is how one set of a table's lines is drawn.
type TableBorderProperties struct {
	Fill      *TableBorderFill `json:"tableBorderFill,omitempty"`
	Weight    *Dimension       `json:"weight,omitempty"`
	DashStyle string           `json:"dashStyle,omitempty"`
}

// UpdateTableBorderRequest draws the lines of a table.
//
// The borders are addressed by position — ALL, OUTER, INNER, and each single side — rather
// than per cell, which is why a table's frame is one request and not a loop over cells.
type UpdateTableBorderRequest struct {
	ObjectID   string                 `json:"objectId"`
	TableRange *TableRange            `json:"tableRange,omitempty"`
	Position   string                 `json:"borderPosition,omitempty"`
	Properties *TableBorderProperties `json:"tableBorderProperties"`
	Fields     string                 `json:"fields"`
}

// ReplaceImageRequest swaps a picture's bytes while it keeps its place and its size.
type ReplaceImageRequest struct {
	ImageObjectID string `json:"imageObjectId"`
	URL           string `json:"url"`
	Method        string `json:"imageReplaceMethod,omitempty"`
}

// ReplaceShapesWithImageRequest turns every shape whose text matches into a picture. It is
// how a deck built from a template gets its illustrations without anybody placing them.
type ReplaceShapesWithImageRequest struct {
	ContainsText  *SlidesTextMatch `json:"containsText"`
	ImageURL      string           `json:"imageUrl"`
	Method        string           `json:"imageReplaceMethod,omitempty"`
	PageObjectIDs []string         `json:"pageObjectIds,omitempty"`
}

// ReplaceAllTextRequest swaps one stretch of text for another wherever it appears in a
// deck. The words around it keep their styling and so does the replacement, which is what
// makes this the way to change a word everywhere: replacing a box's whole text takes the
// paragraphs' styling with it.
type ReplaceAllTextRequest struct {
	ContainsText  *SlidesTextMatch `json:"containsText"`
	ReplaceText   string           `json:"replaceText"`
	PageObjectIDs []string         `json:"pageObjectIds,omitempty"`
}

// SlidesTextMatch is the text a replacement looks for.
type SlidesTextMatch struct {
	Text      string `json:"text"`
	MatchCase bool   `json:"matchCase"`
}

// ReplaceShapesWithChartRequest turns every shape whose text matches into a chart from a
// spreadsheet — the same idea as replacing shapes with a picture, with a live chart.
type ReplaceShapesWithChartRequest struct {
	ContainsText  *SlidesTextMatch `json:"containsText"`
	SpreadsheetID string           `json:"spreadsheetId"`
	ChartID       int              `json:"chartId"`
	LinkingMode   string           `json:"linkingMode,omitempty"`
	PageObjectIDs []string         `json:"pageObjectIds,omitempty"`
}

// UpdateAltTextRequest gives an element the description a screen reader reads out.
type UpdateAltTextRequest struct {
	ObjectID    string `json:"objectId"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// RerouteLineRequest makes a connector find its way between the shapes it is attached to
// again, after they have been moved.
type RerouteLineRequest struct {
	ObjectID string `json:"objectId"`
}

// UpdateLineCategoryRequest changes how a connector runs: straight, bent or curved.
type UpdateLineCategoryRequest struct {
	ObjectID     string `json:"objectId"`
	LineCategory string `json:"lineCategory"`
}

// CreateSheetsChartRequest puts a chart from a spreadsheet onto a slide.
//
// Linked, it keeps a thread back to the workbook and can be refreshed when the numbers
// change; not linked, it is a picture of the chart as it was.
type CreateSheetsChartRequest struct {
	ObjectID      string             `json:"objectId,omitempty"`
	SpreadsheetID string             `json:"spreadsheetId"`
	ChartID       int                `json:"chartId"`
	LinkingMode   string             `json:"linkingMode,omitempty"`
	Element       *ElementProperties `json:"elementProperties,omitempty"`
}

// RefreshSheetsChartRequest pulls the current state of a linked chart.
type RefreshSheetsChartRequest struct {
	ObjectID string `json:"objectId"`
}

// CreateVideoRequest puts a video on a slide.
type CreateVideoRequest struct {
	ObjectID string             `json:"objectId,omitempty"`
	ID       string             `json:"id"`
	Source   string             `json:"source,omitempty"`
	Element  *ElementProperties `json:"elementProperties,omitempty"`
}

// VideoProperties is how a video behaves and looks.
type VideoProperties struct {
	Outline  *Outline `json:"outline,omitempty"`
	AutoPlay *bool    `json:"autoPlay,omitempty"`
	Start    *int     `json:"start,omitempty"`
	End      *int     `json:"end,omitempty"`
	Mute     *bool    `json:"mute,omitempty"`
}

// UpdateVideoPropertiesRequest sets how a video plays.
type UpdateVideoPropertiesRequest struct {
	ObjectID   string           `json:"objectId"`
	Properties *VideoProperties `json:"videoProperties"`
	Fields     string           `json:"fields"`
}

// DuplicateObjectRequest copies an element or a whole slide within one presentation.
//
// This is the only way to reproduce what the API can describe but not create. A shape
// carries adjustment values — the corner radius of a rounded rectangle among them — which
// no request accepts and no response reports; a shape built from its shapeType alone comes
// out with the default radius instead of the author's. A duplicate carries them, because
// the copy happens inside Google rather than through fields this server can name.
//
// The presentation is the boundary: an object identifier from another one is "could not be
// found". Nothing here reaches across presentations, and there is no API that does.
type DuplicateObjectRequest struct {
	ObjectID string `json:"objectId"`
	// ObjectIDs maps identifiers in the source to the ones the copies should get. It is
	// optional; without it Google invents them. For a slide the map covers the slide and
	// every element on it, so a caller that wants to address the copy afterwards names
	// them here.
	ObjectIDs map[string]string `json:"objectIds,omitempty"`
}

// UpdatePagePropertiesRequest sets a page's background.
//
// The mask matters more here than anywhere else: sending the whole properties object
// resets what was not named, and on a title slide that means losing the author's picture
// in exchange for white.
type UpdatePagePropertiesRequest struct {
	ObjectID       string          `json:"objectId"`
	PageProperties *PageProperties `json:"pageProperties"`
	Fields         string          `json:"fields"`
}

// UpdateSlidePropertiesRequest hides a slide from the presentation or brings it back.
//
// A hidden slide stays in the deck and stays editable; it is only skipped when the deck is
// presented. That is what a sample's author uses for a slide kept for reference, and a
// copy that shows it is a copy that says something the original does not.
type UpdateSlidePropertiesRequest struct {
	ObjectID        string           `json:"objectId"`
	SlideProperties *SlideProperties `json:"slideProperties"`
	Fields          string           `json:"fields"`
}

// UpdateShapePropertiesRequest fills a shape, outlines it, or aligns what is inside it.
type UpdateShapePropertiesRequest struct {
	ObjectID        string           `json:"objectId"`
	ShapeProperties *ShapeProperties `json:"shapeProperties"`
	Fields          string           `json:"fields"`
}

// UpdateImagePropertiesRequest crops, dims or outlines a picture that is already there.
type UpdateImagePropertiesRequest struct {
	ObjectID        string           `json:"objectId"`
	ImageProperties *ImageProperties `json:"imageProperties"`
	Fields          string           `json:"fields"`
}

// UpdatePageElementsZOrderRequest says what covers what.
//
// Order is not cosmetic on a rebuilt slide: elements are created one after another, so a
// picture added last sits on top of the text it was behind in the sample.
type UpdatePageElementsZOrderRequest struct {
	PageElementObjectIDs []string `json:"pageElementObjectIds"`
	// Operation is BRING_TO_FRONT, BRING_FORWARD, SEND_BACKWARD or SEND_TO_BACK.
	Operation string `json:"operation"`
}

// GroupObjectsRequest joins elements so they move and scale together.
type GroupObjectsRequest struct {
	ChildrenObjectIDs []string `json:"childrenObjectIds"`
	GroupObjectID     string   `json:"groupObjectId,omitempty"`
}

// UngroupObjectsRequest takes groups apart, leaving their children on the page where they
// were.
type UngroupObjectsRequest struct {
	ObjectIDs []string `json:"objectIds"`
}

// CreateLineRequest draws a line, an arrow or a connector.
type CreateLineRequest struct {
	ObjectID          string             `json:"objectId,omitempty"`
	ElementProperties *ElementProperties `json:"elementProperties"`
	// Category is STRAIGHT, BENT or CURVED. The older lineType field is not sent: the two
	// are alternatives, and Slides refuses a request carrying both.
	Category string `json:"category,omitempty"`
}

// UpdateLinePropertiesRequest styles a line: colour, thickness, dashes, arrowheads.
type UpdateLinePropertiesRequest struct {
	ObjectID       string          `json:"objectId"`
	LineProperties *LineProperties `json:"lineProperties"`
	Fields         string          `json:"fields"`
}

// DeleteObjectRequest removes a slide or an element of one.
//
// Nothing outside a presentation can be deleted through this client: there is no Drive
// delete, no trashing, no sheet or row removal. Inside a deck, removing a slide somebody
// just created is ordinary editing, and without it a failed step leaves rubbish forever.
type DeleteObjectRequest struct {
	ObjectID string `json:"objectId"`
}

// UpdateSlidesPositionRequest moves slides to a place in the deck.
//
// The slides named move together and keep their order among themselves; everything else
// closes up around them. The index is counted in the deck as it is before the move.
type UpdateSlidesPositionRequest struct {
	SlideObjectIDs []string `json:"slideObjectIds"`
	InsertionIndex int      `json:"insertionIndex"`
}

// TableRange is a rectangle of a table, given as a corner and a span.
type TableRange struct {
	Location   *CellLocation `json:"location"`
	RowSpan    int           `json:"rowSpan"`
	ColumnSpan int           `json:"columnSpan"`
}

// MergeTableCellsRequest joins cells into one, which is how a table shows that several
// rows share a heading.
type MergeTableCellsRequest struct {
	ObjectID   string      `json:"objectId"`
	TableRange *TableRange `json:"tableRange"`
}

// TableCellBackgroundFill is the fill behind a cell.
type TableCellBackgroundFill struct {
	SolidFill *SolidFill `json:"solidFill,omitempty"`
}

// PageProperties is how a page looks behind whatever stands on it.
//
// A slide with no properties of its own is not a white slide: it inherits the layout's
// background, and the layout inherits the master's. Reproducing a deck's title slide
// means reading this and setting it, because importing a theme brings the master's
// background but not the one the author put on that one slide.
type PageProperties struct {
	PageBackgroundFill *PageBackgroundFill `json:"pageBackgroundFill,omitempty"`
	ColorScheme        *ColorScheme        `json:"colorScheme,omitempty"`
}

// ColorScheme is a theme's palette: the twelve names every other colour in the deck can
// refer to instead of a value.
//
// It lives on the master and only there. A colour written as a theme name follows the
// theme when it changes; the same colour written literally stops following it, which is
// the difference between a deck that can be restyled and one that has to be repainted.
type ColorScheme struct {
	Colors []ThemeColorPair `json:"colors,omitempty"`
}

// ThemeColorPair is one name of the palette and the colour behind it.
type ThemeColorPair struct {
	Type  string    `json:"type"`
	Color *RGBColor `json:"color"`
}

// ThemeColorTypes are the twelve names a colour scheme has to carry.
//
// The API refuses a scheme with fewer: an update replaces the palette rather than editing
// it, so a caller changing one accent has to send the other eleven back unchanged. The
// remaining names Slides knows (TEXT1, BACKGROUND1 and so on) are aliases of these and
// are ignored on the way in.
var ThemeColorTypes = []string{
	"DARK1", "LIGHT1", "DARK2", "LIGHT2",
	"ACCENT1", "ACCENT2", "ACCENT3", "ACCENT4", "ACCENT5", "ACCENT6",
	"HYPERLINK", "FOLLOWED_HYPERLINK",
}

// PageBackgroundFill is a page's background: a flat colour, a stretched picture, or
// nothing of its own.
type PageBackgroundFill struct {
	// PropertyState is RENDERED (the fill applies), NOT_RENDERED (transparent) or
	// INHERIT (take the layout's). INHERIT is how a background is given back.
	PropertyState        string                `json:"propertyState,omitempty"`
	SolidFill            *SolidFill            `json:"solidFill,omitempty"`
	StretchedPictureFill *StretchedPictureFill `json:"stretchedPictureFill,omitempty"`
}

// StretchedPictureFill is a picture filling a page or a shape.
//
// Reading one gives a contentUrl that Google serves for a while; handing that same
// address back to the API is what copies a background picture across decks, the way a
// picture on a slide is copied.
type StretchedPictureFill struct {
	ContentURL string `json:"contentUrl,omitempty"`
	Size       *Size  `json:"size,omitempty"`
}

// ShapeBackgroundFill is what a shape is filled with behind its text.
type ShapeBackgroundFill struct {
	PropertyState string     `json:"propertyState,omitempty"`
	SolidFill     *SolidFill `json:"solidFill,omitempty"`
}

// Outline is the border of a shape, a picture or a table.
type Outline struct {
	PropertyState string       `json:"propertyState,omitempty"`
	OutlineFill   *OutlineFill `json:"outlineFill,omitempty"`
	Weight        *Dimension   `json:"weight,omitempty"`
	DashStyle     string       `json:"dashStyle,omitempty"`
}

// OutlineFill is the colour of a border.
type OutlineFill struct {
	SolidFill *SolidFill `json:"solidFill,omitempty"`
}

// Shadow is the drop shadow behind an element. It is read and reported but never built
// here: a shadow is a transform of its own, and inventing one is how a copied slide
// stops looking like its sample.
type Shadow struct {
	Type            string       `json:"type,omitempty"`
	Alignment       string       `json:"alignment,omitempty"`
	Alpha           float64      `json:"alpha,omitempty"`
	BlurRadius      *Dimension   `json:"blurRadius,omitempty"`
	Color           *OpaqueColor `json:"color,omitempty"`
	PropertyState   string       `json:"propertyState,omitempty"`
	RotateWithShape *bool        `json:"rotateWithShape,omitempty"`
	Transform       *Transform   `json:"transform,omitempty"`
}

// ImageProperties is how a picture is shown: cropped, dimmed, outlined.
type ImageProperties struct {
	CropProperties *CropProperties `json:"cropProperties,omitempty"`
	Transparency   float64         `json:"transparency,omitempty"`
	Brightness     float64         `json:"brightness,omitempty"`
	Contrast       float64         `json:"contrast,omitempty"`
	Outline        *Outline        `json:"outline,omitempty"`
	Shadow         *Shadow         `json:"shadow,omitempty"`
}

// CropProperties is how much of each side of a picture is cut away, as a fraction of its
// size. A picture that fills a circle in the sample is a cropped picture, not a smaller
// one: copying the box without the crop shows the parts the author hid.
type CropProperties struct {
	LeftOffset   float64 `json:"leftOffset,omitempty"`
	RightOffset  float64 `json:"rightOffset,omitempty"`
	TopOffset    float64 `json:"topOffset,omitempty"`
	BottomOffset float64 `json:"bottomOffset,omitempty"`
	Angle        float64 `json:"angle,omitempty"`
}

// LineProperties is how a line or a connector is drawn.
type LineProperties struct {
	LineFill   *LineFill  `json:"lineFill,omitempty"`
	Weight     *Dimension `json:"weight,omitempty"`
	DashStyle  string     `json:"dashStyle,omitempty"`
	StartArrow string     `json:"startArrow,omitempty"`
	EndArrow   string     `json:"endArrow,omitempty"`
	Link       *Link      `json:"link,omitempty"`
}

// LineFill is the colour of a line.
type LineFill struct {
	SolidFill *SolidFill `json:"solidFill,omitempty"`
}

// ElementGroup is several elements moved and scaled as one.
//
// A group is not a decoration of the reading: its children carry their own transforms,
// which compose with the group's. An element inside a group that is read as if it stood
// on the slide lands in the wrong place by exactly the group's transform.
type ElementGroup struct {
	Children []PageElement `json:"children,omitempty"`
}

// SolidFill is a flat colour with an opacity.
type SolidFill struct {
	Color *OpaqueColor `json:"color,omitempty"`
	Alpha float64      `json:"alpha,omitempty"`
}

// TableCellProperties is how a cell looks behind its text.
type TableCellProperties struct {
	BackgroundFill   *TableCellBackgroundFill `json:"tableCellBackgroundFill,omitempty"`
	ContentAlignment string                   `json:"contentAlignment,omitempty"`
}

// UpdateTableCellPropertiesRequest fills cells with a colour or aligns their content.
type UpdateTableCellPropertiesRequest struct {
	ObjectID            string               `json:"objectId"`
	TableRange          *TableRange          `json:"tableRange,omitempty"`
	TableCellProperties *TableCellProperties `json:"tableCellProperties"`
	Fields              string               `json:"fields"`
}

// TableRowProperties is how tall a row is.
type TableRowProperties struct {
	MinRowHeight *Dimension `json:"minRowHeight,omitempty"`
}

// UpdateTableRowPropertiesRequest sets row heights.
type UpdateTableRowPropertiesRequest struct {
	ObjectID           string              `json:"objectId"`
	RowIndices         []int               `json:"rowIndices,omitempty"`
	TableRowProperties *TableRowProperties `json:"tableRowProperties"`
	Fields             string              `json:"fields"`
}

// CreateShapeRequest adds a shape — in this server always a text box, because a deck's
// other shapes belong to its template.
type CreateShapeRequest struct {
	ObjectID          string             `json:"objectId,omitempty"`
	ShapeType         string             `json:"shapeType"`
	ElementProperties *ElementProperties `json:"elementProperties"`
}

// CreateImageRequest puts a picture on a slide by address. Slides fetches it once and
// keeps its own copy, so the address only has to be reachable at that moment.
type CreateImageRequest struct {
	ObjectID          string             `json:"objectId,omitempty"`
	URL               string             `json:"url"`
	ElementProperties *ElementProperties `json:"elementProperties"`
}

// UpdatePageElementTransformRequest moves an element, resizes it, or both.
//
// Size is not set here and cannot be: Slides resizes by scaling, so a width of half the
// original is scaleX 0.5 against the size the element was created with.
type UpdatePageElementTransformRequest struct {
	ObjectID  string     `json:"objectId"`
	Transform *Transform `json:"transform"`
	// ApplyMode is ABSOLUTE (replace the transform) or RELATIVE (compose with it).
	ApplyMode string `json:"applyMode"`
}

// DeleteParagraphBulletsRequest strips list formatting.
type DeleteParagraphBulletsRequest struct {
	ObjectID     string        `json:"objectId"`
	CellLocation *CellLocation `json:"cellLocation,omitempty"`
	TextRange    *Range        `json:"textRange,omitempty"`
}

// DeleteTextRequest removes text.
type DeleteTextRequest struct {
	ObjectID     string        `json:"objectId"`
	CellLocation *CellLocation `json:"cellLocation,omitempty"`
	TextRange    *Range        `json:"textRange,omitempty"`
}

// InsertTextRequest puts text in.
type InsertTextRequest struct {
	ObjectID       string        `json:"objectId"`
	CellLocation   *CellLocation `json:"cellLocation,omitempty"`
	Text           string        `json:"text"`
	InsertionIndex int64         `json:"insertionIndex"`
}

// CreateParagraphBulletsRequest turns paragraphs into a list.
//
// This is how a nested list is made: the text carries tab characters for depth and
// Slides works out the indents and the markers itself. Placing bullets and indents by
// hand is what makes a deck look wrong on someone else's screen.
type CreateParagraphBulletsRequest struct {
	ObjectID     string        `json:"objectId"`
	CellLocation *CellLocation `json:"cellLocation,omitempty"`
	TextRange    *Range        `json:"textRange,omitempty"`
	BulletPreset string        `json:"bulletPreset,omitempty"`
}

// UpdateParagraphStyleRequest restyles paragraphs.
type UpdateParagraphStyleRequest struct {
	ObjectID     string          `json:"objectId"`
	CellLocation *CellLocation   `json:"cellLocation,omitempty"`
	TextRange    *Range          `json:"textRange,omitempty"`
	Style        *ParagraphStyle `json:"style"`
	Fields       string          `json:"fields"`
}

// UpdateTextStyleRequest restyles a run of text.
type UpdateTextStyleRequest struct {
	ObjectID     string        `json:"objectId"`
	CellLocation *CellLocation `json:"cellLocation,omitempty"`
	TextRange    *Range        `json:"textRange,omitempty"`
	Style        *TextStyle    `json:"style"`
	Fields       string        `json:"fields"`
}

// CreateTableRequest makes a native table.
type CreateTableRequest struct {
	ObjectID          string             `json:"objectId,omitempty"`
	ElementProperties *ElementProperties `json:"elementProperties"`
	Rows              int                `json:"rows"`
	Columns           int                `json:"columns"`
}

// TableColumnProperties is the width of a column.
type TableColumnProperties struct {
	ColumnWidth *Dimension `json:"columnWidth,omitempty"`
}

// UpdateTableColumnPropertiesRequest sets column widths.
type UpdateTableColumnPropertiesRequest struct {
	ObjectID              string                 `json:"objectId"`
	ColumnIndices         []int                  `json:"columnIndices,omitempty"`
	TableColumnProperties *TableColumnProperties `json:"tableColumnProperties"`
	Fields                string                 `json:"fields"`
}

// LayoutReference names the layout a new slide follows.
type LayoutReference struct {
	PredefinedLayout string `json:"predefinedLayout,omitempty"`
	LayoutID         string `json:"layoutId,omitempty"`
}

// PlaceholderIDMapping gives a predictable identifier to a placeholder of a new slide,
// so the text can be filled in without reading the presentation back first.
type PlaceholderIDMapping struct {
	LayoutPlaceholder   *Placeholder `json:"layoutPlaceholder,omitempty"`
	LayoutPlaceholderID string       `json:"layoutPlaceholderObjectId,omitempty"`
	ObjectID            string       `json:"objectId"`
}

// CreateSlideRequest adds a slide.
type CreateSlideRequest struct {
	ObjectID              string                 `json:"objectId,omitempty"`
	InsertionIndex        *int                   `json:"insertionIndex,omitempty"`
	SlideLayoutReference  *LayoutReference       `json:"slideLayoutReference,omitempty"`
	PlaceholderIDMappings []PlaceholderIDMapping `json:"placeholderIdMappings,omitempty"`
}

// BatchUpdateRequest is the body of presentations.batchUpdate.
type BatchUpdateRequest struct {
	Requests []Request `json:"requests"`
}

// BatchUpdateResponse is what came back.
type BatchUpdateResponse struct {
	PresentationID string       `json:"presentationId"`
	Replies        []BatchReply `json:"replies"`
}

// BatchReply is the part of a reply this server passes on: the identifiers of what was
// created, because a caller needs them for the next call.
type BatchReply struct {
	CreateSlide *struct {
		ObjectID string `json:"objectId"`
	} `json:"createSlide,omitempty"`
	CreateTable *struct {
		ObjectID string `json:"objectId"`
	} `json:"createTable,omitempty"`
	DuplicateObject *struct {
		ObjectID string `json:"objectId"`
	} `json:"duplicateObject,omitempty"`
	// ReplaceAllText answers with a count rather than an identifier: nothing was created,
	// and the number is the only way a caller learns that its search text matched nothing.
	ReplaceAllText *struct {
		OccurrencesChanged int `json:"occurrencesChanged"`
	} `json:"replaceAllText,omitempty"`
}

// Presentation is a deck as far as this server reads it.
//
// Layouts and masters are read alongside the slides because a slide's look mostly is not
// on the slide: what a title's size and colour actually are lives on the layout it
// follows, and on the master behind that.
type Presentation struct {
	PresentationID string `json:"presentationId"`
	Title          string `json:"title"`
	PageSize       *Size  `json:"pageSize,omitempty"`
	Slides         []Page `json:"slides"`
	Layouts        []Page `json:"layouts"`
	Masters        []Page `json:"masters"`
}

// EffectiveTextStyle is the style a run of text ends up with: what it sets itself, then
// whatever the layout's placeholder adds, then the master's.
//
// Slides reports only what each level sets, so the answer to "how big is this text" is
// not in any one of them — it is the three merged in that order. Reading only the run
// returns an empty style for most real slides.
func (p *Presentation) EffectiveTextStyle(slideObjectID, objectID string) (TextStyle, map[string]string) {
	from := map[string]string{}
	merged := TextStyle{}

	element := p.findOn(p.Slides, objectID)
	if element == nil {
		return merged, from
	}

	mergeStyleInto(&merged, firstRunStyle(element), "text", from)

	// Up the chain: the placeholder this one inherits from on the layout, then the one
	// that inherits from on the master.
	for _, source := range []struct {
		pages []Page
		name  string
	}{
		{p.Layouts, "layout"},
		{p.Masters, "master"},
	} {
		parent := placeholderParent(element)
		if parent == "" {
			break
		}

		element = p.findOn(source.pages, parent)
		if element == nil {
			break
		}

		mergeStyleInto(&merged, firstRunStyle(element), source.name, from)
	}

	return merged, from
}

// EffectiveParagraphStyle is the spacing a paragraph ends up with: what it sets itself,
// then the layout's placeholder, then the master's.
//
// The same three levels as the text style, and the same trap: a body whose paragraphs all
// report nothing is not a body with no spacing. The air between its sections comes from
// the layout, and a slide rebuilt on a layout that does not have it comes out visibly
// tighter with every font, size and level matching.
func (p *Presentation) EffectiveParagraphStyle(objectID string) (ParagraphStyle, map[string]string) {
	from := map[string]string{}
	merged := ParagraphStyle{}

	element := p.findOn(p.Slides, objectID)
	if element == nil {
		return merged, from
	}

	mergeParagraphStyleInto(&merged, firstParagraphStyle(element), "text", from)

	for _, source := range []struct {
		pages []Page
		name  string
	}{
		{p.Layouts, "layout"},
		{p.Masters, "master"},
	} {
		parent := placeholderParent(element)
		if parent == "" {
			break
		}

		element = p.findOn(source.pages, parent)
		if element == nil {
			break
		}

		mergeParagraphStyleInto(&merged, firstParagraphStyle(element), source.name, from)
	}

	return merged, from
}

// firstParagraphStyle is the style of the first paragraph of an element's text.
func firstParagraphStyle(element *PageElement) *ParagraphStyle {
	if element == nil || element.Shape == nil || element.Shape.Text == nil {
		return nil
	}

	for _, item := range element.Shape.Text.TextElements {
		if item.ParagraphMarker == nil || item.ParagraphMarker.Style == nil {
			continue
		}
		return item.ParagraphMarker.Style
	}

	return nil
}

// mergeParagraphStyleInto fills the fields still unset and records where each came from.
func mergeParagraphStyleInto(into *ParagraphStyle, from *ParagraphStyle, source string, origin map[string]string) {
	if from == nil {
		return
	}

	if into.Alignment == "" && from.Alignment != "" {
		into.Alignment, origin["alignment"] = from.Alignment, source
	}
	if into.LineSpacing == 0 && from.LineSpacing != 0 {
		into.LineSpacing, origin["lineSpacing"] = from.LineSpacing, source
	}
	if into.SpaceAbove == nil && from.SpaceAbove != nil {
		into.SpaceAbove, origin["spaceAbove"] = from.SpaceAbove, source
	}
	if into.SpaceBelow == nil && from.SpaceBelow != nil {
		into.SpaceBelow, origin["spaceBelow"] = from.SpaceBelow, source
	}
	if into.IndentStart == nil && from.IndentStart != nil {
		into.IndentStart, origin["indentStart"] = from.IndentStart, source
	}
	if into.IndentEnd == nil && from.IndentEnd != nil {
		into.IndentEnd, origin["indentEnd"] = from.IndentEnd, source
	}
	if into.IndentFirstLine == nil && from.IndentFirstLine != nil {
		into.IndentFirstLine, origin["indentFirstLine"] = from.IndentFirstLine, source
	}
	if into.Direction == "" && from.Direction != "" {
		into.Direction, origin["direction"] = from.Direction, source
	}
}

// findOn locates an element by identifier across a set of pages.
func (p *Presentation) findOn(pages []Page, objectID string) *PageElement {
	for pageIndex := range pages {
		for elementIndex := range pages[pageIndex].PageElements {
			if pages[pageIndex].PageElements[elementIndex].ObjectID == objectID {
				return &pages[pageIndex].PageElements[elementIndex]
			}
		}
	}

	return nil
}

// placeholderParent is the element this one inherits from, one level up.
func placeholderParent(element *PageElement) string {
	if element == nil || element.Shape == nil || element.Shape.Placeholder == nil {
		return ""
	}

	return element.Shape.Placeholder.ParentObjectID
}

// firstRunStyle is the style of the first non-blank run of an element's text.
func firstRunStyle(element *PageElement) *TextStyle {
	if element == nil || element.Shape == nil || element.Shape.Text == nil {
		return nil
	}

	for _, item := range element.Shape.Text.TextElements {
		if item.TextRun == nil || item.TextRun.Style == nil {
			continue
		}
		return item.TextRun.Style
	}

	return nil
}

// mergeStyleInto fills the fields that are still unset, and records where each came from.
func mergeStyleInto(into *TextStyle, from *TextStyle, source string, origin map[string]string) {
	if from == nil {
		return
	}

	if into.Bold == nil && from.Bold != nil {
		into.Bold, origin["bold"] = from.Bold, source
	}
	if into.Italic == nil && from.Italic != nil {
		into.Italic, origin["italic"] = from.Italic, source
	}
	if into.Underline == nil && from.Underline != nil {
		into.Underline, origin["underline"] = from.Underline, source
	}
	if into.Strikethrough == nil && from.Strikethrough != nil {
		into.Strikethrough, origin["strikethrough"] = from.Strikethrough, source
	}
	if into.SmallCaps == nil && from.SmallCaps != nil {
		into.SmallCaps, origin["smallCaps"] = from.SmallCaps, source
	}
	if into.BaselineOffset == "" && from.BaselineOffset != "" {
		into.BaselineOffset, origin["baselineOffset"] = from.BaselineOffset, source
	}
	if into.FontFamily == "" && from.FontFamily != "" {
		into.FontFamily, origin["fontFamily"] = from.FontFamily, source
	}
	if into.FontSize == nil && from.FontSize != nil {
		into.FontSize, origin["fontSize"] = from.FontSize, source
	}
	if into.WeightedFontFamily == nil && from.WeightedFontFamily != nil {
		into.WeightedFontFamily, origin["weightedFontFamily"] = from.WeightedFontFamily, source
	}
	if into.ForegroundColor == nil && from.ForegroundColor != nil {
		into.ForegroundColor, origin["foregroundColor"] = from.ForegroundColor, source
	}
	if into.BackgroundColor == nil && from.BackgroundColor != nil {
		into.BackgroundColor, origin["backgroundColor"] = from.BackgroundColor, source
	}
}

// Page is a slide, a layout, a master or a page of speaker notes.
type Page struct {
	ObjectID         string            `json:"objectId"`
	PageType         string            `json:"pageType,omitempty"`
	LayoutProperties *LayoutProperties `json:"layoutProperties,omitempty"`
	SlideProperties  *SlideProperties  `json:"slideProperties,omitempty"`
	PageProperties   *PageProperties   `json:"pageProperties,omitempty"`
	NotesProperties  *NotesProperties  `json:"notesProperties,omitempty"`
	PageElements     []PageElement     `json:"pageElements"`
}

// SlideProperties says which layout a slide follows and where its speaker notes live.
// That layout is where a slide's inherited sizes and colours come from, so reproducing a
// slide means reproducing it on the same layout — not on one that merely looks similar in
// the list.
type SlideProperties struct {
	LayoutObjectID string `json:"layoutObjectId,omitempty"`
	MasterObjectID string `json:"masterObjectId,omitempty"`
	NotesPage      *Page  `json:"notesPage,omitempty"`
	// IsSkipped hides a slide from the presentation without removing it. A pointer because
	// the difference between "shown" and "not said" matters on the way out: sending false
	// unhides a slide somebody hid on purpose.
	IsSkipped *bool `json:"isSkipped,omitempty"`
}

// NotesProperties points at the shape on a notes page that holds the speaker's text.
// The notes page has two placeholders — a picture of the slide and the text — and only
// the one named here takes text.
type NotesProperties struct {
	SpeakerNotesObjectID string `json:"speakerNotesObjectId,omitempty"`
}

// LayoutProperties names a layout.
type LayoutProperties struct {
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// PageElement is one thing on a page.
type PageElement struct {
	ObjectID     string        `json:"objectId"`
	Title        string        `json:"title,omitempty"`
	Description  string        `json:"description,omitempty"`
	Size         *Size         `json:"size,omitempty"`
	Transform    *Transform    `json:"transform,omitempty"`
	Shape        *Shape        `json:"shape,omitempty"`
	Table        *Table        `json:"table,omitempty"`
	Image        *Image        `json:"image,omitempty"`
	Video        *Video        `json:"video,omitempty"`
	Line         *Line         `json:"line,omitempty"`
	ElementGroup *ElementGroup `json:"elementGroup,omitempty"`
}

// Shape is a text box or a figure.
type Shape struct {
	ShapeType   string           `json:"shapeType,omitempty"`
	Text        *TextContent     `json:"text,omitempty"`
	Placeholder *Placeholder     `json:"placeholder,omitempty"`
	Properties  *ShapeProperties `json:"shapeProperties,omitempty"`
}

// ShapeProperties is how a shape behaves and looks behind its text.
type ShapeProperties struct {
	Autofit          *Autofit             `json:"autofit,omitempty"`
	BackgroundFill   *ShapeBackgroundFill `json:"shapeBackgroundFill,omitempty"`
	Outline          *Outline             `json:"outline,omitempty"`
	Shadow           *Shadow              `json:"shadow,omitempty"`
	ContentAlignment string               `json:"contentAlignment,omitempty"`
	Link             *Link                `json:"link,omitempty"`
}

// Autofit is Slides shrinking text so it fits its box.
//
// This is why a title reported as 28 pt can measure 25 pt on screen: the size is 28 and
// the scale is 0.89. Nothing in the text or the layout says 25 — it is 28 × the scale,
// and the scale is recomputed by Slides whenever the text changes.
type Autofit struct {
	AutofitType          string  `json:"autofitType,omitempty"`
	FontScale            float64 `json:"fontScale,omitempty"`
	LineSpacingReduction float64 `json:"lineSpacingReduction,omitempty"`
}

// Placeholder says which slot of the layout a shape fills.
type Placeholder struct {
	Type           string `json:"type,omitempty"`
	Index          *int   `json:"index,omitempty"`
	ParentObjectID string `json:"parentObjectId,omitempty"`
}

// TextContent is the text of a shape or a cell.
type TextContent struct {
	TextElements []TextElement `json:"textElements"`
}

// TextElement is a paragraph marker or a run of text.
type TextElement struct {
	StartIndex      *int64           `json:"startIndex,omitempty"`
	EndIndex        *int64           `json:"endIndex,omitempty"`
	ParagraphMarker *ParagraphMarker `json:"paragraphMarker,omitempty"`
	TextRun         *TextRun         `json:"textRun,omitempty"`
}

// ParagraphMarker starts a paragraph and carries its bullet, if it has one.
type ParagraphMarker struct {
	Style  *ParagraphStyle `json:"style,omitempty"`
	Bullet *Bullet         `json:"bullet,omitempty"`
}

// Bullet is the marker of a list item and how deep it sits.
type Bullet struct {
	ListID       string     `json:"listId,omitempty"`
	NestingLevel *int       `json:"nestingLevel,omitempty"`
	Glyph        string     `json:"glyph,omitempty"`
	BulletStyle  *TextStyle `json:"bulletStyle,omitempty"`
}

// TextRun is a stretch of text with one style.
type TextRun struct {
	Content string     `json:"content"`
	Style   *TextStyle `json:"style,omitempty"`
}

// Table is a native table.
type Table struct {
	Rows         int           `json:"rows"`
	Columns      int           `json:"columns"`
	TableRows    []TableRow    `json:"tableRows,omitempty"`
	TableColumns []TableColumn `json:"tableColumns,omitempty"`
}

// TableRow is one row.
type TableRow struct {
	RowHeight  *Dimension  `json:"rowHeight,omitempty"`
	TableCells []TableCell `json:"tableCells,omitempty"`
}

// TableColumn is one column.
type TableColumn struct {
	ColumnWidth *Dimension `json:"columnWidth,omitempty"`
}

// TableCell is one cell.
type TableCell struct {
	Location   *CellLocation        `json:"location,omitempty"`
	RowSpan    int                  `json:"rowSpan,omitempty"`
	ColumnSpan int                  `json:"columnSpan,omitempty"`
	Text       *TextContent         `json:"text,omitempty"`
	Properties *TableCellProperties `json:"tableCellProperties,omitempty"`
}

// Image is a picture on a page. The content address is what copies it into another deck:
// Slides fetches it and keeps its own copy, so a picture never has to be downloaded and
// uploaded to be reproduced.
type Image struct {
	ContentURL string           `json:"contentUrl,omitempty"`
	SourceURL  string           `json:"sourceUrl,omitempty"`
	Properties *ImageProperties `json:"imageProperties,omitempty"`
}

// Video is an embedded video.
type Video struct {
	URL    string `json:"url,omitempty"`
	Source string `json:"source,omitempty"`
	ID     string `json:"id,omitempty"`
}

// Line is a line, an arrow or a connector.
type Line struct {
	LineType     string          `json:"lineType,omitempty"`
	LineCategory string          `json:"lineCategory,omitempty"`
	Properties   *LineProperties `json:"lineProperties,omitempty"`
}

// Thumbnail is a rendered picture of one slide.
type Thumbnail struct {
	ContentURL string `json:"contentUrl"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

// Presentation reads a deck. The fields mask is passed straight through: reading only
// what is needed keeps a large deck from arriving as megabytes of JSON.
func (c *Client) Presentation(ctx context.Context, presentationID, fields string) (*Presentation, error) {
	query := url.Values{}
	if fields != "" {
		query.Set("fields", fields)
	}

	var out Presentation
	if err := c.call(ctx, http.MethodGet,
		endpoint(c.slidesBase, "/presentations/"+url.PathEscape(presentationID), query), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// CreatePresentation makes an empty deck with a title.
//
// It arrives on Google's default theme, and no request moves another deck's theme into it:
// the palette is written with a master colour scheme, the styles with the layouts. That is
// the whole reason this exists beside a copy — a copy brings a theme and everything else
// with it.
func (c *Client) CreatePresentation(ctx context.Context, title string) (*Presentation, error) {
	var out Presentation
	if err := c.call(ctx, http.MethodPost, endpoint(c.slidesBase, "/presentations", nil),
		map[string]string{"title": title}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// SlidesBatchUpdate sends one batch of requests to a presentation.
func (c *Client) SlidesBatchUpdate(ctx context.Context, presentationID string, requests []Request) (*BatchUpdateResponse, error) {
	var out BatchUpdateResponse
	if err := c.call(ctx, http.MethodPost,
		endpoint(c.slidesBase, "/presentations/"+url.PathEscape(presentationID)+":batchUpdate", nil),
		BatchUpdateRequest{Requests: requests}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// Thumbnail renders one slide to a picture and returns the address it can be fetched
// from. The address is short-lived, which is fine for its purpose: an agent looking at
// what it just did.
func (c *Client) Thumbnail(ctx context.Context, presentationID, pageObjectID, mimeType, size string) (*Thumbnail, error) {
	query := url.Values{}
	if mimeType != "" {
		query.Set("thumbnailProperties.mimeType", mimeType)
	}
	if size != "" {
		query.Set("thumbnailProperties.thumbnailSize", size)
	}

	var out Thumbnail
	if err := c.call(ctx, http.MethodGet, endpoint(c.slidesBase,
		"/presentations/"+url.PathEscape(presentationID)+"/pages/"+url.PathEscape(pageObjectID)+"/thumbnail",
		query), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
