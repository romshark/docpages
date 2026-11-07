# Versions

The docs folder documents `main`. Each release has its own copy in `versions/`:

```
docs/
  README.md          main
  1-get-started/
  versions/
    v1.2/            the docs of v1.2.x
      README.md
      1-get-started/
    v1.1/
```

A release folder is named after the minor version, so `v1.2.0` and `v1.2.1`
share `v1.2/`. Its pages are ordinary files: fix them on `main` whenever you
need to.

## Cut a release

When you tag a release, copy the docs for it:

```sh
docpages -snapshot v1.2.0 docs
```

This writes `docs/versions/v1.2/`:

- It holds the pages, the groups and the files they link to. The logo, the
  favicon, `style.css` and the site settings stay in `docs/`, for every version.
- The text of each `source` file goes into its page.
- A link to a file outside the docs folder now leads to the file at the
  release tag, or to the page that shows it, as the changelog. In a docs folder
  at the root of the repository, the docs are `README.md` and the groups, and
  every other file is outside.
- A link from the root of the repository, as `/docs/1-get-started/1-install.md`,
  becomes relative, which leads to the copy of the file.

A patch release needs no new copy. `-snapshot` refuses to overwrite a folder;
delete it to copy the docs again.

## Build

1 build writes every version:

| Path | Holds |
| --- | --- |
| `site/main/` | main |
| `site/v1.2/` | a release |
| `site/index.html` | a redirect to the latest release, or to main without releases |

The version menu lists the releases, the latest first, then `main`. It opens the
same page in the version you pick, or the first page of a version that lacks it.
The links of the header and the footer stay in the version you read.

Search engines skip `main` while you have releases, so their results lead to
released docs.

The build fails if a release links out of its folder or sets site settings.

## Notes

A version shows a note above every page if you give it one, with `note` in the
front matter of its `README.md`. Without one, its pages show none:

```yaml title="docs/versions/v1.1/README.md"
---
note: v1.1 is obsolete, use [v1.2](../v1.2/README.md).
---
```

A note is 1 paragraph of Markdown, and its links and images work like those of
that `README.md`. A note can also link the pages of another version, as
`../v1.2/README.md` from a release, `versions/v1.2/README.md` from main or
`../../README.md` from a release to main. A snapshot leaves the note of `main`
behind.

To color a note, see [Styling](5-styling.md#one-version).
