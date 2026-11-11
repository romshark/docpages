# Example: a Go library

[docs/](docs) documents ringbuf, a made-up Go ring buffer. See https://romshark.github.io/docpages/example/library/

It uses:

- A [template](../../docs/2-guide/6-templates.md) with header and footer links, and a banner that depends on the version
- A [style.css](../../docs/2-guide/5-styling.md)
- [Releases](../../docs/2-guide/7-versions.md) from the tags with the prefix `example/library/`
- A [release folder](../../docs/2-guide/7-versions.md#fix-a-release) that adds a [note](../../docs/2-guide/7-versions.md#notes) to v0.9
- The [project name](../../docs/2-guide/4-site-settings.md#logo-and-favicon) in place of a logo
- A [group](../../docs/2-guide/1-layout.md#groups-and-pages) that its `README.md` opens
- [Links](../../docs/2-guide/2-writing.md#links) to headings of another page
- A [Mermaid diagram](../../docs/2-guide/2-writing.md#diagrams)
- A [`<details>`](../../docs/2-guide/2-writing.md#details)

Serve the docs from the root of the repository, reloading on changes:

```sh
go run ./cmd/docpages -serve 127.0.0.1:7340 example/library/docs
```

Or write the site into `site/`:

```sh
go run ./cmd/docpages -o site example/library/docs
```
