# What can be controlled in a document, and with what

Every size is in **points**. Docs has one unit and states it on every dimension, so unlike
Slides there is nothing to convert and nothing to get wrong — a number read as 56.7 goes
back as 56.7.

Positions are **character indexes**, counted in UTF-16 code units. The body's own text
starts at index 1; a header, a footer or a footnote starts at 0.

Colours read and write as `"#RRGGBB"`, or `"none"` for a colour that is deliberately
absent. A field the reading leaves out altogether is one the document does not set.

## The page

| What | Read with | Write with | Notes |
|---|---|---|---|
| paper size, margins | `read_structure` → `document_style` | `style_document` | `page_size {width_pt, height_pt}` |
| header and footer margins | same | same | `margin_header_pt`, `margin_footer_pt` |
| page background | same | same | a colour only — `"none"` is refused |
| first page has its own | same | — | reported, not settable: see pitfalls |
| page number start, orientation flip | same | `style_document` | |
| which header/footer the document uses | same | `style_document` | identifiers from `add_header_footer` |

## Sections

A section is what carries page setup and its own header and footer. A document whose
second half has a different footer is a document with a section break in the middle.

| What | Read with | Write with |
|---|---|---|
| the break itself | `read_structure` → element `section_break` | `insert_section_break` (`NEXT_PAGE`, `CONTINUOUS`) |
| margins of one section | element `style` | `style_section` |
| columns | `columns` | `style_section` → `column_properties` |
| its header / footer | `default_header_id`, `default_footer_id` | `add_header_footer` with `section_break_index`, then `style_section` |

## Paragraphs

| What | Read with | Write with |
|---|---|---|
| named style (NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1…6) | `style.named_style` | `style_paragraph` |
| what a named style *means* in this document | `named_styles` | `style_named` |
| alignment, direction, spacing mode | `style` | `style_paragraph` |
| line spacing, space above / below | `line_spacing`, `space_*_pt` | `style_paragraph` |
| indents | `indent_start_pt`, `indent_end_pt`, `indent_first_line_pt` | `style_paragraph` |
| borders on any of four sides, and between | `border_top` … `border_between` | `style_paragraph`, `null` takes one off |
| background of the paragraph | `shading_color` | `style_paragraph` |
| keep together, keep with next, widow control, page break before | booleans of the same name | `style_paragraph` |
| heading anchor | `heading_id` | **read only** — Docs assigns it |

## Runs of text

| What | Read with | Write with |
|---|---|---|
| bold, italic, underline, strikethrough, small caps | `runs[].style` | `style_text` |
| size, family, weight | `font_size_pt`, `font_family`, `font_weight` | `style_text` — weight travels with family |
| colours | `color`, `background_color` | `style_text` |
| link | `link` | `style_text`, empty string removes it |
| superscript / subscript | `baseline_offset` | `style_text` |

## Lists

| What | Read with | Write with |
|---|---|---|
| that a paragraph is a list item | `bullet.list_id`, `bullet.nesting_level` | `make_bullets` over the range |
| the glyphs of each level | `lists[id].levels[].glyph_symbol` … | **preset only** — pick the preset whose glyphs match |
| taking a list apart | — | `delete` with `what: "bullets"` — keeps the text |

Depth comes from tab characters at the start of the text, not from an indent.

## Tables

| What | Read with | Write with |
|---|---|---|
| the table | element `table`, `rows`, `columns` | `insert_table` |
| column widths | `column_properties[].width_pt` | `style_table` → `column_widths` (sent as FIXED_WIDTH) |
| row height, header row | `row_data[].style` | `style_table` → `row_heights` |
| cell fill, padding, vertical alignment | `row_data[].cells[].style` | `style_table` → `cells` |
| cell borders | `border_top` … `border_right` | `style_table` → `cells`; state the absent ones as width 0 |
| merged cells | `row_span`, `column_span` | `style_table` → `merge` |
| repeating header rows | — | `style_table` → `pin_header_rows` |
| a row or a column out | — | `delete` with `what: "row"` / `"column"` |

Cell text is written like any other text, at the indexes a reading gives for the cell —
fill a table from its **last** cell backwards, or every insertion moves the next one.

## Headers, footers, footnotes

| What | Read with | Write with |
|---|---|---|
| their content | `headers` / `footers` / `footnotes`, keyed by identifier | any text tool with `segment_id` |
| making one | — | `add_header_footer` (one per section, DEFAULT only) |
| a footnote | `footnotes` | `insert_footnote`, then write into the segment it returns |
| removing one | — | `delete` with `header_id` / `footer_id` |

## Pictures and drawings

| What | Read with | Write with |
|---|---|---|
| a picture in the line of text | `inline_objects[id]`, `kind: "image"` | `insert_image` with a URI Google can fetch |
| its size | `size.width_pt`, `size.height_pt` | `insert_image` — a wish, the ratio is kept |
| a picture from another document | `content_uri` | `insert_image` — the address is signed and expires |
| its margins, border, crop | reported | **nothing writes them** |
| a floating picture | `positioned_objects[id]` | **nothing creates one** — ten refusals in the pitfalls |
| a Google drawing | `kind: "drawing"` | **nothing reads or creates one** |
| removing either | — | `delete` with a range, or `positioned_object_id` |

## Removal, and where it stops

`gdocs_docs_delete` reaches a range, a table row or column, a header, a footer, a floating
object, and the bullets of a list — one per call. It reaches nothing outside the document:
no file, no folder, no drive, and no other document. That half of the rule has not moved.
