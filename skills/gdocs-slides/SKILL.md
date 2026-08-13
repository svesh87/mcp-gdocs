---
name: gdocs-slides
description: Build or edit a Google Slides deck in the house style of decks that already exist. Triggers - "сделай презентацию", "собери колоду по образцу", "как в прошлой демке", "поправь слайд", a deck from a template, a slide with bullets or a table, checking how a slide came out. For a spreadsheet, use gdocs-sheets instead.
---

# Making a deck that belongs

The job is rarely "copy this deck". It is "here are a few decks we make; make another one,
with this month's content, that looks like it belongs beside them". So the shape of the
work is: **read the samples, work out how they are built, decide, write, then check with
numbers.**

There is no tool here that copies styling from one deck into another in one call. That is
deliberate. Copying answers "make this look like that" and hides the numbers on the way —
you never learn that the headings are 25 pt in the theme's accent colour, so you cannot
decide to use 22 pt here, or to keep the size and change the colour. Everything a reading
tool reports, a writing tool takes back in the same units.

The failure this skill exists to prevent: reading a sample's *text*, ignoring its *form*,
and producing something structurally correct that looks nothing like the originals —
default font, default colours, boxes at invented coordinates, a grid of cells where the
sample has a merged heading.

Two references sit beside this file, and they are where the detail lives:

- **[references/controls.md](references/controls.md)** — every knob, by what it changes:
  what reads it, what writes it, in which units.
- **[references/pitfalls.md](references/pitfalls.md)** — every trap found the hard way,
  with what each one does to the slide and what to do instead. Read it before a rebuild,
  and again when something looks wrong for no reason.

Ask `gdocs_reference` for the values that cannot be guessed: shape names by family, bullet
presets, arrowheads, dash styles, theme colour names, export formats, placeholder kinds,
units.

## 1. Read, and read everything

Three readings, in this order. Skipping any of them is how a slide comes out wrong in a way
nobody can name.

1. **`gdocs_slides_read_theme`** — the palette, the master's background, and every layout
   with the styles its placeholders impose. This is what a slide inherits. A title that
   reports no size of its own is not sizeless: it is 28 pt because the layout says so.
2. **`gdocs_slides_inspect_page`** — one slide completely: background, speaker notes, and
   every element with its box, rotation, stacking order, fill, outline, shadow, and for a
   picture its crop. `gdocs_slides_list` is an index, not a description — and it truncates
   text.
3. **`gdocs_slides_inspect_text_structure`** — the paragraphs of one text box: their whole
   text, bullet levels, the space around them, and, where a line mixes styles, its `runs`
   with their ranges.

Read two or three samples, not one. What repeats is the house style; what differs is that
deck's own content.

**Read what is absent as carefully as what is there.** A field the sample does not set is
inherited, and writing it explicitly makes a copy that stops following the template. A field
the sample sets to zero is a decision, and dropping it moves everything below.

## 2. Decide, in numbers

Say to yourself what you are about to build: headings 25 pt bold in `ACCENT1`, body 14 pt,
second level 11 pt grey, panels filled `#FEF2F2` with a `#FEE2E2` outline of 9525 EMU. If
you cannot say it, you have not read enough.

Where the samples disagree, prefer the majority and say which one you followed.

## 3. Write

The full table is in [references/controls.md](references/controls.md). The order that
avoids rework:

1. slides and layouts — `add_slide` with a layout from the deck's own template;
2. text — `set_text`, `set_list`;
3. shapes, pictures, tables, and their stacking;
4. **runs and paragraph styling** — letters first, then the room around them;
5. **bullets last**, because a marker takes the colour of the text under it at the moment
   the list is made, and making the list again clears the paragraph's spacing;
6. links, speaker notes.

## 4. Check with numbers, not with a glance

Two slides side by side hide exactly the differences that matter: a paragraph four points
low, a font that fell back a weight, a box shifted by its own padding.

Render both decks at the same size — `gdocs_slides_export_images` with the same `size` for
each — and compare the images pixel for pixel. Any comparison will do as long as it reports
the share of differing pixels per slide and can draw where they are. Work worst-slide
first, and re-render after every change: fixing one field uncovers the next.

The order that converges fastest, each step feeding the one after it:

1. **paragraph fields** — alignment, spacing, indents, `spacing_mode`;
2. **runs inside the line** — weight, size, family, colour, underline, by range;
3. **elements** — position, size, rotation, vertical alignment, fill, outline;
4. **what the API will not say** — insets, autofit, bullet size, corner radius.

A slide at zero per cent is a slide nobody can tell apart from the sample. Anything above
one per cent has a reason worth naming.

## When the sample's value cannot be written

Some of what makes a slide look the way it does is not in the API at all, and a copy that
matches every reported number still looks wrong. The routine:

1. **Ask what else exists** — `gdocs_reference` lists the enums; guessing one gets
   "invalid argument" naming nothing.
2. **Measure the render.** Where does the ink start, how wide is the line, how far apart
   are the baselines. That turns "looks off" into a number.
3. **Export the deck and read it.** `gdocs_drive_export_file` in `pptx` or `odp` gives a zip of
   XML that shows what the API hides — `<a:bodyPr lIns=…>`, `<a:buSzPts>`,
   `<a:normAutofit fontScale=…>`. If the two decks differ there, you have found it. If they
   agree and the renders still differ, the property is outside both, and that is worth
   saying plainly.
4. **Then compensate**, and only then. Draw candidate shapes at the sample's real size;
   set the font size the shrinking arrives at; move a box by its inset. Never move an
   element that already stands where the sample's does — a difference that lives in the
   data has to be fixed in the data, or the deck ends up looking right while holding wrong
   numbers.
5. **Say what was compensated and why**, in the report. A difference nobody can explain is
   worse than one that is written down.

## What cannot be carried across at all

- **The theme, the master and the layouts.** There is no "apply that deck's theme". Three
  ways out, and the choice decides the rest of the work: start from a copy of the template
  (`gdocs_slides_copy_presentation`) and inherit everything; have a person import the theme
  by hand; or make an empty deck (`gdocs_slides_create`) and build the look — the palette
  with `set_theme_colors`, the sizes, fonts and colours with `style_layout` on each layout
  and on the master. Until one of those, the deck sits on Google's default theme however
  exact the content is. Building it is the only route that leaves every number named, and
  the only one that produces a deck of its own rather than a descendant of someone else's.
- **An element from another presentation, or even another slide.** No request moves one.
  A person can paste it in the browser; from there `gdocs_slides_duplicate` multiplies it
  on its own slide, keeping what the API cannot name.
- **A slide's own background** does come across: read it and pass the `picture_url` to
  `set_page_background`. Importing a theme does not bring it — it is on the slide, not the
  master.
- **Row heights below the first.** Slides computes them from the content; only a minimum
  for the header can be asked for.

## Sheets

A spreadsheet is a different skill — **gdocs-sheets** — with its own knobs and its own
traps. The work has the same shape (read, decide in numbers, write, compare) and nothing
else in common: there is no thumbnail for a sheet, and the check is the reading itself.

## Never

- Delete anything outside the presentation. There is no tool for it, and there will not be.
- Invent coordinates when a sample has them.
- Write text taken from `gdocs_slides_list` — it is shortened.
- Move an element to make a rendering difference go away without knowing what caused it.
- Report a deck as finished without having looked at every slide.
