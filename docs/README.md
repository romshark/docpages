---
name: docpages
repository: https://github.com/romshark/docpages
suggested: true
header:
  - title: Docs
    url: README.md
  - title: Examples
    url: 1-get-started/2-examples.md
footer:
  - title: Go reference
    url: https://pkg.go.dev/github.com/romshark/docpages
    icon: icons/package.svg
---

# Introduction

docpages turns a folder of Markdown files in your Git repository into a
documentation site.

There's no config file to list your pages: the folders and file names are the
navigation. Links that work on GitHub work on the site, and a link to a heading
that doesn't exist fails the build.

```sh title="Terminal"
go run github.com/romshark/docpages/cmd/docpages@latest -serve 127.0.0.1:7340 docs
```

## What you get

- Navigation, page contents and previous and next links, from your folders
- Search that runs in the browser, with no server
- A version menu for `main` and each release
- Light and dark themes, with your own styles on top
- `llms.txt` and a Markdown copy of every page for language models

## Next steps

Start with the [quick start](1-get-started/1-quick-start.md), or see how the
[layout](2-guide/1-layout.md) of your folder becomes the navigation.
