package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerDocsRead adds the reading half of "make one like this" for documents.
//
// gdocs_docs_read answers with text, which is what a person wants and what a template
// filler needs. It is not enough to rebuild a document: a document is decided by the
// paragraph styles, the runs inside each paragraph, the table's cells and the page setup,
// and none of that is in the text. Everything reported here is written back by a tool
// that takes the same numbers in the same units.
func (r *registry) registerDocsRead(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_docs_read_structure",
		mcp.WithDescription("Read a document the way it is built: every paragraph with its style, indents, "+
			"spacing, borders and shading, every run of text with its own font, size, colour and link, "+
			"the list a paragraph belongs to, tables with their column widths, row heights and cell "+
			"styles, the section breaks with their page setup, the headers and footers, the named "+
			"styles, and the pictures. Positions come as the document's own character indexes, and "+
			"every size is in points, which is the only unit Docs uses. This is what to read off a "+
			"sample before building one like it. Big documents make big answers: narrow it with "+
			"start_index and end_index when only part is being rebuilt."),
		mcp.WithString("document_id", mcp.Required(), mcp.Description(documentIDHelp)),
		mcp.WithNumber("start_index", mcp.Description(
			"Report only body elements ending after this index. Omit for the whole document.")),
		mcp.WithNumber("end_index", mcp.Description(
			"Report only body elements starting before this index.")),
		mcp.WithBoolean("include_text", mcp.DefaultBool(true), mcp.Description(
			"Carry the text of each run. Off gives the shape alone, which is much smaller.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.docsReadStructure)
}

func (r *registry) docsReadStructure(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentID, err := requiredString(req, "document_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	document, err := client.Document(ctx, documentID)
	if err != nil {
		return toolError(err), nil
	}

	view := docsView{
		text:  req.GetBool("include_text", true),
		start: int64(req.GetInt("start_index", 0)),
		end:   int64(req.GetInt("end_index", 0)),
	}

	payload := map[string]any{
		"document_id": document.DocumentID,
		"title":       document.Title,
		"revision_id": document.RevisionID,
	}

	if document.Body != nil {
		payload["body"] = view.elements(document.Body.Content)
	}
	if style := docsDocumentStyleInfo(document.DocumentStyle); style != nil {
		payload["document_style"] = style
	}
	if named := docsNamedStylesInfo(document.NamedStyles); len(named) > 0 {
		payload["named_styles"] = named
	}
	if lists := docsListsInfo(document.Lists); len(lists) > 0 {
		payload["lists"] = lists
	}

	// Headers and footers are content like the body, addressed by an identifier rather
	// than by an index. Writing into one means passing that identifier as segment_id.
	whole := docsView{text: view.text}
	if segments := whole.segments(document.Headers); len(segments) > 0 {
		payload["headers"] = segments
	}
	if segments := whole.segments(document.Footers); len(segments) > 0 {
		payload["footers"] = segments
	}
	if segments := whole.segments(document.Footnotes); len(segments) > 0 {
		payload["footnotes"] = segments
	}

	if objects := docsInlineObjectsInfo(document.InlineObjects); len(objects) > 0 {
		payload["inline_objects"] = objects
	}
	if objects := docsPositionedInfo(document.PositionedObjects); len(objects) > 0 {
		payload["positioned_objects"] = objects
	}

	return resultJSON(payload)
}

// docsView is how much of a document a reading reports.
type docsView struct {
	text  bool
	start int64
	end   int64
}

// wanted says whether an element falls in the window a caller asked for.
func (v docsView) wanted(element google.StructuralElement) bool {
	if v.end > 0 && element.StartIndex != nil && *element.StartIndex >= v.end {
		return false
	}
	if v.start > 0 && element.EndIndex != nil && *element.EndIndex <= v.start {
		return false
	}

	return true
}

func (v docsView) segments(segments map[string]google.DocsSegment) map[string]any {
	if len(segments) == 0 {
		return nil
	}

	out := make(map[string]any, len(segments))
	for id, segment := range segments {
		out[id] = v.elements(segment.Content)
	}

	return out
}

// elements describes one run of structural elements: the body, a cell's content, a header.
func (v docsView) elements(elements []google.StructuralElement) []map[string]any {
	out := make([]map[string]any, 0, len(elements))

	for _, element := range elements {
		if !v.wanted(element) {
			continue
		}

		described := map[string]any{}
		putIndex(described, "start", element.StartIndex)
		putIndex(described, "end", element.EndIndex)

		switch {
		case element.Paragraph != nil:
			described["kind"] = "paragraph"
			v.describeParagraph(described, element.Paragraph)
		case element.Table != nil:
			described["kind"] = "table"
			v.describeTable(described, element.Table)
		case element.SectionBreak != nil:
			described["kind"] = "section_break"
			if style := docsSectionStyleInfo(element.SectionBreak.Style); style != nil {
				described["style"] = style
			}
		default:
			described["kind"] = "unknown"
		}

		out = append(out, described)
	}

	return out
}

func (v docsView) describeParagraph(into map[string]any, paragraph *google.DocsParagraph) {
	if style := docsParagraphStyleInfo(paragraph.Style); style != nil {
		into["style"] = style
	}

	// A bullet is not a property of the text: it says the paragraph belongs to a list,
	// and the glyph comes from that list's level. Rebuilding one means making bullets
	// over the paragraph, not writing a "•" into it.
	if paragraph.Bullet != nil {
		bullet := map[string]any{"list_id": paragraph.Bullet.ListID}
		if paragraph.Bullet.NestingLevel != nil {
			bullet["nesting_level"] = *paragraph.Bullet.NestingLevel
		} else {
			bullet["nesting_level"] = 0
		}
		into["bullet"] = bullet
	}

	if len(paragraph.PositionedObjectIDs) > 0 {
		into["positioned_object_ids"] = paragraph.PositionedObjectIDs
	}

	runs := make([]map[string]any, 0, len(paragraph.Elements))
	for _, piece := range paragraph.Elements {
		described := map[string]any{}
		putIndex(described, "start", piece.StartIndex)
		putIndex(described, "end", piece.EndIndex)

		switch {
		case piece.TextRun != nil:
			described["kind"] = "text"
			if v.text {
				described["text"] = piece.TextRun.Content
			}
			described["characters"] = utf16Length(piece.TextRun.Content)
			if style := docsTextStyleInfo(piece.TextRun.Style); style != nil {
				described["style"] = style
			}
		case piece.InlineObject != nil:
			described["kind"] = "inline_object"
			described["inline_object_id"] = piece.InlineObject.InlineObjectID
		case piece.PageBreak != nil:
			described["kind"] = "page_break"
		case piece.ColumnBreak != nil:
			described["kind"] = "column_break"
		case piece.HorizontalRule != nil:
			described["kind"] = "horizontal_rule"
		case piece.Person != nil:
			described["kind"] = "person"
			if piece.Person.Properties != nil {
				described["name"] = piece.Person.Properties.Name
			}
		case piece.RichLink != nil:
			described["kind"] = "rich_link"
			if piece.RichLink.Properties != nil {
				described["title"] = piece.RichLink.Properties.Title
				described["uri"] = piece.RichLink.Properties.URI
			}
		default:
			described["kind"] = "unknown"
		}

		runs = append(runs, described)
	}

	if len(runs) > 0 {
		into["runs"] = runs
	}
}

func (v docsView) describeTable(into map[string]any, table *google.DocsTable) {
	into["rows"] = table.Rows
	into["columns"] = table.Columns

	if table.Style != nil && len(table.Style.ColumnProperties) > 0 {
		columns := make([]map[string]any, 0, len(table.Style.ColumnProperties))
		for _, column := range table.Style.ColumnProperties {
			described := map[string]any{}
			if column.WidthType != "" {
				described["width_type"] = column.WidthType
			}
			putPoints(described, "width_pt", column.Width)
			columns = append(columns, described)
		}
		into["column_properties"] = columns
	}

	rows := make([]map[string]any, 0, len(table.Content))
	for _, row := range table.Content {
		described := map[string]any{}
		putIndex(described, "start", row.StartIndex)

		if row.Style != nil {
			style := map[string]any{}
			putPoints(style, "min_height_pt", row.Style.MinRowHeight)
			putBool(style, "table_header", row.Style.TableHeader)
			putBool(style, "prevent_overflow", row.Style.PreventOverflow)
			if len(style) > 0 {
				described["style"] = style
			}
		}

		cells := make([]map[string]any, 0, len(row.Cells))
		for _, cell := range row.Cells {
			cellInfo := map[string]any{}
			putIndex(cellInfo, "start", cell.StartIndex)
			putIndex(cellInfo, "end", cell.EndIndex)
			if style := docsCellStyleInfo(cell.Style); style != nil {
				cellInfo["style"] = style
			}
			cellInfo["content"] = v.elements(cell.Content)
			cells = append(cells, cellInfo)
		}
		described["cells"] = cells

		rows = append(rows, described)
	}

	into["row_data"] = rows
}

func docsTextStyleInfo(style *google.DocsTextStyle) map[string]any {
	if style == nil {
		return nil
	}

	out := map[string]any{}
	putBool(out, "bold", style.Bold)
	putBool(out, "italic", style.Italic)
	putBool(out, "underline", style.Underline)
	putBool(out, "strikethrough", style.Strikethrough)
	putBool(out, "small_caps", style.SmallCaps)
	if style.BaselineOffset != "" {
		out["baseline_offset"] = style.BaselineOffset
	}
	putPoints(out, "font_size_pt", style.FontSize)
	if style.WeightedFont != nil {
		out["font_family"] = style.WeightedFont.FontFamily
		if style.WeightedFont.Weight != 0 {
			out["font_weight"] = style.WeightedFont.Weight
		}
	}
	putDocsColor(out, "color", style.ForegroundColor)
	putDocsColor(out, "background_color", style.BackgroundColor)
	if style.Link != nil {
		out["link"] = style.Link.URL
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func docsParagraphStyleInfo(style *google.DocsParagraphStyle) map[string]any {
	if style == nil {
		return nil
	}

	out := map[string]any{}
	if style.NamedStyleType != "" {
		out["named_style"] = style.NamedStyleType
	}
	if style.Alignment != "" {
		out["alignment"] = style.Alignment
	}
	if style.Direction != "" {
		out["direction"] = style.Direction
	}
	if style.SpacingMode != "" {
		out["spacing_mode"] = style.SpacingMode
	}
	if style.HeadingID != "" {
		out["heading_id"] = style.HeadingID
	}
	if style.LineSpacing != nil {
		out["line_spacing"] = *style.LineSpacing
	}
	putPoints(out, "space_above_pt", style.SpaceAbove)
	putPoints(out, "space_below_pt", style.SpaceBelow)
	putPoints(out, "indent_start_pt", style.IndentStart)
	putPoints(out, "indent_end_pt", style.IndentEnd)
	putPoints(out, "indent_first_line_pt", style.IndentFirstLine)
	putBool(out, "keep_lines_together", style.KeepLinesTogether)
	putBool(out, "keep_with_next", style.KeepWithNext)
	putBool(out, "avoid_widow_and_orphan", style.AvoidWidowAndOrphan)
	putBool(out, "page_break_before", style.PageBreakBefore)

	if style.Shading != nil {
		putDocsColor(out, "shading_color", style.Shading.BackgroundColor)
	}

	for name, border := range map[string]*google.DocsParagraphBorder{
		"border_top":     style.BorderTop,
		"border_bottom":  style.BorderBottom,
		"border_left":    style.BorderLeft,
		"border_right":   style.BorderRight,
		"border_between": style.BorderBetween,
	} {
		if described := docsParagraphBorderInfo(border); described != nil {
			out[name] = described
		}
	}

	if len(style.TabStops) > 0 {
		stops := make([]map[string]any, 0, len(style.TabStops))
		for _, stop := range style.TabStops {
			described := map[string]any{}
			putPoints(described, "offset_pt", stop.Offset)
			if stop.Alignment != "" {
				described["alignment"] = stop.Alignment
			}
			stops = append(stops, described)
		}
		out["tab_stops"] = stops
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func docsParagraphBorderInfo(border *google.DocsParagraphBorder) map[string]any {
	if border == nil {
		return nil
	}

	out := map[string]any{}
	putDocsColor(out, "color", border.Color)
	putPoints(out, "width_pt", border.Width)
	putPoints(out, "padding_pt", border.Padding)
	if border.DashStyle != "" {
		out["dash_style"] = border.DashStyle
	}

	// A border with a zero width is a border that is not drawn, and it comes back as an
	// object with a dash style and nothing else. Reporting it keeps a copy from inventing
	// a line where the sample has none.
	if len(out) == 0 {
		return nil
	}

	return out
}

func docsCellStyleInfo(style *google.DocsTableCellStyle) map[string]any {
	if style == nil {
		return nil
	}

	out := map[string]any{}
	if style.RowSpan != 0 {
		out["row_span"] = style.RowSpan
	}
	if style.ColumnSpan != 0 {
		out["column_span"] = style.ColumnSpan
	}
	if style.ContentAlignment != "" {
		out["content_alignment"] = style.ContentAlignment
	}
	putDocsColor(out, "background_color", style.BackgroundColor)
	putPoints(out, "padding_top_pt", style.PaddingTop)
	putPoints(out, "padding_bottom_pt", style.PaddingBottom)
	putPoints(out, "padding_left_pt", style.PaddingLeft)
	putPoints(out, "padding_right_pt", style.PaddingRight)

	for name, border := range map[string]*google.DocsTableCellBorder{
		"border_top":    style.BorderTop,
		"border_bottom": style.BorderBottom,
		"border_left":   style.BorderLeft,
		"border_right":  style.BorderRight,
	} {
		if border == nil {
			continue
		}
		described := map[string]any{}
		putDocsColor(described, "color", border.Color)
		putPoints(described, "width_pt", border.Width)
		if border.DashStyle != "" {
			described["dash_style"] = border.DashStyle
		}
		if len(described) > 0 {
			out[name] = described
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func docsSectionStyleInfo(style *google.DocsSectionStyle) map[string]any {
	if style == nil {
		return nil
	}

	out := map[string]any{}
	if style.SectionType != "" {
		out["section_type"] = style.SectionType
	}
	if style.ColumnSeparatorStyle != "" {
		out["column_separator"] = style.ColumnSeparatorStyle
	}
	if style.ContentDirection != "" {
		out["direction"] = style.ContentDirection
	}
	putPoints(out, "margin_top_pt", style.MarginTop)
	putPoints(out, "margin_bottom_pt", style.MarginBottom)
	putPoints(out, "margin_left_pt", style.MarginLeft)
	putPoints(out, "margin_right_pt", style.MarginRight)
	putPoints(out, "margin_header_pt", style.MarginHeader)
	putPoints(out, "margin_footer_pt", style.MarginFooter)
	putString(out, "default_header_id", style.DefaultHeaderID)
	putString(out, "default_footer_id", style.DefaultFooterID)
	putString(out, "first_page_header_id", style.FirstPageHeaderID)
	putString(out, "first_page_footer_id", style.FirstPageFooterID)
	putString(out, "even_page_header_id", style.EvenPageHeaderID)
	putString(out, "even_page_footer_id", style.EvenPageFooterID)
	putBool(out, "use_first_page_header_footer", style.UseFirstPageHeaderFoot)
	putBool(out, "flip_page_orientation", style.FlipPageOrientation)
	if style.PageNumberStart != nil {
		out["page_number_start"] = *style.PageNumberStart
	}

	if len(style.ColumnProperties) > 0 {
		columns := make([]map[string]any, 0, len(style.ColumnProperties))
		for _, column := range style.ColumnProperties {
			described := map[string]any{}
			putPoints(described, "width_pt", column.Width)
			putPoints(described, "padding_end_pt", column.PaddingEnd)
			columns = append(columns, described)
		}
		out["columns"] = columns
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func docsDocumentStyleInfo(style *google.DocsDocumentStyle) map[string]any {
	if style == nil {
		return nil
	}

	out := map[string]any{}
	if style.PageSize != nil {
		size := map[string]any{}
		putPoints(size, "width_pt", style.PageSize.Width)
		putPoints(size, "height_pt", style.PageSize.Height)
		out["page_size"] = size
	}
	if style.Background != nil {
		putDocsColor(out, "background_color", style.Background.Color)
	}
	putPoints(out, "margin_top_pt", style.MarginTop)
	putPoints(out, "margin_bottom_pt", style.MarginBottom)
	putPoints(out, "margin_left_pt", style.MarginLeft)
	putPoints(out, "margin_right_pt", style.MarginRight)
	putPoints(out, "margin_header_pt", style.MarginHeader)
	putPoints(out, "margin_footer_pt", style.MarginFooter)
	putBool(out, "use_custom_header_footer_margins", style.UseCustomHeaderFooter)
	putBool(out, "use_first_page_header_footer", style.UseFirstPageHeaderFoot)
	putBool(out, "use_even_page_header_footer", style.UseEvenPageHeaderFoot)
	putBool(out, "flip_page_orientation", style.FlipPageOrientation)
	putString(out, "default_header_id", style.DefaultHeaderID)
	putString(out, "default_footer_id", style.DefaultFooterID)
	putString(out, "first_page_header_id", style.FirstPageHeaderID)
	putString(out, "first_page_footer_id", style.FirstPageFooterID)
	putString(out, "even_page_header_id", style.EvenPageHeaderID)
	putString(out, "even_page_footer_id", style.EvenPageFooterID)
	if style.PageNumberStart != nil {
		out["page_number_start"] = *style.PageNumberStart
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func docsNamedStylesInfo(styles *google.DocsNamedStyles) []map[string]any {
	if styles == nil {
		return nil
	}

	out := make([]map[string]any, 0, len(styles.Styles))
	for _, style := range styles.Styles {
		described := map[string]any{"named_style": style.NamedStyleType}
		if text := docsTextStyleInfo(style.TextStyle); text != nil {
			described["text_style"] = text
		}
		if paragraph := docsParagraphStyleInfo(style.ParagraphStyle); paragraph != nil {
			described["paragraph_style"] = paragraph
		}
		out = append(out, described)
	}

	return out
}

func docsListsInfo(lists map[string]google.DocsList) map[string]any {
	if len(lists) == 0 {
		return nil
	}

	out := make(map[string]any, len(lists))
	for id, list := range lists {
		if list.Properties == nil {
			continue
		}
		levels := make([]map[string]any, 0, len(list.Properties.NestingLevels))
		for _, level := range list.Properties.NestingLevels {
			described := map[string]any{}
			putString(described, "glyph_symbol", level.GlyphSymbol)
			putString(described, "glyph_type", level.GlyphType)
			putString(described, "glyph_format", level.GlyphFormat)
			putString(described, "bullet_alignment", level.BulletAlignment)
			putPoints(described, "indent_start_pt", level.IndentStart)
			putPoints(described, "indent_first_line_pt", level.IndentFirstLine)
			if level.StartNumber != nil {
				described["start_number"] = *level.StartNumber
			}
			levels = append(levels, described)
		}
		out[id] = map[string]any{"levels": levels}
	}

	return out
}

func docsInlineObjectsInfo(objects map[string]google.DocsInlineObject) map[string]any {
	if len(objects) == 0 {
		return nil
	}

	out := make(map[string]any, len(objects))
	for id, object := range objects {
		if object.Properties == nil {
			continue
		}
		out[id] = docsEmbeddedInfo(object.Properties.EmbeddedObject)
	}

	return out
}

func docsPositionedInfo(objects map[string]google.DocsPositioned) map[string]any {
	if len(objects) == 0 {
		return nil
	}

	out := make(map[string]any, len(objects))
	for id, object := range objects {
		if object.Properties == nil {
			continue
		}
		described := docsEmbeddedInfo(object.Properties.EmbeddedObject)
		if object.Properties.Positioning != nil {
			position := map[string]any{}
			putString(position, "layout", object.Properties.Positioning.Layout)
			putPoints(position, "left_offset_pt", object.Properties.Positioning.LeftOffset)
			putPoints(position, "top_offset_pt", object.Properties.Positioning.TopOffset)
			described["positioning"] = position
		}

		// Said on every one of them, because this is the field a rebuild trips over: the
		// whole of Docs v1 can delete a floating object and cannot make one.
		described["writable"] = false
		described["why"] = "Docs API v1 has no request that creates a positioned object; " +
			"a rebuilt document can only carry this picture in the line of text"

		out[id] = described
	}

	return out
}

func docsEmbeddedInfo(object *google.DocsEmbeddedObject) map[string]any {
	if object == nil {
		return map[string]any{}
	}

	out := map[string]any{}
	putString(out, "title", object.Title)
	putString(out, "description", object.Description)
	if object.Size != nil {
		size := map[string]any{}
		putPoints(size, "width_pt", object.Size.Width)
		putPoints(size, "height_pt", object.Size.Height)
		out["size"] = size
	}
	putPoints(out, "margin_top_pt", object.MarginTop)
	putPoints(out, "margin_bottom_pt", object.MarginBottom)
	putPoints(out, "margin_left_pt", object.MarginLeft)
	putPoints(out, "margin_right_pt", object.MarginRight)

	switch {
	case object.ImageProperties != nil:
		out["kind"] = "image"
		// The content address is what a rebuild feeds back to insert_image. It is a
		// signed link that expires, so reading and inserting belong in one go.
		putString(out, "content_uri", object.ImageProperties.ContentURI)
		putString(out, "source_uri", object.ImageProperties.SourceURI)
	case object.DrawingProperties != nil:
		out["kind"] = "drawing"
		// Not an omission in this server: embeddedDrawingProperties arrives empty from
		// Google, and there is no request that makes a drawing either.
		out["writable"] = false
		out["why"] = "Docs reports a drawing as an empty embeddedDrawingProperties and has no " +
			"request that creates one; neither its content nor its shape can be read or rebuilt"
	default:
		out["kind"] = "unknown"
	}

	return out
}

// putPoints records a size the way Docs states it. Docs has exactly one unit, PT, so a
// number of points is the whole of it — unlike Slides, where the same field arrives in
// EMU or in points depending on what set it.
func putPoints(into map[string]any, name string, value *google.Dimension) {
	if value == nil {
		return
	}

	into[name] = value.Magnitude
}

func putBool(into map[string]any, name string, value *bool) {
	if value == nil {
		return
	}

	into[name] = *value
}

func putString(into map[string]any, name, value string) {
	if value == "" {
		return
	}

	into[name] = value
}

// putIndex reports a position, and reports a missing one as zero rather than leaving it
// out. Google omits an index of zero, and zero is a real place: the first element of the
// body and the first character of every header and footer. A reading that drops it hands
// back an element nothing can be written against.
func putIndex(into map[string]any, name string, value *int64) {
	if value == nil {
		into[name] = int64(0)
		return
	}

	into[name] = *value
}

// putDocsColor reports a colour with its three states kept apart, because they are three
// different instructions to a rebuild: the field absent means "inherited", an object with
// no colour in it means "none" — which is how a transparent page background comes back —
// and an rgbColor with every component left out is black, not empty.
func putDocsColor(into map[string]any, name string, colour *google.DocsColor) {
	if colour == nil {
		return
	}
	if colour.Color == nil || colour.Color.RGBColor == nil {
		into[name] = "none"
		return
	}

	into[name] = describeColor(colour.Color.RGBColor)
}
