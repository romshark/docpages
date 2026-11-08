# CLI

```
docpages [flags] [dir]
```

`dir` is the docs folder, the current folder by default. It must be inside a Git repository.

| Flag | What it does |
| --- | --- |
| `-o dir` | Writes the site into `dir`. Default: `site`. |
| `-serve addr` | Serves the site on `addr`, as `127.0.0.1:7340`, instead of writing it. Open pages reload on changes. Docs that don't build stop it at the start; an error after that, as a dead link, shows in the page until you fix it. Links to other files of the repository open them in your working tree, not on the repository host. |
| `-snapshot tag` | Copies the docs into `versions/` for the release `tag`, as `v1.2.0`, instead of writing the site. |
| `-version` | Prints the version of docpages, its commit and its build date. |

## Examples

Preview the docs:

```sh
docpages -serve 127.0.0.1:7340 docs
```

Build the site into `site/`:

```sh
docpages -o site docs
```

Copy the docs for the release v1.2.0:

```sh
docpages -snapshot v1.2.0 docs
```
