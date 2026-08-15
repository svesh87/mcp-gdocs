# What can be controlled, and with what

Every knob this server offers, arranged by what it changes on the slide rather than by
which tool it belongs to. Read it as: *what you want to change* → *what reads it* → *what
writes it*.

Everything a reading reports, a writing takes back in the same units. There is no tool that
copies styling from one deck into another: you read numbers, decide, and write numbers.

**A tool that is missing was switched off, not forgotten.** The server is started with a
set of groups — `slides-read`, `slides-write`, `slides-delete` — and removal is never in
the default set. A name absent from the listing is the configuration talking; work with
what is there and say what could not be done, rather than looking for a way around.

Units, once: **EMU** for positions and sizes (914400 to the inch, 12700 to the point; a
slide is 9144000 × 5143500), **points** for font sizes and the space around paragraphs,
**percent** for line spacing (100 is single), **UTF-16 code units** for every text range.

## The deck

| To change | Read with | Write with |
|---|---|---|
| which slides exist, in which order | `gdocs_slides_list` | `gdocs_slides_add_slide`, `gdocs_slides_reorder`, `gdocs_slides_delete` |
| a slide kept but not shown | `gdocs_slides_list` (`hidden`), `gdocs_slides_inspect_page` | `gdocs_slides_hide` |
| the deck's palette — twelve colours by name | `gdocs_slides_read_theme` | `gdocs_slides_set_theme_colors` (master only, all twelve at once) |
| anything painted **from** the palette rather than by value | `inspect_page`, `read_theme`, `inspect_title_style` (`theme_color`) | `style_shape` (`fill_theme_color`, `outline_theme_color`), `style_table` (`theme_color` in `fill` and in `cell_styles`), `set_text_style` and `style_layout` (`theme_color`) |
| what every slide on a layout looks like | `gdocs_slides_read_theme`, `gdocs_slides_list_layouts` | `gdocs_slides_style_layout` |
| a new deck from a template | `gdocs_drive_search`, `gdocs_drive_file_info` | `gdocs_slides_copy_presentation` |
| a new deck with no template behind it | — | `gdocs_slides_create`, then `set_theme_colors` and `style_layout` to build the look |
| where the template puts a title, a body, a slide number | `gdocs_slides_read_theme` | `gdocs_slides_place_element` on the layout's or master's element — one move, every slide that follows it |
| furniture on every slide of a layout: a band, a logo, a rule | `gdocs_slides_read_theme` (shapes only — it does not report pictures on a layout) | `gdocs_slides_create_shape`, `gdocs_slides_insert_image`, `gdocs_slides_set_page_background`, all on the layout's page |
| the notes behind a slide | `gdocs_slides_inspect_page` (`speaker_notes`) | `gdocs_slides_set_speaker_notes` |

## The slide

| To change | Read with | Write with |
|---|---|---|
| the background: colour, picture, or back to the layout's | `gdocs_slides_inspect_page` (`background`) | `gdocs_slides_set_page_background` |
| what covers what | `gdocs_slides_inspect_page` (`z`) | `gdocs_slides_order_elements` |
| elements that move together | `gdocs_slides_inspect_page` (children of a group) | `gdocs_slides_group` |
| more of a shape the API cannot build | — | `gdocs_slides_duplicate` (same slide only) |
| that same shape on other slides | — | `gdocs_slides_duplicate` of the **slide**: a copy carries its elements, so one pasted shape covers the deck |
| a picture that lives on disk | — | `gdocs_drive_import_file`, `gdocs_drive_share` (`anyone`, `reader`), then `insert_image` or `set_page_background` by the download address, then `gdocs_drive_unshare` on the next call |

## An element

| To change | Read with | Write with |
|---|---|---|
| position and size | `gdocs_slides_inspect_page` (`x_emu`, `y_emu`, `width_emu`, `height_emu`) | `gdocs_slides_place_element` |
| rotation, mirroring | `inspect_page` (`rotation_deg`, `flipped_*`) | `place_element` (`rotation_deg`, `flip_horizontally`, `flip_vertically`) |
| alignment against the slide or another element | `inspect_page` | `place_element` (`align`, `valign`, `like_object_id`, `below_object_id`, `left_aligned_with_object_id`) |
| a shape's fill and outline | `inspect_page` (`fill`, `outline`) | `gdocs_slides_style_shape` |
| where text sits vertically inside a shape | `inspect_page` (`content_alignment`) | `style_shape` (`content_alignment`) |
| a picture's crop, transparency, brightness, contrast, border | `inspect_page` (`image`) | `gdocs_slides_style_image` |
| a line's ends, dashes, weight, colour | `inspect_page` (`line`) | `gdocs_slides_create_line`, `gdocs_slides_style_shape` |

New elements: `gdocs_slides_create_shape` (a plain text box without `shape_type`, any
shape with one), `gdocs_slides_create_line`, `gdocs_slides_insert_image`,
`gdocs_slides_create_table_with_text`.

## Text

| To change | Read with | Write with |
|---|---|---|
| the words | `gdocs_slides_inspect_text_structure` | `gdocs_slides_set_text`, `gdocs_slides_set_list` |
| one word, marker or date across the whole deck | `gdocs_slides_list` (to see where it appears) | `gdocs_slides_replace_text` — keeps the styling around it, and reports how many places it changed |
| a list of any depth | `inspect_text_structure` (`nesting_level`) | `set_list` (lines with their levels; `plain_first_line` keeps the first line out of the list) |
| size, font, weight, italics, colour, background, caps, super/subscript | `inspect_text_structure` (paragraph fields and `runs`) | `gdocs_slides_set_text_style` |
| giving a field back to the layout | `inspect_text_structure` (absent field = inherited) | `gdocs_slides_reset_text_style` |
| which stretch a style covers | `runs` with their `start_index`/`end_index` | `scope`: `all`, `title`, a nesting level, `paragraph:N`, or `range` with the indices |
| alignment, line spacing, space above and below, indents | `inspect_text_structure` | `gdocs_slides_set_paragraph_style` |
| whether space above survives beside space below | `spacing_mode` | `set_paragraph_style` (`spacing_mode`) |
| list markers | `bullet_glyph`, `bullet_color`, `bullet_size_pt` | `set_paragraph_style` (`bullet_preset`, `remove_bullets`) — see the pitfalls |
| links inside text | `inspect_text_structure` (`links`) | `gdocs_slides_link_text` |
| the size a title is actually drawn at | `gdocs_slides_inspect_title_style` (`displayed_font_size_pt`, `autofit_font_scale`) | font size directly — shrinking cannot be switched on |

## Tables

| To change | Read with | Write with |
|---|---|---|
| cell text | `gdocs_slides_read_table` | `gdocs_slides_update_table_cells` |
| column widths | `read_table` | `update_table_cells` (`column_widths_emu`) |
| the height of the header row | `read_table` | `gdocs_slides_style_table` (`header_row_height_emu`) |
| merges | `read_table` (`row_span`, `column_span`) | `style_table` (`merge`) |
| cell fills | `read_table` | `style_table` (`fill`) |
| per-cell text style and alignment | `read_table` | `style_table` (`cell_styles`) |
| vertical alignment of cell content | `read_table` | `style_table` (`content_alignment`) |
| the lines of a table | `read_table` | `gdocs_slides_set_table_borders` — by position (ALL, OUTER, INNER, one side), across the whole table or a rectangle |
| more rows or columns after it exists | `read_table` | `gdocs_slides_edit_table` (`insert_rows`, `insert_columns`) |
| taking a merge apart | `read_table` | `gdocs_slides_edit_table` (`unmerge`) |

Borders are the one part of a table not written per cell: a position and a rectangle, so
the frame comes out even instead of showing seams where two cells disagreed.

## Pictures, charts and video

| To do | Tool | Notes |
|---|---|---|
| swap a picture's content, keeping place and crop | `gdocs_slides_replace_image` | the only way to change a picture already on a slide |
| turn every shape whose text matches into a picture | `gdocs_slides_replace_shapes_with_image` | a template marked `{{photo}}` becomes an illustrated deck in one call |
| the same with a chart from a workbook | `gdocs_slides_replace_shapes_with_chart` | |
| a chart from a workbook, placed by hand | `gdocs_slides_add_sheets_chart` | linked, it can be brought up to date later |
| bring a linked chart up to date | `gdocs_slides_refresh_sheets_chart` | a deck built last quarter shows last quarter's numbers until this is called |
| turn a linked chart into a picture | read `content_url` from `gdocs_slides_inspect_page`, then `gdocs_slides_insert_image` | the deck stops depending on the workbook |

**Linked or baked is a decision about the deck's life, not about looks.** A linked chart
follows its workbook, which is right while the numbers are still moving and wrong for a deck
that will be opened in a year or sent outside the company: the reader has no access to the
workbook, and the chart is a broken box. Baking it into a picture cuts that tie.

The order matters, because a picture freezes whatever was there: put the chart in linked,
finish its look, `refresh_sheets_chart`, *then* read `content_url` and insert it as an image,
and only then remove the linked one. Get the order wrong and unfinished styling becomes
indistinguishable from the styling you meant.

One thing the baking does for free: the frame around a chart does not render into the
picture, so a chart whose border was still drawn comes out clean. That is not a reason to
skip `no_border` on a chart that stays linked — it is a reason not to spend time on the
border of one you are about to bake.
| a video from YouTube or Drive | `gdocs_slides_add_video` | with autoplay, mute, start and end |
| the description a screen reader reads out | `gdocs_slides_set_alt_text` | nothing else writes it |
| how a connector runs, and rerouting it | `gdocs_slides_route_line` | a connector drawn before its shapes were placed stays where it was drawn until this is called |

## Bringing content in from another document

Slides has no request that copies anything between presentations — `duplicate` works inside
one deck and nowhere else — so these read the source and build it again in the target. Both
are in the `slides-copy` group, which the default set offers and `--tools=slides` does not.

| To do | Tool | Notes |
|---|---|---|
| a slide from another deck | `gdocs_slides_copy_slide` | content, not theme; the answer names what it could not carry |
| **another copy of a slide in this deck** | `gdocs_slides_copy_slide` with the same id both sides, or `gdocs_slides_duplicate` | Google duplicates it: exact, loses nothing |
| one element from another deck | `gdocs_slides_copy_element` | within one deck `gdocs_slides_duplicate` is cheaper and exact |
| a table from a workbook | `gdocs_slides_copy_table_from_sheets` | values **as shown**, with each cell's font, weight, colour, alignment and fill |
| a stretch of a document | `gdocs_slides_copy_text_from_docs` | paragraphs, runs and list depth into a text box |
| a chart from a workbook | `gdocs_slides_add_sheets_chart` | `chart_id` comes from `gdocs_sheets_info` |
| a picture from another deck or document | `gdocs_slides_insert_image` with the `content_url` a reading gave | |

**Across kinds of document, only three things mean the same thing**: a table is values with a
look per cell, text is paragraphs with a look per run, a picture is an address. The bridges
carry those and name the rest. Two are worth knowing before choosing one:

- `copy_table_from_sheets` brings the values **as they are shown**, not as they were typed. A
  formula on a slide is a formula nobody can evaluate, so what lands is the number it produced
  when the copy was made — and the answer says so, along with any rule that was colouring the
  cells, because a table that stopped reacting to its numbers looks exactly like one that
  never did.
- `copy_text_from_docs` drops the document's indents and keeps its list depth, for the same
  reason a copied slide does: Slides works the indents out from the depth, and sending both
  counts the depth twice.

**Inside one deck, never rebuild.** A slide multiplied within its own presentation — one copy
per metric, one per incident — is duplicated by Google itself and comes across whole,
including everything a rebuild cannot reach: an authored corner radius that `create_shape`
cannot make, a drawing, a group, a chart's link to its workbook, the speaker notes.
`gdocs_slides_copy_slide` does this by itself when source and target are the same deck; the
answer says `"method": "duplicate"`.

Three things decide whether the result is what you wanted when the decks are different.

**The theme does not travel.** There is no request that applies one deck's theme to
another. Anything the sample left to its layout — which is most of a well-built deck —
comes out in the target's fonts and colours. A copied slide lands on the target's layout of
the *same name*, matched by name because the sample's layout identifier means nothing in
another deck; if there is none, the slide is made blank and the answer says so. When the
look has to match, start the target as a copy of the sample deck.

**An address for a picture lives about thirty minutes** and is tagged with the account that
read it. Read and write in one pass. A list of pictures gathered today and rebuilt tomorrow
is a list of dead addresses.

**Read the `not_carried` list.** These tools do not fail when they cannot carry something;
they carry the rest and name the loss. A slide reported as copied with two omissions is not
the same slide. What is never carried: the theme, groups (rebuild the children and call
`gdocs_slides_group`), and drawings, which the API does not describe at all. A placeholder
*is* carried when the target's layout has the same slot — the text goes into the real slot
and inherits its look — and is named as lost when the layout has no such slot, because the
copy is then an ordinary shape with none of the layout's styling.

```
gdocs_slides_copy_slide
  source_presentation_id: 1zm5…       source_page_object_id: p12
  target_presentation_id: 13bX…       insert_at: 4
→ {"page_object_id": "SLIDES_API…", "layout": "Заголовок и текст", "elements": 6,
   "not_carried": ["a group of elements, which has to be rebuilt child by child …"]}
```

## Files

| To do | Tool |
|---|---|
| render the whole deck to pictures | `gdocs_slides_export_images` |
| render one slide to a short-lived address | `gdocs_slides_export_thumbnail` |
| save a deck as PDF, PPTX, ODP … | `gdocs_drive_export_file` |
| save a picture, a PDF or an archive off the drive as it is | `gdocs_drive_download_file` |
| bring a .pptx back in as a Google presentation | `gdocs_drive_import_file` |
| look up shapes, bullet presets, arrowheads, dashes, theme colour names, units | `gdocs_reference` |

The exports need the server started with `--files-dir`; importing also needs
`--allow-write`.

## What no tool here does

- **Delete a file, a folder or a drive.** A presentation can be put in the bin with
  `gdocs_drive_delete_to_trash`, where its owner finds it again for thirty days, and only
  when the server was started with `drive-delete`. Nothing empties that bin.
- **Remove a whole slide with `slides-delete` alone.** That group covers elements on a slide;
  the slide itself needs `slides-delete-page` as well, because a stray shape is a moment's
  work to put back and a slide is an hour's. The refusal names the missing group.
- **Make a new layout or apply another deck's theme.** The API has neither request. What
  it does have: the colour scheme and the existing layouts of a deck can be rewritten
  (`set_theme_colors`, `style_layout`), and a deck that must look like a sample is started
  as a copy of it.
- **Send arbitrary API requests.** Every tool builds its own; a caller cannot hand the
  server a batch.
- **Carry a look across without carrying the content.** There is no "make this slide look
  like that one". Content crosses between decks with the copy tools above; a look crosses
  by starting the target as a copy of the sample, or by reading the sample's numbers and
  writing them. Building a deck that belongs is still read, decide, write — copying a slide
  answers a different question, "bring that exact slide here", and answers it exactly.
