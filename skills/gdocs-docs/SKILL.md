---
name: gdocs-docs
description: Build or edit a Google Doc, or rebuild one from a sample, with the mcp-gdocs tools. Triggers - "сделай документ", "собери оффер по образцу", "как в том документе", "поправь текст в доке", "перенеси оформление", a document from a template, a doc with a header, a table, a bulleted list or pictures, checking how a rebuilt document came out.
---

# Making a document that matches

The job is "here is the offer we send; make one like it for this person". So the work is:
**read the sample, decide in numbers, write in the order that holds, then compare.**

No tool answers "make it look like that", and that is deliberate: a look transferred behind
your back hides the numbers — you never learn that the heading row is 116 pt tall or that
the body font is Roboto 12, so you cannot decide to keep one and change the other.
Everything a reading reports, a writing tool takes back in the same units, with the same
key names.

`gdocs_docs_copy_range` does carry a stretch of another document here, and it is for the
other kind of job: "take these three paragraphs from the last offer", not "make this offer
like that one". It rebuilds rather than copies — the API has no copying request at all —
and says in its answer what it could not carry.

The failure this skill exists to prevent: a document with all the right words that behaves
nothing like the sample — an empty paragraph wedged in front of every table, list items
that are text starting with "•", a header on page one that should not be there, and a
table with a black frame the sample has not got.

**A tool that is missing was switched off, not forgotten.** The server is started with a
set of groups — `docs-read`, `docs-write`, `docs-delete` — and removal is never in the
default set. A name absent from the listing is the configuration talking; do what can be
done and say plainly what could not.

Two references sit beside this file:

- **[references/controls.md](references/controls.md)** — every knob: what reads it, what
  writes it, in which units.
- **[references/pitfalls.md](references/pitfalls.md)** — the traps, each with what it does
  to the page and what to do instead, ending with what the API will not do at all.

## 1. Read the sample

1. **`gdocs_docs_read_structure`** — the whole build: every paragraph with its style and
   its runs, the list a paragraph belongs to, the tables with their column widths, row
   heights and cell styles, the section breaks, the headers and footers, the named styles,
   and both kinds of object. Sizes are in points, which is the only unit Docs has.
2. **`gdocs_docs_read`** when only the words matter — it is far smaller.

Read the objects too, even though most of them cannot be rebuilt: the reading says
`"writable": false` and why, and that belongs in the report from the start rather than as
a surprise at the end.

**Keep the reading in scope.** A long document makes a long answer; narrow it with
`start_index` / `end_index` when rebuilding one part.

## 2. Decide, in numbers

Say what you are about to build: A4, margins 56.7 / 27.1 / 42.5 / 42.5 pt, header margin
85 pt; NORMAL_TEXT is Roboto 12; the title paragraph is TITLE, centred, 26 pt bold; a
four-row table, first row 116 pt, cells `#F3F3F3` with dotted `#B7B7B7` borders and 2.8 pt
padding; five bullets at `BULLET_DISC_CIRCLE_SQUARE`; four sections, the last with its own
footer in white italic.

If you cannot say it, you have not read enough.

## 3. Write, in this order

The order is not a preference — each step below would be undone or made impossible by the
next if they were swapped.

1. **`gdocs_docs_style_document`** — page size and margins first, so nothing reflows later.
2. **`gdocs_docs_style_named`** — what NORMAL_TEXT, TITLE and the headings mean. Do this
   before the text: every paragraph written afterwards inherits it, and a document whose
   named styles were set last is a document restyled paragraph by paragraph.
3. **The body**, element by element, always at the end of the segment:
   - a paragraph's text goes in **without its closing newline** — see the pitfalls, this
     is the one that cannot be repaired afterwards;
   - `gdocs_docs_insert_table`, then fill its cells from the last cell backwards;
   - `gdocs_docs_insert_section_break` where the sample has one.
4. **`gdocs_docs_add_header_footer`** — after the breaks exist, because a header belongs
   to a section. Write into it with `segment_id`, where text starts at index 0.
5. **`gdocs_docs_make_bullets`** — before the paragraph styles: making a list rewrites the
   paragraph's indents.
6. **`gdocs_docs_style_paragraph`**, then **`gdocs_docs_style_text`**, against a **fresh
   reading** — the indexes have moved with every step above.
7. **`gdocs_docs_style_table`** — cells, widths, heights. State the borders the sample has
   *and* the ones it has not: a new table comes with a black frame.
8. **`gdocs_docs_insert_image`** last, and in one go with the reading of the sample: the
   `content_uri` a reading reports is signed and expires.

## 4. Check

1. **Read both back and compare** — element by element, then field by field. A copy built
   in the same order pairs up positionally; line the two up **from the end**, because
   anything extra at the top would otherwise shift every pair by one.
2. **Export both to PDF** (`gdocs_drive_export_file`) and compare the pages. This catches what
   no reading can say: a picture in the line of text where the sample has it floating, a
   paragraph that broke to the next page, a table frame nobody asked for.
3. **Say what is left over.** Some of it always is — the floating objects and the drawings
   — and the report names each one rather than leaving it to be noticed.

## When something has to come back out

`gdocs_docs_delete` removes what is inside a document: a range, a table row or column, a
header, a footer, a floating object, or the bullets of a list. One thing per call.

It stops at the document's edge — no files, no folders, ever. Two things to know before
using it: the paragraph directly in front of a table or a section break cannot be removed
on its own (wipe the whole range instead, which takes the table and the break with it),
and there is no undo. Take the indexes from a reading made after the last edit.

## Taking a piece of another document

`gdocs_docs_copy_range` reads a stretch of another document and writes it here: paragraphs,
runs with their styling, lists, inline pictures. The API has no copying request at all, so
this is a rebuild, and a rebuild carries what both ends express the same way. Tables,
section breaks and chips are named in `not_carried` rather than half-built — read that list,
it is the difference between a copy and a copy with two holes in it.

Take the indices from `read_structure` and use the tool in the same pass: a picture's
address is signed and expires in about half an hour. `references/controls.md` has the whole
table of what crosses and what does not.

## Never

- Delete a file, a folder or a drive. There is no tool for it and there will not be.
- Write a paragraph's text with its closing newline and hope to tidy up later.
- Put "•" or a tab at the start of a line to make a list — that is text, not a list.
- Set styles from indexes read before the last insertion.
- Report a document as finished without having read it back.
