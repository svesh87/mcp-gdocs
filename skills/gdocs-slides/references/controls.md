# What can be controlled, and with what

Every knob this server offers, arranged by what it changes on the slide rather than by
which tool it belongs to. Read it as: *what you want to change* → *what reads it* → *what
writes it*.

Everything a reading reports, a writing takes back in the same units. There is no tool that
copies styling from one deck into another: you read numbers, decide, and write numbers.

Units, once: **EMU** for positions and sizes (914400 to the inch, 12700 to the point; a
slide is 9144000 × 5143500), **points** for font sizes and the space around paragraphs,
**percent** for line spacing (100 is single), **UTF-16 code units** for every text range.

## The deck

| To change | Read with | Write with |
|---|---|---|
| which slides exist, in which order | `gdocs_slides_list` | `gdocs_slides_add_slide`, `gdocs_slides_reorder`, `gdocs_slides_delete` |
| a slide kept but not shown | `gdocs_slides_list` (`hidden`), `gdocs_slides_inspect_page` | `gdocs_slides_hide` |
| the deck's palette — twelve colours by name | `gdocs_slides_read_theme` | `gdocs_slides_set_theme_colors` (master only, all twelve at once) |
| what every slide on a layout looks like | `gdocs_slides_read_theme`, `gdocs_slides_list_layouts` | `gdocs_slides_style_layout` |
| a new deck from a template | `gdocs_drive_search`, `gdocs_drive_file_info` | `gdocs_slides_copy_presentation` |
| the notes behind a slide | `gdocs_slides_inspect_page` (`speaker_notes`) | `gdocs_slides_set_speaker_notes` |

## The slide

| To change | Read with | Write with |
|---|---|---|
| the background: colour, picture, or back to the layout's | `gdocs_slides_inspect_page` (`background`) | `gdocs_slides_set_page_background` |
| what covers what | `gdocs_slides_inspect_page` (`z`) | `gdocs_slides_order_elements` |
| elements that move together | `gdocs_slides_inspect_page` (children of a group) | `gdocs_slides_group` |
| more of a shape the API cannot build | — | `gdocs_slides_duplicate` (same slide only) |

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

## Files

| To do | Tool |
|---|---|
| render the whole deck to pictures | `gdocs_export_slide_images` |
| render one slide to a short-lived address | `gdocs_slides_export_thumbnail` |
| save a deck as PDF, PPTX, ODP … | `gdocs_export_file` |
| bring a .pptx back in as a Google presentation | `gdocs_import_file` |
| look up shapes, bullet presets, arrowheads, dashes, theme colour names, units | `gdocs_reference` |

The exports need the server started with `--files-dir`; importing also needs
`--allow-write`.

## What no tool here does

- **Delete anything outside a presentation.** No files, no folders, no spreadsheet tabs, no
  rows. There is no such tool and there will not be one.
- **Send arbitrary API requests.** Every tool builds its own; a caller cannot hand the
  server a batch.
- **Copy styling from one deck to another in one call.** Read, decide, write.
