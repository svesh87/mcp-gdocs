package tools

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// pageMask reads a slide the way reproducing it requires: the background, the speaker
// notes, and every property of every element that shows on screen.
//
// It is long because a slide's look is spread across that many fields. The alternative —
// reading a short mask and copying what it happens to contain — is what left a rebuilt
// deck without its title background and with the layout's empty placeholders showing
// through.
var pageMask = "presentationId,title,pageSize," +
	"layouts(objectId,layoutProperties(name,displayName))," +
	"slides(objectId,pageProperties(pageBackgroundFill)," +
	"slideProperties(layoutObjectId,isSkipped,notesPage(notesProperties(speakerNotesObjectId)," +
	"pageElements(objectId,shape(placeholder(type),text(textElements(textRun(content)))))))," +
	"pageElements(" + elementMask(2) + "))"

// elementMask is the per-element part of the read, nested depth levels deep for groups.
//
// Groups nest, and a field mask cannot say "recursively": each level has to be spelled
// out. Two levels covers what decks actually contain — a group of shapes, and a group of
// those — and an element deeper than that is reported as a group without its children
// rather than silently dropped.
func elementMask(depth int) string {
	mask := "objectId,title,description,size,transform," +
		"shape(shapeType,placeholder,shapeProperties(autofit,shapeBackgroundFill,outline,shadow,contentAlignment)," +
		"text(textElements(textRun(content))))," +
		"table(rows,columns,tableColumns(columnWidth),tableRows(rowHeight))," +
		"image(contentUrl,imageProperties(cropProperties,transparency,brightness,contrast,outline))," +
		"video(url,source,id)," +
		"line(lineType,lineCategory,lineProperties(lineFill,weight,dashStyle,startArrow,endArrow))"

	if depth > 0 {
		mask += ",elementGroup(children(" + elementMask(depth-1) + "))"
	}

	return mask
}

// registerSlidesPage adds the tools that work on a slide as a whole: reading everything
// on it, its background, its speaker notes, and what covers what.
func (r *registry) registerSlidesPage(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("gdocs_slides_inspect_page",
		mcp.WithDescription("Read one slide completely: its background, its speaker notes, and every element "+
			"with its box, rotation, stacking order, fill, outline, shadow and — for a picture — its crop "+
			"and adjustments. This is the tool to start from when reproducing a slide from a sample: "+
			"gdocs_slides_list reports the text and the box and nothing else, so a slide rebuilt from it "+
			"comes out without the sample's background, fills and rotations. "+
			"Everything reported here can be set back with the tools that write."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description(
			"Slide to read. Take it from gdocs_slides_list.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), r.slidesInspectPage)

	if !r.opts.AllowWrite {
		return
	}

	srv.AddTool(mcp.NewTool("gdocs_slides_set_page_background",
		mcp.WithDescription("Set a slide's background: a flat colour, a picture stretched over it, or back to "+
			"whatever the layout says. Importing a theme brings the master's background, not the one an "+
			"author put on a particular slide — a title slide with its own picture stays blank until this "+
			"is set. To copy one, read the sample's background with gdocs_slides_inspect_page and pass the "+
			"content_url it reports."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description("Slide whose background to set.")),
		mcp.WithObject("color", mcp.Description(
			"Flat background colour as {\"red\": 0..1, \"green\": 0..1, \"blue\": 0..1}.")),
		mcp.WithString("picture_url", mcp.Description(
			"Address of a picture to stretch over the slide, reachable by Google. The content_url of a "+
				"sample's background works while it is fresh: Slides fetches it and keeps its own copy.")),
		mcp.WithBoolean("inherit", mcp.Description(
			"Give the background back to the layout, undoing a colour or picture set here earlier.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesSetPageBackground)

	srv.AddTool(mcp.NewTool("gdocs_slides_order_elements",
		mcp.WithDescription("Say what covers what on a slide. Elements stack in the order they were created, "+
			"so a rebuilt slide has its picture on top of the text it sat behind in the sample. Name the "+
			"elements and how to move them through the stack."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithArray("object_ids", mcp.Required(), mcp.WithStringItems(), mcp.Description(
			"Elements to move through the stack, all on the same slide.")),
		mcp.WithString("operation", mcp.Required(), mcp.Description(
			"BRING_TO_FRONT, BRING_FORWARD, SEND_BACKWARD or SEND_TO_BACK.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesOrderElements)

	srv.AddTool(mcp.NewTool("gdocs_slides_group",
		mcp.WithDescription("Join elements into a group, so they move and scale as one, or take groups apart. "+
			"A sample built out of groups is reproduced by grouping the same elements: a group's transform "+
			"applies on top of its children's, and elements left ungrouped drift apart the moment anything "+
			"is resized."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithArray("group_object_ids", mcp.WithStringItems(), mcp.Description(
			"Elements to join into one group. At least two, all on the same slide.")),
		mcp.WithArray("ungroup_object_ids", mcp.WithStringItems(), mcp.Description(
			"Groups to take apart. Their children stay on the slide where they are.")),
		mcp.WithString("object_id", mcp.Description("Identifier to give the new group.")),
	), r.slidesGroup)

	srv.AddTool(mcp.NewTool("gdocs_slides_duplicate",
		mcp.WithDescription("Copy an element, or a whole slide, inside one presentation. "+
			"This is the way to reproduce what can be seen but not built: a shape carries adjustment "+
			"values — the corner radius of a rounded rectangle among them — that no request accepts and no "+
			"response reports, so a panel rebuilt from its shape_type comes out with the default radius "+
			"instead of the author's. A duplicate carries them. "+
			"Two boundaries: the copy lands on the same slide as the original, because no API moves an "+
			"element to another slide, and the original has to be in this presentation, because an "+
			"identifier from another one is not found. Getting a foreign shape here at all takes a person "+
			"pasting it in the browser, or gdocs_slides_copy_presentation of the whole file. "+
			"The copy sits exactly on top of the original; gdocs_slides_place_element moves it."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("object_id", mcp.Required(), mcp.Description(
			"Element or slide to copy.")),
		mcp.WithNumber("copies", mcp.Description(
			"How many copies to make. Default 1. Each is made from the original, so a panel needed in "+
				"four places is one call.")),
		mcp.WithArray("new_object_ids", mcp.WithStringItems(), mcp.Description(
			"Identifiers to give the copies, in order, so they can be addressed without reading the "+
				"slide back. Without them Google invents identifiers and reports them. Duplicating a "+
				"slide renames only the slide; its elements get invented identifiers either way.")),
	), r.slidesDuplicate)

	srv.AddTool(mcp.NewTool("gdocs_slides_hide",
		mcp.WithDescription("Hide slides from the presentation, or bring hidden ones back. A hidden slide stays "+
			"in the deck and stays editable — it is only skipped while presenting. Authors keep slides that "+
			"way: last period's numbers, a backup explanation. A copy that shows them says more than the "+
			"original does, so reproducing a deck means reproducing what it hides. "+
			"gdocs_slides_list and gdocs_slides_inspect_page report which slides are hidden."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithArray("object_ids", mcp.Required(), mcp.WithStringItems(), mcp.Description(
			"Slides to hide or show.")),
		mcp.WithBoolean("hidden", mcp.Required(), mcp.Description(
			"true hides them, false brings them back.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesHide)

	srv.AddTool(mcp.NewTool("gdocs_slides_set_speaker_notes",
		mcp.WithDescription("Replace the speaker notes of a slide. The notes are a page of their own behind "+
			"each slide, and a deck copied without them loses what the presenter was going to say."),
		mcp.WithString("presentation_id", mcp.Required(), mcp.Description(presentationIDHelp)),
		mcp.WithString("page_object_id", mcp.Required(), mcp.Description("Slide whose notes to replace.")),
		mcp.WithString("text", mcp.Required(), mcp.Description(
			"Notes text. Newlines make paragraphs. An empty string clears the notes.")),
		mcp.WithIdempotentHintAnnotation(true),
	), r.slidesSetSpeakerNotes)
}

// describedFill is a fill as a caller reads it back.
type describedFill struct {
	// State is RENDERED, NOT_RENDERED or INHERIT. It is reported even when there is a
	// colour, because INHERIT with a colour means "the layout's colour, shown here" — and
	// copying that as an explicit fill pins down what the sample left free.
	State string `json:"state,omitempty"`
	Color string `json:"color,omitempty"`
	// ThemeColor is a name like DARK1 rather than a value. A theme colour copied literally
	// stops following the theme, so it is reported by name and left to the caller.
	ThemeColor string `json:"theme_color,omitempty"`
	// Alpha is a pointer because zero is a real answer and the commonest one that matters:
	// a text box over a coloured panel carries a solid fill of black at alpha 0, which is
	// "no fill" written the long way. Dropped as empty, it reads as a black box, and a
	// copy made from that reading paints one.
	Alpha *float64 `json:"alpha,omitempty"`
	// PictureURL is where a background picture can be fetched from, for as long as Google
	// serves it. It is the handle that copies a background into another deck.
	PictureURL string `json:"picture_url,omitempty"`
	// Transparent says the fill renders as nothing whatever colour it names. Reproduce it
	// with no_fill rather than with the colour: the colour is what the alpha hides.
	Transparent bool `json:"transparent,omitempty"`
}

// describedOutline is a border as a caller reads it back.
type describedOutline struct {
	State string `json:"state,omitempty"`
	Color string `json:"color,omitempty"`
	// ThemeColor is a name like ACCENT3 rather than a value, for the same reason as on a
	// fill. A colour is stored one way or the other and never both, so an outline painted
	// by name has no value at all: reported without this it comes back as a border with a
	// weight, a dash style and no colour, and a deck recoloured from that reading loses
	// every border it had.
	ThemeColor string  `json:"theme_color,omitempty"`
	WeightEMU  float64 `json:"weight_emu,omitempty"`
	DashStyle  string  `json:"dash_style,omitempty"`
}

// describedImage is what a picture does to itself: cropped, dimmed, washed out.
type describedImage struct {
	ContentURL   string            `json:"content_url,omitempty"`
	Transparency float64           `json:"transparency,omitempty"`
	Brightness   float64           `json:"brightness,omitempty"`
	Contrast     float64           `json:"contrast,omitempty"`
	Crop         *describedCrop    `json:"crop,omitempty"`
	Outline      *describedOutline `json:"outline,omitempty"`
}

// describedCrop is how much of each side is cut away, as a fraction of the picture.
type describedCrop struct {
	Left   float64 `json:"left,omitempty"`
	Right  float64 `json:"right,omitempty"`
	Top    float64 `json:"top,omitempty"`
	Bottom float64 `json:"bottom,omitempty"`
	Angle  float64 `json:"angle,omitempty"`
}

// describedLine is a line or a connector as a caller reads it back.
type describedLine struct {
	Category string `json:"category,omitempty"`
	LineType string `json:"line_type,omitempty"`
	Color    string `json:"color,omitempty"`
	// ThemeColor is the same case as on an outline: a line drawn in a palette colour has no
	// value stored, so without the name it reads as a line with no colour.
	ThemeColor string  `json:"theme_color,omitempty"`
	WeightEMU  float64 `json:"weight_emu,omitempty"`
	DashStyle  string  `json:"dash_style,omitempty"`
	StartArrow string  `json:"start_arrow,omitempty"`
	EndArrow   string  `json:"end_arrow,omitempty"`
}

// describedElement is one thing on a slide, with everything that decides how it looks.
type describedElement struct {
	ObjectID    string `json:"object_id"`
	Kind        string `json:"kind"`
	ShapeType   string `json:"shape_type,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	// PlaceholderEmpty marks a placeholder the layout put there and nobody filled. It is
	// not invisible: Slides renders it as "Click to add text", over whatever was placed on
	// top of it. Fill it or delete it — a rebuilt slide has no third option.
	PlaceholderEmpty bool   `json:"placeholder_empty,omitempty"`
	Text             string `json:"text,omitempty"`
	// Z is the stacking order, from the back forward. It is the element's place in the
	// page's own list, which is what updatePageElementsZOrder moves things through.
	Z int `json:"z"`
	// Geometry is always reported, zeroes included: an element at x=0 sits at the left
	// edge, and a field that vanishes when zero cannot be placed against.
	X      float64 `json:"x_emu"`
	Y      float64 `json:"y_emu"`
	Width  float64 `json:"width_emu"`
	Height float64 `json:"height_emu"`
	// RotationDeg, FlippedH and FlippedV come out of the same 2×2 matrix as the scale.
	// A rotated element read as a plain box is rebuilt straight.
	RotationDeg float64 `json:"rotation_deg,omitempty"`
	FlippedH    bool    `json:"flipped_horizontally,omitempty"`
	FlippedV    bool    `json:"flipped_vertically,omitempty"`

	Fill             *describedFill    `json:"fill,omitempty"`
	Outline          *describedOutline `json:"outline,omitempty"`
	HasShadow        bool              `json:"has_shadow,omitempty"`
	ContentAlignment string            `json:"content_alignment,omitempty"`
	AutofitType      string            `json:"autofit_type,omitempty"`
	// FontScale is Slides shrinking the text to fit. A title reported as 28 pt with a
	// scale of 0.89 measures 25 pt on screen, and that is the number a person compares.
	FontScale float64 `json:"font_scale,omitempty"`

	Rows     int             `json:"rows,omitempty"`
	Columns  int             `json:"columns,omitempty"`
	Image    *describedImage `json:"image,omitempty"`
	Line     *describedLine  `json:"line,omitempty"`
	VideoURL string          `json:"video_url,omitempty"`
	// Children are the elements of a group, with their boxes already composed with the
	// group's transform — the numbers to place a copy at, not the raw ones.
	Children []describedElement `json:"children,omitempty"`
}

// slidesInspectPage reads one slide with everything that decides how it looks.
func (r *registry) slidesInspectPage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "page_object_id")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	presentation, err := client.Presentation(ctx, presentationID, pageMask)
	if err != nil {
		return toolError(err), nil
	}

	var page *google.Page
	for index := range presentation.Slides {
		if presentation.Slides[index].ObjectID == pageObjectID {
			page = &presentation.Slides[index]
			break
		}
	}
	if page == nil {
		return toolError(fmt.Errorf("no slide %s in this presentation; the identifiers are the ones "+
			"gdocs_slides_list reports", pageObjectID)), nil
	}

	layoutNames := map[string]string{}
	for _, layout := range presentation.Layouts {
		if layout.LayoutProperties == nil {
			continue
		}
		name := layout.LayoutProperties.DisplayName
		if name == "" {
			name = layout.LayoutProperties.Name
		}
		layoutNames[layout.ObjectID] = name
	}

	payload := map[string]any{
		"presentation_id": presentation.PresentationID,
		"page_object_id":  page.ObjectID,
		"elements":        describeElements(page.PageElements, nil),
	}

	if presentation.PageSize != nil && presentation.PageSize.Width != nil && presentation.PageSize.Height != nil {
		payload["page_size_emu"] = map[string]float64{
			"width":  presentation.PageSize.Width.InEMU(),
			"height": presentation.PageSize.Height.InEMU(),
		}
	}

	if page.SlideProperties != nil {
		payload["layout_object_id"] = page.SlideProperties.LayoutObjectID
		payload["layout"] = layoutNames[page.SlideProperties.LayoutObjectID]
		if page.SlideProperties.IsSkipped != nil && *page.SlideProperties.IsSkipped {
			payload["hidden"] = true
		}
		if notes := speakerNotes(page.SlideProperties.NotesPage); notes != "" {
			payload["speaker_notes"] = notes
		}
	}

	// The background is reported even when it is inherited: "this slide adds nothing of
	// its own" is the answer a caller reproducing it needs, and its absence from the
	// answer would read as "not looked at".
	background := describeFill(pageBackgroundFill(page))
	if background == nil {
		background = &describedFill{State: "INHERIT"}
	}
	payload["background"] = background

	return resultJSON(payload)
}

// pageBackgroundFill is a page's own background, if it has one.
func pageBackgroundFill(page *google.Page) *google.PageBackgroundFill {
	if page == nil || page.PageProperties == nil {
		return nil
	}

	return page.PageProperties.PageBackgroundFill
}

// speakerNotes is the text of a slide's notes page.
func speakerNotes(notes *google.Page) string {
	if notes == nil {
		return ""
	}

	// Only the shape the notes page names holds the speaker's text; the other placeholder
	// on that page is a picture of the slide itself.
	wanted := ""
	if notes.NotesProperties != nil {
		wanted = notes.NotesProperties.SpeakerNotesObjectID
	}

	var text strings.Builder
	for _, element := range notes.PageElements {
		if element.Shape == nil || element.Shape.Text == nil {
			continue
		}
		if wanted != "" && element.ObjectID != wanted {
			continue
		}
		for _, item := range element.Shape.Text.TextElements {
			if item.TextRun != nil {
				text.WriteString(item.TextRun.Content)
			}
		}
	}

	return strings.TrimSpace(text.String())
}

// describeElements turns the elements of a page into what a caller reads, composing each
// child of a group with the group's own transform.
func describeElements(elements []google.PageElement, parent *google.Transform) []describedElement {
	described := make([]describedElement, 0, len(elements))

	for index := range elements {
		element := elements[index]
		combined := composeTransforms(parent, element.Transform)

		entry := describedElement{
			ObjectID: element.ObjectID,
			Kind:     elementKind(element),
			Z:        index,
		}

		if width, height, err := elementBox(&element); err == nil {
			entry.Width, entry.Height = width, height
		}
		if combined != nil {
			entry.X, entry.Y = combined.TranslateX, combined.TranslateY
			entry.RotationDeg = rotationDegrees(combined)
			entry.FlippedH, entry.FlippedV = flips(combined)
		}

		if shape := element.Shape; shape != nil {
			entry.ShapeType = shape.ShapeType
			entry.Text = shapeText(shape)
			if shape.Placeholder != nil {
				entry.Placeholder = shape.Placeholder.Type
				entry.PlaceholderEmpty = strings.TrimSpace(entry.Text) == ""
			}
			if properties := shape.Properties; properties != nil {
				entry.ContentAlignment = properties.ContentAlignment
				entry.HasShadow = renderedShadow(properties.Shadow)
				entry.Outline = describeOutline(properties.Outline)
				if properties.BackgroundFill != nil {
					entry.Fill = describeFill(&google.PageBackgroundFill{
						PropertyState: properties.BackgroundFill.PropertyState,
						SolidFill:     properties.BackgroundFill.SolidFill,
					})
				}
				if autofit := properties.Autofit; autofit != nil {
					entry.AutofitType, entry.FontScale = autofit.AutofitType, autofit.FontScale
				}
			}
		}

		if table := element.Table; table != nil {
			entry.Rows, entry.Columns = table.Rows, table.Columns
		}

		if image := element.Image; image != nil {
			entry.Image = describeImage(image)
		}

		if line := element.Line; line != nil {
			entry.Line = describeLine(line)
		}

		if video := element.Video; video != nil {
			entry.VideoURL = video.URL
		}

		if group := element.ElementGroup; group != nil {
			entry.Children = describeElements(group.Children, combined)
		}

		described = append(described, entry)
	}

	return described
}

// composeTransforms applies a child's transform inside its parent's.
//
// The two are 3×3 affine matrices in Slides' own notation, and a group's children are
// stored relative to the group. Reading a child as if it stood on the slide puts a copy
// of it out by exactly the group's transform.
func composeTransforms(parent, child *google.Transform) *google.Transform {
	if parent == nil {
		return child
	}
	if child == nil {
		return parent
	}

	parentScaleX, parentScaleY := unitScale(parent.ScaleX), unitScale(parent.ScaleY)
	childScaleX, childScaleY := unitScale(child.ScaleX), unitScale(child.ScaleY)

	return &google.Transform{
		ScaleX:     parentScaleX*childScaleX + parent.ShearX*child.ShearY,
		ShearX:     parentScaleX*child.ShearX + parent.ShearX*childScaleY,
		ShearY:     parent.ShearY*childScaleX + parentScaleY*child.ShearY,
		ScaleY:     parent.ShearY*child.ShearX + parentScaleY*childScaleY,
		TranslateX: parentScaleX*child.TranslateX + parent.ShearX*child.TranslateY + parent.TranslateX,
		TranslateY: parent.ShearY*child.TranslateX + parentScaleY*child.TranslateY + parent.TranslateY,
		Unit:       "EMU",
	}
}

// unitScale reads a scale, treating an absent one as 1: Slides omits a scale of exactly
// one, and multiplying by the zero that leaves collapses the element to nothing.
func unitScale(scale float64) float64 {
	if scale == 0 {
		return 1
	}

	return scale
}

// rotationDegrees is the angle out of a transform, rounded to a tenth of a degree.
//
// Slides has no rotation field: the angle is in the same 2×2 as the scale, so a rotated
// element is one with shear. Rounding keeps a straight element from being reported as
// rotated by a millionth of a degree.
func rotationDegrees(transform *google.Transform) float64 {
	if transform == nil {
		return 0
	}

	angle := math.Atan2(transform.ShearY, unitScale(transform.ScaleX)) * 180 / math.Pi
	rounded := math.Round(angle*10) / 10
	if rounded == 0 {
		return 0
	}

	return rounded
}

// flips says whether an element is mirrored. A mirrored element has a negative scale on
// that axis, which is not the same as being rotated by 180°.
func flips(transform *google.Transform) (bool, bool) {
	if transform == nil {
		return false, false
	}

	return transform.ScaleX < 0, transform.ScaleY < 0
}

// renderedShadow says whether a shadow actually shows. Slides reports a shadow object on
// most shapes with the state NOT_RENDERED, and calling that "has a shadow" would have a
// caller copying shadows onto everything.
func renderedShadow(shadow *google.Shadow) bool {
	return shadow != nil && shadow.PropertyState == "RENDERED"
}

// describeFill renders a background fill for a caller.
func describeFill(fill *google.PageBackgroundFill) *describedFill {
	if fill == nil {
		return nil
	}

	described := &describedFill{State: fill.PropertyState}

	if solid := fill.SolidFill; solid != nil {
		alpha := solid.Alpha
		described.Alpha = &alpha
		if solid.Color != nil {
			described.Color = slideColor(solid.Color.RGBColor)
			described.ThemeColor = solid.Color.ThemeColor
		}
		// Fully transparent is how Slides stores "no fill" for a text box placed over a
		// panel, and the colour underneath it is whatever was there — usually black.
		// Saying so plainly is the difference between a copy that keeps the panel and one
		// that covers it.
		if alpha == 0 {
			described.Transparent = true
		}
	}

	if picture := fill.StretchedPictureFill; picture != nil {
		described.PictureURL = picture.ContentURL
	}

	if described.State == "" && described.Color == "" &&
		described.ThemeColor == "" && described.PictureURL == "" {
		return nil
	}

	return described
}

// describeOutline renders a border for a caller.
func describeOutline(outline *google.Outline) *describedOutline {
	// A shape with no border of its own still reports one, with the state NOT_RENDERED.
	// Reporting those would have every text box come back looking outlined.
	if outline == nil || outline.PropertyState == "NOT_RENDERED" {
		return nil
	}

	described := &describedOutline{State: outline.PropertyState, DashStyle: outline.DashStyle}
	if outline.Weight != nil {
		described.WeightEMU = outline.Weight.InEMU()
	}
	if outline.OutlineFill != nil && outline.OutlineFill.SolidFill != nil &&
		outline.OutlineFill.SolidFill.Color != nil {
		described.Color = slideColor(outline.OutlineFill.SolidFill.Color.RGBColor)
		described.ThemeColor = outline.OutlineFill.SolidFill.Color.ThemeColor
	}

	return described
}

// describeImage renders a picture's own properties for a caller.
func describeImage(image *google.Image) *describedImage {
	described := &describedImage{ContentURL: image.ContentURL}

	if properties := image.Properties; properties != nil {
		described.Transparency = properties.Transparency
		described.Brightness = properties.Brightness
		described.Contrast = properties.Contrast
		described.Outline = describeOutline(properties.Outline)

		if crop := properties.CropProperties; crop != nil &&
			(crop.LeftOffset != 0 || crop.RightOffset != 0 || crop.TopOffset != 0 ||
				crop.BottomOffset != 0 || crop.Angle != 0) {
			described.Crop = &describedCrop{
				Left:   crop.LeftOffset,
				Right:  crop.RightOffset,
				Top:    crop.TopOffset,
				Bottom: crop.BottomOffset,
				Angle:  crop.Angle,
			}
		}
	}

	return described
}

// describeLine renders a line for a caller.
func describeLine(line *google.Line) *describedLine {
	described := &describedLine{Category: line.LineCategory, LineType: line.LineType}

	if properties := line.Properties; properties != nil {
		described.DashStyle = properties.DashStyle
		described.StartArrow = properties.StartArrow
		described.EndArrow = properties.EndArrow
		if properties.Weight != nil {
			described.WeightEMU = properties.Weight.InEMU()
		}
		if properties.LineFill != nil && properties.LineFill.SolidFill != nil &&
			properties.LineFill.SolidFill.Color != nil {
			described.Color = slideColor(properties.LineFill.SolidFill.Color.RGBColor)
			described.ThemeColor = properties.LineFill.SolidFill.Color.ThemeColor
		}
	}

	return described
}

// slideColor renders a colour as hex, white included.
//
// This is describeColor without its one concession to Sheets, where white is the fill of
// every cell and reporting it buries the coloured ones. On a slide white is a decision —
// white text on a dark panel — and swallowing it loses the text when the slide is copied.
func slideColor(colour *google.RGBColor) string {
	if colour == nil {
		return ""
	}

	return fmt.Sprintf("#%02X%02X%02X",
		int(colour.Red*255+0.5), int(colour.Green*255+0.5), int(colour.Blue*255+0.5))
}

// slidesSetPageBackground puts a colour or a picture behind a slide.
func (r *registry) slidesSetPageBackground(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "page_object_id")
	if err != nil {
		return toolError(err), nil
	}

	colour, err := parseColor(req, "color")
	if err != nil {
		return toolError(err), nil
	}
	pictureURL := optionalString(req, "picture_url")
	inherit := req.GetBool("inherit", false)

	chosen := 0
	for _, set := range []bool{colour != nil, pictureURL != "", inherit} {
		if set {
			chosen++
		}
	}
	if chosen == 0 {
		return toolError(fmt.Errorf("name one of color, picture_url or inherit")), nil
	}
	if chosen > 1 {
		return toolError(fmt.Errorf("color, picture_url and inherit are alternatives: name one")), nil
	}
	if pictureURL != "" && !strings.HasPrefix(pictureURL, "https://") && !strings.HasPrefix(pictureURL, "http://") {
		return toolError(fmt.Errorf("picture_url has to be an http or https address Google can fetch, got %q",
			pictureURL)), nil
	}

	fill := &google.PageBackgroundFill{}
	switch {
	case colour != nil:
		fill.PropertyState = "RENDERED"
		fill.SolidFill = &google.SolidFill{Color: &google.OpaqueColor{RGBColor: colour}, Alpha: 1}
	case pictureURL != "":
		fill.PropertyState = "RENDERED"
		fill.StretchedPictureFill = &google.StretchedPictureFill{ContentURL: pictureURL}
	default:
		fill.PropertyState = "INHERIT"
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	// The mask names the background and nothing else. Sending the properties object
	// without one resets everything it does not carry, which on a title slide trades the
	// author's picture for white.
	response, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		UpdatePageProperties: &google.UpdatePagePropertiesRequest{
			ObjectID:       pageObjectID,
			PageProperties: &google.PageProperties{PageBackgroundFill: fill},
			Fields:         "pageBackgroundFill",
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"page_object_id":  pageObjectID,
		"background":      fill.PropertyState,
		"replies":         len(response.Replies),
	})
}

// slidesOrderElements moves elements through the stacking order.
func (r *registry) slidesOrderElements(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	objectIDs, err := req.RequireStringSlice("object_ids")
	if err != nil {
		return toolError(err), nil
	}
	if len(objectIDs) == 0 {
		return toolError(fmt.Errorf("object_ids is empty")), nil
	}

	operation, err := requiredString(req, "operation")
	if err != nil {
		return toolError(err), nil
	}
	operation = strings.ToUpper(operation)
	switch operation {
	case "BRING_TO_FRONT", "BRING_FORWARD", "SEND_BACKWARD", "SEND_TO_BACK":
	default:
		return toolError(fmt.Errorf("operation is %q: it has to be BRING_TO_FRONT, BRING_FORWARD, "+
			"SEND_BACKWARD or SEND_TO_BACK", operation)), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, []google.Request{{
		UpdatePageElementsZOrder: &google.UpdatePageElementsZOrderRequest{
			PageElementObjectIDs: objectIDs,
			Operation:            operation,
		},
	}})
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"moved":           len(objectIDs),
		"operation":       operation,
		"replies":         len(response.Replies),
	})
}

// slidesGroup joins elements into a group or takes groups apart.
func (r *registry) slidesGroup(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	toGroup := req.GetStringSlice("group_object_ids", nil)
	toUngroup := req.GetStringSlice("ungroup_object_ids", nil)

	if len(toGroup) == 0 && len(toUngroup) == 0 {
		return toolError(fmt.Errorf("name group_object_ids or ungroup_object_ids")), nil
	}
	if len(toGroup) == 1 {
		return toolError(fmt.Errorf("a group needs at least two elements, got one")), nil
	}

	var requests []google.Request
	groupObjectID := optionalString(req, "object_id")

	if len(toGroup) > 0 {
		if groupObjectID == "" {
			groupObjectID = r.objectID("group")
		}
		requests = append(requests, google.Request{
			GroupObjects: &google.GroupObjectsRequest{
				ChildrenObjectIDs: toGroup,
				GroupObjectID:     groupObjectID,
			},
		})
	}

	if len(toUngroup) > 0 {
		requests = append(requests, google.Request{
			UngroupObjects: &google.UngroupObjectsRequest{ObjectIDs: toUngroup},
		})
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	payload := map[string]any{
		"presentation_id": response.PresentationID,
		"grouped":         len(toGroup),
		"ungrouped":       len(toUngroup),
		"replies":         len(response.Replies),
	}
	if len(toGroup) > 0 {
		payload["group_object_id"] = groupObjectID
	}

	return resultJSON(payload)
}

// slidesHide hides slides from the presentation or brings them back.
// slidesDuplicate copies an element or a slide inside one presentation.
//
// The copies are made one request per copy rather than by duplicating the duplicate: a
// chain would carry any later edit of the first copy into the rest, and the point of this
// tool is that every copy is the original.
func (r *registry) slidesDuplicate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	objectID, err := requiredString(req, "object_id")
	if err != nil {
		return toolError(err), nil
	}

	copies := int(req.GetFloat("copies", 1))
	if copies < 1 {
		return toolError(fmt.Errorf("copies is %d: it has to be at least 1", copies)), nil
	}

	names := req.GetStringSlice("new_object_ids", nil)
	if len(names) > copies {
		return toolError(fmt.Errorf("new_object_ids names %d identifiers for %d copies",
			len(names), copies)), nil
	}

	requests := make([]google.Request, 0, copies)
	for index := 0; index < copies; index++ {
		duplicate := &google.DuplicateObjectRequest{ObjectID: objectID}
		if index < len(names) && names[index] != "" {
			duplicate.ObjectIDs = map[string]string{objectID: names[index]}
		}
		requests = append(requests, google.Request{DuplicateObject: duplicate})
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	made := make([]string, 0, len(response.Replies))
	for _, reply := range response.Replies {
		if reply.DuplicateObject != nil {
			made = append(made, reply.DuplicateObject.ObjectID)
		}
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"copied":          objectID,
		"new_object_ids":  made,
		"note": "each copy sits exactly on top of the original; " +
			"move it with gdocs_slides_place_element",
	})
}

func (r *registry) slidesHide(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}

	objectIDs, err := req.RequireStringSlice("object_ids")
	if err != nil {
		return toolError(err), nil
	}
	if len(objectIDs) == 0 {
		return toolError(fmt.Errorf("object_ids is empty")), nil
	}

	if _, ok := req.GetArguments()["hidden"]; !ok {
		return toolError(fmt.Errorf("hidden is required: true hides the slides, false brings them back")), nil
	}
	hidden := req.GetBool("hidden", false)

	requests := make([]google.Request, 0, len(objectIDs))
	for _, objectID := range objectIDs {
		requests = append(requests, google.Request{
			UpdateSlideProperties: &google.UpdateSlidePropertiesRequest{
				ObjectID:        objectID,
				SlideProperties: &google.SlideProperties{IsSkipped: &hidden},
				Fields:          "isSkipped",
			},
		})
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"slides":          len(objectIDs),
		"hidden":          hidden,
		"replies":         len(response.Replies),
	})
}

// slidesSetSpeakerNotes replaces the notes behind a slide.
func (r *registry) slidesSetSpeakerNotes(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	presentationID, err := requiredString(req, "presentation_id")
	if err != nil {
		return toolError(err), nil
	}
	pageObjectID, err := requiredString(req, "page_object_id")
	if err != nil {
		return toolError(err), nil
	}
	text, err := req.RequireString("text")
	if err != nil {
		return toolError(err), nil
	}

	client, err := r.client(ctx)
	if err != nil {
		return toolError(err), nil
	}

	// The notes shape is not on the slide and its identifier is not guessable: it lives on
	// the notes page behind the slide, and only that page says which of its shapes takes
	// the speaker's text.
	presentation, err := client.Presentation(ctx, presentationID,
		"slides(objectId,slideProperties(notesPage(notesProperties(speakerNotesObjectId),"+
			"pageElements(objectId,shape(text(textElements(textRun(content))))))))")
	if err != nil {
		return toolError(err), nil
	}

	notesObjectID, existing := "", ""
	for _, slide := range presentation.Slides {
		if slide.ObjectID != pageObjectID || slide.SlideProperties == nil {
			continue
		}
		notes := slide.SlideProperties.NotesPage
		if notes == nil || notes.NotesProperties == nil {
			break
		}
		notesObjectID = notes.NotesProperties.SpeakerNotesObjectID
		existing = speakerNotes(notes)
	}
	if notesObjectID == "" {
		return toolError(fmt.Errorf("slide %s has no speaker notes shape to write to", pageObjectID)), nil
	}

	var requests []google.Request

	// Deleting from an empty shape is an error of its own, so the existing text is read
	// first and removed only when there is some.
	if existing != "" {
		requests = append(requests, google.Request{
			DeleteText: &google.DeleteTextRequest{ObjectID: notesObjectID, TextRange: google.AllText()},
		})
	}
	if text != "" {
		requests = append(requests, google.Request{
			InsertText: &google.InsertTextRequest{ObjectID: notesObjectID, Text: text},
		})
	}
	if len(requests) == 0 {
		return resultJSON(map[string]any{
			"presentation_id": presentationID,
			"page_object_id":  pageObjectID,
			"notes_object_id": notesObjectID,
			"replies":         0,
		})
	}

	response, err := client.SlidesBatchUpdate(ctx, presentationID, requests)
	if err != nil {
		return toolError(err), nil
	}

	return resultJSON(map[string]any{
		"presentation_id": response.PresentationID,
		"page_object_id":  pageObjectID,
		"notes_object_id": notesObjectID,
		"replies":         len(response.Replies),
	})
}
