package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// Spreadsheet is a workbook as far as this server reads it.
type Spreadsheet struct {
	SpreadsheetID  string                `json:"spreadsheetId"`
	Properties     SpreadsheetProperties `json:"properties"`
	Sheets         []Sheet               `json:"sheets"`
	NamedRanges    []NamedRange          `json:"namedRanges,omitempty"`
	SpreadsheetURL string                `json:"spreadsheetUrl,omitempty"`
}

// SpreadsheetProperties is the workbook's own settings.
type SpreadsheetProperties struct {
	Title    string `json:"title,omitempty"`
	Locale   string `json:"locale,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

// Sheet is one tab.
type Sheet struct {
	Properties SheetProperties `json:"properties"`
	// Data is filled in only when the grid was asked for, which is how the formatting of
	// a sample workbook is read before it is reproduced.
	Data               []GridData          `json:"data,omitempty"`
	Merges             []GridRange         `json:"merges,omitempty"`
	ConditionalFormats []ConditionalFormat `json:"conditionalFormats,omitempty"`
	BandedRanges       []BandedRange       `json:"bandedRanges,omitempty"`
	ProtectedRanges    []ProtectedRange    `json:"protectedRanges,omitempty"`
	FilterViews        []FilterView        `json:"filterViews,omitempty"`
	BasicFilter        *BasicFilter        `json:"basicFilter,omitempty"`
	RowGroups          []DimensionGroup    `json:"rowGroups,omitempty"`
	ColumnGroups       []DimensionGroup    `json:"columnGroups,omitempty"`
	// The rest are kept raw and only counted: reproducing a chart or a data source needs
	// a surface of its own, and a tab that has one is a tab a copy will quietly differ
	// from. A count is what lets that be named in a report instead of discovered later.
	Charts  []json.RawMessage `json:"charts,omitempty"`
	Slicers []json.RawMessage `json:"slicers,omitempty"`
	Tables  []json.RawMessage `json:"tables,omitempty"`
}

// ConditionalFormat is a rule that colours cells by what is in them.
type ConditionalFormat struct {
	Ranges       []GridRange   `json:"ranges,omitempty"`
	BooleanRule  *BooleanRule  `json:"booleanRule,omitempty"`
	GradientRule *GradientRule `json:"gradientRule,omitempty"`
}

// BooleanRule paints a cell when its condition holds.
type BooleanRule struct {
	Condition *BooleanCondition `json:"condition,omitempty"`
	Format    *CellFormat       `json:"format,omitempty"`
}

// GradientRule paints a range along a scale.
type GradientRule struct {
	MinPoint *InterpolationPoint `json:"minpoint,omitempty"`
	MidPoint *InterpolationPoint `json:"midpoint,omitempty"`
	MaxPoint *InterpolationPoint `json:"maxpoint,omitempty"`
}

// InterpolationPoint is one end or the middle of a colour scale.
type InterpolationPoint struct {
	Color *RGBColor `json:"color,omitempty"`
	Type  string    `json:"type,omitempty"`
	Value string    `json:"value,omitempty"`
}

// BandedRange is the alternating fill of a table.
type BandedRange struct {
	BandedRangeID    int                `json:"bandedRangeId,omitempty"`
	Range            GridRange          `json:"range"`
	RowProperties    *BandingProperties `json:"rowProperties,omitempty"`
	ColumnProperties *BandingProperties `json:"columnProperties,omitempty"`
}

// BandingProperties is the four colours a banding uses.
type BandingProperties struct {
	HeaderColor     *RGBColor `json:"headerColor,omitempty"`
	FirstBandColor  *RGBColor `json:"firstBandColor,omitempty"`
	SecondBandColor *RGBColor `json:"secondBandColor,omitempty"`
	FooterColor     *RGBColor `json:"footerColor,omitempty"`
}

// ProtectedRange is a range only some people may change.
type ProtectedRange struct {
	ProtectedRangeID int        `json:"protectedRangeId,omitempty"`
	Range            *GridRange `json:"range,omitempty"`
	Description      string     `json:"description,omitempty"`
	WarningOnly      bool       `json:"warningOnly,omitempty"`
	Editors          *Editors   `json:"editors,omitempty"`
}

// Editors is who may change a protected range.
type Editors struct {
	Users              []string `json:"users,omitempty"`
	Groups             []string `json:"groups,omitempty"`
	DomainUsersCanEdit bool     `json:"domainUsersCanEdit,omitempty"`
}

// BasicFilter is the filter a tab carries, the one the toolbar switches on.
type BasicFilter struct {
	Range       *GridRange   `json:"range,omitempty"`
	SortSpecs   []SortSpec   `json:"sortSpecs,omitempty"`
	FilterSpecs []FilterSpec `json:"filterSpecs,omitempty"`
}

// FilterView is a saved way of looking at a tab.
type FilterView struct {
	FilterViewID int          `json:"filterViewId,omitempty"`
	Title        string       `json:"title,omitempty"`
	Range        *GridRange   `json:"range,omitempty"`
	SortSpecs    []SortSpec   `json:"sortSpecs,omitempty"`
	FilterSpecs  []FilterSpec `json:"filterSpecs,omitempty"`
}

// SortSpec is one column of a sort.
type SortSpec struct {
	DimensionIndex int    `json:"dimensionIndex"`
	SortOrder      string `json:"sortOrder,omitempty"`
}

// FilterSpec is what one column of a filter hides.
type FilterSpec struct {
	ColumnIndex    int             `json:"columnIndex"`
	FilterCriteria *FilterCriteria `json:"filterCriteria,omitempty"`
}

// FilterCriteria is the test a filtered column applies.
type FilterCriteria struct {
	HiddenValues []string          `json:"hiddenValues,omitempty"`
	Condition    *BooleanCondition `json:"condition,omitempty"`
}

// DimensionGroup is a run of rows or columns that folds up.
type DimensionGroup struct {
	Range     DimensionRange `json:"range"`
	Depth     int            `json:"depth,omitempty"`
	Collapsed bool           `json:"collapsed,omitempty"`
}

// NamedRange is a range with a name people write in formulas.
type NamedRange struct {
	NamedRangeID string    `json:"namedRangeId,omitempty"`
	Name         string    `json:"name,omitempty"`
	Range        GridRange `json:"range"`
}

// GridData is a rectangle of the grid with the formatting of its cells and the widths of
// its columns.
type GridData struct {
	// StartRow and StartColumn are where the rectangle sits in the sheet. They are absent
	// for a range that starts at A1, and reporting a cell's position without adding them
	// is how a reading of C5:F20 gets written back over C1.
	StartRow       int                   `json:"startRow,omitempty"`
	StartColumn    int                   `json:"startColumn,omitempty"`
	RowData        []RowData             `json:"rowData,omitempty"`
	ColumnMetadata []DimensionProperties `json:"columnMetadata,omitempty"`
	RowMetadata    []DimensionProperties `json:"rowMetadata,omitempty"`
}

// RowData is one row of the grid.
type RowData struct {
	Values []CellValue `json:"values,omitempty"`
}

// CellValue is one cell as it was read: what a person typed, how it is shown, and how it
// is formatted.
//
// Note, Hyperlink and DataValidation sit beside the format rather than inside it, which is
// why a reading that asks only for userEnteredFormat comes back without them.
type CellValue struct {
	FormattedValue    string              `json:"formattedValue,omitempty"`
	UserEnteredFormat *CellFormat         `json:"userEnteredFormat,omitempty"`
	Note              string              `json:"note,omitempty"`
	Hyperlink         string              `json:"hyperlink,omitempty"`
	DataValidation    *DataValidationRule `json:"dataValidation,omitempty"`
	// TextFormatRuns is styling that changes partway through a cell: one cell, several
	// looks. A reading that only reports the cell's format flattens it.
	TextFormatRuns   []TextFormatRun `json:"textFormatRuns,omitempty"`
	UserEnteredValue *ExtendedValue  `json:"userEnteredValue,omitempty"`
}

// TextFormatRun is a stretch of a cell's text that looks different from the rest. It
// starts at StartIndex and runs until the next one begins.
type TextFormatRun struct {
	StartIndex int         `json:"startIndex,omitempty"`
	Format     *SheetsText `json:"format,omitempty"`
}

// ExtendedValue is a cell's content in whichever shape it was stored.
type ExtendedValue struct {
	StringValue  *string  `json:"stringValue,omitempty"`
	NumberValue  *float64 `json:"numberValue,omitempty"`
	BoolValue    *bool    `json:"boolValue,omitempty"`
	FormulaValue *string  `json:"formulaValue,omitempty"`
}

// DataValidationRule is what a cell will accept — a dropdown, in practice.
type DataValidationRule struct {
	Condition    *BooleanCondition `json:"condition,omitempty"`
	InputMessage string            `json:"inputMessage,omitempty"`
	Strict       bool              `json:"strict,omitempty"`
	ShowCustomUI bool              `json:"showCustomUi,omitempty"`
}

// BooleanCondition is the test a validation applies.
type BooleanCondition struct {
	Type   string           `json:"type,omitempty"`
	Values []ConditionValue `json:"values,omitempty"`
}

// ConditionValue is one operand of a condition: a literal, or a range to take the list
// from, written as a formula.
type ConditionValue struct {
	UserEnteredValue string `json:"userEnteredValue,omitempty"`
	RelativeDate     string `json:"relativeDate,omitempty"`
}

// SheetProperties is a tab's own settings.
//
// Every field is omitted when unset, and for the three that mean something at zero that
// takes a pointer. A zero identifier is the first tab and has to be sent when a request
// addresses it; sent while creating a workbook it means "give every tab number 0", which
// Google refuses. A zero index means "make this the first tab" — sent alongside a request
// meant to freeze a row it would move the tab to the front as well. An empty title is a
// request to name a tab nothing.
type SheetProperties struct {
	SheetID   *int       `json:"sheetId,omitempty"`
	Title     string     `json:"title,omitempty"`
	Index     *int       `json:"index,omitempty"`
	Hidden    bool       `json:"hidden,omitempty"`
	GridProps *GridProps `json:"gridProperties,omitempty"`
	TabColor  *RGBColor  `json:"tabColor,omitempty"`
}

// GridProps is how big a tab is and how much of it stays put while the rest scrolls.
type GridProps struct {
	RowCount          int `json:"rowCount,omitempty"`
	ColumnCount       int `json:"columnCount,omitempty"`
	FrozenRowCount    int `json:"frozenRowCount,omitempty"`
	FrozenColumnCount int `json:"frozenColumnCount,omitempty"`
}

// ValueRange is a rectangle of cells.
type ValueRange struct {
	Range          string  `json:"range,omitempty"`
	MajorDimension string  `json:"majorDimension,omitempty"`
	Values         [][]any `json:"values"`
}

// UpdateValuesResponse is what a write reported.
type UpdateValuesResponse struct {
	SpreadsheetID  string `json:"spreadsheetId,omitempty"`
	UpdatedRange   string `json:"updatedRange,omitempty"`
	UpdatedRows    int    `json:"updatedRows,omitempty"`
	UpdatedColumns int    `json:"updatedColumns,omitempty"`
	UpdatedCells   int    `json:"updatedCells,omitempty"`
}

// AppendValuesResponse is what an append reported.
type AppendValuesResponse struct {
	SpreadsheetID string                `json:"spreadsheetId,omitempty"`
	TableRange    string                `json:"tableRange,omitempty"`
	Updates       *UpdateValuesResponse `json:"updates,omitempty"`
}

// Spreadsheet reads a workbook's structure: its tabs, their sizes, their identifiers.
// Cell values are not included — those come from Values, one range at a time.
func (c *Client) Spreadsheet(ctx context.Context, spreadsheetID string) (*Spreadsheet, error) {
	query := url.Values{}
	query.Set("fields", "spreadsheetId,spreadsheetUrl,properties(title,locale,timeZone),"+
		"namedRanges,sheets(properties(sheetId,title,index,hidden,gridProperties,tabColor),"+
		"merges,conditionalFormats,bandedRanges,protectedRanges,charts,slicers,tables,"+
		"filterViews,basicFilter,rowGroups,columnGroups)")

	var out Spreadsheet
	if err := c.call(ctx, http.MethodGet,
		endpoint(c.sheetsBase, "/spreadsheets/"+url.PathEscape(spreadsheetID), query), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// SpreadsheetGrid reads one range with its formatting: fonts, colours, alignment, number
// formats, and the widths of the columns it covers.
//
// This is what reading a sample workbook means — the values come back too, but the point
// is everything around them, because that is what has to be reproduced.
func (c *Client) SpreadsheetGrid(ctx context.Context, spreadsheetID, a1Range string) (*Spreadsheet, error) {
	query := url.Values{}
	query.Set("includeGridData", "true")
	if a1Range != "" {
		query.Set("ranges", a1Range)
	}
	// The note, the link and the validation are asked for by name: each sits beside the
	// format rather than inside it, and a mask that names only userEnteredFormat comes
	// back without them however much of the cell was actually stored.
	query.Set("fields", "spreadsheetId,properties(title),"+
		"sheets(properties(sheetId,title,index,gridProperties),merges,conditionalFormats,"+
		"bandedRanges,protectedRanges,basicFilter,filterViews,rowGroups,columnGroups,"+
		"data(startRow,startColumn,rowData(values(formattedValue,note,hyperlink,dataValidation,"+
		"textFormatRuns,userEnteredFormat(numberFormat,backgroundColor,hyperlinkDisplayType,"+
		"horizontalAlignment,verticalAlignment,wrapStrategy,textRotation,padding,borders,"+
		"textFormat))),"+
		"columnMetadata(pixelSize,hiddenByUser),rowMetadata(pixelSize,hiddenByUser)))")

	var out Spreadsheet
	if err := c.call(ctx, http.MethodGet,
		endpoint(c.sheetsBase, "/spreadsheets/"+url.PathEscape(spreadsheetID), query), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// Values reads one range in A1 notation. An empty range name reads a whole tab.
func (c *Client) Values(ctx context.Context, spreadsheetID, a1Range, majorDimension, renderOption string) (*ValueRange, error) {
	query := url.Values{}
	if majorDimension != "" {
		query.Set("majorDimension", majorDimension)
	}
	if renderOption != "" {
		query.Set("valueRenderOption", renderOption)
	}

	var out ValueRange
	if err := c.call(ctx, http.MethodGet, endpoint(c.sheetsBase,
		"/spreadsheets/"+url.PathEscape(spreadsheetID)+"/values/"+url.PathEscape(a1Range), query),
		nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateValues writes a rectangle of cells over whatever was there.
//
// valueInputOption decides whether "=SUM(A1:A2)" is a formula or the literal text: RAW
// stores what it was given, USER_ENTERED parses it the way typing it would.
func (c *Client) UpdateValues(ctx context.Context, spreadsheetID, a1Range string, values [][]any, valueInputOption string) (*UpdateValuesResponse, error) {
	query := url.Values{}
	query.Set("valueInputOption", valueInputOption)

	var out UpdateValuesResponse
	if err := c.call(ctx, http.MethodPut, endpoint(c.sheetsBase,
		"/spreadsheets/"+url.PathEscape(spreadsheetID)+"/values/"+url.PathEscape(a1Range), query),
		ValueRange{Range: a1Range, MajorDimension: "ROWS", Values: values}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// AppendValues adds rows after the last one already in the range.
func (c *Client) AppendValues(ctx context.Context, spreadsheetID, a1Range string, values [][]any, valueInputOption string) (*AppendValuesResponse, error) {
	query := url.Values{}
	query.Set("valueInputOption", valueInputOption)
	// Rows are inserted rather than written over whatever sits below the table: an
	// append that overwrites the next block of a sheet is not an append.
	query.Set("insertDataOption", "INSERT_ROWS")

	var out AppendValuesResponse
	if err := c.call(ctx, http.MethodPost, endpoint(c.sheetsBase,
		"/spreadsheets/"+url.PathEscape(spreadsheetID)+"/values/"+url.PathEscape(a1Range)+":append", query),
		ValueRange{MajorDimension: "ROWS", Values: values}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// CreateSpreadsheetRequest is a new workbook.
type CreateSpreadsheetRequest struct {
	Properties SpreadsheetProperties `json:"properties"`
	Sheets     []Sheet               `json:"sheets,omitempty"`
}

// CreateSpreadsheet makes a workbook with the tabs described.
//
// The size of a tab is settable here and nowhere else without loss: a new tab arrives
// 1000 by 26, and making it smaller afterwards deletes rows and columns. Asking for the
// sample's size at creation is the only way to match it without deleting anything.
func (c *Client) CreateSpreadsheet(ctx context.Context, body CreateSpreadsheetRequest) (*Spreadsheet, error) {
	var out Spreadsheet
	if err := c.call(ctx, http.MethodPost, endpoint(c.sheetsBase, "/spreadsheets", nil), body, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// SheetsRequest is one operation in a workbook batch. Exactly one field is set.
type SheetsRequest struct {
	AddSheet        *AddSheetRequest          `json:"addSheet,omitempty"`
	RepeatCell      *RepeatCellRequest        `json:"repeatCell,omitempty"`
	UpdateSheet     *UpdateSheetRequest       `json:"updateSheetProperties,omitempty"`
	AutoResize      *AutoResizeRequest        `json:"autoResizeDimensions,omitempty"`
	UpdateBorders   *UpdateBordersRequest     `json:"updateBorders,omitempty"`
	UpdateDimension *UpdateDimensionRequest   `json:"updateDimensionProperties,omitempty"`
	MergeCells      *MergeCellsRequest        `json:"mergeCells,omitempty"`
	SetValidation   *SetDataValidationRequest `json:"setDataValidation,omitempty"`

	UpdateCells       *UpdateCellsRequest             `json:"updateCells,omitempty"`
	AddConditional    *AddConditionalFormatRequest    `json:"addConditionalFormatRule,omitempty"`
	UpdateConditional *UpdateConditionalFormatRequest `json:"updateConditionalFormatRule,omitempty"`
	AddBanding        *AddBandingRequest              `json:"addBanding,omitempty"`
	UpdateBanding     *UpdateBandingRequest           `json:"updateBanding,omitempty"`
	SetBasicFilter    *SetBasicFilterRequest          `json:"setBasicFilter,omitempty"`
	AddProtected      *AddProtectedRangeRequest       `json:"addProtectedRange,omitempty"`
	AddNamedRange     *AddNamedRangeRequest           `json:"addNamedRange,omitempty"`
	AddDimensionGroup *AddDimensionGroupRequest       `json:"addDimensionGroup,omitempty"`
	InsertDimension   *InsertDimensionRequest         `json:"insertDimension,omitempty"`
	AppendDimension   *AppendDimensionRequest         `json:"appendDimension,omitempty"`
	MoveDimension     *MoveDimensionRequest           `json:"moveDimension,omitempty"`
	SortRange         *SortRangeRequest               `json:"sortRange,omitempty"`
	DuplicateSheet    *DuplicateSheetRequest          `json:"duplicateSheet,omitempty"`
	UpdateSpreadsheet *UpdateSpreadsheetPropsRequest  `json:"updateSpreadsheetProperties,omitempty"`

	UnmergeCells     *UnmergeCellsRequest         `json:"unmergeCells,omitempty"`
	ClearBasicFilter *ClearBasicFilterRequest     `json:"clearBasicFilter,omitempty"`
	FindReplace      *FindReplaceRequest          `json:"findReplace,omitempty"`
	TrimWhitespace   *TrimWhitespaceRequest       `json:"trimWhitespace,omitempty"`
	TextToColumns    *TextToColumnsRequest        `json:"textToColumns,omitempty"`
	AutoFill         *AutoFillRequest             `json:"autoFill,omitempty"`
	AddChart         *AddChartRequest             `json:"addChart,omitempty"`
	AddTable         *AddTableRequest             `json:"addTable,omitempty"`
	UpdateGroup      *UpdateDimensionGroupRequest `json:"updateDimensionGroup,omitempty"`
}

// UnmergeCellsRequest takes a merge apart. The cells come back; nothing is removed.
type UnmergeCellsRequest struct {
	Range GridRange `json:"range"`
}

// ClearBasicFilterRequest takes the tab's filter off. The rows it hid come back.
type ClearBasicFilterRequest struct {
	SheetID int `json:"sheetId"`
}

// FindReplaceRequest changes text across a range, a tab or the whole workbook.
type FindReplaceRequest struct {
	Find            string     `json:"find"`
	Replacement     string     `json:"replacement"`
	Range           *GridRange `json:"range,omitempty"`
	SheetID         *int       `json:"sheetId,omitempty"`
	AllSheets       bool       `json:"allSheets,omitempty"`
	MatchCase       bool       `json:"matchCase,omitempty"`
	MatchEntireCell bool       `json:"matchEntireCell,omitempty"`
	SearchByRegex   bool       `json:"searchByRegex,omitempty"`
	IncludeFormulas bool       `json:"includeFormulas,omitempty"`
}

// TrimWhitespaceRequest takes the spaces off both ends of every cell in a range.
type TrimWhitespaceRequest struct {
	Range GridRange `json:"range"`
}

// TextToColumnsRequest splits one column into several by a separator.
type TextToColumnsRequest struct {
	Source        GridRange `json:"source"`
	DelimiterType string    `json:"delimiterType,omitempty"`
	Delimiter     string    `json:"delimiter,omitempty"`
}

// AutoFillRequest continues a series the way dragging the corner of a cell does.
type AutoFillRequest struct {
	SourceAndDestination *SourceAndDestination `json:"sourceAndDestination,omitempty"`
	UseAlternateSeries   bool                  `json:"useAlternateSeries,omitempty"`
}

// SourceAndDestination says which part of a rectangle is the example and how far it goes.
type SourceAndDestination struct {
	Source     GridRange `json:"source"`
	Dimension  string    `json:"dimension"`
	FillLength int       `json:"fillLength"`
}

// AddChartRequest puts a chart on a tab.
type AddChartRequest struct {
	Chart EmbeddedChart `json:"chart"`
}

// EmbeddedChart is a chart and where it sits.
type EmbeddedChart struct {
	Spec     ChartSpec              `json:"spec"`
	Position EmbeddedObjectPosition `json:"position"`
}

// ChartSpec is what a chart shows. One of the kinds is set.
type ChartSpec struct {
	Title      string          `json:"title,omitempty"`
	Subtitle   string          `json:"subtitle,omitempty"`
	AltText    string          `json:"altText,omitempty"`
	FontName   string          `json:"fontName,omitempty"`
	BasicChart *BasicChartSpec `json:"basicChart,omitempty"`
	PieChart   *PieChartSpec   `json:"pieChart,omitempty"`
}

// BasicChartSpec is the family of charts drawn against two axes.
type BasicChartSpec struct {
	ChartType      string             `json:"chartType"`
	LegendPosition string             `json:"legendPosition,omitempty"`
	StackedType    string             `json:"stackedType,omitempty"`
	HeaderCount    int                `json:"headerCount,omitempty"`
	Domains        []BasicChartDomain `json:"domains,omitempty"`
	Series         []BasicChartSeries `json:"series,omitempty"`
	Axis           []BasicChartAxis   `json:"axis,omitempty"`
}

// BasicChartDomain is what runs along the bottom.
type BasicChartDomain struct {
	Domain ChartData `json:"domain"`
}

// BasicChartSeries is one line or one set of bars.
type BasicChartSeries struct {
	Series     ChartData `json:"series"`
	TargetAxis string    `json:"targetAxis,omitempty"`
	Color      *RGBColor `json:"color,omitempty"`
}

// BasicChartAxis is one axis and its title.
type BasicChartAxis struct {
	Position string `json:"position,omitempty"`
	Title    string `json:"title,omitempty"`
}

// PieChartSpec is a pie, which has one slice per row rather than a series.
type PieChartSpec struct {
	Domain         ChartData `json:"domain"`
	Series         ChartData `json:"series"`
	LegendPosition string    `json:"legendPosition,omitempty"`
	PieHole        float64   `json:"pieHole,omitempty"`
	ThreeD         bool      `json:"threeDimensional,omitempty"`
}

// ChartData points a chart at the cells it draws.
type ChartData struct {
	SourceRange *ChartSourceRange `json:"sourceRange,omitempty"`
}

// ChartSourceRange is the rectangles one part of a chart reads.
type ChartSourceRange struct {
	Sources []GridRange `json:"sources"`
}

// EmbeddedObjectPosition is where a chart lands: floating over a tab, or on one of its own.
type EmbeddedObjectPosition struct {
	NewSheet        bool             `json:"newSheet,omitempty"`
	OverlayPosition *OverlayPosition `json:"overlayPosition,omitempty"`
}

// OverlayPosition is a chart floating over cells.
type OverlayPosition struct {
	AnchorCell    GridCoord `json:"anchorCell"`
	OffsetXPixels int       `json:"offsetXPixels,omitempty"`
	OffsetYPixels int       `json:"offsetYPixels,omitempty"`
	WidthPixels   int       `json:"widthPixels,omitempty"`
	HeightPixels  int       `json:"heightPixels,omitempty"`
}

// AddTableRequest turns a rectangle into one of Sheets' tables: a named range whose columns
// have types of their own, and whose DROPDOWN columns are the ones drawn as coloured chips.
type AddTableRequest struct {
	Table SheetsTable `json:"table"`
}

// SheetsTable is a rectangle with a name and typed columns. Slides has a Table of its own,
// which is a different thing entirely: this one is a block of a spreadsheet.
type SheetsTable struct {
	Name             string               `json:"name,omitempty"`
	Range            GridRange            `json:"range"`
	ColumnProperties []SheetsTableColumn  `json:"columnProperties,omitempty"`
	RowsProperties   *TableRowsProperties `json:"rowsProperties,omitempty"`
}

// SheetsTableColumn is one column of a table and what it holds.
type SheetsTableColumn struct {
	ColumnIndex        int                    `json:"columnIndex"`
	ColumnName         string                 `json:"columnName,omitempty"`
	ColumnType         string                 `json:"columnType,omitempty"`
	DataValidationRule *TableColumnValidation `json:"dataValidationRule,omitempty"`
}

// TableColumnValidation is the list a DROPDOWN column takes its values from.
type TableColumnValidation struct {
	Condition *BooleanCondition `json:"condition,omitempty"`
}

// TableRowsProperties is the banding a table draws itself with.
type TableRowsProperties struct {
	HeaderColorStyle     *ColorStyle `json:"headerColorStyle,omitempty"`
	FirstBandColorStyle  *ColorStyle `json:"firstBandColorStyle,omitempty"`
	SecondBandColorStyle *ColorStyle `json:"secondBandColorStyle,omitempty"`
	FooterColorStyle     *ColorStyle `json:"footerColorStyle,omitempty"`
}

// ColorStyle is a colour written the newer way, which the table requests take.
type ColorStyle struct {
	RGBColor *RGBColor `json:"rgbColor,omitempty"`
}

// UpdateDimensionGroupRequest folds a group up or opens it.
type UpdateDimensionGroupRequest struct {
	DimensionGroup DimensionGroup `json:"dimensionGroup"`
	Fields         string         `json:"fields"`
}

// UpdateCellsRequest writes whole cells, which is the only way to say that a cell's text
// changes style partway through.
type UpdateCellsRequest struct {
	Range  *GridRange `json:"range,omitempty"`
	Start  *GridCoord `json:"start,omitempty"`
	Rows   []RowData  `json:"rows,omitempty"`
	Fields string     `json:"fields"`
}

// GridCoord is one cell, named by numbers.
type GridCoord struct {
	SheetID     int `json:"sheetId"`
	RowIndex    int `json:"rowIndex"`
	ColumnIndex int `json:"columnIndex"`
}

// AddConditionalFormatRequest adds a rule at a position in the tab's list; the first rule
// that matches a cell wins, so the index is not decoration.
type AddConditionalFormatRequest struct {
	Rule  ConditionalFormat `json:"rule"`
	Index *int              `json:"index,omitempty"`
}

// UpdateConditionalFormatRequest replaces the rule at an index.
type UpdateConditionalFormatRequest struct {
	Rule     *ConditionalFormat `json:"rule,omitempty"`
	Index    int                `json:"index"`
	SheetID  *int               `json:"sheetId,omitempty"`
	NewIndex *int               `json:"newIndex,omitempty"`
}

// AddBandingRequest paints alternating rows or columns.
type AddBandingRequest struct {
	BandedRange BandedRange `json:"bandedRange"`
}

// UpdateBandingRequest changes a banding already there.
type UpdateBandingRequest struct {
	BandedRange BandedRange `json:"bandedRange"`
	Fields      string      `json:"fields"`
}

// SetBasicFilterRequest puts the tab's own filter on a range.
type SetBasicFilterRequest struct {
	Filter BasicFilter `json:"filter"`
}

// AddProtectedRangeRequest keeps a range from being changed by the wrong people.
type AddProtectedRangeRequest struct {
	ProtectedRange ProtectedRange `json:"protectedRange"`
}

// AddNamedRangeRequest names a range.
type AddNamedRangeRequest struct {
	NamedRange NamedRange `json:"namedRange"`
}

// AddDimensionGroupRequest folds a run of rows or columns.
type AddDimensionGroupRequest struct {
	Range DimensionRange `json:"range"`
}

// InsertDimensionRequest makes room in the middle: rows or columns appear, the rest moves
// down or right. Nothing is removed.
type InsertDimensionRequest struct {
	Range             DimensionRange `json:"range"`
	InheritFromBefore *bool          `json:"inheritFromBefore,omitempty"`
}

// AppendDimensionRequest adds rows or columns at the end of a tab.
type AppendDimensionRequest struct {
	SheetID   int    `json:"sheetId"`
	Dimension string `json:"dimension"`
	Length    int    `json:"length"`
}

// MoveDimensionRequest moves rows or columns somewhere else in the same tab.
type MoveDimensionRequest struct {
	Source           DimensionRange `json:"source"`
	DestinationIndex int            `json:"destinationIndex"`
}

// SortRangeRequest sorts a rectangle by one or more of its columns.
type SortRangeRequest struct {
	Range     GridRange  `json:"range"`
	SortSpecs []SortSpec `json:"sortSpecs"`
}

// DuplicateSheetRequest copies a whole tab inside the same workbook, with everything on it
// that no pair of read-and-write tools can carry.
type DuplicateSheetRequest struct {
	SourceSheetID    int    `json:"sourceSheetId"`
	NewSheetName     string `json:"newSheetName,omitempty"`
	InsertSheetIndex *int   `json:"insertSheetIndex,omitempty"`
}

// UpdateSpreadsheetPropsRequest changes the workbook's own settings after it exists.
type UpdateSpreadsheetPropsRequest struct {
	Properties SpreadsheetProperties `json:"properties"`
	Fields     string                `json:"fields"`
}

// SetDataValidationRequest puts a rule — a dropdown, usually — on a rectangle of cells.
type SetDataValidationRequest struct {
	Range GridRange           `json:"range"`
	Rule  *DataValidationRule `json:"rule,omitempty"`
}

// DimensionProperties is how wide a column is or how tall a row is.
type DimensionProperties struct {
	PixelSize    int   `json:"pixelSize,omitempty"`
	HiddenByUser *bool `json:"hiddenByUser,omitempty"`
}

// UpdateDimensionRequest sets column widths or row heights.
//
// Widths are in pixels here, not EMU: Sheets measures its own grid that way, and a person
// reading a column width in the interface sees the same number.
type UpdateDimensionRequest struct {
	Range      DimensionRange      `json:"range"`
	Properties DimensionProperties `json:"properties"`
	Fields     string              `json:"fields"`
}

// MergeCellsRequest joins a rectangle of cells into one.
type MergeCellsRequest struct {
	Range GridRange `json:"range"`
	// MergeType is MERGE_ALL, MERGE_COLUMNS or MERGE_ROWS.
	MergeType string `json:"mergeType"`
}

// AddSheetRequest adds a tab.
type AddSheetRequest struct {
	Properties SheetProperties `json:"properties"`
}

// GridRange is a rectangle addressed by numbers rather than A1 notation. The end indexes
// are exclusive, and an absent one means "to the edge of the sheet".
type GridRange struct {
	SheetID          int  `json:"sheetId"`
	StartRowIndex    *int `json:"startRowIndex,omitempty"`
	EndRowIndex      *int `json:"endRowIndex,omitempty"`
	StartColumnIndex *int `json:"startColumnIndex,omitempty"`
	EndColumnIndex   *int `json:"endColumnIndex,omitempty"`
}

// CellFormat is how a cell looks.
type CellFormat struct {
	NumberFormat        *NumberFormat `json:"numberFormat,omitempty"`
	BackgroundColor     *RGBColor     `json:"backgroundColor,omitempty"`
	HorizontalAlignment string        `json:"horizontalAlignment,omitempty"`
	VerticalAlignment   string        `json:"verticalAlignment,omitempty"`
	WrapStrategy        string        `json:"wrapStrategy,omitempty"`
	TextFormat          *SheetsText   `json:"textFormat,omitempty"`
	// HyperlinkDisplayType decides whether a cell that points somewhere is drawn as a
	// link or as plain text. It is a property of the cell, not of the link, and it
	// outlives the link: a cell whose link was removed keeps it.
	HyperlinkDisplayType string        `json:"hyperlinkDisplayType,omitempty"`
	TextRotation         *TextRotation `json:"textRotation,omitempty"`
	Padding              *Padding      `json:"padding,omitempty"`
	Borders              *Borders      `json:"borders,omitempty"`
}

// TextRotation turns a cell's text. An angle and "vertical" are alternatives, not a pair:
// Sheets accepts one of them.
type TextRotation struct {
	Angle    *int  `json:"angle,omitempty"`
	Vertical *bool `json:"vertical,omitempty"`
}

// Padding is the room between a cell's edges and its text, in pixels.
type Padding struct {
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
	Left   int `json:"left"`
}

// Borders is the four edges of one cell, as they are read back. Writing them goes through
// UpdateBordersRequest, which also draws the lines between cells.
type Borders struct {
	Top    *Border `json:"top,omitempty"`
	Bottom *Border `json:"bottom,omitempty"`
	Left   *Border `json:"left,omitempty"`
	Right  *Border `json:"right,omitempty"`
}

// CellLink is the address a cell points at. Slides names the same thing "url" and Sheets
// names it "uri", which is why this is its own type rather than the one in slides.go.
type CellLink struct {
	URI string `json:"uri,omitempty"`
}

// NumberFormat is how a number is displayed.
type NumberFormat struct {
	Type    string `json:"type,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

// SheetsText is the text style of a cell.
type SheetsText struct {
	FontFamily      string    `json:"fontFamily,omitempty"`
	FontSize        int       `json:"fontSize,omitempty"`
	Bold            *bool     `json:"bold,omitempty"`
	Italic          *bool     `json:"italic,omitempty"`
	Strikethrough   *bool     `json:"strikethrough,omitempty"`
	Underline       *bool     `json:"underline,omitempty"`
	ForegroundColor *RGBColor `json:"foregroundColor,omitempty"`
	// Link is a cell's own link. It lives in the text format rather than in the value, so
	// a cell can point somewhere without its text turning into a HYPERLINK formula.
	Link *CellLink `json:"link,omitempty"`
}

// CellData is the content and format of a cell.
type CellData struct {
	UserEnteredFormat *CellFormat `json:"userEnteredFormat,omitempty"`
	Note              string      `json:"note,omitempty"`
}

// RepeatCellRequest applies one cell's format across a range.
type RepeatCellRequest struct {
	Range  GridRange `json:"range"`
	Cell   CellData  `json:"cell"`
	Fields string    `json:"fields"`
}

// UpdateSheetRequest changes a tab's settings.
type UpdateSheetRequest struct {
	Properties SheetProperties `json:"properties"`
	Fields     string          `json:"fields"`
}

// DimensionRange names rows or columns.
type DimensionRange struct {
	SheetID    int    `json:"sheetId"`
	Dimension  string `json:"dimension"`
	StartIndex *int   `json:"startIndex,omitempty"`
	EndIndex   *int   `json:"endIndex,omitempty"`
}

// AutoResizeRequest fits columns to their contents.
type AutoResizeRequest struct {
	Dimensions DimensionRange `json:"dimensions"`
}

// Border is one edge of a cell.
type Border struct {
	Style string    `json:"style,omitempty"`
	Width int       `json:"width,omitempty"`
	Color *RGBColor `json:"color,omitempty"`
}

// UpdateBordersRequest draws borders around a range.
type UpdateBordersRequest struct {
	Range           GridRange `json:"range"`
	Top             *Border   `json:"top,omitempty"`
	Bottom          *Border   `json:"bottom,omitempty"`
	Left            *Border   `json:"left,omitempty"`
	Right           *Border   `json:"right,omitempty"`
	InnerHorizontal *Border   `json:"innerHorizontal,omitempty"`
	InnerVertical   *Border   `json:"innerVertical,omitempty"`
}

// SheetsBatchUpdateRequest is the body of spreadsheets.batchUpdate.
type SheetsBatchUpdateRequest struct {
	Requests []SheetsRequest `json:"requests"`
}

// SheetsBatchUpdateResponse is what came back.
type SheetsBatchUpdateResponse struct {
	SpreadsheetID string             `json:"spreadsheetId,omitempty"`
	Replies       []SheetsBatchReply `json:"replies,omitempty"`
}

// SheetsBatchReply carries the identifiers of what was created.
type SheetsBatchReply struct {
	AddSheet *struct {
		Properties SheetProperties `json:"properties"`
	} `json:"addSheet,omitempty"`
}

// SheetsBatchUpdate sends one batch of requests to a workbook.
func (c *Client) SheetsBatchUpdate(ctx context.Context, spreadsheetID string, requests []SheetsRequest) (*SheetsBatchUpdateResponse, error) {
	var out SheetsBatchUpdateResponse
	if err := c.call(ctx, http.MethodPost,
		endpoint(c.sheetsBase, "/spreadsheets/"+url.PathEscape(spreadsheetID)+":batchUpdate", nil),
		SheetsBatchUpdateRequest{Requests: requests}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// SheetIDByTitle finds a tab's numeric identifier, which is what the batch requests
// address tabs by while people address them by name.
func (s *Spreadsheet) SheetIDByTitle(title string) (int, bool) {
	for _, sheet := range s.Sheets {
		if sheet.Properties.Title == title {
			return sheet.Properties.ID(), true
		}
	}
	return 0, false
}

// ID is a tab's identifier, with the first tab's zero spelt out.
func (p SheetProperties) ID() int {
	if p.SheetID == nil {
		return 0
	}
	return *p.SheetID
}

// SheetIDOf makes an identifier that will be sent even when it is zero.
func SheetIDOf(id int) *int { return &id }

// SheetByTitle finds a whole tab, which is what a caller needs when the answer depends on
// how big the tab already is rather than only on its identifier.
func (s *Spreadsheet) SheetByTitle(title string) (*Sheet, bool) {
	for index, sheet := range s.Sheets {
		if sheet.Properties.Title == title {
			return &s.Sheets[index], true
		}
	}
	return nil, false
}

// SheetTitles lists the tabs in the order they appear.
func (s *Spreadsheet) SheetTitles() []string {
	titles := make([]string, 0, len(s.Sheets))
	for _, sheet := range s.Sheets {
		titles = append(titles, sheet.Properties.Title)
	}
	return titles
}

// A1Range builds a range name out of a tab title and an optional rectangle, quoting the
// title the way Sheets expects when it contains a space or a quote.
func A1Range(sheetTitle, cells string) string {
	if sheetTitle == "" {
		return cells
	}

	quoted := "'" + strings.ReplaceAll(sheetTitle, "'", "''") + "'"
	if cells == "" {
		return quoted
	}

	return quoted + "!" + cells
}

// ColumnLetters turns a zero-based column index into its spreadsheet letters, so an
// error can name C rather than "column 2".
func ColumnLetters(index int) string {
	if index < 0 {
		return ""
	}

	letters := ""
	for index >= 0 {
		letters = string(rune('A'+index%26)) + letters
		index = index/26 - 1
	}

	return letters
}
