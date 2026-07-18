# staplegun

A stupid-simple text templating engine for Go.

staplegun processes a directory of source documents and writes finished documents
to an output directory. Its one job is **composition**: it lets you build a large
document out of smaller pieces by *importing* one file into another and by
*defining* reusable blocks of content that can be *inserted* wherever you need
them. It also does basic *variable* substitution.

It was written for assembling HTML, but it operates on plain text and knows
nothing about HTML — you can use it for any text format (Markdown, config files,
SQL, etc.). The only HTML-flavored detail is the tracking comments it leaves in
the output (see [Output markers](#output-markers)).

- No dependencies.
- No expression language, no loops, no conditionals. Just imports, blocks, and
  variables.
- Directives look like `{{ staplegun <directive> ... }}`.

---

## Mental model

There are three ideas to understand:

1. **Documents** — every source file is either a *parent*, a *child*, or *not a
   staplegun file at all*. The first line of the file decides which. Only
   **parent** documents are written to the output directory. **child** documents
   exist only to be imported into other documents. Everything else is ignored.

2. **Import** — a document can pull the entire contents of another file into
   itself with `import_file`. This is how you split a big document into pieces
   and reassemble them.

3. **Blocks** — a document can carve out a named chunk of content with
   `define_block` … `end`, and drop that chunk in elsewhere with `insert_block`.
   Blocks are shared across a whole import tree, so a block defined in one file
   can be inserted in another (see [How resolution works](#how-resolution-works)).

Variable substitution (`var`) is a fourth, simpler feature layered on top.

---

## Installation

staplegun is a Go library (module `staplegun`). Add it to your module and import
it:

```go
import "staplegun"
```

It also ships a command-line tool for driving the same processing from build
scripts and CI (see [Command-line tool](#command-line-tool)).

---

## Quick start

Given a source directory `templates/` containing:

**`templates/parent.html`**

```html
{{ staplegun parent }}
<body>
    This file will be processed and saved

    The Foo div will get inserted here
    {{ staplegun insert_block foo }}

    The child div will get inserted here
    {{ staplegun import_file child.html }}
</body>

{{ staplegun define_block foo }}
<div>foo</div>
{{ staplegun end }}
```

**`templates/child.html`**

```html
{{ staplegun child }}
<div class="child">
    This file will be used for injections.
</div>
```

Run it from Go:

```go
package main

import "staplegun"

func main() {
    err := staplegun.MakeTemplates(
        "./templates",       // source directory
        "./out",             // destination directory (must already exist)
        false,               // verbose logging
        staplegun.VarMap{},  // variables (none here)
    )
    if err != nil {
        panic(err)
    }
}
```

staplegun writes **`out/parent.html`** (and *not* `child.html`, because it is a
child document):

```html
<body>
    This file will be processed and saved

    The Foo div will get inserted here
    <!-- sg:block:start:foo -->
    <div>foo</div>
    <!-- sg:block:end:foo -->

    The child div will get inserted here
    <!-- sg:file:start:child.html -->
    <div class="child">
        This file will be used for injections.
    </div>
    <!-- sg:file:end:child.html -->
</body>
```

Notice that the inserted content is indented to match the directive it replaced,
and each insertion is wrapped in tracking comments.

---

## Directive reference

All directives use the form `{{ staplegun <directive> [argument] }}`. Whitespace
around the braces and words is flexible.

> **Important:** every directive **except `var`** must sit on a line by itself
> (leading whitespace is fine, but nothing else may share the line). `var` is the
> only directive that can appear inline in the middle of other text.

### `{{ staplegun parent }}`

Marks the file as a **parent** document. Must be the **first line** of the file.
Parent documents are fully processed and written to the destination directory
using the same base filename.

### `{{ staplegun child }}`

Marks the file as a **child** document. Must be the **first line** of the file.
Child documents are *never* written to the destination on their own; they only
contribute content and blocks when imported by another document.

A file whose first line is neither `parent` nor `child` is treated as "not a
staplegun file" and is silently ignored (never written, and if imported it
contributes nothing).

### `{{ staplegun import_file <filename> }}`

Inserts the fully-processed contents of `<filename>` at this location. The path is
resolved **relative to the source directory** passed to `MakeTemplates` (not
relative to the importing file). Subdirectory paths like
`partials/header.html` are allowed.

The imported file must itself be a parent or child document (its first-line
directive is consumed and not emitted). Blocks defined in the imported file become
available to the importer — see [How resolution works](#how-resolution-works).

The inserted content is indented to match the `import_file` directive and wrapped
in `<!-- sg:file:start:... -->` / `<!-- sg:file:end:... -->` markers.

### `{{ staplegun define_block <name> }}` … `{{ staplegun end }}`

Defines a named, reusable block. Everything between `define_block` and the
matching `end` is captured as the block's content and **removed** from the
document's own output — a block is only rendered where it is *inserted*, never
where it is *defined*.

- Block names match `\w+` (letters, digits, underscore).
- Blocks may be empty (a `define_block` immediately followed by `end`).
- Blocks cannot be nested; a second `define_block` before an `end` is an error,
  and an unclosed block at end-of-file is an error.
- If the same block name is defined more than once, the **last** definition wins.

### `{{ staplegun insert_block <name> }}`

Inserts the content of the named block at this location, indented to match the
directive and wrapped in `<!-- sg:block:start:name -->` /
`<!-- sg:block:end:name -->` markers.

- In a **parent** document, inserting a block that was never defined anywhere in
  the import tree is a hard **error**.
- In a **child** document, an undefined block is *not* an error: the
  `insert_block` directive is left in place so that a parent higher up the import
  tree can resolve it. If it is still unresolved once processing reaches the
  parent, the parent reports the error.

### `{{ staplegun var <name> }}`

Replaced with the value supplied for `<name>` in the `VarMap` passed to
`MakeTemplates`. Unlike the other directives, `var` may appear **inline**
anywhere in a line, and a line may contain several.

- Variable names match `\w+`.
- If a name is **not** present in the `VarMap`, the directive is left untouched in
  the output (it is *not* replaced with an empty string). This makes missing
  variables easy to spot.

---

## How resolution works

Understanding block flow is the key to using staplegun for layout/partial
patterns. Blocks are shared across an entire import tree and flow in **both
directions**:

- **Down** — a block defined in a document is visible to the files it imports, so
  an imported (child) file can `insert_block` something its importer defined.
- **Up** — a block defined inside an imported file becomes visible to the importer,
  so a parent can `insert_block` something one of its children defined.

Because children defer unresolved blocks upward (see `insert_block` above), you can
write a reusable **layout** as a child document full of `insert_block` holes, and
have each **page** (a parent) import the layout and fill those holes with
`define_block`.

### Worked example: layout + page

**`src/page.html`** (a parent)

```html
{{ staplegun parent }}
PAGE-TOP
{{ staplegun import_file layout.html }}
Signed by: {{ staplegun var name }}
{{ staplegun define_block title }}
Hello from the page
{{ staplegun end }}
```

**`src/layout.html`** (a child)

```html
{{ staplegun child }}
LAYOUT title is:
    {{ staplegun insert_block title }}
LAYOUT sidebar is:
    {{ staplegun insert_block sidebar }}
{{ staplegun define_block sidebar }}
[sidebar defined in layout]
{{ staplegun end }}
```

Running with `VarMap{"name": "Eric"}` produces **`out/page.html`**:

```html
PAGE-TOP
<!-- sg:file:start:layout.html -->
LAYOUT title is:
    <!-- sg:block:start:title -->
    Hello from the page
    <!-- sg:block:end:title -->
LAYOUT sidebar is:
    <!-- sg:block:start:sidebar -->
    [sidebar defined in layout]
    <!-- sg:block:end:sidebar -->

<!-- sg:file:end:layout.html -->
Signed by: Eric
```

Here `title` flowed **down** (defined in the page, inserted in the layout) and
`sidebar` was defined and inserted within the layout itself. `out/layout.html` is
**not** written, because the layout is a child document.

---

## Output markers

Every insertion is bracketed by HTML-style comment markers so you can see, in the
finished document, exactly where content came from:

| Marker | Meaning |
| --- | --- |
| `<!-- sg:block:start:NAME -->` … `<!-- sg:block:end:NAME -->` | Content inserted by `insert_block NAME` |
| `<!-- sg:file:start:FILE -->` … `<!-- sg:file:end:FILE -->` | Content inserted by `import_file FILE` |

These markers are always emitted as HTML comments regardless of the source file's
format. If you are templating a non-HTML format where `<!-- -->` is not a comment,
be aware they will appear as literal text in the output.

---

## Processing rules & gotchas

- **Only the top level of the source directory is scanned.** `MakeTemplates`
  globs `<source>/*` and skips subdirectories. Files in subdirectories are not
  processed on their own, but they *can* be pulled in via `import_file
  subdir/name`.
- **The destination directory must already exist.** staplegun writes into it but
  does not create it. Output files are written with mode `0644`.
- **Only parent documents are written**, using the same base filename as the
  source. Child and non-staplegun files produce no output file.
- **Directives (except `var`) must be alone on their line.** A line like
  `Title: {{ staplegun insert_block title }}` will *not* be recognized as a
  directive — it will be copied through verbatim. Put the directive on its own
  line.
- **The parent/child directive must be the literal first line** of the file — no
  blank line or content before it.
- **Circular imports are not detected** (a file importing itself, directly or
  indirectly, will recurse until it crashes). Don't create import cycles.
- **Names** for blocks and variables are `\w+`; import filenames are any run of
  non-whitespace characters.

---

## Command-line tool

The same processing is available as a command so a build script can generate the
finished templates *before* compiling — useful when an app serves the processed
files from an embedded filesystem and therefore needs them to exist at build time.

The command lives in `cmd/staplegun`. Install it with the Go toolchain:

```sh
# from a checkout of this repo
go install ./cmd/staplegun

# or straight from source control (pins a version)
go install staplegun/cmd/staplegun@latest
```

That drops a `staplegun` binary in `$(go env GOBIN)` (or `$(go env GOPATH)/bin`).

### Usage

```
staplegun [flags] <sourceDir> <destDir>

Flags:
  -var name=value   define a template variable (may be repeated)
  -verbose          print a trace of the processing steps
  -version          print version information and exit
```

- `<sourceDir>` / `<destDir>` are the same two directories `MakeTemplates` takes.
- **The destination directory is created for you** (`mkdir -p` semantics) if it
  doesn't exist. This is deliberate: the processed output directory is typically
  generated and git-ignored, so it won't be present on a fresh checkout.
- Pass a `-var` flag once per variable. Values may contain `=` (only the first
  `=` separates name from value) and spaces (quote them in your shell).
- Exit status is `0` on success, `2` for bad usage (wrong argument count, malformed
  `-var`), and `1` for a processing error (bad directory, malformed block,
  undefined block in a parent, etc.).

### Example

The runtime call from an app's startup…

```go
staplegun.MakeTemplates(
    "../data/templates/raw",
    "../data/templates/processed",
    false,
    staplegun.VarMap{"cacheBustingVersion": config.Version},
)
```

…is replicated at build time as:

```sh
staplegun -var cacheBustingVersion="$VERSION" \
    ../data/templates/raw ../data/templates/processed
```

A typical Go build that embeds the processed output would run staplegun first, for
example in a `Makefile`:

```make
VERSION := $(shell git describe --tags --always)

generate:
	go run ./cmd/staplegun -var cacheBustingVersion=$(VERSION) \
		./data/templates/raw ./data/templates/processed

build: generate
	go build ./...
```

Using `go run ./cmd/staplegun` (as above) needs no install step and always
matches the pinned module version, which is convenient in CI. Use `go install`
when you want a reusable `staplegun` binary on your `PATH`.

---

## Go API

The package exposes a small surface.

### `func MakeTemplates(dirSource, dirDest string, verbose bool, varMap VarMap) error`

Processes every file at the top level of `dirSource` and writes each parent
document to `dirDest`.

- `dirSource` — directory of source templates (must exist and be a directory).
- `dirDest` — directory to write finished parent documents into (must exist and be
  a directory).
- `verbose` — when `true`, prints an indented trace of the parsing/resolution
  steps to stdout. Useful for debugging import trees and block resolution.
- `varMap` — variable name/value pairs used to resolve `{{ staplegun var ... }}`.

Returns an error if either directory is invalid, a file can't be read/written, a
block is malformed (unclosed, doubly-opened, closed with no open block), or a
parent references an undefined block.

### `type VarMap map[string]string`

Simple map of variable name → replacement value, passed to `MakeTemplates`.

```go
vars := staplegun.VarMap{
    "site_name": "Acme",
    "year":      "2026",
}
```

### `func VersionInfo() string`

Returns a human-readable name-and-version string, e.g. `"staplegun v1.0.0"`.

---

## Syntax cheat sheet

```
{{ staplegun parent }}                     first line only — output is written
{{ staplegun child }}                      first line only — output is not written

{{ staplegun import_file <filename> }}     splice in another file (path relative to source dir)

{{ staplegun define_block <blockname> }}   start a reusable block (content is captured, not emitted here)
{{ staplegun end }}                         end the block

{{ staplegun insert_block <blockname> }}   emit a block's content here

{{ staplegun var <varname> }}              replace with a VarMap value (may appear inline)
```

---

## License

MIT — see [LICENSE.md](LICENSE.md).
