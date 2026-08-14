# Pitfalls, and what each one does to the slide

Every entry here was paid for by a deck that came out wrong. Each says what the trap is,
**what it looks like on the slide** — because that is how it gets noticed — and what to do
instead.

They are grouped by how much they cost. The first group changes what the reader sees at a
glance; the last group is pixels.

---

## Wrong content, wrong meaning

### Text taken from a listing is truncated
`gdocs_slides_list` shortens text to 200 characters and adds an ellipsis.

**On the slide:** a bullet ending in "…" in the middle of a sentence, saved as the real
text.
**Instead:** take the words from `gdocs_slides_inspect_text_structure`.

### A merge removes the cells it swallowed
Under a first column merged down five rows, those rows hold two cells, and the first of
them is column 1 — not column 0.

**On the slide:** every row's text piled into the merged heading, and the columns beside it
looking shifted one to the left. Slides accepts the write silently.
**Instead:** address cells by the coordinates `read_table` reports. This server refuses a
swallowed coordinate rather than writing it.

### An unfilled placeholder is visible
A slide made from a layout gets all of the layout's placeholders.

**On the slide:** "Click to add text" showing through whatever was placed on top.
**Instead:** fill them or delete them; `inspect_page` marks them `placeholder_empty`.

### Links do not survive a rebuild
Replacing a paragraph's text drops the links inside it.

**On the slide:** an incident number that is no longer clickable, and nobody notices until
someone tries it.
**Instead:** read `links` before writing, put them back with `gdocs_slides_link_text`.

---

## Right content, wrong look

### "Not set" and "set to false" are different
A heading with no `bold` of its own is bold because the layout says so.

**On the slide:** writing `bold: false` there makes it plain — the copy looks lighter than
the sample everywhere the sample inherited.
**Instead:** write only what the sample sets; clear the rest with
`gdocs_slides_reset_text_style`, by range when only part of a line should be cleared.

### Zero is a value, and dropping it moves everything below
Slides stores an explicit zero as a dimension with no magnitude —
`"spaceBelow": {"unit": "PT"}`. A paragraph with that zero sits tight against the next
line; a paragraph with nothing there inherits the master's twelve points.

**On the slide:** the block starts in the right place and drifts further down with every
paragraph, ending twenty or more pixels low.
**Instead:** this server reports the zero (`space_below_pt: 0`) — write it back as a zero.

### Space above collapses inside lists unless told not to
The default merges the space above a paragraph with the space below the one before it.

**On the slide:** ten points asked for above a heading render as nothing, and everything
under it sits ten points high.
**Instead:** write `spacing_mode: NEVER_COLLAPSE` alongside the spacing, as the sample does.

### A fill at alpha 0 is "no fill", and its colour is black
A text box over a coloured panel carries a solid fill of black at zero opacity.

**On the slide:** copying the colour paints a black rectangle over the panel.
**Instead:** the reading marks it `transparent` — use `no_fill`.

### A theme colour name is not its value
`ACCENT5` follows the theme; `#0097A7` stops following it. A colour is stored one way or the
other and never both, so a fill, an outline or a line painted by name has **no value at all**
— the reading carries `theme_color` and nothing else.

**On the slide:** nothing at first, then a deck that stops changing colour when the theme
does. Worse on a border: read as a weight and a dash style with no colour, it gets rebuilt
without one, and the panel loses its edge.
**Instead:** write back `theme_color` when the reading gave a name —
`fill_theme_color` and `outline_theme_color` on `style_shape`, `theme_color` inside
`style_table`.

### Style lives inside the line
"Итог:" bold with the rest of the sentence plain is one paragraph and two runs.

**On the slide:** a bullet that is entirely bold or entirely plain, where the sample has
both.
**Instead:** if `inspect_text_structure` reports `runs`, write them one by one with
`scope: range`, and clear what they inherit with `reset_text_style` over the same range.

### A font family without its weight is not stored
Writing `font_family` alone is discarded when the value equals what is inherited.

**On the slide:** the line stays at the layout's weight — 400 where the sample is 500 — and
reads visibly narrower.
**Instead:** always send `font_weight` with `font_family`.

### A bullet's colour is frozen at the moment the list is made
The marker takes the colour of the text under it when `bullet_preset` runs, and keeps it.

**On the slide:** turquoise markers beside grey ones, because the list was made while the
first words were a link.
**Instead:** `remove_bullets`, colour the text the way the marker should look, make the list
again, then put the runs' colours back. Making the list again **clears the paragraph's
spacing** — write it afterwards, never before.

### Text typed into an empty cell arrives in the default font
**On the slide:** one cell in Arial among twenty in Rubik.
**Instead:** style the cells after filling them.

### A shape with no text is still an element
The panel behind the text, the white card under a chart, the block under a QR code.

**On the slide:** text floating on the background, or worse, text underneath the panel that
was added later.
**Instead:** build them with `create_shape` and send them back with `order_elements`.

### A picture does not fill the box you name
Slides fits it inside, keeping its proportions, and centres it.

**On the slide:** a photograph smaller than its slot with white margins around it.
**Instead:** `insert_image`, then `place_element` with the same four numbers.

### Rebuilding the list throws the heading away
`set_list` replaces everything in the box. The paragraph fields survive — indents, alignment,
spacing, and the colour of the runs that were there — but the first line comes back as a
plain item of the list's own styling, and `plain_first_line` only keeps it out of the list.

**On the slide:** a panel whose heading is the same size and weight as the three lines under
it, so the eye finds no way in.
**Instead:** style after listing, not before: the whole box first (family, size, the body
colour), then `scope=title` for the heading. On a panel copied from one of another colour,
the inherited run colour is the giveaway — grey text in a red panel means the `all` pass was
skipped.

### Replacing the text throws the paragraphs' styling away
`set_text` puts new words in the box and nothing else survives below the box's own level: a
two-paragraph panel whose heading was 14 pt bold slate and whose caption was 12 pt grey comes
back as two identical paragraphs. The box keeps its fill, its outline and its alignment, so
the loss is easy to miss on a small render.

**On the slide:** a caption the same size and colour as the heading above it, in a panel that
looked right five minutes ago.
**Instead:** when only part of the words changes — a marker, a date, one word in twenty
panels — use `gdocs_slides_replace_text`: it swaps the stretch in place and everything around
it keeps its styling, and it answers with how many places it changed, so a search that matched
nothing says so. When the whole box really is being rewritten, read it with
`inspect_text_structure` first and set the paragraphs back afterwards by number —
`scope=paragraph:0`, `paragraph:1` — with the values the reading reported.

### Recreating a table because its shape has to change
A copied slide brings a table of the size it had. The instinct is to delete it and build a new
one with `create_table_with_text`, and that throws away every fill, every merge and every
per-cell style the original carried, all of which then has to be set again.

**On the slide:** a table that is the right size and the wrong colour, with the header row no
longer bold.
**Instead:** `gdocs_slides_edit_table` adds rows and columns beside a cell and takes merges
apart. The size a table was created with is not final, and reshaping keeps the styling.

### Furniture put on the slide instead of the layout
A band, a logo, a rule, a background: each one added to a slide is one that has to be added
to the next nineteen, and moved on all twenty when it moves.

**On the slide:** nothing at first — the fault appears a month later, on the first slide
somebody adds by hand, which comes off the layout without any of it.
**Instead:** put it on the layout's page. `create_shape`, `insert_image` and
`set_page_background` all take a layout's identifier, and so do `place_element` and removal.

### Decoration baked into a background picture
A rule under the title, drawn into the PNG that the background is made of, cannot be nudged:
every attempt is a new render, a new upload and a new file on the drive that the server
cannot delete.

**On the slide:** a line four points below where the title's baseline actually is, and no
cheap way to fix it.
**Instead:** the picture carries only what cannot be a shape — gradients, patterns, texture.
Anything with an edge that has to line up with text is a shape on the layout, where moving it
costs one call.

### A picture that Slides never fetched
Both `insert_image` and `set_page_background` take an address Google itself has to reach; a
path on disk is not one, and neither is a file on a drive that nobody may read.

**On the slide:** the call fails outright, or — worse — succeeds against a stale address and
the deck keeps a broken picture.
**Instead:** upload, open the file to anyone with the link, insert, and close the link again
**on the very next call**. Slides fetches the picture once and keeps its own copy, so the
grant is needed for one request and no longer. Leaving it open is how a private deck's
artwork ends up readable by anyone who guessed the address.

---

## Right look, wrong numbers

### EMU stays EMU, and a unit read as a number is a factor of 12700 off
Google answers in whichever unit the value was stored in: positions in EMU, indents in
points.

**On the slide:** an indent of 36 written back as 36 EMU disappears entirely.
**Instead:** this server converts on the way out — `indent_start_emu` is EMU whatever came
back. Do not convert sizes to points "for convenience".

### A table's size is a lie
Slides reports 3000000 × 3000000 for every table.

**On the slide:** nothing; the mistake shows when `place_element` is asked to resize one
and silently scales nothing.
**Instead:** the real extent is the sum of column widths and row heights; widths go through
`update_table_cells`.

### Rotation is not a field
It shares a matrix with the scale, so writing a new width into a rotated element's matrix
unrotates it.

**On the slide:** an element that jumps to a new angle when it was only meant to get wider.
**Instead:** `place_element` with `rotation_deg`.

### Text is counted in UTF-16 code units
**On the slide:** for Russian a byte count is twice too long, the range lands mid-character
and the API refuses the whole batch.

### Elements match by geometry, not by identifier
A rebuilt slide's identifiers share nothing with the sample's, and the stacking order
differs.

**Instead:** pair them by position down the slide when comparing.

---

## What the API will not say at all

These have no field, in the API or in a PPTX export. They are found by measuring the render
and confirmed by exporting the deck and reading the XML. Compensate deliberately, and say
in the report what was compensated and why.

### A shape's adjustment values — the corner radius among them
The sample's panels are `ROUND_RECTANGLE` with the radius turned down; a panel built from
that shape type comes out with corners six times rounder. Nothing reports the radius and
nothing accepts it.

**Instead:** draw the candidates at the sample's real size and keep the closest — a plain
`RECTANGLE` was twelve pixels off against ninety for anything rounded. Or have a person paste
the shape in from the sample: a pasted shape carries everything the API cannot name.

Multiply it by duplicating the **slide**, not the shape. `duplicate` puts a copy of an
element on the same page and no request moves one to another slide, so multiplying the shape
means one paste per slide; a slide copy carries its elements with it, so one paste covers the
whole deck. Build the first slide of that kind completely, then copy it and edit the copies.

### Pictures on a layout or the master are not reported
`read_theme` walks the elements of every layout and keeps the shapes. A picture there — a
logo in the band, most often — is dropped: no address, no size, no hint that it exists.

**On the slide:** a reading that says the template is empty where a person plainly sees a
logo, and a rebuild that quietly loses it.
**Instead:** get the file from wherever it actually lives — a brand asset, or the media
folder of a PPTX export — and put it back with `insert_image` on the layout. Record that it
is there, because the next reading will not say so.

### Text insets — the padding between a box's border and its text
Default 91440 EMU on each side; an author can set them to zero, and nothing reports it.

**On the slide:** identical positions, sizes, paragraphs and runs, and text sixteen pixels
right and down of where the sample's sits.
**Instead:** confirm it in a PPTX export (`<a:bodyPr lIns=… tIns=…>`), then move the box out
by one inset on each side and widen it by two, so the text lands where the sample's does and
wraps at the same width. Do this only when the export proves the insets differ — otherwise
the difference is in the data and moving the box hides it.

### Autofit shrinking cannot be switched on
A title reported as 28 pt is drawn at 25.2 because its box shrinks text to fit;
`autofit` accepts only `NONE`.

**On the slide:** a heading a tenth wider than the sample's.
**Instead:** set the font size to `effective_font_size_pt × autofit_font_scale`, both of
which `inspect_title_style` reports.

### A bullet's size follows the text and cannot be pinned
The colour freezes when the list is made; the size does not — setting the words back to
11.5 pt takes the markers with them.

**On the slide:** a sample with 14 pt markers over 11.5 pt words has taller lines, and the
copy runs short by a few pixels on every paragraph. This one has no workaround: name it in
the report.
