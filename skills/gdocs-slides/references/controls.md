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
| a video from YouTube or Drive | `gdocs_slides_add_video` | with autoplay, mute, start and end |
| the description a screen reader reads out | `gdocs_slides_set_alt_text` | nothing else writes it |
| how a connector runs, and rerouting it | `gdocs_slides_route_line` | a connector drawn before its shapes were placed stays where it was drawn until this is called |

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
- **Make a new layout or apply another deck's theme.** The API has neither request. What
  it does have: the colour scheme and the existing layouts of a deck can be rewritten
  (`set_theme_colors`, `style_layout`), and a deck that must look like a sample is started
  as a copy of it.
- **Send arbitrary API requests.** Every tool builds its own; a caller cannot hand the
  server a batch.
- **Copy styling from one deck to another in one call.** Read, decide, write.
