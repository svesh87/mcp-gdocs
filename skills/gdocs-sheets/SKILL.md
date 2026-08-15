---
name: gdocs-sheets
description: Build or edit a Google Sheets workbook, or copy one from a sample, with the mcp-gdocs tools. Triggers - "сделай таблицу", "собери книгу по образцу", "как в той таблице", "поправь лист", "перенеси формат", a workbook from a template, a sheet with dropdowns or a frozen header, checking how a copied tab came out.
---

# Making a workbook that matches

The job is "here is a workbook we keep; make another one like it, with this quarter's
content". So the work is: **read the sample, decide in numbers, write, then compare field
by field.**

No tool answers "make it look like that", and that is deliberate: a look transferred behind
your back hides the numbers — you never learn that the heading row is 68 pixels tall with a
`#EFEFEF` fill, so you cannot decide to keep the height and change the colour. Everything a
reading tool reports, a writing tool takes back in the same units.

Carrying **content** across is a different question and has its own tools:
`gdocs_sheets_copy_sheet` for a whole tab, `gdocs_sheets_copy_range` for a rectangle. Reach
for them when the answer really is "put that there" — last quarter's tab into this year's
workbook — and not when the job is the one above. `references/controls.md` says what each
carries and, more usefully, what it does not.

The failure this skill exists to prevent: a copy with every value in place that behaves
nothing like the sample — free text where the sample has dropdowns, default row heights
where the sample has tall wrapped cells, a grid the wrong size, and a locale that stored
something else than what was typed.

Two references sit beside this file:

- **[references/controls.md](references/controls.md)** — every knob: what reads it, what
  writes it, in which units.
- **[references/pitfalls.md](references/pitfalls.md)** — the traps, each with what it does
  to the sheet and what to do instead, ending with what the API will not report at all.

On a connection that started with only the readings and `gdocs_find_tools`, ask that for the
rest — it answers with each tool's arguments in full, which matters most for
`gdocs_sheets_update_chart`: it writes a chart's whole specification back, so a call built
from a guess at its arguments erases the data the chart draws. A name it added that the
client still does not list is called through `gdocs_call_tool`, by name, with the arguments
as an object.

## 1. Read the sample

1. **`gdocs_sheets_info`** — the tabs, their sizes, their frozen rows, and the count of
   what a copy cannot carry: conditional formats, banded ranges, protected ranges, charts,
   filters. A tab that reports any of those is a tab the copy will differ from, and that
   belongs in the report from the start rather than as a discovery later.
2. **`gdocs_sheets_read`** — the values. `FORMATTED_VALUE` is what a person sees;
   `UNFORMATTED_VALUE` is what is stored; `FORMULA` is what computes them. Read the one
   you are going to write back.
3. **`gdocs_sheets_read_format`** — the range cell by cell, plus the widths, the heights,
   the merges, the dropdowns and the borders.
4. **`gdocs_sheets_read_dropdown_colors`** — only when the sample's dropdowns are coloured,
   and only to know by how much the copy will differ: the colours read here cannot be
   written back anywhere. It renders the whole workbook, so it is a deliberate call rather
   than part of the sweep.

Read the used rectangle plus a handful of rows, then **probe further down**: a tab is
usually formatted wholesale to its last row, and a copy that stops at the content stops
looking alike the moment somebody adds a row.

**Keep each reading small.** This is a description per cell — a thousand rows of thirty
columns is megabytes, and it is the answer, not a file.

## 2. Decide, in numbers

Say what you are about to build: heading row 68 px tall, `#EFEFEF`, bold, middle-aligned,
wrapped; data rows 50 px, Arial, bottom-aligned; column A 262 px; a dropdown of five
statuses over K2:K20; the grid 993 by 31; locale `ru_RU`.

If you cannot say it, you have not read enough.

## 3. Write, in this order

1. **`gdocs_sheets_create`** with the locale, the time zone and the size of every tab.
   Both the locale and the sizes are settable here and nowhere else without loss — a tab
   arrives 1000 by 26, and cutting it down afterwards would delete rows.
2. **Values**, tab by tab.
3. **Formatting**, as non-overlapping rectangles — see below, this is the part that goes
   wrong.
4. **Links, notes and runs** — a cell whose text changes style partway through needs
   `gdocs_sheets_set_text_runs`, which writes the text and the runs together.
5. **Dropdowns**, straight from the rectangles the reading reported.
6. **Rules that colour by content, banding, borders**, then **widths, heights, frozen
   rows, groups, the filter and protection**.
7. **Charts**, last, because they read the cells and the cells have to be right first.

### Formatting is a decomposition, not a wash

`gdocs_sheets_format_cells` sends one request with a field mask, and **fields it does not
name keep whatever was there**. Two calls over the same cell therefore add up, and the copy
ends up carrying more than the sample does.

So: take every cell's set of *set* properties as its signature, and cut the region into the
fewest rectangles of one signature each, none overlapping — one call per rectangle. A real
tab comes out at thirty to fifty calls instead of thirty thousand.

The tempting shortcut is to paint a base over the whole grid and overlay the exceptions. It
does not work. A sample's heading row carries `MIDDLE` while its data rows carry nothing at
all, and a base of `MIDDLE` cannot be taken back off them — the text sits centred where the
sample has it on the baseline.

## 4. Check field by field

There is no thumbnail for a sheet, so the check is the reading itself:

1. **Read both workbooks back and compare** — every value, every reported property of every
   cell, every dropdown, every width and height. Anything that differs has a name, and the
   name is what gets fixed. Work from the noisiest field down.
2. **Export both to PDF** (`gdocs_drive_export_file`, format `pdf`) and compare the pages pixel
   for pixel when the render matters. This catches what no reading can say — above all the
   colours of dropdown options, which is where a copy that matches on every single field
   still looks different.
3. **Read the dropdown colours** (`gdocs_sheets_read_dropdown_colors`) on both sides when
   the sample has any. It is the only way to see that difference as data rather than as
   pixels: the sample answers with a colour per option, the copy with nothing. Neither the
   colour nor the pill shape can be written back — [pitfalls](references/pitfalls.md#a-dropdowns-colours-and-its-pill-shape-are-not-in-the-api)
   has the sixteen refusals that prove it, and what each way around actually costs. Decide
   with the owner rather than quietly painting cells.

A tab that differs in nothing is a tab nobody can tell from the sample. Anything left over
has a reason worth naming in the report, and the dropdown colours are the one thing that
stays over however well the rest is done.

## Never

- Delete anything. There is no tool for it, and there will not be: no row, no column, no
  tab, no file. A grid that has to get smaller is a grid that should have been created
  smaller.
- Write formatting over a range twice to "fix it up" — decompose instead.
- Write `=HYPERLINK()` where the sample has a linked cell: that replaces the value with a
  formula.
- Report a workbook as finished without having read it back.
