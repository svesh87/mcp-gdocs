*English · [Русский](docs/README.ru.md)*

# mcp-gdocs

An MCP server for Google Slides, Sheets, Docs and Drive that acts as one signed-in
person and edits presentations the way the editor does.

The presentations are the point. A general-purpose Google server edits a deck by moving
text boxes to coordinates it invented, writing bullet characters into plain text and
replacing a real table with a scattering of shapes. What comes out looks broken to
everyone who opens it, and nobody can fix it without redoing the slide. The tools here
work with the template instead of around it: native nested lists, styles read off the
deck's own slides, real tables, and a thumbnail to check the result with.

## Why the narrow slides tools work

Each of these is a decision, not an implementation detail:

- **Native lists, not hand-drawn ones.** `gdocs_slides_set_list` sends the text with tab
  characters for depth and asks Slides to make a list of it. Google then works out the
  indents, the markers and the spacing from the template. A server that positions bullets
  itself gets them almost right, which is worse than wrong.
- **Styles are read, never invented.** `gdocs_slides_inspect_title_style` reports what a
  real title on a real slide is set to, field by field, and `gdocs_slides_set_text_style`
  writes those fields. Nothing guesses what "the heading font" is.
- **Look before changing.** `gdocs_slides_inspect_text_structure` shows what is in a text
  box — paragraphs, bullets, nesting — before anything is replaced.
- **A table is a table.** `gdocs_slides_create_table_with_text` creates a real Slides
  table with column widths, fonts and per-column alignment. Rows of text boxes look
  similar until somebody edits one.
- **Check with your eyes.** `gdocs_slides_export_thumbnail` renders the slide so an agent
  can look at what it did rather than trust the batch it sent.
- **No arbitrary batches.** There is no tool that forwards a caller's own requests to
  `presentations.batchUpdate`. That tool is what produces crooked decks, so it does not
  exist here.

The approach comes from a working server a colleague wrote for exactly this problem;
this is that behaviour in Go, with the rest of the Google editors around it.

## Removal stops at the bin

Inside a presentation, a document or a workbook, removal is ordinary editing: a copied
deck comes down to the slides that apply, and a step that landed wrong leaves a paragraph
or a table behind that somebody has to be able to take out. A file itself goes no further
than the bin, where its owner can find it again for thirty days. Emptying that bin,
deleting a file outright, and removing a folder or a drive have no code here at all — a
test checks that, and checks the client behind the tools as well.

None of it is on by default. Every removing tool lives in a group of its own —
`slides-delete`, `docs-delete`, `sheets-delete`, `drive-delete` — and a configuration that
wants one names it: `--tools=all,docs-delete`. The same is true of sharing, which is the
one thing here that makes something visible outside the account and lives in `drive-share`.

Removing a whole page takes a second switch on top: `slides-delete-page` for a slide,
`sheets-delete-tab` and `docs-delete-tab` for a tab. The line is what a mistake costs — a
stray shape is a moment's work to put back, a tab of a workbook is data nobody has any more —
and it makes "tidy up the shapes but do not drop a slide" something a configuration can say.
A refusal names the group that is missing, so a narrow server does not look like a broken one.

Going back is `gdocs_drive_restore_revision`. For an ordinary file it is exact. For a Google
document, workbook or deck there is no restore request at all, so the version is exported and
written back, and the conversion loses things — the tool refuses without `confirm_conversion`
and lists what. The file keeps its identifier either way, so every link and permission
survives, and the restore is itself a new version rather than an undo.

Everything that changes anything at all needs `--allow-write`. Without it the server
offers only the reading tools.

## Choosing the set of tools

A hundred and forty-nine tool descriptions in an agent's context is a hundred and forty it
will never call. `--tools` picks what a server offers, by family and by what the tools do:

```
--tools=slides-read,docs-read     # exactly those two groups
--tools=docs                      # the family: reading and writing, nothing else
--tools=all,sheets-delete         # everything ordinary, plus one kind of removal
--tools=slides,slides-copy        # slides, and bringing content in from other decks
GDOCS_TOOLS=drive-read            # the same through the environment, for compose
```

The groups are `slides-`, `sheets-` and `docs-`, each with `read`, `write`, `copy` and
`delete`, plus one more apiece for removing a whole page — `slides-delete-page`,
`sheets-delete-tab`, `docs-delete-tab`. `drive-` has all but the copying and page ones, plus
`drive-share`. `copy` is the tools that carry
content in from another document, which is the kind of thing a project may want to switch
off on its own — "work here, but do not drag things in from elsewhere". Copying a *file* is
not in it: `gdocs_drive_copy` and `gdocs_slides_copy_presentation` are ordinary Drive
writing, and they are how a deck starts from a template.

Naming a family means its reading and writing halves and nothing more; `all` means
everything except removal and sharing, copying included. An unknown name is refused at
startup with the list of the known ones. Left out altogether, the set is everything except
removal and sharing.

Over HTTP the same process also serves one path per family, so a project can connect to
just the one it needs:

```
/mcp          the whole set
/mcp/slides   /mcp/sheets   /mcp/docs   /mcp/drive
```

They are windows on one server — same sign-in, same token, same token store — and a
window cannot show what `--tools` did not allow.

## Tools

A hundred and forty-nine of them, covering every request the three APIs have except the
five that reach BigQuery. A server started without `--allow-write` registers the reading
ones and nothing else, and the four that touch the disk — `gdocs_drive_export_file`,
`gdocs_drive_download_file`, `gdocs_drive_import_file` and `gdocs_slides_export_images` —
appear only when `--files-dir` names a directory they may use.

They come in pairs on purpose: whatever a reading tool reports, a writing tool takes back
in the same units. No tool answers "make this look like that" — the job is to build a deck
that looks right, having read how a dozen others are built, and a style transferred behind
the caller's back hides the numbers it was made of: a caller that never learns the sample's
headings are 25 pt in the theme's accent colour cannot decide to use 22 pt here, or to keep
the size and change the colour.

Carrying content between documents is a different question and has its own tools, in groups
of their own — `slides-copy`, `sheets-copy`, `docs-copy`. Those groups also hold the bridges
between the three kinds of document, which are named for where the content lands and read the
family it comes from: three things mean the same thing in all three — a table is values with a
look per cell, text is paragraphs with a look per run, a picture is an address — and a bridge
carries those and names the rest. That does mean a window on one family can read another: the
sub-paths stay a boundary on what may be changed, and stop being one on what may be read. "Bring that slide here", "put this
tab in that workbook", "take these paragraphs from the last offer" are ordinary work, and
the answer to each is exact rather than approximate. Google gives one request for it in
total, `sheets.copyTo`; everywhere else these tools read the source and build it again, and
say in the answer what they could not carry rather than leaving it to be discovered. What
never crosses is a deck's theme: no request in the Slides API applies one, so a deck that
must look like a sample is started as a copy of it.

Reading, always offered:

| Tool | What it does |
|---|---|
| `gdocs_slides_list` | slides of a deck with the objects on each: identifiers, kinds, placeholders, sizes |
| `gdocs_slides_inspect_page` | one slide completely: background, notes, and every element with its box, turn, stacking, fill, outline and crop |
| `gdocs_slides_read_theme` | the palette, the master's background, and every layout with the styles its placeholders impose |
| `gdocs_slides_list_layouts` | the layouts the deck's own template offers |
| `gdocs_slides_inspect_text_structure` | paragraphs of a text box: bullets, nesting, styles, spacing and indents |
| `gdocs_slides_inspect_title_style` | what a line of text really looks like, merged across text, layout and master |
| `gdocs_slides_read_table` | a table's cells, column widths, row heights and cell styling |
| `gdocs_slides_export_thumbnail` | a rendered picture of one slide |
| `gdocs_sheets_info` | a spreadsheet's tabs, their identifiers and sizes |
| `gdocs_sheets_read` | cells of a range or a whole tab |
| `gdocs_sheets_read_format` | a range cell by cell, with widths, heights, merges, links, notes and dropdowns |
| `gdocs_sheets_read_dropdown_colors` | the colours a dropdown paints its options in, which no API answer carries |
| `gdocs_docs_read` | a document as text, tables tab-separated |
| `gdocs_docs_read_structure` | a document as it is built: paragraphs, runs, lists, tables, sections, headers, named styles, pictures |
| `gdocs_docs_list_named_ranges` | the names in a document and where they currently are |
| `gdocs_sheets_list_metadata` | labels attached to rows, columns or tabs, which move with them |
| `gdocs_drive_search` | files across Drive and shared drives |
| `gdocs_drive_file_info` | one file's name, kind, owners and folders |
| `gdocs_drive_list_folder` | what is in a folder |
| `gdocs_drive_list_permissions` | who can reach a file, and whether it is open by link |
| `gdocs_drive_list_comments` | comments with their replies, what they hang on, and whether they are resolved |
| `gdocs_drive_list_revisions` | the saved versions of a file |
| `gdocs_drive_export` | a file exported to another format |

Writing, only with `--allow-write`.

Slides, the content of a deck:

| Tool | What it does |
|---|---|
| `gdocs_slides_create` | an empty deck on the default theme, for a look built rather than inherited |
| `gdocs_slides_copy_presentation` | start a deck from a template, keeping master, layouts and fonts |
| `gdocs_slides_add_slide` | add a slide following one of the deck's own layouts |
| `gdocs_slides_set_text` | replace a text box's text, styling left to the template |
| `gdocs_slides_replace_text` | swap a stretch of text everywhere it appears, keeping the styling around it |
| `gdocs_slides_set_list` | turn a text box into a native nested list, depth by tab characters |
| `gdocs_slides_copy_slide` | build a slide from another deck again here: content, not theme |
| `gdocs_slides_copy_element` | the same for one element |
| `gdocs_slides_copy_table_from_sheets` | a rectangle of a workbook as a real table, values as shown |
| `gdocs_slides_copy_text_from_docs` | a stretch of a document as a text box |
| `gdocs_slides_create_table_with_text` | a real table with widths, fonts, colours and alignment |
| `gdocs_slides_update_table_cells` | new values in a table that already exists, keeping its widths and styling |
| `gdocs_slides_style_table` | merge cells, fill them, align their content, set row heights |
| `gdocs_slides_insert_image` | put a picture on a slide by address |
| `gdocs_slides_create_shape` | a text box, or a panel, arrow or circle, with its text, fill and outline |
| `gdocs_slides_create_line` | a line, an arrow or a connector |
| `gdocs_slides_set_speaker_notes` | replace the notes behind a slide |
| `gdocs_slides_reorder` | put the slides in a given order |
| `gdocs_slides_delete` | remove a slide or an element of one, and nothing outside the deck |

Slides, how it looks:

| Tool | What it does |
|---|---|
| `gdocs_slides_place_element` | move, resize, turn or mirror anything, by edge, by anchor or by a sample's own place |
| `gdocs_slides_order_elements` | say what covers what |
| `gdocs_slides_group` | join elements into a group, or take groups apart |
| `gdocs_slides_set_page_background` | a colour, a picture, or back to the layout's |
| `gdocs_slides_style_shape` | fill, outline and content alignment of a shape |
| `gdocs_slides_style_image` | crop, transparency, brightness, contrast and border of a picture |
| `gdocs_slides_set_text_style` | size, font, weight, italics, colour — literal or by theme name |
| `gdocs_slides_set_paragraph_style` | alignment, line spacing, the space around paragraphs, indents |
| `gdocs_slides_reset_text_style` | give text back to its layout |
| `gdocs_slides_link_text` | turn a piece of text into a link |
| `gdocs_slides_style_layout` | write a style into a layout or the master, so every slide following it inherits |
| `gdocs_slides_set_theme_colors` | replace the deck's palette, all twelve colours at once |

Sheets, Docs and Drive:

| Tool | What it does |
|---|---|
| `gdocs_sheets_write` | write a rectangle of cells |
| `gdocs_sheets_append` | add rows after the last one, inserting rather than overwriting |
| `gdocs_sheets_create` | create a spreadsheet with its locale and the size of each tab |
| `gdocs_sheets_add_tab` | add a tab, with its size |
| `gdocs_sheets_duplicate_tab` | copy a tab inside the same workbook |
| `gdocs_sheets_update_properties` | title, locale and time zone of an existing workbook |
| `gdocs_sheets_format_cells` | font, weight, colours, both alignments, wrapping, number format, rotation, padding, link, note |
| `gdocs_sheets_set_text_runs` | style parts of one cell's text differently |
| `gdocs_sheets_set_borders` | edges of a rectangle and the lines inside it |
| `gdocs_sheets_set_validation` | put a dropdown on a rectangle of cells |
| `gdocs_sheets_set_conditional_format` | colour cells by what is in them, by condition or gradient |
| `gdocs_sheets_set_banding` | alternating stripes that follow rows added to the range |
| `gdocs_sheets_set_filter` | the tab's filter: what each column hides, how it sorts |
| `gdocs_sheets_protect_range` | keep a range from being changed |
| `gdocs_sheets_add_named_range` | give a range a name formulas can use |
| `gdocs_sheets_set_layout` | widths, heights, frozen rows, merges, hiding, tab colour, a bigger grid |
| `gdocs_sheets_insert_dimensions` | add rows or columns in the middle or at the end |
| `gdocs_sheets_move_dimensions` | move rows or columns elsewhere on the tab |
| `gdocs_sheets_group_dimensions` | fold a run of rows or columns into a group |
| `gdocs_sheets_collapse_group` | fold a group up or open it |
| `gdocs_sheets_sort_range` | sort a rectangle by its columns |
| `gdocs_sheets_find_replace` | replace text across a range, a tab or the workbook |
| `gdocs_sheets_trim_whitespace` | take the spaces off both ends of every cell |
| `gdocs_sheets_split_column` | split one column into several by a separator |
| `gdocs_sheets_auto_fill` | carry a series on the way dragging a corner does |
| `gdocs_sheets_add_chart` | draw a column, bar, line, area, scatter or pie chart from a range |
| `gdocs_sheets_add_table` | turn a rectangle into a table with typed columns |
| `gdocs_docs_create` | create a document |
| `gdocs_docs_append` | add text at the end |
| `gdocs_docs_insert_text` | insert text at a position, in the body or in a header |
| `gdocs_docs_replace_text` | replace every occurrence of a string |
| `gdocs_docs_style_text` | weight, slant, size, font, colours and links over a range |
| `gdocs_docs_style_paragraph` | named style, alignment, indents, spacing, borders, shading |
| `gdocs_docs_style_named` | what NORMAL_TEXT, TITLE or a heading means in this document |
| `gdocs_docs_style_document` | paper size, margins, background, header and footer margins |
| `gdocs_docs_make_bullets` | turn paragraphs into a list, depth by tab characters |
| `gdocs_docs_insert_table` | put a table in |
| `gdocs_docs_style_table` | cell fills, borders, padding, column widths, row heights, merges, pinned rows |
| `gdocs_docs_insert_section_break` | start a section, which is what carries its own header |
| `gdocs_docs_style_section` | margins and columns of one section |
| `gdocs_docs_add_header_footer` | make a header or a footer and hand back its segment |
| `gdocs_docs_insert_image` | a picture from an address Google can fetch |
| `gdocs_docs_insert_page_break` | start a new page |
| `gdocs_docs_insert_footnote` | a footnote and the segment to write it in |
| `gdocs_docs_add_named_range` | name a stretch of a document, so later edits find it without counting characters |
| `gdocs_docs_fill_named_range` | replace what a named range holds — the safe way to fill a template |
| `gdocs_docs_add_tab`, `gdocs_docs_update_tab` | tabs of a document |
| `gdocs_docs_insert_chip` | a smart chip: a person, another Google file, or a date |
| `gdocs_docs_replace_image` | swap a picture's content while it keeps its place and size |
| `gdocs_docs_edit_table` | add a row or column to a table, or take a merge apart |
| `gdocs_docs_delete` | remove something inside a document: a range, a table row or column, a header, a footer, a floating object, a tab, a named range, the bullets of a list |
| `gdocs_slides_edit_table` | grow a table on a slide, or take a merge apart |
| `gdocs_slides_set_table_borders` | the lines of a table, by position across a rectangle |
| `gdocs_slides_replace_image` | swap a picture's content while it keeps its place and crop |
| `gdocs_slides_replace_shapes_with_image` | turn every shape whose text matches into a picture |
| `gdocs_slides_replace_shapes_with_chart` | the same, with a chart from a workbook |
| `gdocs_slides_set_alt_text` | the description a screen reader reads out |
| `gdocs_slides_route_line` | how a connector runs, and rerouting it after the shapes moved |
| `gdocs_slides_add_sheets_chart`, `gdocs_slides_refresh_sheets_chart` | a live chart from a workbook |
| `gdocs_slides_add_video` | a video from YouTube or Drive, with how it plays |
| `gdocs_sheets_move_range` | copy or move a rectangle: values, formatting, formulas, validation |
| `gdocs_sheets_paste_text` | paste delimited text or an HTML table, split on Google's side |
| `gdocs_sheets_shape_range` | insert cells and push the rest aside, or shuffle rows |
| `gdocs_sheets_append_rows` | add rows after the last one with anything in it |
| `gdocs_sheets_update_chart` | move a chart, frame it, change its titles and labels, or point it at a different range — keeping its number, and so every slide that shows it |
| `gdocs_sheets_copy_sheet` | copy a tab into another workbook, with everything Google copies and this server cannot write |
| `gdocs_sheets_copy_range` | copy a rectangle into another workbook: what was typed, with its format, notes and dropdowns |
| `gdocs_docs_copy_range` | build a stretch of another document again here: paragraphs, runs, lists, pictures |
| `gdocs_docs_copy_table_from_sheets` | a rectangle of a workbook as a real table, in two passes |
| `gdocs_docs_copy_slide_image` | a picture of a slide, for a report quoting a deck |
| `gdocs_sheets_copy_table_from_docs` | a table out of a document or off a slide, as cells that can be summed |
| `gdocs_sheets_filter_view` | a saved way of looking at a range, without changing anybody else's view |
| `gdocs_sheets_slicer` | the control a reader clicks to filter by one column |
| `gdocs_sheets_set_metadata` | a label that travels with the row it is attached to |
| `gdocs_sheets_delete` | remove something inside a workbook: rows, columns, cells, a tab, a grouping, a banding, a rule, a protection, a named range, a filter view, duplicates, a chart, a table, a label |
| `gdocs_drive_copy` | copy a file |
| `gdocs_drive_create_folder` | make a folder |
| `gdocs_drive_rename`, `gdocs_drive_move` | a file's name and where it sits |
| `gdocs_drive_add_comment`, `gdocs_drive_reply_comment` | leave a comment, answer one, resolve a thread |
| `gdocs_drive_keep_revision` | keep a version from being pruned |
| `gdocs_drive_restore_revision` | put a file back to an earlier version, keeping its identifier — exact for an ordinary file, through a conversion for a Google one |
| `gdocs_drive_delete_to_trash` | put a file in the bin, or take it back out |
| `gdocs_drive_share`, `gdocs_drive_unshare` | give access and take it back — only with `--tools=…,drive-share` |

## Access

The server acts as one person: whoever completed the sign-in. There is no service
account and no domain-wide delegation, deliberately — a service account key with
delegation can impersonate any employee without their knowledge, and an image carrying
one would be a company-wide compromise waiting for a leak. See
[docs/SETUP.md](docs/SETUP.md) for the Google Workspace setup, field by field, and the
reasoning behind it.

Two secrets, kept apart:

- The **OAuth client** identifies the application. Either the file Google Cloud Console
  downloads, named by `--credentials`, or its two halves as `GOOGLE_OAUTH_CLIENT_ID` and
  `GOOGLE_OAUTH_CLIENT_SECRET` — which is what a password store hands over, and then
  nothing about the application is on disk at all. The environment wins when both are
  given, so a file left over from an earlier setup cannot quietly outrank it.
- The **token** is what one person's consent turned into. It is created by their own
  sign-in and stays a file, 0600, in the directory named by `--token-dir`. A file rather
  than a variable because the server rewrites it whenever the access token is refreshed,
  and a secret store is not somewhere a process writes to on its own.

Neither is ever committed or built into an image; `.gitignore` and `.dockerignore` carry
both.

## Signing in

On a machine with a browser:

```bash
mcp-gdocs login --credentials ./gcp-oauth.keys.json --token-dir ./tokens
```

It prints an address, waits on a loopback port for the browser to come back, and writes
the token.

In a container there is no browser, so the sign-in happens through a page the server
serves itself. Publish its port on loopback and open:

```
http://127.0.0.1:8819/login?key=<the server's MCP_AUTH_TOKEN>
```

The page says whether anybody is signed in and what will be asked for, and one button
sends the browser to Google's consent screen. Google returns it to `127.0.0.1` on the
same port — an address a Google client of type Desktop accepts without anything being
registered in the console — and the token is written on the way back.

The key is in the address because a browser sends no `Authorization` header of its own,
so the page is served `no-store` and `no-referrer`, and the trip to Google is a form
submission rather than a link: a forwarded `/login` address shows a page, not a consent
screen.

For a server on another machine, tunnel the port rather than exposing it — Google only
redirects to loopback: `ssh -L 8819:127.0.0.1:8819 <host>`.

## Running

```bash
mcp-gdocs --credentials /config/gcp-oauth.keys.json --token-dir /config/tokens --allow-write
```

| Flag | What it does |
|---|---|
| `--transport` | `stdio` (default) or `streamable-http` |
| `--address` | address to listen on with `streamable-http`, default `127.0.0.1:8819` |
| `--credentials` | path to the OAuth client file. Required unless the client comes from the environment |
| `--token-dir` | directory the token lives in. Required |
| `--scopes` | scopes to ask for, comma separated. Empty means `drive`, `spreadsheets`, `presentations`, `documents` |
| `--allow-write` | register the tools that change documents. Off by default |
| `--version` | print the version and exit |

Three environment variables, because a secret in a flag shows up in a process list:

| Variable | What it is |
|---|---|
| `MCP_AUTH_TOKEN` | bearer token the HTTP transport demands. The server refuses to start on `streamable-http` without it: whatever reaches that port acts as the person who signed in |
| `GOOGLE_OAUTH_CLIENT_ID` | the OAuth client, instead of `--credentials`. Both halves or neither |
| `GOOGLE_OAUTH_CLIENT_SECRET` | the other half |

The full `drive` scope rather than `drive.file` is deliberate: a deck is made by copying
a template, and `drive.file` only ever sees files the application created itself, so it
cannot open the template at all.

In a container, with the client injected and only the token mounted:

```bash
docker run --rm -i --user "$(id -u):$(id -g)" \
  -e GOOGLE_OAUTH_CLIENT_ID -e GOOGLE_OAUTH_CLIENT_SECRET \
  -v "$HOME/.local/share/mcp-gdocs:/tokens" \
  ghcr.io/svesh87/mcp-gdocs:latest \
  --token-dir /tokens --allow-write
```

The image is `FROM scratch` and runs as 65532, so whoever mounts a token directory
overrides the user with their own identifier — otherwise the directory is not writable
and the server exits saying so.

## The skills

One per editor, because a deck and a workbook go wrong in different ways:

- [`skills/gdocs-slides/`](skills/gdocs-slides/SKILL.md) — building a deck: the order of
  calls, what to read before changing anything, why blocks are never moved by hand.
- [`skills/gdocs-sheets/`](skills/gdocs-sheets/SKILL.md) — building a workbook: why
  formatting is a decomposition into rectangles rather than a wash, and why the size and
  the locale are decided at creation.

Each has the same two references beside it, because the way of working and the facts about
the API age differently: `references/controls.md` lists every knob by what it changes —
what reads it, what writes it, in which units — and `references/pitfalls.md` lists the
traps, each with what it does to the document and what to do instead, ending with the
handful of things the API will not report at all.

## Development

```bash
gofmt -l .
go vet ./...
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1     # not below 80%
docker build -t mcp-gdocs:dev .
```

The tests hold **golden request bodies**: the exact JSON this server sends to Slides,
Sheets and Docs, in `internal/tools/testdata/`. They are the point of the test suite,
because what a deck ends up looking like is decided by those bodies and not by which
methods were called. `go test ./... -update` rewrites them, and a rewrite belongs in a
commit that says what changed about the result.

## Credit

The approach to editing presentations — native nested lists, styles copied from the
template, real tables, thumbnails for verification, and no arbitrary `batchUpdate` — is
taken from a colleague's Node server built for the same problem.

Request shapes and endpoint use were checked against
[`piotr-agier/google-drive-mcp`](https://github.com/piotr-agier/google-drive-mcp), MIT
licensed. No code was copied; this server is Go and its own.
