*English · [Русский](AGENTS.ru.md)*

# Rules for this repository

mcp-gdocs is an MCP server for Google Slides, Sheets, Docs and Drive, in Go. The points
below are the ones that are expensive to rediscover; the rest of the reasoning is in
[README.md](README.md) and [docs/SETUP.md](docs/SETUP.md).

## What this server will not do

- **Deletion stops at the bin.** Inside a presentation, a document or a workbook, removal
  is ordinary editing: a copied deck comes down to the slides that apply, a step that
  landed wrong leaves a paragraph or a table behind, and without a way back the only
  repair is a person in a browser. A file itself can go as far as the bin, which its
  owner can undo for thirty days. Emptying that bin, deleting a file outright, and
  removing a folder or a drive have no code here at all.

  Every removing tool lives in a group of its own — `slides-delete`, `docs-delete`,
  `sheets-delete`, `drive-delete` — and none of them is in the default set: a
  configuration that wants one names it in `--tools`. `deletionTools` in
  `register_test.go` is the full list, and a new name that removes anything fails the
  build until it is added there deliberately.
- **No arbitrary `batchUpdate`.** A caller must not be able to hand this server its own
  list of API requests. Assembled batches are exactly what puts text boxes at invented
  coordinates and leaves a deck looking broken. Every tool builds its own requests.
- **Copying carries content, never a look.** Bringing a slide, a tab or a stretch of a
  document in from elsewhere is ordinary work and has tools of its own, in groups of their
  own — `slides-copy`, `sheets-copy`, `docs-copy`. A copy of a *file* is Drive's ordinary
  writing and stays in `*-write`; `fileCopyTools` in `groups.go` names the two exceptions,
  because putting them behind the copy switch would break starting a deck from a template.
  What stays refused is "make this look like that": a style moved behind the caller's back
  hides the numbers it was made of. Google gives one real cross-document request,
  `sheets.copyTo`; everywhere else the source is read and built again in **one pass** — an
  image address dies in about thirty minutes — and whatever could not be carried is **named
  in the answer**. A copying tool that reports success while quietly leaving a table behind
  is a defect, not a limitation.
- **No service account, no domain-wide delegation.** The server acts as the person who
  signed in, and nothing else. The reasoning is in `docs/SETUP.md`; it is not a default
  to revisit casually.

## Slides

- **EMU stays EMU.** Positions and sizes go in and out in English Metric Units, the unit
  Slides itself uses. Do not convert to points "for convenience": the rounding is what
  turns into visibly shifted layout.
- **Depth is a tab character.** A nested list is built by sending text with tabs and
  asking Slides to make a list of it. Never place indents or bullet characters by hand.
- **Styles are read, not invented.** A style applied to a title comes from a real run of
  text on a real slide, together with the field mask that says which parts of it apply.
- **Text is counted in UTF-16 code units.** `utf16Length` exists for that: for Russian a
  byte count is twice too large, the range lands in the middle of nothing and the API
  refuses the whole batch.

## Golden files

`internal/tools/testdata/*.json` hold the exact request bodies this server sends. They
are the test suite's point: what a deck ends up looking like is decided by those bodies,
not by which methods were called.

`go test ./... -update` rewrites them. A rewrite belongs in a commit that says **what
changes about the result in the document** — "the title's indent is now reset before the
bullets are made, so a rebuilt slide no longer inherits the list indent". A golden diff
with no such explanation is a defect, not a formatting change.

## Gates

```bash
gofmt -l .
go vet ./...
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1     # not below 80%
docker build -t mcp-gdocs:dev .
```

Nothing is worked around: no weakened assertions, no deleted tests, no files excluded
from coverage.

There is no Go toolchain on the owner's machine. The gates run in a container:

```bash
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/src" -w /src \
  -v <cache-outside-the-tree>:/cache \
  -e HOME=/cache/home -e GOCACHE=/cache/build -e GOMODCACHE=/cache/mod \
  golang:1.25-alpine go test ./...
```

The caches must live outside the working tree: a module cache under it gets scanned as
packages and `go mod tidy` fails.

## Secrets

`gcp-oauth.keys.json` and the token directory are never committed and never reach an
image. Both are in `.gitignore` and `.dockerignore`. Errors must not quote the contents
of either file — an error message ends up in logs.

The OAuth client may also arrive as `GOOGLE_OAUTH_CLIENT_ID` and
`GOOGLE_OAUTH_CLIENT_SECRET`, which is how it comes out of a password store, and the
environment outranks the file. Secrets stay out of flags, because a flag is visible in a
process list. The token is the exception that stays a file: the server rewrites it on
every refresh, and a secret store is not somewhere a process writes to on its own.

## Documentation is bilingual

Every document has an English original and a Russian copy: `README.md` ↔
`docs/README.ru.md`, `docs/SETUP.md` ↔ `docs/SETUP.ru.md`, this file ↔ `AGENTS.ru.md`.
Each starts with the language line — `*English · [Русский](…)*` — and English is the
original: a change lands there first and the Russian copy is brought level in the same
commit. A copy that drifted is worse than no copy, because it is read as current.

Three things stay in one language on purpose. Code, comments and error messages are
English, because they end up in agent transcripts and issues. So is everything under
`skills/` — it is read by a model, not by a person. The sign-in
page (`internal/auth/login_page.go`) is Russian, because it is read by the people who
sign in rather than by anyone reading the repository.

## Drafts

Plans, audits and trial runs live in `tmp/`, which is not committed and is never
referenced from anything that is.
