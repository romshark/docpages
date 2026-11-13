# CLI

```
docpages [flags] [dir]
```

`dir` is the docs folder, the current folder by default. It must be inside a Git repository.

| Flag | What it does |
| --- | --- |
| `-o dir` | Writes the site into `dir`. Default: `site`. |
| `-serve addr` | Serves the site on `addr`, as `127.0.0.1:7340`, instead of writing it. Open pages reload on changes. Docs that don't build stop it at the start; an error after that, as a dead link, shows in the page until you fix it. Links to other files of the repository open them in your working tree, not on the repository host. |
| `-offline=false` | Leaves out [offline viewing](3-output.md#offline-viewing): `sw.js`, `offline.json` and the button. Works with `-o` and `-serve`. |
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
