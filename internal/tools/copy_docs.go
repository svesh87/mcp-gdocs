package tools

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// registerDocsCopy adds the one way content comes into a document from another one.
//
// The Docs API has no copying request of any kind — forty-odd request types and not one of
// them reaches outside the document being edited — so this reads the source and writes it
// again. What that can carry is decided by what both ends express the same way: text, the
// style of every run inside it, the paragraphs' own styling, lists, and pictures, which
// travel by address the way they do between slides.
func (r *registry) registerDocsCopy(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_docs_copy_range",
		mcp.WithDescription("Copy a stretch of another document into this one: paragraphs with their "+
			"alignment, indents and spacing, every run with its font, size, weight, colour and link, "+
			"bulleted and numbered lists, and inline pictures. Indices are the document's own, as "+
			"gdocs_docs_read_structure reports them, and they count UTF-16 code units — for Russian a "+
			"byte count is twice too large and the batch is refused. Tables inside the range are not "+
			"carried and are named in the answer instead. Read what the answer says was left behind."),
		mcp.WithString("source_document_id", mcp.Required(), mcp.Description("Document to copy from.")),
		mcp.WithNumber("start_index", mcp.Required(), mcp.Description(
			"First index of the stretch, as gdocs_docs_read_structure reports it.")),
		mcp.WithNumber("end_index", mcp.Required(), mcp.Description("One past its last index.")),
		mcp.WithString("target_document_id", mcp.Required(), mcp.Description("Document to copy into.")),
		mcp.WithNumber("target_index", mcp.Description(
			"Where to put it. Without one the copy goes at the end of the document.")),
	), r.docsCopyRange)
}

func (r *registry) docsCopyRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID, err := requiredString(req, "source_document_id")
	if err != nil {
		return toolError(err), nil
	}
	targetID, err := requiredString(req, "target_document_id")
	if err != nil {
		return toolError(err), nil
	}

	start, err := req.RequireInt("start_index")
	if err != nil {
		return toolError(err), nil
	}
	end, err := req.RequireInt("end_index")
	if err != nil {
		return toolError(err), nil
	}
	if end <= start {
		return toolError(fmt.Errorf("the range is empty: end_index is exclusive, so a single "+
			"character at index 5 is start 5 and end 6, got %d and %d", start, end)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	source, err := client.Document(ctx, sourceID)
	if err != nil {
		return toolError(err), nil
	}
	if source.Body == nil {
		return toolError(fmt.Errorf("the source document has no body to copy from")), nil
	}

	// The target is read to find where the copy lands. Without a target_index that is the
	// end of the body, and the end of the body is one index before the segment's end: Docs
	// keeps a final newline that nothing may be inserted after.
	target, err := client.Document(ctx, targetID)
	if err != nil {
		return toolError(err), nil
	}

	at := int64(req.GetInt("target_index", 0))
	if _, given := req.GetArguments()["target_index"]; !given {
		at = docsEndOfBody(target)
	}
	if at < 1 {
		return toolError(fmt.Errorf("target_index is %d: a document's own content starts at 1", at)), nil
	}

	work := &docsCopy{cursor: at}
	work.gather(source, int64(start), int64(end))

	if len(work.requests) == 0 {
		message := "nothing in that range can be carried"
		if len(work.lost) > 0 {
			message += ": " + strings.Join(work.lost, "; ")
		}
		return toolError(fmt.Errorf("%s", message)), nil
	}

	if _, err := client.DocsBatchUpdate(ctx, targetID, work.requests); err != nil {
		return toolError(err), nil
	}

	return resultJSON(withLosses(map[string]any{
		"source_document_id": sourceID,
		"start_index":        start,
		"end_index":          end,
		"target_document_id": targetID,
		"target_index":       at,
		"paragraphs":         work.paragraphs,
		"characters":         work.cursor - at,
		"requests":           len(work.requests),
	}, work.lost))
}

// docsCopy builds the batch that writes a stretch of one document into another.
//
// The cursor is the whole of the arithmetic. Text goes in at a growing index, and every
// style that follows names a range in the target's coordinates, not the source's — a style
// applied at the source's indices lands wherever those indices happen to fall in the other
// document, which is how a copy paints somebody else's paragraph bold.
type docsCopy struct {
	requests   []google.DocsRequest
	lost       []string
	cursor     int64
	paragraphs int
}

func (d *docsCopy) lose(what string) {
	for _, already := range d.lost {
		if already == what {
			return
		}
	}
	d.lost = append(d.lost, what)
}

// gather walks the source's body and turns everything inside the range into requests.
func (d *docsCopy) gather(source *google.Document, start, end int64) {
	for index := range source.Body.Content {
		element := &source.Body.Content[index]
		if !docsTouches(element, start, end) {
			continue
		}

		switch {
		case element.Paragraph != nil:
			d.paragraph(source, element.Paragraph, start, end)
		case element.Table != nil:
			// A table cannot be built in the same batch as the text around it: its cells
			// only get indices once it exists, and those indices are not predictable from
			// the request that made it. Carrying it half-way would put text in the wrong
			// cells, which is worse than not carrying it.
			d.lose(fmt.Sprintf("a table of %d by %d, which has to be made separately with "+
				"gdocs_docs_insert_table and filled with gdocs_docs_edit_table",
				element.Table.Rows, element.Table.Columns))
		case element.SectionBreak != nil:
			d.lose("a section break, which carries page setup and its own headers: make it " +
				"with gdocs_docs_insert_section_break and style it with gdocs_docs_style_section")
		}
	}
}

// paragraph writes one paragraph: its text, the style of every run, its own styling and its
// bullet.
func (d *docsCopy) paragraph(source *google.Document, paragraph *google.DocsParagraph, start, end int64) {
	began := d.cursor
	wrote := false

	for index := range paragraph.Elements {
		item := &paragraph.Elements[index]

		switch {
		case item.TextRun != nil:
			text := docsClip(item, item.TextRun.Content, start, end)
			if text == "" {
				continue
			}

			length := utf16Length(text)
			d.requests = append(d.requests, google.DocsRequest{
				InsertText: &google.DocsInsertText{
					Location: &google.DocsLocation{Index: d.cursor},
					Text:     text,
				},
			})

			if style := item.TextRun.Style; style != nil {
				if fields := docsTextStyleFields(style); fields != "" {
					d.requests = append(d.requests, google.DocsRequest{
						UpdateTextStyle: &google.DocsUpdateTextStyle{
							Range:  google.DocsRange{StartIndex: d.cursor, EndIndex: d.cursor + length},
							Style:  style,
							Fields: fields,
						},
					})
				}
			}

			d.cursor += length
			wrote = true

		case item.InlineObject != nil:
			d.image(source, item.InlineObject.InlineObjectID)
			wrote = true

		case item.Person != nil:
			d.lose("a person chip, which is written with gdocs_docs_insert_chip")
		case item.RichLink != nil:
			d.lose("a link chip to another file, which is written with gdocs_docs_insert_chip")
		case item.PageBreak != nil:
			d.requests = append(d.requests, google.DocsRequest{
				InsertPageBreak: &google.DocsInsertPageBreak{
					Location: &google.DocsLocation{Index: d.cursor},
				},
			})
			d.cursor++
			wrote = true
		case item.HorizontalRule != nil:
			d.lose("a horizontal rule, which the API has no request for at all")
		}
	}

	if !wrote {
		return
	}

	d.paragraphs++

	if style := paragraph.Style; style != nil {
		if fields := docsParagraphFields(style); fields != "" {
			d.requests = append(d.requests, google.DocsRequest{
				UpdateParagraph: &google.DocsUpdateParagraph{
					Range:  google.DocsRange{StartIndex: began, EndIndex: d.cursor},
					Style:  style,
					Fields: fields,
				},
			})
		}
	}

	// A list is made from the text rather than typed. Slides and Docs agree on this and on
	// the reason: a marker placed by hand is a character in the paragraph, which reflows,
	// sorts and copies as text and never behaves as a list.
	if paragraph.Bullet != nil {
		d.requests = append(d.requests, google.DocsRequest{
			CreateBullets: &google.DocsCreateBullets{
				Range:        google.DocsRange{StartIndex: began, EndIndex: d.cursor},
				BulletPreset: "BULLET_DISC_CIRCLE_SQUARE",
			},
		})
	}
}

// image carries a picture across by its address.
//
// Google fetches the address and keeps its own copy of the bytes, so nothing is downloaded
// or uploaded. The address is short-lived and tagged with the account that read it, which
// is why this has to happen in the same pass as the reading.
func (d *docsCopy) image(source *google.Document, objectID string) {
	object, ok := source.InlineObjects[objectID]
	if !ok || object.Properties == nil || object.Properties.EmbeddedObject == nil {
		d.lose("a picture the source document does not describe, most likely a drawing, " +
			"which the API does not report at all")
		return
	}

	embedded := object.Properties.EmbeddedObject
	if embedded.ImageProperties == nil || embedded.ImageProperties.ContentURI == "" {
		d.lose("a drawing made in the editor: the API reports it without any address, " +
			"so there is nothing to fetch and nothing to insert")
		return
	}

	insert := &google.DocsInsertInlineImage{
		URI:      embedded.ImageProperties.ContentURI,
		Location: &google.DocsLocation{Index: d.cursor},
	}
	if embedded.Size != nil {
		insert.Size = embedded.Size
	}

	d.requests = append(d.requests, google.DocsRequest{InsertInlineImage: insert})
	// A picture occupies exactly one index however large it is drawn.
	d.cursor++
}

// docsTouches says whether a structural element has any part inside the range.
func docsTouches(element *google.StructuralElement, start, end int64) bool {
	if element.StartIndex == nil || element.EndIndex == nil {
		// The very first paragraph of a body has no start index, which means zero.
		return start == 0
	}

	return *element.StartIndex < end && *element.EndIndex > start
}

// docsClip cuts a run down to the part inside the range.
func docsClip(item *google.DocsParagraphElement, text string, start, end int64) string {
	if item.StartIndex == nil || item.EndIndex == nil {
		return text
	}

	from, to := *item.StartIndex, *item.EndIndex
	if from >= start && to <= end {
		return text
	}

	first, last := int64(0), to-from
	if start > from {
		first = start - from
	}
	if end < to {
		last = end - from
	}

	return utf16Slice(text, first, last)
}

// docsTextStyleFields is the mask for a run's style read off a sample: exactly the fields
// that were set, so the copy takes the sample's decisions and inherits the rest.
func docsTextStyleFields(style *google.DocsTextStyle) string {
	var fields []string

	for _, field := range []struct {
		name  string
		isSet bool
	}{
		{"bold", style.Bold != nil},
		{"italic", style.Italic != nil},
		{"underline", style.Underline != nil},
		{"strikethrough", style.Strikethrough != nil},
		{"smallCaps", style.SmallCaps != nil},
		{"baselineOffset", style.BaselineOffset != ""},
		{"fontSize", style.FontSize != nil},
		{"weightedFontFamily", style.WeightedFont != nil},
		{"foregroundColor", style.ForegroundColor != nil},
		{"backgroundColor", style.BackgroundColor != nil},
		{"link", style.Link != nil},
	} {
		if field.isSet {
			fields = append(fields, field.name)
		}
	}

	return strings.Join(fields, ",")
}

// docsParagraphFields is the same for a paragraph.
//
// The named style comes first and matters most: a heading copied without namedStyleType is
// a paragraph in a large font, which looks right and is absent from the outline.
func docsParagraphFields(style *google.DocsParagraphStyle) string {
	var fields []string

	for _, field := range []struct {
		name  string
		isSet bool
	}{
		{"namedStyleType", style.NamedStyleType != ""},
		{"alignment", style.Alignment != ""},
		{"lineSpacing", style.LineSpacing != nil},
		{"direction", style.Direction != ""},
		{"spacingMode", style.SpacingMode != ""},
		{"spaceAbove", style.SpaceAbove != nil},
		{"spaceBelow", style.SpaceBelow != nil},
		{"borderBetween", style.BorderBetween != nil},
		{"borderTop", style.BorderTop != nil},
		{"borderBottom", style.BorderBottom != nil},
		{"borderLeft", style.BorderLeft != nil},
		{"borderRight", style.BorderRight != nil},
		{"indentFirstLine", style.IndentFirstLine != nil},
		{"indentStart", style.IndentStart != nil},
		{"indentEnd", style.IndentEnd != nil},
		{"keepLinesTogether", style.KeepLinesTogether != nil},
		{"keepWithNext", style.KeepWithNext != nil},
		{"avoidWidowAndOrphan", style.AvoidWidowAndOrphan != nil},
		{"shading", style.Shading != nil},
		{"pageBreakBefore", style.PageBreakBefore != nil},
		{"tabStops", len(style.TabStops) > 0},
	} {
		if field.isSet {
			fields = append(fields, field.name)
		}
	}

	return strings.Join(fields, ",")
}

// utf16Slice cuts text by the units the API counts in.
//
// Slicing by bytes or by runes is the same mistake in two forms: for Russian a byte offset
// is twice too large and a rune offset too small, and either one cuts a range that does not
// line up with the indices the document reported.
func utf16Slice(text string, from, to int64) string {
	units := utf16.Encode([]rune(text))
	if from < 0 {
		from = 0
	}
	if to > int64(len(units)) {
		to = int64(len(units))
	}
	if from >= to {
		return ""
	}

	return string(utf16.Decode(units[from:to]))
}

// docsEndOfBody is the last index a document will accept an insertion at.
//
// Docs keeps a newline at the very end that nothing may be inserted after, so the answer is
// one before the body's end index rather than the end index itself.
func docsEndOfBody(document *google.Document) int64 {
	if document.Body == nil || len(document.Body.Content) == 0 {
		return 1
	}

	last := document.Body.Content[len(document.Body.Content)-1]
	if last.EndIndex == nil || *last.EndIndex < 2 {
		return 1
	}

	return *last.EndIndex - 1
}
