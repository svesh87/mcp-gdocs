package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// copyMask reads a slide with everything needed to build it again somewhere else.
//
// It is wider than any of the other masks because a rebuild has no second chance: a field
// left out is a field the copy will not have, and the difference only shows up on the
// finished slide. Text is read run by run with its style and its paragraph's style, because
// a block of text whose second sentence is bold is one shape, not two.
const copyMask = "presentationId,title,pageSize," +
	"layouts(objectId,layoutProperties(name,displayName)," +
	"pageElements(objectId,shape(placeholder(type,index))))," +
	"slides(objectId,slideProperties(layoutObjectId,isSkipped," +
	"notesPage(notesProperties(speakerNotesObjectId)," +
	"pageElements(objectId,shape(placeholder(type),text(textElements(textRun(content)))))))," +
	"pageElements(objectId,title,description,size,transform," +
	"shape(shapeType,placeholder(type),shapeProperties(shapeBackgroundFill,outline,contentAlignment)," +
	"text(textElements(startIndex,endIndex,paragraphMarker(bullet(nestingLevel,glyph),style)," +
	"textRun(content,style(" + google.TextStyleFields + ",link)))))," +
	"table(rows,columns,tableColumns(columnWidth),tableRows(rowHeight," +
	"tableCells(location,rowSpan,columnSpan,tableCellProperties(tableCellBackgroundFill,contentAlignment)," +
	"text(textElements(paragraphMarker(style(alignment)),textRun(content,style(" +
	google.TextStyleFields + ")))))))," +
	"image(contentUrl,imageProperties(cropProperties,transparency,brightness,contrast,outline))," +
	"video(url,source,id)," +
	"line(lineType,lineCategory,lineProperties(lineFill,weight,dashStyle,startArrow,endArrow))," +
	"sheetsChart(spreadsheetId,chartId)," +
	"elementGroup(children(objectId))))"

// registerSlidesCopy adds the two ways a slide's content comes in from another deck.
//
// Slides has no request for this at all — duplicateObject works inside one presentation and
// nowhere else — so both tools read the source and build it again in the target. What that
// means in practice is worth saying plainly: the content is reproduced, the theme is not.
// A slide carried into a deck on a different theme comes out in that deck's fonts and
// colours wherever the sample left them to the layout, which is most of the time. Starting
// the target as a copy of the sample deck is the only way to carry a look.
func (r *registry) registerSlidesCopy(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_slides_copy_slide",
		mcp.WithDescription("Copy a slide from another presentation and build it again in the target: "+
			"shapes with their text, per-run styling, bullets, fills, outlines and geometry; tables "+
			"with their cells and column widths; pictures; lines; charts linked to a workbook; speaker "+
			"notes. Where the target deck's layout has the same slot as the sample's, the text goes "+
			"into that slot and inherits its look. The theme does not travel — Slides has no request "+
			"that applies one deck's theme to another — so anything the sample left to its layout comes "+
			"out in the target's own fonts and colours. The answer names everything that could not be "+
			"carried; read it, because a slide reported as copied with two omissions is not the same "+
			"slide. Naming the same presentation as source and target is allowed and does something "+
			"different: Google duplicates the slide, which is exact and loses nothing at all."),
		mcp.WithString("source_presentation_id", mcp.Required(), mcp.Description("Deck to copy from.")),
		mcp.WithString("source_page_object_id", mcp.Required(), mcp.Description("Slide to copy.")),
		mcp.WithString("target_presentation_id", mcp.Required(), mcp.Description("Deck to copy into.")),
		mcp.WithNumber("insert_at", mcp.Description(
			"Position for the new slide, counted from 0. Without it the slide goes last.")),
		mcp.WithString("layout_name", mcp.Description(
			"Layout the new slide follows, by name as gdocs_slides_list_layouts reports it. Without one "+
				"the target's layout of the same name as the sample's is used, and BLANK if there is none.")),
		mcp.WithBoolean("copy_speaker_notes", mcp.DefaultBool(true), mcp.Description(
			"Bring the speaker notes across as well.")),
	), r.slidesCopySlide)

	srv.AddTool(mcp.NewTool("gdocs_slides_copy_element",
		mcp.WithDescription("Copy one element — a shape with its text, a table, a picture, a line, a "+
			"chart — onto a slide of another presentation, or of this one, keeping its size and its "+
			"place. Within one deck gdocs_slides_duplicate is cheaper and exact; this exists for "+
			"crossing between decks, which no request in the Slides API does. The answer names what "+
			"could not be carried."),
		mcp.WithString("source_presentation_id", mcp.Required(), mcp.Description("Deck to copy from.")),
		mcp.WithString("source_object_id", mcp.Required(), mcp.Description("Element to copy.")),
		mcp.WithString("target_presentation_id", mcp.Required(), mcp.Description("Deck to copy into.")),
		mcp.WithString("target_page_object_id", mcp.Required(), mcp.Description("Slide to put it on.")),
		mcp.WithNumber("x_emu", mcp.Description("Where to put it, in EMU. Without it the sample's own place.")),
		mcp.WithNumber("y_emu", mcp.Description("The other coordinate, in EMU.")),
	), r.slidesCopyElement)
}

// copied is what one rebuild produced: the requests to send and the losses to report.
type copied struct {
	requests []google.Request
	lost     []string
}

func (c *copied) lose(what string) {
	for _, already := range c.lost {
		if already == what {
			return
		}
	}
	c.lost = append(c.lost, what)
}

func (r *registry) slidesCopySlide(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID, err := requiredString(req, "source_presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	sourcePage, err := requiredString(req, "source_page_object_id")
	if err != nil {
		return toolError(err), nil
	}
	targetID, err := requiredString(req, "target_presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	source, err := client.Presentation(ctx, sourceID, copyMask)
	if err != nil {
		return toolError(err), nil
	}

	page := pageNamed(source, sourcePage)
	if page == nil {
		return toolError(fmt.Errorf("no slide %q in that presentation: it has %s",
			sourcePage, strings.Join(slideIDs(source), ", "))), nil
	}

	// Inside one deck there is nothing to rebuild: Google duplicates the slide itself, which
	// is exact and cheap. Rebuilding it here would be strictly worse — createShape cannot
	// reproduce an authored corner radius, so a panel with rounded corners comes back square,
	// and a linked chart would be recreated rather than carried.
	if sourceID == targetID {
		return r.duplicateSlide(ctx, client, targetID, page, req)
	}

	target, err := client.Presentation(ctx, targetID, copyMask)
	if err != nil {
		return toolError(err), nil
	}

	layoutName := optionalString(req, "layout_name")
	if layoutName == "" {
		layoutName = layoutNameOf(source, page)
	}

	slideObjectID := r.objectID("slide")
	create := &google.CreateSlideRequest{ObjectID: slideObjectID}
	if index := req.GetInt("insert_at", -1); index >= 0 {
		create.InsertionIndex = &index
	}

	// A layout is named rather than pointed at: the sample's layout identifier means
	// nothing in the target deck. Matching by name is what makes a slide land on the
	// target's own version of "Title and body" instead of on a blank page.
	work := &copied{}
	if layoutID := layoutIDByName(target, layoutName); layoutID != "" {
		create.SlideLayoutReference = &google.LayoutReference{LayoutID: layoutID}
	} else {
		create.SlideLayoutReference = &google.LayoutReference{PredefinedLayout: "BLANK"}
		if layoutName != "" {
			work.lose("the layout " + layoutName + ", which the target deck does not have: the slide " +
				"was made blank and its elements placed by their own geometry")
		}
	}

	// A title copied as an ordinary text box comes out in the target's default grey, because
	// almost everything about a title — its size, weight and colour — lives on the layout's
	// placeholder rather than on the slide. Asking createSlide for the layout's placeholders
	// by name gives them identifiers of our own, and then the text goes straight into the
	// real slot and inherits the look it is supposed to have.
	placeholders := map[string]string{}
	if create.SlideLayoutReference.LayoutID != "" {
		available := layoutPlaceholders(target, create.SlideLayoutReference.LayoutID)
		for index := range page.PageElements {
			element := &page.PageElements[index]
			if element.Shape == nil || element.Shape.Placeholder == nil {
				continue
			}

			slot := element.Shape.Placeholder
			if !available[placeholderKey(slot)] {
				work.lose("the placeholder role " + slot.Type + ", which the target layout does not " +
					"have: the copy is an ordinary shape and takes none of the layout's styling")
				continue
			}

			assigned := r.objectID("ph")
			placeholders[element.ObjectID] = assigned
			create.PlaceholderIDMappings = append(create.PlaceholderIDMappings, google.PlaceholderIDMapping{
				LayoutPlaceholder: &google.Placeholder{Type: slot.Type, Index: slot.Index},
				ObjectID:          assigned,
			})
		}
	}

	work.requests = append(work.requests, google.Request{CreateSlide: create})

	for index := range page.PageElements {
		element := &page.PageElements[index]
		if into, ok := placeholders[element.ObjectID]; ok {
			r.fillPlaceholder(element, into, work)
			continue
		}
		r.rebuild(element, slideObjectID, work, nil)
	}

	response, err := client.SlidesBatchUpdate(ctx, targetID, work.requests)
	if err != nil {
		return toolError(err), nil
	}

	newID := slideObjectID
	for _, reply := range response.Replies {
		if reply.CreateSlide != nil && reply.CreateSlide.ObjectID != "" {
			newID = reply.CreateSlide.ObjectID
		}
	}

	payload := map[string]any{
		"source_presentation_id": sourceID,
		"source_page_object_id":  sourcePage,
		"target_presentation_id": targetID,
		"page_object_id":         newID,
		"layout":                 layoutName,
		"elements":               len(page.PageElements),
		"requests":               len(work.requests),
	}

	// The notes are a second batch and cannot be anything else: they live on a page behind
	// the slide, and that page does not exist until the slide does. Its shape's identifier
	// has to be read back before anything can be written into it.
	if req.GetBool("copy_speaker_notes", true) {
		if notes := notesTextOf(page); notes != "" {
			if err := r.carryNotes(ctx, client, targetID, newID, notes); err != nil {
				work.lose("the speaker notes, which failed to write (" + err.Error() +
					"): send them with gdocs_slides_set_speaker_notes")
			} else {
				payload["speaker_notes"] = true
			}
		}
	}

	return resultJSON(withLosses(payload, work.lost))
}

// duplicateSlide copies a slide inside its own deck, which Google does exactly.
//
// Everything survives, including the things a rebuild cannot reach: an authored corner
// radius, a drawing, a group, a placeholder's tie to its layout, a chart's link to its
// workbook, the speaker notes. So this is not an optimisation — it is the only way to get a
// copy of a slide that is actually the same slide.
func (r *registry) duplicateSlide(ctx context.Context, client *google.Client, presentationID string,
	page *google.Page, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	copyID := r.objectID("slide")

	requests := []google.Request{{
		DuplicateObject: &google.DuplicateObjectRequest{
			ObjectID:  page.ObjectID,
			ObjectIDs: map[string]string{page.ObjectID: copyID},
		},
	}}

	// The duplicate lands right after its original, so a position asked for is a second
	// request rather than a field on the first.
	if index := req.GetInt("insert_at", -1); index >= 0 {
		requests = append(requests, google.Request{
			UpdateSlidesPosition: &google.UpdateSlidesPositionRequest{
				SlideObjectIDs: []string{copyID},
				InsertionIndex: index,
			},
		})
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	newID := copyID
	for _, reply := range response.Replies {
		if reply.DuplicateObject != nil && reply.DuplicateObject.ObjectID != "" {
			newID = reply.DuplicateObject.ObjectID
		}
	}

	return resultJSON(map[string]any{
		"source_presentation_id": presentationID,
		"source_page_object_id":  page.ObjectID,
		"target_presentation_id": presentationID,
		"page_object_id":         newID,
		"elements":               len(page.PageElements),
		"requests":               len(requests),
		"carried_whole":          true,
		"method":                 "duplicate",
		"note": "the source and the target are the same deck, so Google duplicated the slide " +
			"rather than this server rebuilding it: everything came across, including what a " +
			"rebuild cannot reach — an authored corner radius, a drawing, a group, a chart's link " +
			"to its workbook, and the speaker notes",
	})
}

// carryNotes writes the sample's speaker notes onto the slide just created.
func (r *registry) carryNotes(ctx context.Context, client *google.Client, presentationID, pageObjectID, text string) error {
	presentation, err := client.Presentation(ctx, presentationID,
		"slides(objectId,slideProperties(notesPage(notesProperties(speakerNotesObjectId))))")
	if err != nil {
		return err
	}

	for _, slide := range presentation.Slides {
		if slide.ObjectID != pageObjectID || slide.SlideProperties == nil {
			continue
		}
		notes := slide.SlideProperties.NotesPage
		if notes == nil || notes.NotesProperties == nil || notes.NotesProperties.SpeakerNotesObjectID == "" {
			break
		}

		_, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
			InsertText: &google.InsertTextRequest{
				ObjectID:       notes.NotesProperties.SpeakerNotesObjectID,
				Text:           text,
				InsertionIndex: 0,
			},
		}})

		return err
	}

	return fmt.Errorf("the new slide reports no notes page")
}

func (r *registry) slidesCopyElement(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID, err := requiredString(req, "source_presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	objectID, err := requiredString(req, "source_object_id")
	if err != nil {
		return toolError(err), nil
	}
	targetID, err := requiredString(req, "target_presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	targetPage, err := requiredString(req, "target_page_object_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	source, err := client.Presentation(ctx, sourceID, copyMask)
	if err != nil {
		return toolError(err), nil
	}

	element := elementNamed(source, objectID)
	if element == nil {
		return toolError(fmt.Errorf("no element %q on any slide of that presentation", objectID)), nil
	}

	var place *google.Transform
	_, hasX := req.GetArguments()["x_emu"]
	_, hasY := req.GetArguments()["y_emu"]
	if hasX || hasY {
		place = &google.Transform{
			ScaleX: 1, ScaleY: 1, Unit: "EMU",
			TranslateX: req.GetFloat("x_emu", 0),
			TranslateY: req.GetFloat("y_emu", 0),
		}
		if element.Transform != nil {
			place.ScaleX, place.ScaleY = element.Transform.ScaleX, element.Transform.ScaleY
			place.ShearX, place.ShearY = element.Transform.ShearX, element.Transform.ShearY
			if !hasX {
				place.TranslateX = element.Transform.TranslateX
			}
			if !hasY {
				place.TranslateY = element.Transform.TranslateY
			}
		}
	}

	work := &copied{}
	r.rebuild(element, targetPage, work, place)
	if len(work.requests) == 0 {
		return toolError(fmt.Errorf("nothing about %q can be built again: %s", objectID,
			strings.Join(work.lost, "; "))), nil
	}

	if _, err := client.SlidesBatchUpdate(ctx, targetID, work.requests); err != nil {
		return toolError(err), nil
	}

	return resultJSON(withLosses(map[string]any{
		"source_presentation_id": sourceID,
		"source_object_id":       objectID,
		"target_presentation_id": targetID,
		"target_page_object_id":  targetPage,
		"requests":               len(work.requests),
	}, work.lost))
}

// fillPlaceholder writes a sample's placeholder into the slot the new slide already has.
//
// Nothing is created and no geometry is sent: the slot is where the layout puts it, which is
// the point — a title moved to the sample's coordinates would sit where the sample's layout
// wanted it rather than where this deck's does. Only what the sample said about itself over
// and above its layout is written: the text, its runs, and any fill or outline the sample
// set explicitly.
func (r *registry) fillPlaceholder(element *google.PageElement, objectID string, work *copied) {
	shape := element.Shape

	if fill, outline, alignment, fields := shapeLook(shape); len(fields) > 0 {
		work.requests = append(work.requests, google.Request{
			UpdateShapeProperties: &google.UpdateShapePropertiesRequest{
				ObjectID: objectID,
				ShapeProperties: &google.ShapeProperties{
					BackgroundFill:   fill,
					Outline:          outline,
					ContentAlignment: alignment,
				},
				Fields: strings.Join(fields, ","),
			},
		})
	}

	work.requests = append(work.requests, textRequests(objectID, shape.Text, nil)...)
}

// layoutPlaceholders is the set of slots one layout offers, as createSlide will match them.
func layoutPlaceholders(presentation *google.Presentation, layoutID string) map[string]bool {
	slots := map[string]bool{}

	for _, layout := range presentation.Layouts {
		if layout.ObjectID != layoutID {
			continue
		}
		for _, element := range layout.PageElements {
			if element.Shape == nil || element.Shape.Placeholder == nil {
				continue
			}
			slots[placeholderKey(element.Shape.Placeholder)] = true
		}
	}

	return slots
}

// placeholderKey names a slot by what identifies it: a layout can hold two bodies, told
// apart only by their index.
func placeholderKey(slot *google.Placeholder) string {
	index := 0
	if slot.Index != nil {
		index = *slot.Index
	}

	return fmt.Sprintf("%s/%d", slot.Type, index)
}

// rebuild turns one element of a sample into the requests that build it again.
//
// place overrides where it lands; nil keeps the sample's own transform, which is what
// copying a whole slide wants.
func (r *registry) rebuild(element *google.PageElement, pageObjectID string, work *copied, place *google.Transform) {
	properties := &google.ElementProperties{PageObjectID: pageObjectID, Size: element.Size}
	if place != nil {
		properties.Transform = place
	} else if element.Transform != nil {
		properties.Transform = element.Transform
	}

	switch {
	case element.Shape != nil:
		r.rebuildShape(element, properties, work)
	case element.Table != nil:
		r.rebuildTable(element, properties, work)
	case element.Image != nil:
		r.rebuildImage(element, properties, work)
	case element.Line != nil:
		r.rebuildLine(element, properties, work)
	case element.SheetsChart != nil:
		work.requests = append(work.requests, google.Request{
			CreateSheetsChart: &google.CreateSheetsChartRequest{
				ObjectID:      r.objectID("chart"),
				Element:       properties,
				SpreadsheetID: element.SheetsChart.SpreadsheetID,
				ChartID:       element.SheetsChart.ChartID,
				LinkingMode:   "LINKED",
			},
		})
	case element.Video != nil:
		work.requests = append(work.requests, google.Request{
			CreateVideo: &google.CreateVideoRequest{
				ObjectID: r.objectID("video"),
				Element:  properties,
				Source:   element.Video.Source,
				ID:       element.Video.ID,
			},
		})
	case element.ElementGroup != nil:
		// A group cannot be built in one request: its children have to exist first and be
		// grouped afterwards, and their transforms are relative to the group's. Carrying
		// the children flat would move every one of them.
		work.lose("a group of elements, which has to be rebuilt child by child and grouped " +
			"afterwards with gdocs_slides_group")
	default:
		work.lose("an element of a kind this server cannot create — a drawing, most likely, " +
			"which the API does not describe at all")
	}
}

func (r *registry) rebuildShape(element *google.PageElement, properties *google.ElementProperties, work *copied) {
	shape := element.Shape
	objectID := r.objectID("shape")

	shapeType := shape.ShapeType
	if shapeType == "" {
		shapeType = "TEXT_BOX"
	}

	work.requests = append(work.requests, google.Request{
		CreateShape: &google.CreateShapeRequest{
			ObjectID:          objectID,
			ElementProperties: properties,
			ShapeType:         shapeType,
		},
	})

	if shape.Placeholder != nil && shape.Placeholder.Type != "" {
		// A placeholder is a slot the layout owns, and a created shape is never one. The
		// text and the look come across; what does not is the slide's tie to its layout, so
		// a later change to that layout will not move this shape.
		work.lose("the placeholder role " + shape.Placeholder.Type +
			", which only a layout can grant: the copy is an ordinary shape and will not " +
			"follow later changes to the layout")
	}

	if fill, outline, alignment, fields := shapeLook(shape); len(fields) > 0 {
		work.requests = append(work.requests, google.Request{
			UpdateShapeProperties: &google.UpdateShapePropertiesRequest{
				ObjectID: objectID,
				ShapeProperties: &google.ShapeProperties{
					BackgroundFill:   fill,
					Outline:          outline,
					ContentAlignment: alignment,
				},
				Fields: strings.Join(fields, ","),
			},
		})
	}

	work.requests = append(work.requests, textRequests(objectID, shape.Text, nil)...)
}

func (r *registry) rebuildTable(element *google.PageElement, properties *google.ElementProperties, work *copied) {
	table := element.Table
	objectID := r.objectID("table")

	work.requests = append(work.requests, google.Request{
		CreateTable: &google.CreateTableRequest{
			ObjectID:          objectID,
			ElementProperties: properties,
			Rows:              table.Rows,
			Columns:           table.Columns,
		},
	})

	for index, column := range table.TableColumns {
		if column.ColumnWidth == nil {
			continue
		}
		work.requests = append(work.requests, google.Request{
			UpdateTableColumnProperties: &google.UpdateTableColumnPropertiesRequest{
				ObjectID:              objectID,
				ColumnIndices:         []int{index},
				TableColumnProperties: &google.TableColumnProperties{ColumnWidth: column.ColumnWidth},
				Fields:                "columnWidth",
			},
		})
	}

	for _, row := range table.TableRows {
		for cellIndex := range row.TableCells {
			cell := &row.TableCells[cellIndex]
			location := &google.CellLocation{
				RowIndex:    cell.Location.RowIndex,
				ColumnIndex: cell.Location.ColumnIndex,
			}

			if cell.RowSpan > 1 || cell.ColumnSpan > 1 {
				work.requests = append(work.requests, google.Request{
					MergeTableCells: &google.MergeTableCellsRequest{
						ObjectID: objectID,
						TableRange: &google.TableRange{
							Location:   location,
							RowSpan:    cell.RowSpan,
							ColumnSpan: cell.ColumnSpan,
						},
					},
				})
			}

			if properties := cell.Properties; properties != nil {
				var fields []string
				if properties.BackgroundFill != nil {
					fields = append(fields, "tableCellBackgroundFill")
				}
				if properties.ContentAlignment != "" {
					fields = append(fields, "contentAlignment")
				}
				if len(fields) > 0 {
					work.requests = append(work.requests, google.Request{
						UpdateTableCellProperties: &google.UpdateTableCellPropertiesRequest{
							ObjectID:            objectID,
							TableRange:          &google.TableRange{Location: location, RowSpan: 1, ColumnSpan: 1},
							TableCellProperties: properties,
							Fields:              strings.Join(fields, ","),
						},
					})
				}
			}

			work.requests = append(work.requests, textRequests(objectID, cell.Text, location)...)
		}
	}
}

func (r *registry) rebuildImage(element *google.PageElement, properties *google.ElementProperties, work *copied) {
	image := element.Image
	if image.ContentURL == "" {
		work.lose("a picture whose address Slides did not hand out, which cannot be fetched " +
			"and so cannot be put anywhere else")
		return
	}

	objectID := r.objectID("image")

	// Google fetches the address itself and keeps its own copy of the bytes. The address is
	// short-lived and tagged with the account that read it, which is why reading and writing
	// have to happen in one pass rather than a list today and a rebuild tomorrow.
	work.requests = append(work.requests, google.Request{
		CreateImage: &google.CreateImageRequest{
			ObjectID:          objectID,
			ElementProperties: properties,
			URL:               image.ContentURL,
		},
	})

	if image.Properties == nil {
		return
	}

	var fields []string
	for _, field := range []struct {
		name  string
		isSet bool
	}{
		{"cropProperties", image.Properties.CropProperties != nil},
		{"transparency", image.Properties.Transparency != 0},
		{"brightness", image.Properties.Brightness != 0},
		{"contrast", image.Properties.Contrast != 0},
		{"outline", image.Properties.Outline != nil},
	} {
		if field.isSet {
			fields = append(fields, field.name)
		}
	}
	if len(fields) == 0 {
		return
	}

	work.requests = append(work.requests, google.Request{
		UpdateImageProperties: &google.UpdateImagePropertiesRequest{
			ObjectID:        objectID,
			ImageProperties: image.Properties,
			Fields:          strings.Join(fields, ","),
		},
	})
}

func (r *registry) rebuildLine(element *google.PageElement, properties *google.ElementProperties, work *copied) {
	line := element.Line
	objectID := r.objectID("line")

	category := line.LineCategory
	if category == "" {
		category = "STRAIGHT"
	}

	work.requests = append(work.requests, google.Request{
		CreateLine: &google.CreateLineRequest{
			ObjectID:          objectID,
			ElementProperties: properties,
			Category:          category,
		},
	})

	if line.Properties == nil {
		return
	}

	var fields []string
	for _, field := range []struct {
		name  string
		isSet bool
	}{
		{"lineFill", line.Properties.LineFill != nil},
		{"weight", line.Properties.Weight != nil},
		{"dashStyle", line.Properties.DashStyle != ""},
		{"startArrow", line.Properties.StartArrow != ""},
		{"endArrow", line.Properties.EndArrow != ""},
	} {
		if field.isSet {
			fields = append(fields, field.name)
		}
	}
	if len(fields) == 0 {
		return
	}

	work.requests = append(work.requests, google.Request{
		UpdateLineProperties: &google.UpdateLinePropertiesRequest{
			ObjectID:       objectID,
			LineProperties: line.Properties,
			Fields:         strings.Join(fields, ","),
		},
	})
}

// builtText is a shape's or a cell's text laid out for the target, with the ranges every
// style will need already worked out against it.
//
// It exists because three things have to line up and cannot be worked out separately: the
// depth of a list item is a tab character in the text, the trailing newline of the last
// paragraph must not be sent, and every style range is counted in the resulting text rather
// than in the sample's.
type builtText struct {
	text       string
	runs       []styledRange
	paragraphs []styledRange
	bullets    []styledRange
}

// styledRange is a stretch of the built text and what applies to it.
type styledRange struct {
	start, end int64
	style      *google.TextStyle
	paragraph  *google.ParagraphStyle
	level      int
}

// buildText lays a sample's text out for the target.
//
// Two things here were learned on a real slide rather than from the documentation.
//
// A container always ends with a newline of its own, and a run read off a sample carries
// that newline in its content. Sending it adds an empty paragraph at the end — invisible in
// a text box, and in a table cell it makes every row half as tall again, which pushed the
// last row of a copied table off the slide.
//
// Depth is a tab character. Slides reports a list item's depth as a number and accepts it
// only as tabs in the text handed to createParagraphBullets; a copy that sends the text
// without them gets a list of one level, whatever the sample had.
func buildText(content *google.TextContent) builtText {
	var built builtText
	var out strings.Builder
	var at int64

	paragraphStart, paragraphEnd := int64(0), int64(0)
	var paragraphStyle *google.ParagraphStyle
	level := -1

	flush := func() {
		if paragraphEnd <= paragraphStart {
			return
		}
		if paragraphStyle != nil {
			built.paragraphs = append(built.paragraphs,
				styledRange{start: paragraphStart, end: paragraphEnd, paragraph: paragraphStyle, level: level})
		}
		if level >= 0 {
			built.bullets = append(built.bullets,
				styledRange{start: paragraphStart, end: paragraphEnd, level: level})
		}
	}

	for index := range content.TextElements {
		item := &content.TextElements[index]

		switch {
		case item.ParagraphMarker != nil:
			flush()
			paragraphStart, paragraphEnd = at, at
			paragraphStyle = item.ParagraphMarker.Style
			level = -1

			if bullet := item.ParagraphMarker.Bullet; bullet != nil {
				level = 0
				if bullet.NestingLevel != nil {
					level = *bullet.NestingLevel
				}
				// The tabs belong to no run: they are how depth is spelled, and Slides
				// takes them out again when it makes the list.
				if level > 0 {
					tabs := strings.Repeat("\t", level)
					out.WriteString(tabs)
					at += utf16Length(tabs)
					paragraphEnd = at
				}
			}

		case item.TextRun != nil:
			content := item.TextRun.Content
			if content == "" {
				continue
			}

			out.WriteString(content)
			length := utf16Length(content)
			if style := item.TextRun.Style; style != nil && !style.IsEmpty() {
				built.runs = append(built.runs, styledRange{start: at, end: at + length, style: style})
			}
			at += length
			paragraphEnd = at
		}
	}
	flush()

	built.text = strings.TrimSuffix(out.String(), "\n")
	built.clamp(utf16Length(built.text))

	return built
}

// without drops named fields from a mask.
func without(fields []string, drop ...string) []string {
	unwanted := map[string]bool{}
	for _, name := range drop {
		unwanted[name] = true
	}

	kept := fields[:0]
	for _, field := range fields {
		if !unwanted[field] {
			kept = append(kept, field)
		}
	}

	return kept
}

// clamp cuts every range down to the text that will actually be sent, which is one newline
// shorter than what was read. A range reaching past the end is refused by the API outright.
func (b *builtText) clamp(length int64) {
	for _, group := range []*[]styledRange{&b.runs, &b.paragraphs, &b.bullets} {
		kept := (*group)[:0]
		for _, one := range *group {
			if one.start >= length {
				continue
			}
			if one.end > length {
				one.end = length
			}
			if one.end > one.start {
				kept = append(kept, one)
			}
		}
		*group = kept
	}
}

// textRequests writes a shape's or a cell's text back.
//
// The text goes in whole and the styles are laid over it afterwards, by range. Writing run
// by run instead would need an insertion index per run and would put every style at the
// wrong place the moment one run's text changed length. The ranges are counted in UTF-16
// code units, which is what the API indexes by: a byte count is twice too large for
// Russian and lands in the middle of a character, and the whole batch is refused.
func textRequests(objectID string, content *google.TextContent, cell *google.CellLocation) []google.Request {
	if content == nil {
		return nil
	}

	built := buildText(content)
	if strings.TrimSpace(built.text) == "" {
		return nil
	}

	requests := []google.Request{{
		InsertText: &google.InsertTextRequest{
			ObjectID:       objectID,
			CellLocation:   cell,
			Text:           built.text,
			InsertionIndex: 0,
		},
	}}

	for _, run := range built.runs {
		requests = append(requests, google.Request{
			UpdateTextStyle: &google.UpdateTextStyleRequest{
				ObjectID:     objectID,
				CellLocation: cell,
				TextRange:    google.FixedRange(run.start, run.end),
				Style:        run.style,
				Fields:       strings.Join(run.style.Fields(), ","),
			},
		})
	}

	for _, paragraph := range built.paragraphs {
		fields := paragraphFields(paragraph.paragraph)
		if paragraph.level >= 0 {
			// The indents of a list item are not a decision the author made: Slides works
			// them out from the item's depth. Copying them and then asking for a list of the
			// same depth counts the depth twice — on a real slide a second-level item came
			// out at the third level, with square markers where the sample had circles.
			fields = without(fields, "indentStart", "indentFirstLine", "indentEnd")
		}
		if len(fields) == 0 {
			continue
		}
		requests = append(requests, google.Request{
			UpdateParagraphStyle: &google.UpdateParagraphStyleRequest{
				ObjectID:     objectID,
				CellLocation: cell,
				TextRange:    google.FixedRange(paragraph.start, paragraph.end),
				Style:        paragraph.paragraph,
				Fields:       strings.Join(fields, ","),
			},
		})
	}

	// Bullets are made from the text rather than typed: asking Slides to turn a range into
	// a list is the only way to get real ones, and a marker character placed by hand is a
	// character, not a list. One request over the whole bulleted stretch rather than one per
	// paragraph — the depth is already in the text as tabs.
	if len(built.bullets) > 0 {
		first, last := built.bullets[0].start, built.bullets[len(built.bullets)-1].end
		requests = append(requests, google.Request{
			CreateParagraphBullets: &google.CreateParagraphBulletsRequest{
				ObjectID:     objectID,
				CellLocation: cell,
				TextRange:    google.FixedRange(first, last),
				BulletPreset: "BULLET_DISC_CIRCLE_SQUARE",
			},
		})
	}

	return requests
}

// paragraphFields is the mask for a paragraph style read off a sample.
func paragraphFields(style *google.ParagraphStyle) []string {
	var fields []string

	for _, field := range []struct {
		name  string
		isSet bool
	}{
		{"alignment", style.Alignment != ""},
		{"indentStart", style.IndentStart != nil},
		{"indentEnd", style.IndentEnd != nil},
		{"indentFirstLine", style.IndentFirstLine != nil},
		{"lineSpacing", style.LineSpacing != 0},
		{"spaceAbove", style.SpaceAbove != nil},
		{"spaceBelow", style.SpaceBelow != nil},
		{"direction", style.Direction != ""},
		{"spacingMode", style.SpacingMode != ""},
	} {
		if field.isSet {
			fields = append(fields, field.name)
		}
	}

	return fields
}

// shapeLook is a shape's fill, outline and content alignment with the mask that applies them.
func shapeLook(shape *google.Shape) (*google.ShapeBackgroundFill, *google.Outline, string, []string) {
	if shape.Properties == nil {
		return nil, nil, "", nil
	}

	properties := shape.Properties
	var fields []string

	// A fill or an outline that Slides reports as INHERIT is not a decision the sample made
	// — it is the layout's, and writing it here would pin down what the sample left free.
	fill := properties.BackgroundFill
	if fill != nil && fill.PropertyState == "INHERIT" {
		fill = nil
	}
	if fill != nil {
		fields = append(fields, "shapeBackgroundFill")
	}

	outline := properties.Outline
	if outline != nil && outline.PropertyState == "INHERIT" {
		outline = nil
	}
	if outline != nil {
		fields = append(fields, "outline")
	}

	if properties.ContentAlignment != "" {
		fields = append(fields, "contentAlignment")
	}

	return fill, outline, properties.ContentAlignment, fields
}

// withLosses adds what could not be carried, and says so in words rather than by omission.
func withLosses(payload map[string]any, lost []string) map[string]any {
	if len(lost) == 0 {
		payload["carried_whole"] = true
		return payload
	}

	payload["not_carried"] = lost
	payload["note"] = "the copy is not the same as its source: what is listed above was left " +
		"behind, and each item says what to do about it"

	return payload
}

func pageNamed(presentation *google.Presentation, objectID string) *google.Page {
	for index, page := range presentation.Slides {
		if page.ObjectID == objectID {
			return &presentation.Slides[index]
		}
	}

	return nil
}

func elementNamed(presentation *google.Presentation, objectID string) *google.PageElement {
	for _, page := range presentation.Slides {
		for index, element := range page.PageElements {
			if element.ObjectID == objectID {
				return &page.PageElements[index]
			}
		}
	}

	return nil
}

func slideIDs(presentation *google.Presentation) []string {
	names := make([]string, 0, len(presentation.Slides))
	for _, page := range presentation.Slides {
		names = append(names, page.ObjectID)
	}

	return names
}

// layoutNameOf is the name of the layout a slide follows, which is the only thing about a
// layout that means anything in another deck.
func layoutNameOf(presentation *google.Presentation, page *google.Page) string {
	if page.SlideProperties == nil {
		return ""
	}

	for _, layout := range presentation.Layouts {
		if layout.ObjectID != page.SlideProperties.LayoutObjectID {
			continue
		}
		if layout.LayoutProperties == nil {
			return ""
		}
		if layout.LayoutProperties.DisplayName != "" {
			return layout.LayoutProperties.DisplayName
		}

		return layout.LayoutProperties.Name
	}

	return ""
}

func layoutIDByName(presentation *google.Presentation, name string) string {
	if name == "" {
		return ""
	}

	for _, layout := range presentation.Layouts {
		if layout.LayoutProperties == nil {
			continue
		}
		if layout.LayoutProperties.DisplayName == name || layout.LayoutProperties.Name == name {
			return layout.ObjectID
		}
	}

	return ""
}

// notesTextOf is the text behind a slide, which lives on a page of its own.
func notesTextOf(page *google.Page) string {
	if page.SlideProperties == nil || page.SlideProperties.NotesPage == nil {
		return ""
	}

	return speakerNotes(page.SlideProperties.NotesPage)
}
