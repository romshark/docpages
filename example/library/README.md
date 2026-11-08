# Example: a Go library

[docs/](docs) documents ringbuf, a made-up Go ring buffer, in a monospace style. It shows the name of the project in place of a logo, a group that a `README.md` opens, a Mermaid diagram, a link in the footer with an icon, links to the headings of another page, a `<details>` that opens and 2 releases in [docs/versions/](docs/versions): v1.0 and v0.9, whose note sends readers to v1.0. Its [style.css](docs/style.css) overrides the color and font tokens of docpages.

Serve the docs from the root of the repository, reloading on changes:

```sh
go run ./cmd/docpages -serve 127.0.0.1:7340 example/library/docs
```

Or write the site into `site/`:

```sh
go run ./cmd/docpages -o site example/library/docs
```
