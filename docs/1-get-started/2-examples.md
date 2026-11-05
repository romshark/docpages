# Examples

The repository has 2 example docs folders, each with a style of its own. Serve
one from the root of a clone of the repository to read it as a site.

## A command-line tool

[`example/cli/docs`](../../example/cli/docs/) documents wc, the Unix tool that
counts lines, words and bytes, in the style of a man page in a terminal.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="img/cli-dark.webp">
  <img src="img/cli-light.webp" alt="The Installation page of the wc docs, in the style of a man page">
</picture>

It shows:

- A logo, a favicon and an icon for each group
- A link in the header
- Callouts and a code block with a title
- A `style.css` that sets the colors and the fonts and adds the indentation,
  the capitals and the square corners of a man page

```sh title="Terminal"
go run ./cmd/docpages -serve 127.0.0.1:7340 example/cli/docs
```

## A Go library

[`example/library/docs`](../../example/library/docs/) documents ringbuf, a
made-up Go ring buffer, in a monospace style.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="img/library-dark.webp">
  <img src="img/library-light.webp" alt="The Concurrency page of the ringbuf docs, with a sequence diagram">
</picture>

It shows:

- The name of the project in place of a logo
- A group that its `README.md` opens
- A Mermaid diagram
- A link in the footer with an icon
- Links to the headings of another page
- A `<details>` that opens
- 2 releases, v1.0 and v0.9, whose note sends readers to v1.0
- A `style.css` that sets the colors and the fonts

```sh title="Terminal"
go run ./cmd/docpages -serve 127.0.0.1:7340 example/library/docs
```

## This site

The docs of docpages are an example too: a logo, a favicon, images, videos and
the colors of [`docs/style.css`](../style.css).
