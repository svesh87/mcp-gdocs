# Pitfalls, and what each one does to the sheet

Every entry was paid for by a copy that came out wrong. Each says what the trap is, **what
it looks like on the sheet** — because that is how it gets noticed — and what to do
instead. They are grouped by how much they cost.

---

## Wrong content, wrong meaning

### Values are read as they are shown unless asked otherwise
`gdocs_sheets_read` defaults to `FORMATTED_VALUE`.

**On the sheet:** `1 234,50 ₽` written into a fresh cell as the text it looks like — a
string where the sample has a number, and a column that no longer adds up.
**Instead:** read `UNFORMATTED_VALUE` when the numbers are going somewhere, and `FORMULA`
when the sample computes rather than stores.

### The locale decides what got stored
`USER_ENTERED` reads text the way typing it would, and that depends on the workbook's
locale: `01.02` is a date under one and a number under another.

**On the sheet:** the same characters and a different value underneath, found the first
time something sorts or sums the column.
**Instead:** set `locale` and `time_zone` when the workbook is created.

### The dropdowns are not part of the formatting
`dataValidation` sits beside the format on the cell, not inside it.

**On the sheet:** every colour right and a status column that is free text — nobody
notices until somebody types "ок" and a filter stops matching it.
**Instead:** read `validations` and write them back with `gdocs_sheets_set_validation`.

### A merge swallows the cells under it
A heading merged across three columns leaves one addressable cell.

**On the sheet:** text written into the swallowed coordinates goes nowhere, or the columns
beside the merge look shifted.
**Instead:** read `merges`, and address what is left.

---

## Right content, wrong look

### Entered format and effective format are different things
What the author set is the entered format; what a cell ends up with is the effective one,
after the tab's and the workbook's defaults. The reading here reports the **entered** one,
because that is what gets written back.

**On the sheet:** a copy built from effective values carries an explicit font, size and
alignment on every cell, and stops following the workbook's defaults the moment anybody
changes them.

### White is a decision
It looks like noise — every cell of every workbook is effectively white — but a white that
was *entered* is a cell covering a coloured block, or white letters on a dark heading.

**On the sheet:** dropped, the coloured block paints through, and the heading's letters
turn black on black.
**Instead:** write back the `#FFFFFF` the reading gives.

### Formatting written twice adds up
The write names its fields, and fields it does not name keep whatever was there.

**On the sheet:** a copy where the data rows sit vertically centred because a base was
painted over the whole grid first and the sample's rows carry no vertical alignment at
all. It cannot be taken off by writing the rectangle again.
**Instead:** decompose into non-overlapping rectangles, one call each.

### Vertical alignment is invisible until the row is tall
Sheets defaults to `BOTTOM`.

**On the sheet:** in a 50-pixel row with one line of text, `MIDDLE` and `BOTTOM` are
sixteen pixels apart, and every row of the copy is off by the same amount.

### A font family without its size, or a size without its family
Both are stored on the cell, and a sample that names one on every cell means the tab's
default is something else.

**On the sheet:** the copy renders in the workbook's default face, which is close enough
to look like a rendering artefact and wrong on every line.

### Text typed into an empty cell arrives in the default font
**On the sheet:** one cell in Arial among twenty in Roboto.
**Instead:** write the values first, then format.

### A cell's link is a text style, not a value
Writing `=HYPERLINK("…";"…")` replaces the value with a formula.

**On the sheet:** the same words, and a cell whose content is now a formula — which the
next read returns instead of the text.
**Instead:** `gdocs_sheets_format_cells` with `link`. Beside it lives `link_display`
(`LINKED` / `PLAIN_TEXT`), a property of the **cell** that outlives the link: samples carry
it on cells whose link was removed long ago.

### Row heights are not a consequence of the content
A tab of wrapped text is readable because its rows were made tall.

**On the sheet:** the copy's rows collapse to 21 pixels and three lines of wrapped text
become one clipped line.
**Instead:** read `row_heights_pixels` and write them back, a run at a time.

---

## Right look, wrong numbers

### A pattern does not say what kind of format it is
`0%` stored as `NUMBER` shows the same digits as `0%` stored as `PERCENT`.

**On the sheet:** nothing, until somebody types `0.3` and gets `0.3` where the sample would
have shown `30 %`.
**Instead:** pass `number_type` with `number_format`.

### The reading's row numbers are the sheet's, not the rectangle's
A reading of `C5:F20` reports its first cell as row 4, column 2.

**On the sheet:** taken as 0/0, the write lands on `C1` — sixteen rows above the target,
over the heading.
**Instead:** the numbers are already absolute; `first_row` and `first_column` say where the
rectangle starts.

### The grid can only grow
A new tab is 1000 by 26; making it smaller deletes rows, and this server does not delete.

**On the sheet:** empty columns to the right of the sample's last one.
**Instead:** ask for the size in `gdocs_sheets_create` or `gdocs_sheets_add_tab`.

### A tab's identifier is zero for the first tab
Which means "no identifier" and "the first tab" look alike, and a workbook created with
several tabs that all send zero is refused outright.

**On the sheet:** nothing — the workbook is simply not created, with
`Duplicate ids are not allowed`.

---

## What the API will not say at all

### A dropdown's colours and its pill shape are not in the API
A dropdown made in the interface is drawn as a coloured pill — green "Done", red "High" —
and a dropdown made through the API is drawn as plain text with a small arrow at the edge
of the cell. Both are the same rule as far as any API answer goes: read the two cells with
`includeGridData` and they are identical, down to `showCustomUi: true`. The look is simply
not in the model, in either direction:

- **on reading** — the cell answers black on white in `effectiveFormat`, and
  `DataValidationRule` is `condition`, `inputMessage`, `showCustomUi` and `strict`, with
  `ConditionValue` being `userEnteredValue` and `relativeDate`. A grep of the whole
  discovery document finds no colour field for a dropdown anywhere;
- **on writing** — Google rejects every plausible shape rather than ignoring it, which
  makes the refusals evidence rather than guesswork. `colorStyle`, `style`, `valueColors`
  and `chipStyle` on the rule or its values; `colorStyle`, `valueColorStyles` and a
  coloured condition value on a table column; `displayStyle`, `uiStyle`, `chipStyle`,
  `dropdownStyle`, `renderStyle`, `displayType`, `valueStyle`, `showCustomUiStyle` and
  `chipDisplayStyle` for the shape — sixteen names, every one `Cannot find field`, while
  the same request without them returns 200;
- **not in the file formats either** — the XLSX export carries the list as a plain
  `dataValidation` with no colours, no `x14:dataValidation`, no conditional formatting and
  no `dxf`. The colour lives only in the live Google document.

The one thing that *is* reachable is the pill shape, and only as a table column:
`gdocs_sheets_add_table` with `type: DROPDOWN` draws pills — grey ones, `#e8eaed` on
`#434343`.

**On the sheet:** an otherwise identical copy whose dropdown columns are plain text with an
arrow where the sample has coloured pills. About one per cent of the rendered pixels of a
dense tab, and the whole of the remaining difference once everything else matches.

**What can be done, and what each costs.** `gdocs_sheets_read_dropdown_colors` gets the
sample's real colours — per column, per option, fill and text — by reading the editors'
HTML rendering, since that is the only place they surface. From there:

- **a rule that colours by content** (`gdocs_sheets_set_conditional_format`, one `TEXT_EQ`
  per option) puts the sample's colours on the sheet and keeps following the value. It
  colours the **whole cell**, not a pill, and it adds a rule the sample does not have;
- **a table column** gives the pill and no colour;
- **both together** is worth knowing about because it looks like the answer and is not: the
  rule paints the cell *around* the pill and the pill stays grey, so the cell ends up a
  block of colour with a grey pill sitting in it — further from the sample than either
  half alone. This was tried on a live tab before being written down;
- **leaving it** keeps the copy an exact match of everything the API expresses, and the
  difference gets named in the report instead.

What is never the answer is painting the text by hand: it puts a property in the copy that
the sample has not got, and it stops following the value the moment somebody changes it.

### A chart cannot be read back
Charts are counted, not described: `gdocs_sheets_add_chart` draws one from a description,
and nothing turns a sample's chart into that description.

**On the sheet:** a copy with the numbers and no picture of them.
**Instead:** read the chart in the interface, decide what it shows, and say so —
`labels_column`, `value_columns`, the type. `gdocs_sheets_info` counts what was there, so
the gap can be named rather than missed.

### Slicers and data-source blocks
Readable as a count, nothing more.

**On the sheet:** a copy that looks right until somebody reaches for the control that
filtered it.
