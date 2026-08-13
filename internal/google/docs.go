package google

import (
	"context"
	"net/http"
	"net/url"
)

// Document is a Google Doc as far as this server reads it.
//
// The shape follows the API's own: a body of structural elements, plus the segments and
// dictionaries the body refers to by identifier. A paragraph says it is a list item by
// naming a list; a picture is an identifier in the text and its size lives in the
// dictionary. Reading one without the others reports a document that cannot be rebuilt.
type Document struct {
	DocumentID        string                      `json:"documentId"`
	Title             string                      `json:"title"`
	RevisionID        string                      `json:"revisionId,omitempty"`
	Body              *DocsBody                   `json:"body,omitempty"`
	Headers           map[string]DocsSegment      `json:"headers,omitempty"`
	Footers           map[string]DocsSegment      `json:"footers,omitempty"`
	Footnotes         map[string]DocsSegment      `json:"footnotes,omitempty"`
	DocumentStyle     *DocsDocumentStyle          `json:"documentStyle,omitempty"`
	NamedStyles       *DocsNamedStyles            `json:"namedStyles,omitempty"`
	Lists             map[string]DocsList         `json:"lists,omitempty"`
	InlineObjects     map[string]DocsInlineObject `json:"inlineObjects,omitempty"`
	PositionedObjects map[string]DocsPositioned   `json:"positionedObjects,omitempty"`
}

// DocsBody is the document's content.
type DocsBody struct {
	Content []StructuralElement `json:"content"`
}

// DocsSegment is a header, a footer or a footnote: content addressed by its own
// identifier rather than by an index into the body.
type DocsSegment struct {
	Content []StructuralElement `json:"content,omitempty"`
}

// StructuralElement is one block of a document: a paragraph, a table, a section break.
type StructuralElement struct {
	StartIndex   *int64            `json:"startIndex,omitempty"`
	EndIndex     *int64            `json:"endIndex,omitempty"`
	Paragraph    *DocsParagraph    `json:"paragraph,omitempty"`
	Table        *DocsTable        `json:"table,omitempty"`
	SectionBreak *DocsSectionBreak `json:"sectionBreak,omitempty"`
}

// DocsSectionBreak starts a new section, which is what carries page setup and its own
// headers and footers.
type DocsSectionBreak struct {
	Style *DocsSectionStyle `json:"sectionStyle,omitempty"`
}

// DocsParagraph is a paragraph and the runs inside it.
type DocsParagraph struct {
	Elements            []DocsParagraphElement `json:"elements"`
	Style               *DocsParagraphStyle    `json:"paragraphStyle,omitempty"`
	Bullet              *DocsBullet            `json:"bullet,omitempty"`
	PositionedObjectIDs []string               `json:"positionedObjectIds,omitempty"`
}

// DocsParagraphElement is a piece of a paragraph.
type DocsParagraphElement struct {
	StartIndex     *int64                   `json:"startIndex,omitempty"`
	EndIndex       *int64                   `json:"endIndex,omitempty"`
	TextRun        *DocsTextRun             `json:"textRun,omitempty"`
	InlineObject   *DocsInlineObjectElement `json:"inlineObjectElement,omitempty"`
	PageBreak      *DocsPageBreakElement    `json:"pageBreak,omitempty"`
	ColumnBreak    *DocsPageBreakElement    `json:"columnBreak,omitempty"`
	HorizontalRule *DocsPageBreakElement    `json:"horizontalRule,omitempty"`
	Person         *DocsPersonElement       `json:"person,omitempty"`
	RichLink       *DocsRichLinkElement     `json:"richLink,omitempty"`
}

// DocsInlineObjectElement is where a picture or a drawing sits in the text. It occupies
// exactly one index, and its size lives in the document's inlineObjects.
type DocsInlineObjectElement struct {
	InlineObjectID string         `json:"inlineObjectId"`
	Style          *DocsTextStyle `json:"textStyle,omitempty"`
}

// DocsPageBreakElement is a break or a rule: it has no content of its own, only a style.
type DocsPageBreakElement struct {
	Style *DocsTextStyle `json:"textStyle,omitempty"`
}

// DocsPersonElement is a person chip.
type DocsPersonElement struct {
	PersonID   string `json:"personId,omitempty"`
	Properties *struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email,omitempty"`
	} `json:"personProperties,omitempty"`
}

// DocsRichLinkElement is a chip pointing at another Google file.
type DocsRichLinkElement struct {
	RichLinkID string `json:"richLinkId,omitempty"`
	Properties *struct {
		Title    string `json:"title,omitempty"`
		URI      string `json:"uri,omitempty"`
		MimeType string `json:"mimeType,omitempty"`
	} `json:"richLinkProperties,omitempty"`
}

// DocsTextRun is a stretch of text with one style.
type DocsTextRun struct {
	Content string         `json:"content"`
	Style   *DocsTextStyle `json:"textStyle,omitempty"`
}

// DocsTextStyle is the style of a run in a document.
type DocsTextStyle struct {
	Bold            *bool               `json:"bold,omitempty"`
	Italic          *bool               `json:"italic,omitempty"`
	Underline       *bool               `json:"underline,omitempty"`
	Strikethrough   *bool               `json:"strikethrough,omitempty"`
	SmallCaps       *bool               `json:"smallCaps,omitempty"`
	BaselineOffset  string              `json:"baselineOffset,omitempty"`
	FontSize        *Dimension          `json:"fontSize,omitempty"`
	WeightedFont    *WeightedFontFamily `json:"weightedFontFamily,omitempty"`
	ForegroundColor *DocsColor          `json:"foregroundColor,omitempty"`
	BackgroundColor *DocsColor          `json:"backgroundColor,omitempty"`
	Link            *Link               `json:"link,omitempty"`
}

// DocsColor is a colour in a document, which nests one level deeper than in Slides.
//
// An empty object is not black: it is "no colour", which is how a transparent background
// and an inherited foreground both come back. Writing one back means the same thing.
type DocsColor struct {
	Color *DocsColorValue `json:"color,omitempty"`
}

// DocsColorValue is the colour inside a DocsColor.
type DocsColorValue struct {
	RGBColor *RGBColor `json:"rgbColor,omitempty"`
}

// DocsParagraphStyle is a paragraph's own style. Every field the API has is here,
// because a paragraph that keeps its borders and loses its spacing is not a copy.
type DocsParagraphStyle struct {
	HeadingID           string               `json:"headingId,omitempty"`
	NamedStyleType      string               `json:"namedStyleType,omitempty"`
	Alignment           string               `json:"alignment,omitempty"`
	LineSpacing         *float64             `json:"lineSpacing,omitempty"`
	Direction           string               `json:"direction,omitempty"`
	SpacingMode         string               `json:"spacingMode,omitempty"`
	SpaceAbove          *Dimension           `json:"spaceAbove,omitempty"`
	SpaceBelow          *Dimension           `json:"spaceBelow,omitempty"`
	BorderBetween       *DocsParagraphBorder `json:"borderBetween,omitempty"`
	BorderTop           *DocsParagraphBorder `json:"borderTop,omitempty"`
	BorderBottom        *DocsParagraphBorder `json:"borderBottom,omitempty"`
	BorderLeft          *DocsParagraphBorder `json:"borderLeft,omitempty"`
	BorderRight         *DocsParagraphBorder `json:"borderRight,omitempty"`
	IndentFirstLine     *Dimension           `json:"indentFirstLine,omitempty"`
	IndentStart         *Dimension           `json:"indentStart,omitempty"`
	IndentEnd           *Dimension           `json:"indentEnd,omitempty"`
	KeepLinesTogether   *bool                `json:"keepLinesTogether,omitempty"`
	KeepWithNext        *bool                `json:"keepWithNext,omitempty"`
	AvoidWidowAndOrphan *bool                `json:"avoidWidowAndOrphan,omitempty"`
	Shading             *DocsShading         `json:"shading,omitempty"`
	PageBreakBefore     *bool                `json:"pageBreakBefore,omitempty"`
	TabStops            []DocsTabStop        `json:"tabStops,omitempty"`
}

// DocsParagraphBorder is one side of a paragraph's frame.
type DocsParagraphBorder struct {
	Color     *DocsColor `json:"color,omitempty"`
	Width     *Dimension `json:"width,omitempty"`
	Padding   *Dimension `json:"padding,omitempty"`
	DashStyle string     `json:"dashStyle,omitempty"`
}

// DocsShading is a paragraph's own background.
type DocsShading struct {
	BackgroundColor *DocsColor `json:"backgroundColor,omitempty"`
}

// DocsTabStop is one stop of a paragraph's ruler.
type DocsTabStop struct {
	Offset    *Dimension `json:"offset,omitempty"`
	Alignment string     `json:"alignment,omitempty"`
}

// DocsBullet says a paragraph is a list item.
type DocsBullet struct {
	ListID       string         `json:"listId,omitempty"`
	NestingLevel *int           `json:"nestingLevel,omitempty"`
	TextStyle    *DocsTextStyle `json:"textStyle,omitempty"`
}

// DocsList is one list of the document, and the glyphs of each of its levels.
type DocsList struct {
	Properties *DocsListProperties `json:"listProperties,omitempty"`
}

// DocsListProperties holds a list's nesting levels.
type DocsListProperties struct {
	NestingLevels []DocsNestingLevel `json:"nestingLevels,omitempty"`
}

// DocsNestingLevel is how one depth of a list looks.
type DocsNestingLevel struct {
	BulletAlignment string         `json:"bulletAlignment,omitempty"`
	GlyphFormat     string         `json:"glyphFormat,omitempty"`
	GlyphSymbol     string         `json:"glyphSymbol,omitempty"`
	GlyphType       string         `json:"glyphType,omitempty"`
	IndentFirstLine *Dimension     `json:"indentFirstLine,omitempty"`
	IndentStart     *Dimension     `json:"indentStart,omitempty"`
	StartNumber     *int           `json:"startNumber,omitempty"`
	TextStyle       *DocsTextStyle `json:"textStyle,omitempty"`
}

// DocsTable is a table in a document.
type DocsTable struct {
	Rows    int             `json:"rows"`
	Columns int             `json:"columns"`
	Content []DocsTableRow  `json:"tableRows,omitempty"`
	Style   *DocsTableStyle `json:"tableStyle,omitempty"`
}

// DocsTableStyle is the table's column widths.
type DocsTableStyle struct {
	ColumnProperties []DocsTableColumnProperties `json:"tableColumnProperties,omitempty"`
}

// DocsTableColumnProperties is one column's width and how it is decided.
type DocsTableColumnProperties struct {
	Width     *Dimension `json:"width,omitempty"`
	WidthType string     `json:"widthType,omitempty"`
}

// DocsTableRow is one row of a document table.
type DocsTableRow struct {
	StartIndex *int64             `json:"startIndex,omitempty"`
	EndIndex   *int64             `json:"endIndex,omitempty"`
	Cells      []DocsTableCell    `json:"tableCells,omitempty"`
	Style      *DocsTableRowStyle `json:"tableRowStyle,omitempty"`
}

// DocsTableRowStyle is a row's height and whether it repeats as a header.
type DocsTableRowStyle struct {
	MinRowHeight    *Dimension `json:"minRowHeight,omitempty"`
	TableHeader     *bool      `json:"tableHeader,omitempty"`
	PreventOverflow *bool      `json:"preventOverflow,omitempty"`
}

// DocsTableCell is one cell of a document table.
type DocsTableCell struct {
	StartIndex *int64              `json:"startIndex,omitempty"`
	EndIndex   *int64              `json:"endIndex,omitempty"`
	Content    []StructuralElement `json:"content,omitempty"`
	Style      *DocsTableCellStyle `json:"tableCellStyle,omitempty"`
}

// DocsTableCellStyle is how one cell is painted.
type DocsTableCellStyle struct {
	RowSpan          int                  `json:"rowSpan,omitempty"`
	ColumnSpan       int                  `json:"columnSpan,omitempty"`
	BackgroundColor  *DocsColor           `json:"backgroundColor,omitempty"`
	BorderTop        *DocsTableCellBorder `json:"borderTop,omitempty"`
	BorderBottom     *DocsTableCellBorder `json:"borderBottom,omitempty"`
	BorderLeft       *DocsTableCellBorder `json:"borderLeft,omitempty"`
	BorderRight      *DocsTableCellBorder `json:"borderRight,omitempty"`
	PaddingTop       *Dimension           `json:"paddingTop,omitempty"`
	PaddingBottom    *Dimension           `json:"paddingBottom,omitempty"`
	PaddingLeft      *Dimension           `json:"paddingLeft,omitempty"`
	PaddingRight     *Dimension           `json:"paddingRight,omitempty"`
	ContentAlignment string               `json:"contentAlignment,omitempty"`
}

// DocsTableCellBorder is one side of a cell.
type DocsTableCellBorder struct {
	Color     *DocsColor `json:"color,omitempty"`
	Width     *Dimension `json:"width,omitempty"`
	DashStyle string     `json:"dashStyle,omitempty"`
}

// DocsSectionStyle is the page setup of one section, and which headers it uses.
type DocsSectionStyle struct {
	SectionType            string              `json:"sectionType,omitempty"`
	ColumnSeparatorStyle   string              `json:"columnSeparatorStyle,omitempty"`
	ContentDirection       string              `json:"contentDirection,omitempty"`
	ColumnProperties       []DocsSectionColumn `json:"columnProperties,omitempty"`
	MarginTop              *Dimension          `json:"marginTop,omitempty"`
	MarginBottom           *Dimension          `json:"marginBottom,omitempty"`
	MarginLeft             *Dimension          `json:"marginLeft,omitempty"`
	MarginRight            *Dimension          `json:"marginRight,omitempty"`
	MarginHeader           *Dimension          `json:"marginHeader,omitempty"`
	MarginFooter           *Dimension          `json:"marginFooter,omitempty"`
	DefaultHeaderID        string              `json:"defaultHeaderId,omitempty"`
	DefaultFooterID        string              `json:"defaultFooterId,omitempty"`
	FirstPageHeaderID      string              `json:"firstPageHeaderId,omitempty"`
	FirstPageFooterID      string              `json:"firstPageFooterId,omitempty"`
	EvenPageHeaderID       string              `json:"evenPageHeaderId,omitempty"`
	EvenPageFooterID       string              `json:"evenPageFooterId,omitempty"`
	UseFirstPageHeaderFoot *bool               `json:"useFirstPageHeaderFooter,omitempty"`
	PageNumberStart        *int                `json:"pageNumberStart,omitempty"`
	FlipPageOrientation    *bool               `json:"flipPageOrientation,omitempty"`
}

// DocsSectionColumn is one column of a multi-column section.
type DocsSectionColumn struct {
	Width      *Dimension `json:"width,omitempty"`
	PaddingEnd *Dimension `json:"paddingEnd,omitempty"`
}

// DocsDocumentStyle is the page setup of the document as a whole.
type DocsDocumentStyle struct {
	Background             *DocsBackground `json:"background,omitempty"`
	PageSize               *DocsSize       `json:"pageSize,omitempty"`
	MarginTop              *Dimension      `json:"marginTop,omitempty"`
	MarginBottom           *Dimension      `json:"marginBottom,omitempty"`
	MarginLeft             *Dimension      `json:"marginLeft,omitempty"`
	MarginRight            *Dimension      `json:"marginRight,omitempty"`
	MarginHeader           *Dimension      `json:"marginHeader,omitempty"`
	MarginFooter           *Dimension      `json:"marginFooter,omitempty"`
	UseCustomHeaderFooter  *bool           `json:"useCustomHeaderFooterMargins,omitempty"`
	UseFirstPageHeaderFoot *bool           `json:"useFirstPageHeaderFooter,omitempty"`
	UseEvenPageHeaderFoot  *bool           `json:"useEvenPageHeaderFooter,omitempty"`
	DefaultHeaderID        string          `json:"defaultHeaderId,omitempty"`
	DefaultFooterID        string          `json:"defaultFooterId,omitempty"`
	FirstPageHeaderID      string          `json:"firstPageHeaderId,omitempty"`
	FirstPageFooterID      string          `json:"firstPageFooterId,omitempty"`
	EvenPageHeaderID       string          `json:"evenPageHeaderId,omitempty"`
	EvenPageFooterID       string          `json:"evenPageFooterId,omitempty"`
	PageNumberStart        *int            `json:"pageNumberStart,omitempty"`
	FlipPageOrientation    *bool           `json:"flipPageOrientation,omitempty"`
}

// DocsBackground is the page colour.
type DocsBackground struct {
	Color *DocsColor `json:"color,omitempty"`
}

// DocsSize is the size of a page or an embedded object.
type DocsSize struct {
	Height *Dimension `json:"height,omitempty"`
	Width  *Dimension `json:"width,omitempty"`
}

// DocsNamedStyles are the document's heading styles: what NORMAL_TEXT and HEADING_1 mean
// in this document. A copy that writes the paragraphs without these inherits the new
// document's defaults instead.
type DocsNamedStyles struct {
	Styles []DocsNamedStyle `json:"styles,omitempty"`
}

// DocsNamedStyle is one entry of the named styles.
type DocsNamedStyle struct {
	NamedStyleType string              `json:"namedStyleType,omitempty"`
	TextStyle      *DocsTextStyle      `json:"textStyle,omitempty"`
	ParagraphStyle *DocsParagraphStyle `json:"paragraphStyle,omitempty"`
}

// DocsInlineObject is a picture or a drawing sitting in the text.
type DocsInlineObject struct {
	ObjectID   string                      `json:"objectId,omitempty"`
	Properties *DocsInlineObjectProperties `json:"inlineObjectProperties,omitempty"`
}

// DocsInlineObjectProperties wraps the embedded object.
type DocsInlineObjectProperties struct {
	EmbeddedObject *DocsEmbeddedObject `json:"embeddedObject,omitempty"`
}

// DocsPositioned is an object floating beside the text.
//
// It is read-only as far as any API caller is concerned: the whole of Docs v1 has
// deletePositionedObject and nothing that creates one. What a document has here, a
// rebuilt document cannot get.
type DocsPositioned struct {
	ObjectID   string                    `json:"objectId,omitempty"`
	Properties *DocsPositionedProperties `json:"positionedObjectProperties,omitempty"`
}

// DocsPositionedProperties is where a floating object sits and what it is.
type DocsPositionedProperties struct {
	Positioning    *DocsPositioning    `json:"positioning,omitempty"`
	EmbeddedObject *DocsEmbeddedObject `json:"embeddedObject,omitempty"`
}

// DocsPositioning is a floating object's layout and offsets.
type DocsPositioning struct {
	Layout     string     `json:"layout,omitempty"`
	LeftOffset *Dimension `json:"leftOffset,omitempty"`
	TopOffset  *Dimension `json:"topOffset,omitempty"`
}

// DocsEmbeddedObject is the picture or drawing itself.
//
// A drawing arrives as an empty embeddedDrawingProperties: the API reports that it is a
// drawing and nothing about what is in it. There is no request that makes one either.
type DocsEmbeddedObject struct {
	Title             string                    `json:"title,omitempty"`
	Description       string                    `json:"description,omitempty"`
	ImageProperties   *DocsImageProperties      `json:"imageProperties,omitempty"`
	DrawingProperties map[string]any            `json:"embeddedDrawingProperties,omitempty"`
	Border            *DocsEmbeddedObjectBorder `json:"embeddedObjectBorder,omitempty"`
	Size              *DocsSize                 `json:"size,omitempty"`
	MarginTop         *Dimension                `json:"marginTop,omitempty"`
	MarginBottom      *Dimension                `json:"marginBottom,omitempty"`
	MarginLeft        *Dimension                `json:"marginLeft,omitempty"`
	MarginRight       *Dimension                `json:"marginRight,omitempty"`
}

// DocsImageProperties is where a picture's bytes are and how they are shown.
type DocsImageProperties struct {
	ContentURI   string          `json:"contentUri,omitempty"`
	SourceURI    string          `json:"sourceUri,omitempty"`
	Angle        float64         `json:"angle,omitempty"`
	Brightness   float64         `json:"brightness,omitempty"`
	Contrast     float64         `json:"contrast,omitempty"`
	Transparency float64         `json:"transparency,omitempty"`
	Crop         *DocsCropValues `json:"cropProperties,omitempty"`
}

// DocsCropValues is how much of a picture is cut off on each side.
type DocsCropValues struct {
	Angle        float64 `json:"angle,omitempty"`
	OffsetTop    float64 `json:"offsetTop,omitempty"`
	OffsetBottom float64 `json:"offsetBottom,omitempty"`
	OffsetLeft   float64 `json:"offsetLeft,omitempty"`
	OffsetRight  float64 `json:"offsetRight,omitempty"`
}

// DocsEmbeddedObjectBorder is the frame around a picture.
type DocsEmbeddedObjectBorder struct {
	Color         *DocsColor `json:"color,omitempty"`
	Width         *Dimension `json:"width,omitempty"`
	DashStyle     string     `json:"dashStyle,omitempty"`
	PropertyState string     `json:"propertyState,omitempty"`
}

// DocsLocation is a place in a document, counted in characters from its start. The
// segment identifier is what makes it a place in a header or a footer instead of the body.
type DocsLocation struct {
	Index     int64  `json:"index"`
	SegmentID string `json:"segmentId,omitempty"`
}

// DocsSegmentEnd is the end of a segment: the body when the identifier is empty.
type DocsSegmentEnd struct {
	SegmentID string `json:"segmentId,omitempty"`
}

// DocsRange is a stretch of a document.
type DocsRange struct {
	StartIndex int64  `json:"startIndex"`
	EndIndex   int64  `json:"endIndex"`
	SegmentID  string `json:"segmentId,omitempty"`
}

// DocsRequest is one operation in a document batch. Exactly one field is set.
type DocsRequest struct {
	InsertText           *DocsInsertText           `json:"insertText,omitempty"`
	DeleteContent        *DocsDeleteContent        `json:"deleteContentRange,omitempty"`
	ReplaceAllText       *DocsReplaceAllText       `json:"replaceAllText,omitempty"`
	UpdateTextStyle      *DocsUpdateTextStyle      `json:"updateTextStyle,omitempty"`
	UpdateParagraph      *DocsUpdateParagraph      `json:"updateParagraphStyle,omitempty"`
	CreateBullets        *DocsCreateBullets        `json:"createParagraphBullets,omitempty"`
	InsertTable          *DocsInsertTable          `json:"insertTable,omitempty"`
	UpdateTableCellStyle *DocsUpdateTableCellStyle `json:"updateTableCellStyle,omitempty"`
	UpdateTableColumn    *DocsUpdateTableColumn    `json:"updateTableColumnProperties,omitempty"`
	UpdateTableRow       *DocsUpdateTableRow       `json:"updateTableRowStyle,omitempty"`
	MergeTableCells      *DocsMergeTableCells      `json:"mergeTableCells,omitempty"`
	PinTableHeaderRows   *DocsPinTableHeaderRows   `json:"pinTableHeaderRows,omitempty"`
	InsertTableRow       *DocsInsertTableRow       `json:"insertTableRow,omitempty"`
	InsertTableColumn    *DocsInsertTableColumn    `json:"insertTableColumn,omitempty"`
	InsertSectionBreak   *DocsInsertSectionBreak   `json:"insertSectionBreak,omitempty"`
	UpdateSectionStyle   *DocsUpdateSectionStyle   `json:"updateSectionStyle,omitempty"`
	CreateHeader         *DocsCreateHeaderFooter   `json:"createHeader,omitempty"`
	CreateFooter         *DocsCreateHeaderFooter   `json:"createFooter,omitempty"`
	CreateFootnote       *DocsCreateFootnote       `json:"createFootnote,omitempty"`
	UpdateDocumentStyle  *DocsUpdateDocumentStyle  `json:"updateDocumentStyle,omitempty"`
	UpdateNamedStyle     *DocsUpdateNamedStyle     `json:"updateNamedStyle,omitempty"`
	InsertInlineImage    *DocsInsertInlineImage    `json:"insertInlineImage,omitempty"`
	InsertPageBreak      *DocsInsertPageBreak      `json:"insertPageBreak,omitempty"`
	CreateNamedRange     *DocsCreateNamedRange     `json:"createNamedRange,omitempty"`
	DeleteTableRow       *DocsDeleteTableRow       `json:"deleteTableRow,omitempty"`
	DeleteTableColumn    *DocsDeleteTableColumn    `json:"deleteTableColumn,omitempty"`
	DeleteHeader         *DocsDeleteHeader         `json:"deleteHeader,omitempty"`
	DeleteFooter         *DocsDeleteFooter         `json:"deleteFooter,omitempty"`
	DeletePositioned     *DocsDeletePositioned     `json:"deletePositionedObject,omitempty"`
	DeleteBullets        *DocsDeleteBullets        `json:"deleteParagraphBullets,omitempty"`
}

// DocsDeleteTableRow takes out the row a cell is in.
type DocsDeleteTableRow struct {
	CellLocation DocsTableCellLocation `json:"tableCellLocation"`
}

// DocsDeleteTableColumn takes out the column a cell is in.
type DocsDeleteTableColumn struct {
	CellLocation DocsTableCellLocation `json:"tableCellLocation"`
}

// DocsDeleteHeader removes a header segment and everything in it.
type DocsDeleteHeader struct {
	HeaderID string `json:"headerId"`
}

// DocsDeleteFooter removes a footer segment and everything in it.
type DocsDeleteFooter struct {
	FooterID string `json:"footerId"`
}

// DocsDeletePositioned removes a floating object. It is the only half of that pair the
// API has: nothing in Docs v1 creates one.
type DocsDeletePositioned struct {
	ObjectID string `json:"objectId"`
}

// DocsDeleteBullets turns list items back into ordinary paragraphs. The text stays; the
// glyphs and the list's own indents go.
type DocsDeleteBullets struct {
	Range DocsRange `json:"range"`
}

// DocsInsertText puts text at one place in a document.
type DocsInsertText struct {
	Location *DocsLocation   `json:"location,omitempty"`
	EndOfDoc *DocsSegmentEnd `json:"endOfSegmentLocation,omitempty"`
	Text     string          `json:"text"`
}

// DocsDeleteContent removes a stretch of a document.
type DocsDeleteContent struct {
	Range DocsRange `json:"range"`
}

// DocsSubstringMatch is the text a replacement looks for.
type DocsSubstringMatch struct {
	Text      string `json:"text"`
	MatchCase bool   `json:"matchCase"`
}

// DocsReplaceAllText swaps every occurrence of a string.
type DocsReplaceAllText struct {
	ContainsText DocsSubstringMatch `json:"containsText"`
	ReplaceText  string             `json:"replaceText"`
}

// DocsUpdateTextStyle restyles a stretch of a document.
type DocsUpdateTextStyle struct {
	Range  DocsRange      `json:"range"`
	Style  *DocsTextStyle `json:"textStyle"`
	Fields string         `json:"fields"`
}

// DocsUpdateParagraph restyles paragraphs of a document.
type DocsUpdateParagraph struct {
	Range  DocsRange           `json:"range"`
	Style  *DocsParagraphStyle `json:"paragraphStyle"`
	Fields string              `json:"fields"`
}

// DocsCreateBullets turns paragraphs into list items. The preset decides the glyphs;
// there is no request that sets a glyph directly.
type DocsCreateBullets struct {
	Range        DocsRange `json:"range"`
	BulletPreset string    `json:"bulletPreset,omitempty"`
}

// DocsInsertTable puts an empty table into a document.
type DocsInsertTable struct {
	Rows     int             `json:"rows"`
	Columns  int             `json:"columns"`
	Location *DocsLocation   `json:"location,omitempty"`
	EndOfDoc *DocsSegmentEnd `json:"endOfSegmentLocation,omitempty"`
}

// DocsTableCellLocation names one cell by its table, row and column.
type DocsTableCellLocation struct {
	TableStart  DocsLocation `json:"tableStartLocation"`
	RowIndex    int          `json:"rowIndex"`
	ColumnIndex int          `json:"columnIndex"`
}

// DocsTableRange is a rectangle of cells.
type DocsTableRange struct {
	CellLocation DocsTableCellLocation `json:"tableCellLocation"`
	RowSpan      int                   `json:"rowSpan"`
	ColumnSpan   int                   `json:"columnSpan"`
}

// DocsUpdateTableCellStyle paints a rectangle of cells.
type DocsUpdateTableCellStyle struct {
	Range  *DocsTableRange     `json:"tableRange,omitempty"`
	Start  *DocsLocation       `json:"tableStartLocation,omitempty"`
	Style  *DocsTableCellStyle `json:"tableCellStyle"`
	Fields string              `json:"fields"`
}

// DocsUpdateTableColumn sets the width of some columns.
type DocsUpdateTableColumn struct {
	Start      DocsLocation               `json:"tableStartLocation"`
	Indices    []int                      `json:"columnIndices,omitempty"`
	Properties *DocsTableColumnProperties `json:"tableColumnProperties"`
	Fields     string                     `json:"fields"`
}

// DocsUpdateTableRow sets the height of some rows.
type DocsUpdateTableRow struct {
	Start   DocsLocation       `json:"tableStartLocation"`
	Indices []int              `json:"rowIndices,omitempty"`
	Style   *DocsTableRowStyle `json:"tableRowStyle"`
	Fields  string             `json:"fields"`
}

// DocsMergeTableCells joins a rectangle of cells into one.
type DocsMergeTableCells struct {
	Range DocsTableRange `json:"tableRange"`
}

// DocsPinTableHeaderRows repeats the first rows on every page.
type DocsPinTableHeaderRows struct {
	Start DocsLocation `json:"tableStartLocation"`
	Count int          `json:"pinnedHeaderRowsCount"`
}

// DocsInsertTableRow adds a row beside a cell.
type DocsInsertTableRow struct {
	CellLocation DocsTableCellLocation `json:"tableCellLocation"`
	InsertBelow  bool                  `json:"insertBelow"`
}

// DocsInsertTableColumn adds a column beside a cell.
type DocsInsertTableColumn struct {
	CellLocation DocsTableCellLocation `json:"tableCellLocation"`
	InsertRight  bool                  `json:"insertRight"`
}

// DocsInsertSectionBreak starts a new section.
type DocsInsertSectionBreak struct {
	SectionType string          `json:"sectionType"`
	Location    *DocsLocation   `json:"location,omitempty"`
	EndOfDoc    *DocsSegmentEnd `json:"endOfSegmentLocation,omitempty"`
}

// DocsUpdateSectionStyle sets the page setup of the section a range falls in.
type DocsUpdateSectionStyle struct {
	Range  DocsRange         `json:"range"`
	Style  *DocsSectionStyle `json:"sectionStyle"`
	Fields string            `json:"fields"`
}

// DocsCreateHeaderFooter makes a header or a footer, either for the document or for the
// section that a break starts.
type DocsCreateHeaderFooter struct {
	Type                 string        `json:"type"`
	SectionBreakLocation *DocsLocation `json:"sectionBreakLocation,omitempty"`
}

// DocsCreateFootnote makes a footnote and puts its reference in the text.
type DocsCreateFootnote struct {
	Location *DocsLocation   `json:"location,omitempty"`
	EndOfDoc *DocsSegmentEnd `json:"endOfSegmentLocation,omitempty"`
}

// DocsUpdateDocumentStyle sets the page setup of the whole document.
type DocsUpdateDocumentStyle struct {
	Style  *DocsDocumentStyle `json:"documentStyle"`
	Fields string             `json:"fields"`
}

// DocsUpdateNamedStyle changes what a named style means in this document.
type DocsUpdateNamedStyle struct {
	Style  *DocsNamedStyle `json:"namedStyle"`
	Fields string          `json:"fields"`
}

// DocsInsertInlineImage puts a picture into the text.
//
// The four fields here are the whole request: there is no positioning. A picture put in
// by an API caller sits in the line of text, never beside it.
type DocsInsertInlineImage struct {
	URI      string          `json:"uri"`
	Location *DocsLocation   `json:"location,omitempty"`
	EndOfDoc *DocsSegmentEnd `json:"endOfSegmentLocation,omitempty"`
	Size     *DocsSize       `json:"objectSize,omitempty"`
}

// DocsInsertPageBreak starts a new page.
type DocsInsertPageBreak struct {
	Location *DocsLocation   `json:"location,omitempty"`
	EndOfDoc *DocsSegmentEnd `json:"endOfSegmentLocation,omitempty"`
}

// DocsCreateNamedRange names a stretch of the document so later edits can find it
// without counting characters.
type DocsCreateNamedRange struct {
	Name  string    `json:"name"`
	Range DocsRange `json:"range"`
}

// DocsBatchUpdateRequest is the body of documents.batchUpdate.
type DocsBatchUpdateRequest struct {
	Requests []DocsRequest `json:"requests"`
}

// DocsBatchUpdateResponse is what came back.
type DocsBatchUpdateResponse struct {
	DocumentID string           `json:"documentId,omitempty"`
	Replies    []DocsBatchReply `json:"replies,omitempty"`
}

// DocsBatchReply is one answer to one request, for the requests that answer anything.
type DocsBatchReply struct {
	ReplaceAllText *struct {
		OccurrencesChanged int `json:"occurrencesChanged"`
	} `json:"replaceAllText,omitempty"`
	CreateHeader *struct {
		HeaderID string `json:"headerId"`
	} `json:"createHeader,omitempty"`
	CreateFooter *struct {
		FooterID string `json:"footerId"`
	} `json:"createFooter,omitempty"`
	CreateFootnote *struct {
		FootnoteID string `json:"footnoteId"`
	} `json:"createFootnote,omitempty"`
	InsertInlineImage *struct {
		ObjectID string `json:"objectId"`
	} `json:"insertInlineImage,omitempty"`
}

// Document reads a document.
func (c *Client) Document(ctx context.Context, documentID string) (*Document, error) {
	var out Document
	if err := c.call(ctx, http.MethodGet,
		endpoint(c.docsBase, "/documents/"+url.PathEscape(documentID), nil), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// CreateDocument makes an empty document with a title.
func (c *Client) CreateDocument(ctx context.Context, title string) (*Document, error) {
	var out Document
	if err := c.call(ctx, http.MethodPost, endpoint(c.docsBase, "/documents", nil),
		map[string]string{"title": title}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// DocsBatchUpdate sends one batch of requests to a document.
func (c *Client) DocsBatchUpdate(ctx context.Context, documentID string, requests []DocsRequest) (*DocsBatchUpdateResponse, error) {
	var out DocsBatchUpdateResponse
	if err := c.call(ctx, http.MethodPost,
		endpoint(c.docsBase, "/documents/"+url.PathEscape(documentID)+":batchUpdate", nil),
		DocsBatchUpdateRequest{Requests: requests}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// PlainText renders a document's body as text, which is what a caller usually wants to
// read. Tables come out row by row, cells separated by tabs.
func (d *Document) PlainText() string {
	if d.Body == nil {
		return ""
	}

	return elementsText(d.Body.Content)
}

func elementsText(elements []StructuralElement) string {
	var out []byte

	for _, element := range elements {
		switch {
		case element.Paragraph != nil:
			for _, piece := range element.Paragraph.Elements {
				if piece.TextRun != nil {
					out = append(out, piece.TextRun.Content...)
				}
			}
		case element.Table != nil:
			for _, row := range element.Table.Content {
				for index, cell := range row.Cells {
					if index > 0 {
						out = append(out, '\t')
					}
					out = append(out, trimTrailingNewline(elementsText(cell.Content))...)
				}
				out = append(out, '\n')
			}
		}
	}

	return string(out)
}

func trimTrailingNewline(text string) string {
	for len(text) > 0 && (text[len(text)-1] == '\n' || text[len(text)-1] == '\v') {
		text = text[:len(text)-1]
	}
	return text
}
