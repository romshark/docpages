---
name: docpages
repository: https://github.com/romshark/docpages
suggested: true
---

# Introduction

docpages builds a static documentation site from a folder of Markdown files in your Git repository. Write GitHub Flavored Markdown with syntax-highlighted code, Mermaid diagrams, callouts, collapsible sections, responsive images, video and audio. docpages turns the folder structure into navigation and adds in-browser search, versions for `main` and every release, light and dark themes, custom styles and templates, and `llms.txt` with a Markdown copy of every page.

Preview the site as you write: saving a file reloads the open page, and build errors appear in the browser until you fix them:

```sh title="Terminal"
go run github.com/romshark/docpages/cmd/docpages@latest -serve 127.0.0.1:7340 docs
```

## Next steps

Follow the [quick start](1-get-started/1-quick-start.md) to build your first site, then see how the folder [layout](2-guide/1-layout.md) becomes its navigation.
