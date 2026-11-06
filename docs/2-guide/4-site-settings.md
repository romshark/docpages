# Site settings

The front matter of `docs/README.md` sets up the whole site:

```yaml title="docs/README.md"
---
name: My Project
home: ../
repository: https://github.com/me/project
header:
  - title: Docs
    url: README.md
  - title: FAQ
    url: 4-more/1-faq.md
    icon: icons/faq.svg
footer:
  - title: Changelog
    url: 3-reference/2-changelog.md
---
```

## Name and repository

`name` shows in the browser tab and in `llms.txt`. It defaults to the last part
of the repository URL.

`repository` is the URL of your repository. It defaults to the URL of the Git
remote `origin`. If your repository isn't on GitHub, set
[`host`](7-repository-host.md) too.

## Logo and favicon

Put `logo.svg` in the docs folder to show it in place of the name. The logo
leads to the first page, or to `home`: a URL relative to the output folder, as
`../` for a landing page above the docs.

`favicon.svg`, `favicon.png` or `favicon.ico` becomes the icon of the browser
tab.

## Header

`header` lists the links of the header. Each has a `title`, a `url` and an
optional `icon`, an SVG file.

- Without `header`, the header has 1 link, "Docs", to the first page.
- `header: []` leaves the header without links.
- A link is highlighted on the page it leads to. A link to the first page is
  highlighted on every page that no other link leads to.

## Footer

`footer` lists more links, written like those of the header. They sit between
the link to your repository and `llms.txt`.

The footer ends with `license`, as written. Without it, the footer has no such
line:

```yaml title="docs/README.md"
---
license: MIT License © 2026 Jane Doe
---
```

## Link URLs

A `url` of the header or the footer works like a link of `README.md`. A Markdown
file leads to its page, any other file of the repository to the file in the
repository, and a full URL off the site.
