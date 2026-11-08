# Example: a command-line tool

[docs/](docs) documents wc, the Unix tool that counts lines, words and bytes.

It uses:

- A [template](../../docs/2-guide/6-templates.md#a-page-of-your-own) that builds a page of its own from the parts of docpages
- A [style.css](../../docs/2-guide/5-styling.md)
- A [logo and a favicon](../../docs/2-guide/4-site-settings.md#logo-and-favicon)
- [Icons](../../docs/2-guide/1-layout.md#icons) for the groups
- [Callouts](../../docs/2-guide/2-writing.md#callouts)
- A [code block with a title](../../docs/2-guide/2-writing.md#code)

Serve the docs from the root of the repository, reloading on changes:

```sh
go run ./cmd/docpages -serve 127.0.0.1:7340 example/cli/docs
```

Or write the site into `site/`:

```sh
go run ./cmd/docpages -o site example/cli/docs
```
