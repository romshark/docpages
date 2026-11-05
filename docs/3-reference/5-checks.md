# Checks

A build stops at the mistakes below and names the file of each, and in
Markdown and CSS its line, as `docs/2-guide/1-layout.md:12:`. `-serve` stops
at them when it starts. A mistake that you make while it serves shows in the
page until you fix it.

## Folders

- A docs folder without a group, as `1-get-started/`.
- A group without a page, and a group in a group.
- A Markdown file in a group without a position, as `1-install.md`, and 2
  entries of a folder with the same position.
- A release folder not named after a minor version, as `v1.2`.
- A file that nothing uses: no page shows it, and no link or setting names
  it. That catches an image that no page shows any more, and a page in a
  folder without a position, which the navigation would leave out. Hidden
  files don't count. In a docs folder at the root of the repository, only the
  groups count.

## Front matter

- YAML that doesn't parse, a key that docpages doesn't know, and front matter
  without its closing `---`.
- `url` with `source`, `url` without `title`, and a `source` outside the
  repository.
- Site settings outside `docs/README.md`, `note` outside the `README.md` of a
  docs folder, and a note of more than 1 paragraph.
- A missing icon of a link of the header or the footer, or of the host, and a
  host without a name or a file URL.

## Pages

- A page without a level 1 heading or a `title`.
- A link to a heading that its page lacks.
- A link of a release that leaves the folder of the release.
- An image or a video whose file doesn't exist, and a media file of a release
  outside its folder.
- A code block in a language that
  [Chroma doesn't highlight](https://github.com/alecthomas/chroma#supported-languages).
  A block without a language and a `mermaid` block pass.
- Raw HTML that the page would leave out: an element other than `<img>`,
  `<picture>`, `<source>`, `<video>`, `<audio>`, `<track>`, `<details>` and
  `<summary>`, an attribute other than
  [those that pages keep](../2-guide/3-media.md#the-html-that-pages-keep), a
  `javascript:` URL and text outside a `<details>`. A comment passes, which
  GitHub hides too.
- Lines that start with `|` but make no table, which needs a delimiter row as
  `| --- | --- |` below its first row, with as many cells.
- A list item that CommonMark reads as text: `-item` or `1.item`, without the
  space after the marker, and a list that starts at another number than 1
  right below a paragraph.

## Stylesheet

[esbuild](https://esbuild.github.io) reads `style.css`, which stops a build at
what a browser would skip, with its line and column, as
`docs/style.css:12:3: "colr" is not a known CSS property (Did you mean "color" instead?)`:

- A `{` or `(` that the file doesn't close, and a `}` that nothing opens.
- A string without its end, and a comment without its `*/`.
- A declaration without its colon, as `color red;`, and a rule without a
  selector.
- A property that CSS doesn't have, as a typo. A custom property, as
  `--accent`, and a name with a vendor prefix, as `-webkit-line-clamp`, pass.
- An `@import` after other rules.
- What esbuild passes and docpages checks: a declaration without a value, as
  `color: ;`, and an at-rule that CSS doesn't have, as `@meida`.

Nesting passes, as in `.card { &:hover { … } @media (…) { … } }`.

## Not checked

- Links to other files of the repository, which lead to the repository host.
  Under `-serve`, a missing one shows as a missing file.
- Links to other sites.
- The values of CSS properties, as `color: rde`, and selectors.
