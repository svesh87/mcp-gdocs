# What goes wrong in a document, and what it looks like on the page

Every item here cost a wrong document first. They are ordered by how expensive the mistake
is to undo, not by how likely it is.

## The newline that cannot be taken back

**A paragraph's text goes in without its closing newline.**

Sending `"Условия работы\n"` splits the paragraph in two and leaves an empty one behind.
Harmless in the middle of running text — and permanent in front of a table or a section
break, because **the paragraph directly before either of them cannot be deleted**. Docs
answers `Invalid deletion range. Cannot delete the requested range.` and there is no other
way at it: the table and the break both insist on a paragraph in front of them.

On the page it is a blank line before every table and at the top of every new section,
which is exactly where a copy looks sloppiest.

So: write `"Условия работы"`, and let the *next* thing supply the newline —

- another paragraph: send it as `"\nСледующий абзац"`;
- a table or a section break: send neither, they make a paragraph of their own to carry on
  in.

If it happened anyway, the only repair is to wipe the whole range — one `delete` from the
first index to the last, which takes the tables and the breaks with it — and rebuild.

## Indexes move under you

Every insertion shifts everything after it. A style applied from indexes read before the
last edit lands on the wrong words, and the failure is quiet: the text is right, the
weight is one character off.

Two habits fix it:

- write everything first, style afterwards, against a **fresh** reading;
- when filling a table or inserting several pictures, go **backwards** — from the last
  position to the first — and nothing you have already measured moves.

## The body starts at 1, a header starts at 0

Index 0 in the body is the document itself, not a place in it, and Docs refuses it with a
message about a segment. In a header, a footer or a footnote, 0 is the first character —
and a freshly made header is a single newline, so index 1 is already past its end
(`Index 1 must be less than the end index of the referenced segment, 1`).

## Text is counted in UTF-16 code units

For Russian a byte count is twice too large: the range lands in the middle of nothing and
the API refuses the whole batch. Count as `utf16Length` does.

## Private-use characters disappear, and the API says nothing

Docs **silently drops** every character in U+E000–U+F8FF. A sample full of icon-font
glyphs (`` and friends) comes back one character shorter per glyph, the batch
answers 200, and every index computed from the sample's own lengths is now off by one.

Probe, verbatim: sent `[||•|]`, landed `[||•|]`.

So strip them before writing and count on what will actually land. An icon that was a
glyph in the sample cannot be reproduced as text at all — it was a font, not a character.

## Making a list rewrites the indents

`createParagraphBullets` sets its own `indentStart` and `indentFirstLine`. Do the bullets
**before** the paragraph styles, or the indents you copied are replaced by the preset's.

The glyphs themselves are not settable: there is no request that writes a nesting level.
Match the sample by choosing the preset whose glyphs it uses — `BULLET_DISC_CIRCLE_SQUARE`
is ● ○ ■, which is what a Google Doc's default list looks like.

Depth is a **tab character** at the start of the text. Never an indent, never a "•".

## A new table has a black frame the sample has not got

Docs does not report the outer edges of a table at all — a corner cell comes back with
`border_top`, `border_right`, `border_bottom` and no `border_left`. A table this server
creates has all four, black and a point wide. Copy the reported borders and the copy grows
a frame around it.

State the missing sides explicitly as nothing: same colour and dash style, `width_pt: 0`.

## Fields that are read and refused

Four of them, each checked against a live document. The refusal matters because a batch
that fails leaves everything else in it unwritten.

| Field | What Google says | What to do |
|---|---|---|
| `heading_id` | `Unallowed field: headingId` | leave it out; setting `named_style` is what makes a heading |
| `use_custom_header_footer_margins` | `Unallowed field` | it follows from the header and footer margins themselves |
| document background `"none"` | `Cannot set a transparent background` | a page with no colour reads back as none and is what a new document already has |
| `named_style` inside a named style's paragraph style | `Unallowed field: paragraphStyle.namedStyleType` | the style being changed is named by the argument |

The server refuses all four before they reach Google, with a sentence saying which.

## updateNamedStyle needs the type in the mask

`namedStyle: {namedStyleType: "NORMAL_TEXT", …}` with `fields: "textStyle.fontSize"` is
answered with **`Named style type is required.`** — about a request that plainly carries
one. Google does not read the field unless the mask names it. `fields` has to start with
`namedStyleType`.

## There is one header per section, and only the ordinary kind

`createHeader` knows a single type: `DEFAULT`. The enum has two values and the other one
is "unspecified" — ask for `FIRST_PAGE` and Google answers `Header type not specified`,
because it failed to read the field at all.

A document whose first page has its own header was made that way in the editor. Through
the API the same effect is **sections**: the document's own header serves the first page,
and each later section gets one of its own by `section_break_index`. A second header for
the same section is refused (`Default header already exists`) — reuse what is there.

## Floating pictures cannot be created. Ten refusals

Docs v1 has `deletePositionedObject` and nothing that makes one. Probed on a live
document, every one answered `Cannot find field`:

```
insertPositionedObject   createPositionedObject   insertImage
insertInlineImage + positioning / objectPositioning / layout / positionedObjectProperties
updatePositionedObject   updatePositionedObjectPositioning   updateEmbeddedObjectPosition
```

So a picture put in by an API caller sits **in the line of text**. In a sample where it
floats beside the text, or behind it, the copy's text is pushed down by exactly the height
of the picture. Nothing in the reading or the writing changes that.

What does change it: a person clicking the picture in the editor and choosing wrap — after
which the API reports it as a positioned object and can delete it. Google's own way to
create one is `Paragraph.addPositionedImage` in Apps Script, which is a different API and
a different set of permissions.

## Drawings cannot be read or created

`embeddedDrawingProperties` arrives **empty**: Google says "this is a drawing" and nothing
about what is in it. There is no request that makes one either. A sample built out of
drawings — a title banner, a chat mock-up — cannot be rebuilt through the API in any form.

## Some pictures are invisible to the API altogether

A sample of 22 images reported 16: twelve positioned objects and four drawings. The other
six — the title banner, the yellow strip repeated on each page, the dark strips — are in
neither the body, nor the headers and footers, nor the tabs (checked with
`includeTabsContent=true`; there was one tab). The only place they exist is the editors'
own export.

If they are needed, the route is: export the document as HTML (`/export?format=zip`, one
file plus an `images/` folder), then get each picture to an address Google can fetch —
`insertInlineImage` takes a URI and fetches it itself. In practice that means uploading to
Drive, opening it by link for the length of one request, inserting, and closing the link
again. **That is a file made public, however briefly, so it is the owner's decision every
time.** It is not something this server does: there is no tool here that changes who can
see a file.

A picture already inside another Google document needs none of that — its `content_uri`
works directly, which is how the sample's own pictures are carried over. That address is
signed and short-lived, so read the sample and insert in one go.

## A picture's size is a wish

`objectSize` is honoured within the picture's own ratio: asked for 598.28 × 133.61 pt, a
picture landed 595.72 × 133.61. And an inline picture cannot be wider than the text
column, while a floating one in the sample may well be wider than the page.

Nothing else about a picture can be set: margins, border, crop and description are all
reported and none of them can be written.
