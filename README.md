# docpages

docpages renders the Markdown files of a docs directory of a Git repository as a static documentation site, with a version for the main branch and a version for each release. Links between the files work on the site as they do on GitHub, and a link to a heading that does not exist fails the build.

```sh
go run github.com/romshark/docpages/cmd/docpages@latest -o site docs
```

## Layout

The layout of the docs directory makes the navigation. No configuration file lists the pages.

```
docs/
  README.md               the first page, at the root of the version
  logo.svg                the logo of the header
  favicon.svg             the icon of the browser tab, or favicon.png or favicon.ico
  style.css               loaded after the stylesheet of docpages
  1-get-started/          a group of the navigation, "Get started"
    icon.svg              the icon of the group
    1-installation.md     a page, at get-started/installation
    2-upgrading.md        a page, at get-started/upgrading
  2-tutorial/
    README.md             the first page of the group, at tutorial/
    1-create-project.md   at tutorial/create-project
  versions/               the docs of the releases, see Versions
    v1.2/                 a docs directory of the same layout, for v1.2.x
```

- A directory whose name starts with a number and `-` is a group. The numbers order the groups. The rest of the name is the title, with `-` as a space and the first letter capitalized.
- A Markdown file in a group whose name starts with a number and `-` is a page. The numbers order the pages of the group, which its `README.md` opens. Leading zeros, as in `01-pages.md`, keep a listing sorted by name in the same order.
- The `README.md` of the docs directory is the first page and the first entry of the first group.
- The numbers stay out of the URLs and the titles. The title of a page is the text of its level 1 heading.
- `logo.svg`, `favicon.svg`, `style.css` and the `icon.svg` of a group are optional. Without a logo the header shows the name of the project. The pages inline the SVG files, so `fill="currentColor"` takes the color of the theme. The site holds `style.css` minified, and `-serve` serves it as written.

A Markdown file without a number, other than `README.md`, fails the build, as do 2 entries with the same number and a group in a group. A directory without a number, as `icons/`, holds files that the front matter names, except `versions/`, which holds the docs of the releases.

## Front matter

A page may start with YAML front matter between 2 lines of `---`. GitHub shows it as a table above the text.

| Key | Effect |
| --- | ------ |
| `title` | Replaces the level 1 heading in the navigation and in `<title>`. |
| `suggested` | `true` makes the search offer the page while its field is empty. |
| `source` | A Markdown file, relative to the page, that the page shows in place of its own text, as `../../CHANGELOG.md`. The file stays where it is, and links to it lead to the page. |
| `url` | Makes the entry a link that leaves the site, as to an API reference. Requires `title`. |
| `note` | Only in the `README.md` of the docs directory or of a release: replaces the note above every page of that version. See [Versions](#versions). |

The front matter of the `README.md` of the docs directory also holds the settings of the site.

| Key | Effect |
| --- | ------ |
| `name` | The name of the project in `<title>` and `llms.txt`. The default is the last element of the repository URL. |
| `home` | The URL that the logo leads to, relative to the output directory, as `../` for a landing page above it. The default is the first page of the version. |
| `repository` | The URL of the repository. The default is the URL of the Git remote `origin`. |
| `host` | Where the repository is hosted, GitHub by default. See [Hosting](#hosting). |
| `header` | Links of the header before the repository, each with `title`, `url` and an optional `icon`, the path of an SVG file. The default is 1 link to the first page, `Docs`, and `[]` leaves the header without links. |
| `footer` | Links of the footer between the repository and `llms.txt`, each with `title`, `url` and an optional `icon`, the path of an SVG file. |
| `license` | The line that ends the footer, as written, as `MIT License © 2026 Jane Doe`. Without it the footer has none. |

A `url` of `header` and `footer` is a link of that `README.md`: a Markdown file leads to its page. A link of the header is the current one on the page that it leads to, and a link to the first page on every page that no other link of the header leads to.

```yaml
---
name: Datapages
home: ../
suggested: true
header:
  - title: Guide
    url: README.md
    icon: icons/guide.svg
  - title: FAQ
    url: 6-more/2-faq.md
footer:
  - title: Changelog
    url: 5-reference/5-changelog.md
    icon: icons/changelog.svg
---
```

## Links

A link is relative to the file that holds it, as on GitHub. A link to the file of a page leads to the page, a link to a directory to the page of its `README.md` and a link to another file of the repository to the file in the repository on `main`, at the URL that [`host`](#hosting) gives. A link to a fragment must name a heading of its page. goldmark derives the ID of a heading with `_` or a non-ASCII letter differently from GitHub, which would leave a link that works on GitHub dead on the site.

## Hosting

The pages link the repository in the header and the footer, link the file of each page as "View this page on GitHub" and link the files of the repository that are no page, all on GitHub by default. The `host` setting of the front matter of the `README.md` of the docs directory names another host and gives the URLs of its files:

```yaml
---
repository: https://git.example.com/o/demo
host:
  name: Forgejo                           # the name of the links
  icon: icons/forgejo.svg                 # optional, an SVG file
  file: "{repository}/src/{ref}/{path}"   # the URL of a file
  dir: "{repository}/src/{ref}/{path}/"   # optional, the URL of a directory
---
```

In `file` and `dir`, `{repository}` stands for the URL of the repository, `{ref}` for `main` or the tag of a release and `{path}` for the path of the file from the root of the repository. `dir` defaults to `file`, and the links show no icon without `icon`. Without `host` the URLs are those of GitHub, `{repository}/blob/{ref}/{path}` and `{repository}/tree/{ref}/{path}/`, and the links show the mark of GitHub.

## Versions

The docs directory documents main. Each directory of `versions/` documents a release and holds a docs directory of the same layout, named after the feature version, as `v1.2` for `v1.2.0`, `v1.2.1` and so on. Its pages stay editable on main, which keeps the docs of a release correctable after its tag.

`-snapshot v1.2.0` copies the docs directory into `versions/v1.2/` when a release is cut, instead of writing the site:

- The copy holds the `README.md`, the groups and the other files of the docs directory that their links name. `logo.svg`, the favicon, `style.css` and the settings of the site stay in the docs directory, since every version shows those.
- A page with a `source` holds the text of the source in place of its own.
- A link to a file outside the docs directory becomes the URL of the file at the tag in the repository, or leads to the page that shows the file, as the changelog.

A patch release takes no snapshot, and an existing directory of the release fails one. A link of a release that leads out of its directory fails the build, as does a `source` outside it and a setting of the site in its `README.md`.

1 run writes every version:

| Path | Pages |
| ---- | ----- |
| `<out>/main/` | main. `noindex` keeps search engines on the releases. |
| `<out>/v1.2/` | A release. |
| `<out>/index.html` | Opens the latest release, or main without releases. |
| `<out>/llms.txt` | Links the `llms.txt` of each version. |

The `note` of the front matter of the `README.md` of the docs directory, or of a release, shows above every page of that version, which shows no note without one. It is 1 paragraph of Markdown whose links are links of that `README.md`, and which can also link the pages of another version, as `../v1.2/README.md` from a release. A snapshot leaves the note of main behind.

```yaml
---
note: v1.1 is obsolete, use [v1.2](../v1.2/README.md).
---
```

The `<body>` of each page names its version in `data-version`, which lets `style.css` style a version, as `[data-version="v1.1"] .version-note { --note: red; }`.

The version menu of each page lists the releases, the latest first, followed by main, and opens the same page in the version picked, or the first page of a version that lacks it. A link of the header or the footer leads to the page at the same path in the version, or to that of main if the version lacks it. A run removes what an earlier run wrote first, as the directory of a release that `versions/` no longer holds.

Each version holds 1 HTML page and 1 Markdown copy for each page, `search.json`, `llms.txt` and `llms-full.txt`, which list and concatenate the Markdown copies for language models, and the stylesheet, the scripts and the Morpheus bundle.

## Pages

1. goldmark parses the file as GitHub Flavored Markdown.
2. Chroma highlights code blocks. `title="..."` after the language adds a title bar. `mermaid` blocks become diagrams.
3. GitHub alerts such as `> [!NOTE]` become callouts.
4. A `<details>` that starts with a `<summary>` becomes a section that opens and closes, its text in a `<neo-elastic>` of Morpheus, which animates its height. The text between them is Markdown, set apart by blank lines, as on GitHub. A `<details>` without its `<summary>` or its end fails the build. The `<summary>` keeps its text with `<code>`, `<b>`, `<strong>`, `<i>` and `<em>`, and the `<details>` keeps `open`.
5. An image, or a media element of the raw HTML (`<img>`, `<picture>`, `<source>`, `<video>`, `<audio>` and `<track>`), leads to a copy of its file in `media/` of the version, in a directory named after its content. A missing file fails the build. The rest of the raw HTML, and the attributes of media elements other than those of files, sizes and playback, fail the build, as do the other [checks](docs/3-reference/5-checks.md) of a page. A `<source>` whose `media` names `prefers-color-scheme` follows the theme switch, and the system theme while the switch is on System.
6. The contents of a page list its headings down to level 4, or above the first level at which 2 headings below level 2 share their text, as the Flags of each command of a CLI reference.

The search splits each page into sections at the headings that its contents list and writes 1 entry per section into `search.json`. `docpages.js` downloads the file on first use and searches it in the browser. A result contains every word of the query, and matches in headings rank above matches in the text. `docpages.js` also runs the navigation overlay, the highlight of the current section, the version menu, the closing of a `<details>`, whose text shrinks away before it closes, the search dialog, which fits above a screen keyboard, and the Mermaid diagrams, which it loads only on pages with a diagram.

## Serving

`-serve 127.0.0.1:7340` serves the site instead of writing it. It reads the docs directory on every request, and an open page reloads when a file of the docs directory or a source of a page changes. Docs that don't build at the start stop the command with the error. An error after that, such as a dead link, shows in the tab, which reloads after the fix. A directory that holds a `docs` directory with a `README.md`, as each example does, gets the hint to name that one.

A link to a file of the repository that no page shows leads to the file in the working tree, served in `_repository/`, instead of the host, which has only what was pushed. The server serves only the files and directories that the links name and what the directories hold, except names that start with `.`, as `.git`. Text shows as plain text and a directory as a list of its files. A written site links the host.

## Examples

[example/cli](example/cli) documents a command-line tool and [example/library](example/library) a Go library, each in a style of its own. The README of each example explains it and how to build it.

## Library

[Read] reads a docs directory with its releases, [Site.Write] writes the site, [Handler] serves it and [Snapshot] copies the docs directory for a release. A project that adds pages around the documentation, as a landing page, renders those itself and writes the docs into a directory below it.

```go
site, err := docpages.Read(docpages.Options{Root: ".", Dir: "guide"})
err = site.Write("docs/docs")
```

[Read]: https://pkg.go.dev/github.com/romshark/docpages#Read
[Site.Write]: https://pkg.go.dev/github.com/romshark/docpages#Site.Write
[Handler]: https://pkg.go.dev/github.com/romshark/docpages#Handler
[Snapshot]: https://pkg.go.dev/github.com/romshark/docpages#Snapshot

## Development

`go generate ./...` writes the generated files, which are committed: the Go code of the templ templates, `static/style.min.css` and the Morpheus bundle in `static/morpheus/` at the version that `go.mod` requires. `go test ./...` fails when the bundle is of another version.
