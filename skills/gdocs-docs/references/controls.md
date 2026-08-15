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

## Bringing content in from another document

The Docs API has no copying request of any kind — forty-odd request types, not one of them
reaching outside the document being edited — so `gdocs_docs_copy_range` reads the source and
writes it again. It is in the `docs-copy` group, which the default set offers and
`--tools=docs` does not.

What crosses: paragraphs with their named style, alignment, indents and spacing; every run
with its font, size, weight, colour and link; bulleted and numbered lists, made from the
text rather than typed; inline pictures, by address; page breaks.

What does not, and is named in `not_carried` instead:

| Left behind | Why | What to do |
|---|---|---|
| a table | its cells only get indices once it exists, and those are not predictable from the request that made it | `insert_table`, then `edit_table` |
| a section break | it carries page setup and its own headers | `insert_section_break`, then `style_section` |
| a person or file chip | it is a live object, not text | `insert_chip` |
| a horizontal rule | the API has no request for one | — |
| a drawing | the API reports it with no address at all, so there is nothing to fetch | a person pastes it |

Two more bring content in from the other kinds of document:

| To do | Tool | Notes |
|---|---|---|
| a rectangle of a workbook as a real table | `gdocs_docs_copy_table_from_sheets` | values **as shown**, with each cell's weight, colour, alignment and fill |
| a picture of a slide | `gdocs_docs_copy_slide_image` | a snapshot; it stops following the slide the moment it is taken |

The table is made in two passes and cannot be otherwise: a table's cells have no indices until
the table exists, and those indices are not predictable from the request that made it. So the
table is inserted, the document is read back, and the cells are filled — from the last one
backwards, because every insertion moves everything after it.

Indices are the document's own, as `read_structure` reports them, and they count **UTF-16
code units**. Every style the copy writes names a range in the *target's* coordinates, not
the source's — that arithmetic is most of what the tool does, and it is why a range cannot
simply be replayed. Without a `target_index` the copy lands at the end of the body, which is
one index before the segment's end: Docs keeps a final newline nothing may be inserted after.

The address of a picture is signed and lives about thirty minutes, so read and write in one
pass.

```
gdocs_docs_copy_range
  source_document_id: 1nUu…  start_index: 1240  end_index: 1980
  target_document_id: 1cIX…
→ {"paragraphs": 7, "characters": 740,
   "not_carried": ["a table of 4 by 3, which has to be made separately …"]}
```

## Pointing at a place that does not move

Every index in this file shifts when anything is inserted before it. A **named range** does
not: it is attached to the text rather than to a number, and it survives the edits that
make every index stale. For a template filled more than once, this is the difference
between a fill that works and one that works today.

| What | Read with | Write with |
|---|---|---|
| the names and where they are | `list_named_ranges` | `add_named_range` over a range |
| the text under a name | — | `fill_named_range`, no indexes involved |
| forgetting a name | — | `delete` with `named_range`; the text stays |

## Tabs

A document can hold several tabs, the pages in the editor's left rail.

| What | Read with | Write with |
|---|---|---|
| that they exist | `read_structure` reports the body of the first one | `add_tab`, with a parent to nest it |
| name, position, icon | — | `update_tab` |
| removing one | — | `delete` with `tab_id`, and everything on it goes |

## Chips, and replacing a picture

| What | Write with | Notes |
|---|---|---|
| a person chip | `insert_chip` kind=person | stays live, like typing @ in the editor |
| a chip for another Google file | `insert_chip` kind=file | |
| a date chip | `insert_chip` kind=date | needs an RFC 3339 timestamp |
| swapping a picture's content | `replace_image` | keeps its place and size — the only way to change a picture already in a document |

## Removal, and where it stops

`gdocs_docs_delete` reaches a range, a table row or column, a header, a footer, a floating
object, and the bullets of a list — one per call. It reaches nothing outside the document:
no file, no folder, no drive, and no other document. That half of the rule has not moved.

A whole tab is a second switch: `tab_id` takes the tab and everything on it, and needs
`docs-delete-tab` as well as `docs-delete`. The refusal names the group, so a server that was
started narrowly says so rather than looking broken.

Going back further than one edit is `gdocs_drive_restore_revision`, and for a document it is
not free: Drive has no restore request, so the version is exported as DOCX and written back,
which loses comments, chips and drawings. It refuses without `confirm_conversion` and lists
what the round trip costs. The browser's own version history restores without a conversion —
when that matters, that is the answer.
