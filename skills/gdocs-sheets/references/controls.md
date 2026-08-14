# What can be controlled in a workbook

Arranged by what it changes. Every line is a pair: what reports it, what writes it.

**A tool that is missing was switched off, not forgotten.** The server is started with a
set of groups — `sheets-read`, `sheets-write`, `sheets-delete` — and removal is never in
the default set. A name absent from the listing is the configuration talking; do what can
be done and say what could not.

**Units.** Widths and heights are in **pixels** — the number the interface shows. Colours
go in as `#RRGGBB` or `{"red": 0..1, "green": 0..1, "blue": 0..1}`, and always come back as
`#RRGGBB`. Rows and columns are counted from 0, and the end of a range is exclusive: one
cell at the top left is start 0/0, end 1/1. A1 ranges quote a tab whose name has a space or
a quote: `'Лист 1'!A1:F20`.

## The workbook

| What | Read | Write |
|---|---|---|
| title, tabs, their identifiers and sizes | `gdocs_sheets_info` | `gdocs_sheets_create`, `gdocs_sheets_add_tab` |
| locale, time zone | `gdocs_sheets_info` | at creation, or `gdocs_sheets_update_properties` |
| named ranges | `gdocs_sheets_info` (`named_ranges`) | `gdocs_sheets_add_named_range` |
| a copy of a whole tab | — | `gdocs_sheets_duplicate_tab` (inside one workbook) |
| what nothing here writes: charts read back, slicers, data sources | `gdocs_sheets_info` counts them per tab | — |

The locale is not decoration: with `USER_ENTERED` it decides how typed text is read.

## The tab

| What | Read | Write |
|---|---|---|
| size of the grid | `gdocs_sheets_info`, `gdocs_sheets_read_format` | at creation; `gdocs_sheets_set_layout` to grow; `gdocs_sheets_insert_dimensions` to add in the middle |
| frozen rows and columns | both readings | `gdocs_sheets_set_layout` |
| column widths, row heights | `gdocs_sheets_read_format` | `gdocs_sheets_set_layout` (`column_widths`, `row_heights` with `through_row`) |
| columns fitted to their contents | — | `gdocs_sheets_set_layout` (`auto_resize_columns`) |
| merged cells | `gdocs_sheets_read_format` (`merges`) | `gdocs_sheets_set_layout` (`merge`, `unmerge`) |
| hidden rows and columns | `gdocs_sheets_read_format` (`hidden_rows`, `hidden_columns`) | `gdocs_sheets_set_layout` (`hide_rows`, `hide_columns`, with `hidden` false to show) |
| tab colour | `gdocs_sheets_info` (`tab_color`) | `gdocs_sheets_set_layout` (`tab_color`) |
| rows or columns moved | — | `gdocs_sheets_move_dimensions` |
| groups that fold up | `gdocs_sheets_read_format` (`row_groups`, `column_groups`) | `gdocs_sheets_group_dimensions`, `gdocs_sheets_collapse_group` |
| the filter and its saved views | `gdocs_sheets_read_format` (`basic_filter`, `filter_views`) | `gdocs_sheets_set_filter` |
| protection | `gdocs_sheets_read_format` (`protected_ranges`) | `gdocs_sheets_protect_range` |
| alternating stripes | `gdocs_sheets_read_format` (`bandings`) | `gdocs_sheets_set_banding` |
| rules that colour by content | `gdocs_sheets_read_format` (`conditional_formats`) | `gdocs_sheets_set_conditional_format` |

The grid only grows. Making it smaller deletes rows, which this server does not do — the
refusal says to ask for the size at creation, and that is the answer.

## The values

| What | Read | Write |
|---|---|---|
| as a person sees them | `gdocs_sheets_read` (`FORMATTED_VALUE`, the default) | `gdocs_sheets_write` |
| as they are stored | `gdocs_sheets_read` (`UNFORMATTED_VALUE`) | `gdocs_sheets_write` |
| the formulas | `gdocs_sheets_read` (`FORMULA`) | `gdocs_sheets_write` with `USER_ENTERED` |
| rows after the last one used | — | `gdocs_sheets_append` (inserts, never overwrites what is below) |
| a series carried on | — | `gdocs_sheets_auto_fill` |
| text replaced across a range, a tab or the workbook | — | `gdocs_sheets_find_replace` (with `regex`) |
| spaces trimmed off both ends | — | `gdocs_sheets_trim_whitespace` |
| one column split into several | — | `gdocs_sheets_split_column` |
| a rectangle sorted | — | `gdocs_sheets_sort_range` |

`USER_ENTERED` reads what is written the way typing it would: formulas become formulas,
`30%` becomes 0.3 with a percent format. `RAW` stores the characters.

## The look of a cell

All of it is read by `gdocs_sheets_read_format` and written by `gdocs_sheets_format_cells`
over a rectangle.

| What | Reported as | Written as |
|---|---|---|
| weight and shape | `bold`, `italic`, `underline`, `strikethrough` | the same, as booleans |
| font | `font_family`, `font_size` | `font_family`, `font_size` |
| colours | `background`, `text_color` | `background_color`, `text_color` |
| alignment | `alignment`, `vertical_alignment` | `horizontal_alignment` (LEFT/CENTER/RIGHT), `vertical_alignment` (TOP/MIDDLE/BOTTOM) |
| wrapping | `wrap` | `wrap` (WRAP / OVERFLOW_CELL / CLIP) |
| number format | `number_type`, `number_pattern` | `number_type`, `number_format` |
| turned or stacked text | `rotation_angle`, `vertical_text` | `rotation_angle` (-90…90), `vertical_text` |
| room inside the cell | `padding` | `padding` |
| the cell's link | `link` | `link` |
| how a link is drawn | `link_display` | `link_display` (LINKED / PLAIN_TEXT) |
| the hover note | `note` | `note` |
| borders | `borders` | `gdocs_sheets_set_borders` |
| style that changes partway through the text | `runs` | `gdocs_sheets_set_text_runs` |

Positions in the reading are the **sheet's own** row and column numbers, whatever range was
asked for; `first_row` and `first_column` say where the rectangle starts.

## Borders

`gdocs_sheets_set_borders` draws the edges of a rectangle and the lines inside it, by
naming `sides`: `top`, `bottom`, `left`, `right`, `inner_horizontal`, `inner_vertical`, or
`all`. The style is `SOLID`, `SOLID_MEDIUM`, `SOLID_THICK`, `DASHED`, `DOTTED`, `DOUBLE` or
`NONE`. `NONE` takes a line away; it removes a border, never a cell.

## Dropdowns

| What | Read | Write |
|---|---|---|
| the rule, gathered into rectangles | `gdocs_sheets_read_format` (`validations`) | `gdocs_sheets_set_validation` |
| the colours its options are drawn in | `gdocs_sheets_read_dropdown_colors` | **nothing** — see [pitfalls](pitfalls.md#a-dropdowns-colours-and-its-pill-shape-are-not-in-the-api) |
| the pill shape, as a column of a table | `gdocs_sheets_info` (`tables`) | `gdocs_sheets_add_table`, `type: DROPDOWN` |

A rule reports its rectangle, its `type`, its `values`, whether it is `strict` and whether
it shows a dropdown arrow — the same names the writing tool takes, so a rule read off a
sample goes back verbatim.

`ONE_OF_LIST` takes the list. `ONE_OF_RANGE` takes exactly one value, the range as a
formula: `=Списки!A2:A9`.

What does **not** go back is how the dropdown looks. `gdocs_sheets_read_dropdown_colors`
reports each option's fill and text colour per column, read out of the editors' HTML
rendering because no API answer carries them; there is nothing to write them back with, and
the two ways around it both fall short in a way worth knowing before choosing. The pitfalls
page has the measurements.

## Rules that colour by content

`gdocs_sheets_set_conditional_format` writes either

- a **condition** — `TEXT_EQ`, `TEXT_CONTAINS`, `NUMBER_GREATER`, `NUMBER_BETWEEN`,
  `DATE_BEFORE`, `BLANK`, `CUSTOM_FORMULA` and the rest — with the look a matching cell
  takes: `background_color`, `text_color`, `bold`, `italic`, `strikethrough`; or
- a **gradient** — two or three points of `{type, color, value}`, where the type is `MIN`,
  `MAX`, `NUMBER`, `PERCENT` or `PERCENTILE`.

Rules are tried in the order the tab holds them and the first match wins, so `index` places
a new one and `replace_index` overwrites one already there.

## Charts

`gdocs_sheets_add_chart` draws `COLUMN`, `BAR`, `LINE`, `AREA`, `STEPPED_AREA`, `SCATTER`
or `PIE` from a rectangle: `labels_column` for what runs along the bottom, `value_columns`
for the numbers, one series each. It floats over the tab at `anchor_row` / `anchor_column`,
or lands on a tab of its own with `own_tab`. `header_rows` decides whether the first row
names the series or is drawn as data.

The chart reads the cells rather than a copy of them, so it follows the numbers.

`gdocs_sheets_update_chart` changes one that exists: where it sits, how big it is, its
frame, its titles. Changing beats recreating — a chart made again loses its place on the
tab and every reference to it from a slide.

## Tables

`gdocs_sheets_add_table` turns a rectangle into one of Sheets' tables: a named block whose
columns have types — `TEXT`, `DOUBLE`, `CURRENCY`, `PERCENT`, `DATE`, `TIME`, `DATE_TIME`,
`BOOLEAN`, `DROPDOWN`. A `DROPDOWN` column with `values` is the modern chip-style list, and
the table's own banding follows rows added to it. Neither is reachable by formatting cells.
Given a `table_id` the same tool changes the table instead of making another one.

## Moving values about

Writing a rectangle cell by cell keeps the values and loses everything else — the
formatting, the validation, the notes, the rules that paint by content. These carry exactly
what they are told to.

| What | Tool | Notes |
|---|---|---|
| copy or move a rectangle | `gdocs_sheets_move_range` | `what` picks what travels: NORMAL, VALUES, FORMAT, FORMULA, DATA_VALIDATION, CONDITIONAL_FORMATTING. A copy repeats itself to fill a larger destination |
| paste delimited text or an HTML table | `gdocs_sheets_paste_text` | the splitting happens on Google's side |
| insert cells and push the rest aside | `gdocs_sheets_shape_range` (`insert_cells`) | not the same as inserting rows: only the rectangle's own columns move |
| shuffle rows | `gdocs_sheets_shape_range` (`randomize`) | |
| add rows after the last filled one | `gdocs_sheets_append_rows` | a string beginning with `=` goes in as a formula |

## Views, slicers and labels

| What | Read with | Write with |
|---|---|---|
| a saved view of a filter, private to whoever opens it | `gdocs_sheets_read_format` reports the tab's own filter | `gdocs_sheets_filter_view` — with an id it changes one, with `duplicate` it copies one |
| a slicer, the control a reader clicks | `gdocs_sheets_info` counts them | `gdocs_sheets_slicer` |
| labels that move with the row they are on | `gdocs_sheets_list_metadata` | `gdocs_sheets_set_metadata` |

A filter hides rows for everyone in the workbook; a filter view only for whoever opens it.
A label is the one way of remembering "the totals are on this row" that survives somebody
inserting a line above it — a row number does not.

## Removal

`gdocs_sheets_delete` takes out one thing per call: `rows`, `columns`, `cells` (with the
rest shifted up or left), `tab`, `group`, `banding`, `conditional_format`, `protection`,
`named_range`, `filter_view`, `duplicates`, `chart`, `table`, `metadata`.

It exists because building is not one-shot, and it is off unless the server was started
with `sheets-delete`. There is no undo: take the indexes from a reading made after the last
edit. Two of the targets leave the values alone and remove only the wrapper — `named_range`
and `table` — and the answer says so.

## Files

| What | Tool |
|---|---|
| export a workbook to pdf, xlsx, ods, csv | `gdocs_drive_export_file` |
| save a picture, a PDF or an archive off the drive as it is | `gdocs_drive_download_file` |
| find a workbook by name | `gdocs_drive_search` |
| copy a whole workbook as a starting point | `gdocs_drive_copy` |
| put a workbook in the bin | `gdocs_drive_delete_to_trash`, only with `drive-delete` |

## What these tools do not do

- **Delete a file outright.** A workbook goes as far as the bin and no further; nothing
  here empties it.
- **Take arbitrary API requests.** Every tool builds its own.
- **Copy anything from one workbook into another.** The API can copy a tab across, and this
  server does not: what it writes has to be something a caller decided and could name.
- **Reach a data source.** The five BigQuery requests are deliberately absent.
