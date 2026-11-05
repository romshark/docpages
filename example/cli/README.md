# Example: a command-line tool

[docs/](docs) documents wc, the Unix tool that counts lines, words and bytes,
in the style of a man page in a terminal. It shows a logo, a favicon, icons for
the groups, a link in the header, callouts and a code block with a title. Its
[style.css](docs/style.css) overrides the color and font tokens of docpages and
adds the indentation, the capitals and the square corners of a man page.

Serve the docs from the root of the repository, reloading on changes:

```sh
go run ./cmd/docpages -serve 127.0.0.1:7340 example/cli/docs
```

Or write the site into `site/`:

```sh
go run ./cmd/docpages -o site example/cli/docs
```
