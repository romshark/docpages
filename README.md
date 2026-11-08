# docpages

[![CI](https://github.com/romshark/docpages/actions/workflows/ci.yml/badge.svg)](https://github.com/romshark/docpages/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/romshark/docpages.svg)](https://pkg.go.dev/github.com/romshark/docpages) [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

docpages renders the Markdown files of a docs folder of a Git repository as a static documentation site, with a version for `main` and for each release. The folders and file names make the navigation, links work as they do on GitHub, and a link to a file or a heading that doesn't exist fails the build. The site has search in the browser, light and dark themes, and `llms.txt` for language models.

```sh
go run github.com/romshark/docpages/cmd/docpages@latest -serve 127.0.0.1:7340 docs
```

`-serve` previews the site and reloads it on changes, and `-o site` writes it. docpages needs Go 1.27.3 or later.

## Compared to other tools

| | docpages | [Hugo](https://gohugo.io) | [Docusaurus](https://docusaurus.io) | [VitePress](https://vitepress.dev) | [Starlight](https://starlight.astro.build) | [MkDocs](https://www.mkdocs.org) | [mdBook](https://rust-lang.github.io/mdBook/) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Ecosystem | Go | Go | npm | npm | npm | pip | Cargo |
| Structure | Folder and file names | Folders and `weight` | `sidebars.js` or folders | Config | Config or folders | `mkdocs.yml` or folders | `SUMMARY.md` |
| Relative `.md` links | Yes | With a render hook | Yes | Yes | With a plugin | Yes | Yes |
| Versions\* | Yes | No | Yes | With a plugin | With a plugin | With mike | No |
| Search | Yes | With a theme | Algolia or a plugin | Yes | Yes | Yes | Yes |
| Plugins | Not yet 🚧 | Yes | Yes | Yes | Yes | Yes | Yes |
| Templates | [templ](https://templ.guide) | Go templates | React components | Vue components | Astro components | Jinja | Handlebars |

\* Versions are copies of the docs for each release, with a menu that switches between them.

## Documentation

The documentation is a docpages site itself, in [docs/](docs/README.md):

- [Quick start](docs/1-get-started/1-quick-start.md)
- [Layout](docs/2-guide/1-layout.md)
- [Versions](docs/2-guide/7-versions.md)
- [Front matter](docs/3-reference/2-front-matter.md)
- [Go library](docs/3-reference/4-go-library.md)

[example/cli](example/cli) and [example/library](example/library) are complete docs folders, each in a style of its own.

## Development

`make gen` writes the Go code of the templ templates, `static/style.min.css`, `static/docs.min.js` and the Morpheus bundle. They are committed, and CI fails when they are out of date. `make fmt` formats the code with gofumpt, which CI checks too.
