package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// This file turns what a reading reports back into what the API takes.
//
// The two halves are deliberately the same shape: gdocs_docs_read_structure answers with
// "bold": true and "space_above_pt": 18, and every writing tool takes a style object with
// those very keys. Copying a paragraph is then reading one and handing it back, with no
// step in between where a unit or a name has to be guessed.
//
// Every writing request to Docs carries a field mask, and a field the mask does not name
// keeps whatever was there. So the mask is built from the keys the caller actually sent —
// naming a field and leaving it out of the object is how a style gets cleared, and that
// has to stay possible: a copy that cannot say "no indent here" inherits one.

// docsStyleFields is a style ready to send: the value and the mask that says which parts
// of it apply.
type docsStyleFields struct {
	fields []string
}

func (f *docsStyleFields) add(name string) { f.fields = append(f.fields, name) }

// mask is the field mask, sorted so the same call always sends the same bytes — a golden
// file compares whole request bodies, and Go randomises map order deliberately.
func (f *docsStyleFields) mask() string {
	sorted := append([]string(nil), f.fields...)
	sort.Strings(sorted)

	return strings.Join(sorted, ",")
}

// docsTextStyleFrom reads a text style the way a reading reports one.
func docsTextStyleFrom(object map[string]any) (*google.DocsTextStyle, string, error) {
	style := &google.DocsTextStyle{}
	mask := &docsStyleFields{}

	for name, target := range map[string]**bool{
		"bold":          &style.Bold,
		"italic":        &style.Italic,
		"underline":     &style.Underline,
		"strikethrough": &style.Strikethrough,
		"small_caps":    &style.SmallCaps,
	} {
		value, ok, err := fieldBool(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*target = value
			mask.add(docsTextStyleField(name))
		}
	}

	if value, ok, err := fieldString(object, "baseline_offset"); err != nil {
		return nil, "", err
	} else if ok {
		style.BaselineOffset = value
		mask.add("baselineOffset")
	}

	if value, ok, err := fieldFloat(object, "font_size_pt"); err != nil {
		return nil, "", err
	} else if ok {
		style.FontSize = points(*value)
		mask.add("fontSize")
	}

	family, hasFamily, err := fieldString(object, "font_family")
	if err != nil {
		return nil, "", err
	}
	weight, hasWeight, err := fieldFloat(object, "font_weight")
	if err != nil {
		return nil, "", err
	}
	if hasFamily || hasWeight {
		// The weight travels with the family: sending a family alone resets the weight to
		// 400, which is how a bold heading quietly turns regular.
		font := &google.WeightedFontFamily{FontFamily: family, Weight: 400}
		if hasWeight {
			font.Weight = int(*weight)
		}
		style.WeightedFont = font
		mask.add("weightedFontFamily")
	}

	for name, field := range map[string]string{
		"color":            "foregroundColor",
		"background_color": "backgroundColor",
	} {
		colour, ok, err := fieldDocsColor(object, name)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			continue
		}
		if field == "foregroundColor" {
			style.ForegroundColor = colour
		} else {
			style.BackgroundColor = colour
		}
		mask.add(field)
	}

	if value, ok, err := fieldString(object, "link"); err != nil {
		return nil, "", err
	} else if ok {
		if value != "" {
			style.Link = &google.Link{URL: value}
		}
		mask.add("link")
	}

	if len(mask.fields) == 0 {
		return nil, "", fmt.Errorf("the style names nothing: give at least one of bold, italic, " +
			"underline, strikethrough, small_caps, baseline_offset, font_size_pt, font_family, " +
			"font_weight, color, background_color, link")
	}

	return style, mask.mask(), nil
}

// docsTextStyleField is the API's name for one of the reading's keys.
func docsTextStyleField(name string) string {
	if name == "small_caps" {
		return "smallCaps"
	}

	return name
}

// docsParagraphStyleFrom reads a paragraph style the way a reading reports one.
func docsParagraphStyleFrom(object map[string]any) (*google.DocsParagraphStyle, string, error) {
	style := &google.DocsParagraphStyle{}
	mask := &docsStyleFields{}

	// A reading reports heading_id, because that is what a link to a heading points at,
	// and Docs refuses it on the way back: "Unallowed field: headingId". It is assigned
	// when a paragraph becomes a heading, never chosen. Saying so beats a batch that
	// fails with every other field of the style still unwritten.
	if _, ok := object["heading_id"]; ok {
		return nil, "", fmt.Errorf("heading_id cannot be written: Docs assigns it when a paragraph " +
			"becomes a heading. Leave it out — setting named_style is what makes the heading")
	}

	for name, pair := range map[string]struct {
		target *string
		field  string
	}{
		"named_style":  {&style.NamedStyleType, "namedStyleType"},
		"alignment":    {&style.Alignment, "alignment"},
		"direction":    {&style.Direction, "direction"},
		"spacing_mode": {&style.SpacingMode, "spacingMode"},
	} {
		value, ok, err := fieldString(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = value
			mask.add(pair.field)
		}
	}

	if value, ok, err := fieldFloat(object, "line_spacing"); err != nil {
		return nil, "", err
	} else if ok {
		style.LineSpacing = value
		mask.add("lineSpacing")
	}

	for name, pair := range map[string]struct {
		target **google.Dimension
		field  string
	}{
		"space_above_pt":       {&style.SpaceAbove, "spaceAbove"},
		"space_below_pt":       {&style.SpaceBelow, "spaceBelow"},
		"indent_start_pt":      {&style.IndentStart, "indentStart"},
		"indent_end_pt":        {&style.IndentEnd, "indentEnd"},
		"indent_first_line_pt": {&style.IndentFirstLine, "indentFirstLine"},
	} {
		value, ok, err := fieldFloat(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = points(*value)
			mask.add(pair.field)
		}
	}

	for name, pair := range map[string]struct {
		target **bool
		field  string
	}{
		"keep_lines_together":    {&style.KeepLinesTogether, "keepLinesTogether"},
		"keep_with_next":         {&style.KeepWithNext, "keepWithNext"},
		"avoid_widow_and_orphan": {&style.AvoidWidowAndOrphan, "avoidWidowAndOrphan"},
		"page_break_before":      {&style.PageBreakBefore, "pageBreakBefore"},
	} {
		value, ok, err := fieldBool(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = value
			mask.add(pair.field)
		}
	}

	if colour, ok, err := fieldDocsColor(object, "shading_color"); err != nil {
		return nil, "", err
	} else if ok {
		style.Shading = &google.DocsShading{BackgroundColor: colour}
		mask.add("shading")
	}

	for name, target := range map[string]**google.DocsParagraphBorder{
		"border_top":     &style.BorderTop,
		"border_bottom":  &style.BorderBottom,
		"border_left":    &style.BorderLeft,
		"border_right":   &style.BorderRight,
		"border_between": &style.BorderBetween,
	} {
		border, ok, err := docsParagraphBorderFrom(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*target = border
			mask.add(docsBorderField(name))
		}
	}

	if len(mask.fields) == 0 {
		return nil, "", fmt.Errorf("the style names nothing: give at least one of named_style, " +
			"alignment, direction, spacing_mode, line_spacing, space_above_pt, space_below_pt, " +
			"indent_start_pt, indent_end_pt, indent_first_line_pt, keep_lines_together, " +
			"keep_with_next, avoid_widow_and_orphan, page_break_before, shading_color, border_top, " +
			"border_bottom, border_left, border_right, border_between")
	}

	return style, mask.mask(), nil
}

// docsBorderField is the API's name for a border key.
func docsBorderField(name string) string {
	switch name {
	case "border_top":
		return "borderTop"
	case "border_bottom":
		return "borderBottom"
	case "border_left":
		return "borderLeft"
	case "border_right":
		return "borderRight"
	default:
		return "borderBetween"
	}
}

func docsParagraphBorderFrom(object map[string]any, name string) (*google.DocsParagraphBorder, bool, error) {
	raw, ok := object[name]
	if !ok {
		return nil, false, nil
	}

	// An explicit null is how a border is taken off: the field is named in the mask and
	// sent empty. A copy needs it — a sample without a frame has to be able to say so.
	if raw == nil {
		return nil, true, nil
	}

	fields, ok := raw.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("%s must be an object like {\"color\": \"#B7B7B7\", \"width_pt\": 1, \"dash_style\": \"SOLID\"}", name)
	}

	border := &google.DocsParagraphBorder{}
	if colour, ok, err := fieldDocsColor(fields, "color"); err != nil {
		return nil, false, err
	} else if ok {
		border.Color = colour
	}
	if value, ok, err := fieldFloat(fields, "width_pt"); err != nil {
		return nil, false, err
	} else if ok {
		border.Width = points(*value)
	}
	if value, ok, err := fieldFloat(fields, "padding_pt"); err != nil {
		return nil, false, err
	} else if ok {
		border.Padding = points(*value)
	}
	if value, ok, err := fieldString(fields, "dash_style"); err != nil {
		return nil, false, err
	} else if ok {
		border.DashStyle = value
	}

	return border, true, nil
}

// docsCellStyleFrom reads a table cell's style the way a reading reports one.
func docsCellStyleFrom(object map[string]any) (*google.DocsTableCellStyle, string, error) {
	style := &google.DocsTableCellStyle{}
	mask := &docsStyleFields{}

	if colour, ok, err := fieldDocsColor(object, "background_color"); err != nil {
		return nil, "", err
	} else if ok {
		style.BackgroundColor = colour
		mask.add("backgroundColor")
	}

	if value, ok, err := fieldString(object, "content_alignment"); err != nil {
		return nil, "", err
	} else if ok {
		style.ContentAlignment = value
		mask.add("contentAlignment")
	}

	for name, pair := range map[string]struct {
		target **google.Dimension
		field  string
	}{
		"padding_top_pt":    {&style.PaddingTop, "paddingTop"},
		"padding_bottom_pt": {&style.PaddingBottom, "paddingBottom"},
		"padding_left_pt":   {&style.PaddingLeft, "paddingLeft"},
		"padding_right_pt":  {&style.PaddingRight, "paddingRight"},
	} {
		value, ok, err := fieldFloat(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = points(*value)
			mask.add(pair.field)
		}
	}

	for name, target := range map[string]**google.DocsTableCellBorder{
		"border_top":    &style.BorderTop,
		"border_bottom": &style.BorderBottom,
		"border_left":   &style.BorderLeft,
		"border_right":  &style.BorderRight,
	} {
		raw, ok := object[name]
		if !ok {
			continue
		}
		mask.add(docsBorderField(name))
		if raw == nil {
			continue
		}
		fields, ok := raw.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("%s must be an object like {\"color\": \"#B7B7B7\", \"width_pt\": 0, \"dash_style\": \"DOT\"}", name)
		}

		border := &google.DocsTableCellBorder{}
		if colour, ok, err := fieldDocsColor(fields, "color"); err != nil {
			return nil, "", err
		} else if ok {
			border.Color = colour
		}
		if value, ok, err := fieldFloat(fields, "width_pt"); err != nil {
			return nil, "", err
		} else if ok {
			border.Width = points(*value)
		}
		if value, ok, err := fieldString(fields, "dash_style"); err != nil {
			return nil, "", err
		} else if ok {
			border.DashStyle = value
		}
		*target = border
	}

	if len(mask.fields) == 0 {
		return nil, "", fmt.Errorf("the cell style names nothing: give at least one of " +
			"background_color, content_alignment, padding_top_pt, padding_bottom_pt, " +
			"padding_left_pt, padding_right_pt, border_top, border_bottom, border_left, border_right")
	}

	return style, mask.mask(), nil
}

// docsSectionStyleFrom reads a section's page setup the way a reading reports one.
func docsSectionStyleFrom(object map[string]any) (*google.DocsSectionStyle, string, error) {
	style := &google.DocsSectionStyle{}
	mask := &docsStyleFields{}

	for name, pair := range map[string]struct {
		target *string
		field  string
	}{
		"column_separator":     {&style.ColumnSeparatorStyle, "columnSeparatorStyle"},
		"direction":            {&style.ContentDirection, "contentDirection"},
		"default_header_id":    {&style.DefaultHeaderID, "defaultHeaderId"},
		"default_footer_id":    {&style.DefaultFooterID, "defaultFooterId"},
		"first_page_header_id": {&style.FirstPageHeaderID, "firstPageHeaderId"},
		"first_page_footer_id": {&style.FirstPageFooterID, "firstPageFooterId"},
		"even_page_header_id":  {&style.EvenPageHeaderID, "evenPageHeaderId"},
		"even_page_footer_id":  {&style.EvenPageFooterID, "evenPageFooterId"},
	} {
		value, ok, err := fieldString(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = value
			mask.add(pair.field)
		}
	}

	for name, pair := range map[string]struct {
		target **google.Dimension
		field  string
	}{
		"margin_top_pt":    {&style.MarginTop, "marginTop"},
		"margin_bottom_pt": {&style.MarginBottom, "marginBottom"},
		"margin_left_pt":   {&style.MarginLeft, "marginLeft"},
		"margin_right_pt":  {&style.MarginRight, "marginRight"},
		"margin_header_pt": {&style.MarginHeader, "marginHeader"},
		"margin_footer_pt": {&style.MarginFooter, "marginFooter"},
	} {
		value, ok, err := fieldFloat(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = points(*value)
			mask.add(pair.field)
		}
	}

	for name, pair := range map[string]struct {
		target **bool
		field  string
	}{
		"use_first_page_header_footer": {&style.UseFirstPageHeaderFoot, "useFirstPageHeaderFooter"},
		"flip_page_orientation":        {&style.FlipPageOrientation, "flipPageOrientation"},
	} {
		value, ok, err := fieldBool(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = value
			mask.add(pair.field)
		}
	}

	if value, ok, err := fieldFloat(object, "page_number_start"); err != nil {
		return nil, "", err
	} else if ok {
		start := int(*value)
		style.PageNumberStart = &start
		mask.add("pageNumberStart")
	}

	if len(mask.fields) == 0 {
		return nil, "", fmt.Errorf("the section style names nothing: give at least one of " +
			"column_separator, direction, margin_top_pt, margin_bottom_pt, margin_left_pt, " +
			"margin_right_pt, margin_header_pt, margin_footer_pt, default_header_id, " +
			"default_footer_id, first_page_header_id, first_page_footer_id, even_page_header_id, " +
			"even_page_footer_id, use_first_page_header_footer, flip_page_orientation, " +
			"page_number_start")
	}

	return style, mask.mask(), nil
}

// docsDocumentStyleFrom reads the page setup of a whole document.
func docsDocumentStyleFrom(object map[string]any) (*google.DocsDocumentStyle, string, error) {
	style := &google.DocsDocumentStyle{}
	mask := &docsStyleFields{}

	if raw, ok := object["page_size"]; ok {
		fields, ok := raw.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("page_size must be an object like {\"width_pt\": 595.28, \"height_pt\": 841.89}")
		}
		size := &google.DocsSize{}
		if value, ok, err := fieldFloat(fields, "width_pt"); err != nil {
			return nil, "", err
		} else if ok {
			size.Width = points(*value)
		}
		if value, ok, err := fieldFloat(fields, "height_pt"); err != nil {
			return nil, "", err
		} else if ok {
			size.Height = points(*value)
		}
		style.PageSize = size
		mask.add("pageSize")
	}

	// Two fields a reading reports and this request will not take. Both are refusals from
	// Google, checked against a live document rather than guessed: a page with no colour
	// of its own reads back as a transparent background, and the margins flag is derived.
	if _, ok := object["use_custom_header_footer_margins"]; ok {
		return nil, "", fmt.Errorf("use_custom_header_footer_margins cannot be written: Docs " +
			"answers \"Unallowed field\". It follows from the header and footer margins themselves — " +
			"set margin_header_pt and margin_footer_pt and it takes care of itself")
	}

	if colour, ok, err := fieldDocsColor(object, "background_color"); err != nil {
		return nil, "", err
	} else if ok {
		if colour.Color == nil || colour.Color.RGBColor == nil {
			return nil, "", fmt.Errorf("a document's background cannot be set to \"none\": Docs " +
				"answers \"Cannot set a transparent background\". A page with no colour of its own " +
				"reads back as none and is already what a new document has — leave the field out")
		}
		style.Background = &google.DocsBackground{Color: colour}
		mask.add("background")
	}

	for name, pair := range map[string]struct {
		target **google.Dimension
		field  string
	}{
		"margin_top_pt":    {&style.MarginTop, "marginTop"},
		"margin_bottom_pt": {&style.MarginBottom, "marginBottom"},
		"margin_left_pt":   {&style.MarginLeft, "marginLeft"},
		"margin_right_pt":  {&style.MarginRight, "marginRight"},
		"margin_header_pt": {&style.MarginHeader, "marginHeader"},
		"margin_footer_pt": {&style.MarginFooter, "marginFooter"},
	} {
		value, ok, err := fieldFloat(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = points(*value)
			mask.add(pair.field)
		}
	}

	for name, pair := range map[string]struct {
		target **bool
		field  string
	}{
		"use_custom_header_footer_margins": {&style.UseCustomHeaderFooter, "useCustomHeaderFooterMargins"},
		"use_first_page_header_footer":     {&style.UseFirstPageHeaderFoot, "useFirstPageHeaderFooter"},
		"use_even_page_header_footer":      {&style.UseEvenPageHeaderFoot, "useEvenPageHeaderFooter"},
		"flip_page_orientation":            {&style.FlipPageOrientation, "flipPageOrientation"},
	} {
		value, ok, err := fieldBool(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = value
			mask.add(pair.field)
		}
	}

	for name, pair := range map[string]struct {
		target *string
		field  string
	}{
		"default_header_id":    {&style.DefaultHeaderID, "defaultHeaderId"},
		"default_footer_id":    {&style.DefaultFooterID, "defaultFooterId"},
		"first_page_header_id": {&style.FirstPageHeaderID, "firstPageHeaderId"},
		"first_page_footer_id": {&style.FirstPageFooterID, "firstPageFooterId"},
		"even_page_header_id":  {&style.EvenPageHeaderID, "evenPageHeaderId"},
		"even_page_footer_id":  {&style.EvenPageFooterID, "evenPageFooterId"},
	} {
		value, ok, err := fieldString(object, name)
		if err != nil {
			return nil, "", err
		}
		if ok {
			*pair.target = value
			mask.add(pair.field)
		}
	}

	if value, ok, err := fieldFloat(object, "page_number_start"); err != nil {
		return nil, "", err
	} else if ok {
		start := int(*value)
		style.PageNumberStart = &start
		mask.add("pageNumberStart")
	}

	if len(mask.fields) == 0 {
		return nil, "", fmt.Errorf("the document style names nothing: give at least one of page_size, " +
			"background_color, margin_top_pt, margin_bottom_pt, margin_left_pt, margin_right_pt, " +
			"margin_header_pt, margin_footer_pt, use_custom_header_footer_margins, " +
			"use_first_page_header_footer, use_even_page_header_footer, flip_page_orientation, " +
			"default_header_id, default_footer_id, first_page_header_id, first_page_footer_id, " +
			"even_page_header_id, even_page_footer_id, page_number_start")
	}

	return style, mask.mask(), nil
}

// points is a size in the only unit Docs has.
func points(value float64) *google.Dimension {
	return &google.Dimension{Magnitude: value, Unit: "PT"}
}

// fieldBool reads a boolean out of a decoded object, saying whether it was there at all.
func fieldBool(object map[string]any, name string) (*bool, bool, error) {
	raw, ok := object[name]
	if !ok {
		return nil, false, nil
	}
	if raw == nil {
		return nil, true, nil
	}

	value, ok := raw.(bool)
	if !ok {
		return nil, false, fmt.Errorf("%s must be true or false, got %T", name, raw)
	}

	return &value, true, nil
}

// fieldFloat reads a number, which arrives as a float from JSON and as an int from a
// client that decoded it itself.
func fieldFloat(object map[string]any, name string) (*float64, bool, error) {
	raw, ok := object[name]
	if !ok {
		return nil, false, nil
	}
	if raw == nil {
		return nil, true, nil
	}

	switch value := raw.(type) {
	case float64:
		return &value, true, nil
	case float32:
		converted := float64(value)
		return &converted, true, nil
	case int:
		converted := float64(value)
		return &converted, true, nil
	case int64:
		converted := float64(value)
		return &converted, true, nil
	default:
		return nil, false, fmt.Errorf("%s must be a number, got %T", name, raw)
	}
}

// fieldString reads a string. An empty one is a value, not an absence: it is how a link
// is taken off.
func fieldString(object map[string]any, name string) (string, bool, error) {
	raw, ok := object[name]
	if !ok {
		return "", false, nil
	}
	if raw == nil {
		return "", true, nil
	}

	value, ok := raw.(string)
	if !ok {
		return "", false, fmt.Errorf("%s must be a string, got %T", name, raw)
	}

	return strings.TrimSpace(value), true, nil
}

// fieldDocsColor reads a colour the way a reading reports one: "#RRGGBB" for a colour,
// "none" for a colour that is deliberately absent — a transparent background — and an
// explicit null to hand the field back to whatever the document inherits.
func fieldDocsColor(object map[string]any, name string) (*google.DocsColor, bool, error) {
	text, ok, err := fieldString(object, name)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	if text == "" || strings.EqualFold(text, "none") {
		return &google.DocsColor{}, true, nil
	}

	rgb, err := parseHexColor(text)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", name, err)
	}

	return &google.DocsColor{Color: &google.DocsColorValue{RGBColor: rgb}}, true, nil
}
