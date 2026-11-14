---
suggested: true
---

# Quick start

You need Go 1.27.4 or later, and a Git repository for your docs.

## Install

```sh
go install github.com/romshark/docpages/cmd/docpages@latest
```

Or skip the install and run `go run github.com/romshark/docpages/cmd/docpages@latest` in its place.

## Create the docs

Make a `docs` folder with a first page and one group:

```
docs/
  README.md
  1-get-started/
    1-install.md
```

Every page starts with a level 1 heading, which becomes its title:

```markdown title="docs/README.md"
# Introduction

My project does useful things.
```

## Preview

```sh
docpages -serve 127.0.0.1:7340 docs
```

Open <http://127.0.0.1:7340/>. A page reloads when you save a file, and an error, such as a broken link, shows up in the browser.

## Build

```sh
docpages -o site docs
```

`site/` now holds the whole site, ready to [deploy](../2-guide/9-deploy.md).

> [!NOTE]
> docpages takes the URL of your repository from the Git remote `origin`. Without one, set [`repository`](../2-guide/4-site-settings.md#name-and-repository) in the front matter of `docs/README.md`.
