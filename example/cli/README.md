# Example: a command-line tool

[docs/](docs) documents wc, the Unix tool that counts lines, words and bytes, in the style of a man page in a terminal window. It shows a logo, a favicon, icons for the groups, callouts and a code block with a title. Its [template.templ](docs/template.templ) builds a page of its own from the parts of docpages, with a banner across the top, the navigation right of the text and no contents column. The text sits in a window between the title line and the footer line of a man page. Its [style.css](docs/style.css) overrides the color and font tokens of docpages, frames the window, shows the contents bar at every width and marks the current page in reverse video. It also adds the indentation, the capitals and the square corners of a man page.

Serve the docs from the root of the repository, reloading on changes:

```sh
go run ./cmd/docpages -serve 127.0.0.1:7340 example/cli/docs
```

Or write the site into `site/`:

```sh
go run ./cmd/docpages -o site example/cli/docs
```
