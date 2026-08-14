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

### Decide for each thing: the layout or the slide

Everything a writing tool does to a slide it does to a layout and to the master as well —
`create_shape`, `insert_image`, `set_page_background`, `place_element`, and removal. The
question is not what is possible but what belongs where, and the answer decides how the deck
ages.

**On the layout** goes anything that repeats: the band across the top, the logo in it, the
background, and — the one most often got wrong — **the grid**. Where the title sits and how
much room the body gets are properties of the template, not of any slide. Set them on the
layout with `place_element` and every slide that follows it moves, including the ones a
person adds in the browser next month. Set them slide by slide and the template says one
thing while the deck does another; the first hand-added slide shows it.

**On the slide** goes only what that slide says: its text, its own picture, panels whose
number and colour change from slide to slide.

Two consequences worth knowing before starting:

- A layout can be *added to* freely, and its own elements moved and removed, but the layout
  itself cannot be removed and no request creates a new one. The set of layouts is whatever
  the theme came with.
- `read_theme` reports the shapes on a layout and **not the pictures**. A logo placed there
  is invisible to every reading; note it somewhere the next agent will look, because the
  deck will not tell them.

### Finish the whole theme before touching a single slide

When the job is a deck's *look* — a template, a restyle, a seasonal variant — the theme is
built first and completely, and only then do the slides get touched. Not "the layouts this
deck happens to use": **every layout the theme carries**, and the master.

A theme comes with about twelve layouts and a deck of twenty slides usually stands on four or
five of them. The other seven look unused, and they are not: the first slide somebody adds by
hand next month comes off one of them — "Заголовок и два столбца", "Один столбец", "Чистый" —
and it lands with the master's flat background instead of the deck's, in a colour nothing else
on the deck uses. Nobody reads that as "an unstyled layout"; they read it as the deck being
broken. Six extra `set_page_background` calls at the start are the whole price of not having
that happen.

The order that works, and why it is this order:

1. **The palette**, `set_theme_colors`. Everything painted by name follows it, so it goes
   first and nothing has to be repainted after.
2. **The master** — the title's colour and the body's, by palette name. Every layout inherits
   from it, so a colour set here is a colour not set eleven more times.
3. **Every layout**: background, and whatever the layout's own placeholders override. The
   ones the deck uses today and the ones it does not.
4. **The slides**: panels, tables, words. Last, because until the theme is finished there is
   nothing stable to check them against.

Doing it in the other order — slides first, theme after — costs the work twice: the panels get
painted against a look that then changes under them, and the difference shows up as a slide
that is nearly right, which is the expensive kind of wrong.

### A colour by value, or a colour by name

Every fill, outline and letter can be painted either with a value or with a name from the
deck's palette — `ACCENT1`, `LIGHT1`, `DARK2` and the rest. The two look identical on the
slide and behave nothing alike: a named colour follows `set_theme_colors`, a value never
does. So the choice is not about style, it is about what a later recolour will reach.

The rule that follows: **paint by name whatever the deck's look owns, and by value only what
belongs to that one slide.** Panel fills, panel outlines, heading colour, body colour — by
name. A one-off highlight on one word — by value.

It matters most for a *series*: twelve decks of one family, one per month, each with its own
season. Painted from the palette, a season is one call — `set_theme_colors` — and every panel
and table follows. Painted with values, the same change is every shape on every slide, and
the ones missed are found by a person reading the deck.

Where the names go in: `style_shape` takes `fill_theme_color` and `outline_theme_color`,
`style_table` takes `theme_color` inside `fill` and inside `cell_styles`, `set_text_style`
and `style_layout` take `theme_color`. A reading reports which one a sample used —
`inspect_page` and `inspect_title_style` answer `theme_color` where the author picked from
the palette, and copying that as a value is how a copy stops following its own theme.

The palette holds twelve slots and a deck usually wants more roles than that, so decide the
mapping before painting: which accents carry the panel colours, which carries the heading,
what stays a literal because it never changes with the season.

**A panel is four colours, and the text is two of them.** Converting a deck to the palette by
walking its fills and outlines looks finished and is half done: the heading's colour and the
body's colour are stored on the text, not on the shape, and they stay literal. Nothing shows
until the palette is inverted — and then every panel comes back with its fill dark and its
words still dark, which is exactly the fault that reads as "the text merges with the panel".
`inspect_page` will not warn about it either: it reports the shape, and the text colours are
in `inspect_text_structure`.

When the roles outnumber the slots, take them from the outlines rather than from the text.
A panel's meaning is carried by its fill, so all four kinds can share **one** edge colour and
release two slots for the ink — problem-heading and done-heading — which is what a dark
variant cannot do without. Two calls per panel do it: `scope=all` for the body, `scope=title`
for the heading.

### A series of decks that differ only in season

A template that comes in twelve variants — one per month, one per event — is not twelve decks.
It is one deck plus twelve palettes and twelve sets of background pictures, and it stays that
way only if it is built that way from the start.

The shape that works:

1. **One base deck** holds the composition: every slide, every panel, every word of the
   placeholder text, the grid, the speaker notes.
2. **Everything the season touches is painted from the palette**, so a variant is
   `set_theme_colors` plus the backgrounds and nothing else. See the section above for which
   colours those are.
3. **Each variant is a copy of the base**, made with `copy_presentation`. The copy keeps the
   originals' object identifiers — the same `cardRed01` addresses the panel in every deck of
   the series — so the same short sequence of calls fills in any month, and a fix to the base
   can be replayed across the variants without reading each one back.

Two more things the variants need, and one place to put them:

- **The markers belong to the season too.** A heading that opens with an emoji is read marker
  first, words second — so a rocket in a Halloween deck argues with the background louder than
  any colour does. Swap them with `replace_text`, one call per pair, keeping the meaning:
  "done", "problem", "next" are recognised by their place in the layout, and the season only
  dresses them. Watch for a marker used in two roles — replace the longer phrase first, or the
  second role gets the first one's costume.
- **Say what the theme is, in the title slide's speaker notes.** The deck travels to people
  without whoever built it; a note travels inside the file, and a plan in a scratch directory
  does not. One paragraph: what the picture is made of, what changes by the final slide, which
  markers the season uses. A year later that paragraph is the only reason anybody knows why
  February's dial has twenty-eight lamps.

Two consequences worth knowing before the second variant:

- **A copy of a copy inherits the drift.** Fix the base first, then re-copy; a fix applied to
  three variants and forgotten on the fourth is the one people notice.
- **Every background is a file on the drive for ever.** Four pictures per variant is four
  files per month, and there is no removal here. Draw and check them locally first — a
  stand-in page at the layouts' real geometry (EMU / 9144000 × 2560) shows a collision with
  the title before the picture costs anything.

### Size the panels against the decks people actually write

A panel sized against the words a template puts in it overflows on the first real month. The
numbers to size against are in the decks already delivered: export a dozen of them as text,
find the blocks by their own headings, and count.

What that answered for one such series, and what it changed: the block "what we did" runs to
five lines at the median and ten at the ninth decile, of forty-eight and ninety-three
characters — so the wide panel, which holds fourteen lines, is right. The blocks beside it run
to one and six lines, but their lines are long — up to a hundred and fifty-eight characters,
four wrapped lines each — so splitting the right-hand column in half was wrong, and it now
divides by weight instead.

Measuring the capacity is the other half: a panel's width in points, minus Slides' own insets
of 7.2 pt a side, over the average glyph width of the actual font at the actual size. Render
the font and measure it rather than assuming a ratio — Rubik at 11.5 pt fits seventy-six
characters where the guess said sixty.

### Multiplying a shape the API cannot build

A panel with the author's corner radius, or anything else carrying adjustment values, arrives
once — a person pastes it in the browser. From there the instinct is to duplicate the shape,
and that is the slow way: `duplicate` puts the copy on the same page, always, so a deck
needing that panel on nine slides would need nine pastes.

Duplicate the **slide** instead. A slide copy carries every element with it, adjustment
values included, and one paste covers the whole deck: build the first slide properly, copy
it as many times as there are slides of that kind, then edit each copy's geometry, colours
and words. Cheaper by an order of magnitude, and every panel in the deck is provably the
same shape.

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

### When there is no sample: check the deck against itself

A deck built from scratch — a template, or a first deck of its kind — has nothing to diff
against, and the question changes from "does this match" to "do these twenty look like one
deck". That is not visible one slide at a time.

Render the whole deck at one size and put every slide on a single sheet, small, in order.
Three faults show up there and nowhere else: a title that sits lower on the slides made
first, a panel colour used for two different meanings, and a slide still wearing the words of
the slide it was copied from. All three are invisible while the slide fills the screen.

Then measure what should be identical rather than trusting the eye: every title at the same
`y`, every content band the same width, and no font size on any slide outside the handful the
deck was designed with.

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
