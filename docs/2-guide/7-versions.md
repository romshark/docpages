# Versions

The docs folder documents `main`, and each release shows the docs folder as it was at the release's Git tag. To turn releases on, set `releases` in the front matter of `docs/README.md`, with `from` as the oldest version that the site shows:

```yaml title="docs/README.md"
---
releases:
  from: v1.2
---
```

With this setting, every tag `v1.2.0` and later is a release. The site shows each [minor version](https://www.conventionalcommits.org/) once, as `v1.2`, with the docs at its latest patch tag. If the tags `v1.2.0` and `v1.2.3` exist, v1.2 shows the docs at `v1.2.3`. Pre-release tags such as `v1.3.0-rc.1` are skipped. Without `releases`, the site shows only `main`, and its pages have no version menu.

## Cut a release

Tag the release and push the tag. The next build adds it to the site.

The build fails if the repository has no tag of the version in `from`, as `v1.2.x` for `from: v1.2`. That happens in a clone without tags. `actions/checkout` fetches no tags by default, so set `fetch-depth: 0` as [Deploy](9-deploy.md#build-in-ci) shows.

## Shared theme

Every version uses the logo, the favicon, `style.css`, the templates and the site settings of `main`. The `README.md` at the tag is still the first page of the release, but docpages ignores the site settings and the note in its front matter, which were those of `main` when you tagged.

Links to files outside the docs folder, and "View this page on GitHub", lead to the files at the tag, such as `https://github.com/me/project/blob/v1.2.3/main.go`.

## Fix a release

To fix the docs of a release after tagging it, put the corrected files on `main` in a folder of `versions/` named after the minor version, as `versions/v0.2/`. Each file goes to the same path as in the docs folder:

```
docs/
  2-guide/
    3-media.md          the page of main
  versions/
    v0.2/
      2-guide/
        3-media.md      replaces 2-guide/3-media.md of v0.2
```

To fix that page of v0.2 while `main` is at v0.5, copy `docs/2-guide/3-media.md` from the latest v0.2 tag, as `v0.2.3`, to `docs/versions/v0.2/2-guide/3-media.md` and edit the copy.

- A file in the folder replaces the file at the same path at the tag. A file that the tag lacks, such as a new page or image, is added. All other files come from the tag.
- Relative links in the file resolve as if the file were at its path in the docs folder, as `docs/2-guide/3-media.md`.
- "View this page on GitHub", and links to files of the folder that aren't pages, lead to the file in the folder on `main`.
- A file in the folder also replaces the file at a later patch tag, as `v0.2.4`. Delete the file from the folder once a tag holds the fix.

A newer docpages can add a check that a page at an old tag fails. Fix that page in the folder of its release the same way.

## Build

Each build writes every version:

| Path | Holds |
| --- | --- |
| `site/main/` | main |
| `site/v1.2/` | a release |
| `site/index.html` | a redirect to the latest release, or to main without releases |

The version menu lists the releases, newest first, then `main`. Picking a version opens the same page in that version, or the first page of that version if it lacks the page. Header and footer links stay in the version you're reading.

While the site has releases, search engines skip the pages of `main`, so search results lead to released docs.

The build fails on a folder in `versions/` that matches no release, on a file in a release folder that no page of the release shows or links, and on site settings in the `README.md` of a release folder.

## Notes

A note shows above every page of its version. For `main`, set `note` in the front matter of `docs/README.md`. For a release, copy the `README.md` at its tag to its folder, as `docs/versions/v1.1/README.md`, and set `note` there:

```yaml title="docs/versions/v1.1/README.md"
---
note: v1.1 is obsolete, use [v1.2](../v1.2/README.md).
---

# Introduction
```

A note is 1 paragraph of Markdown, and its links and images resolve from the `README.md` that holds it. A link to a path in a release folder leads to the page of that release, even if the folder doesn't hold the file. From a release folder, `../v1.2/README.md` leads to v1.2 and `../../README.md` to main. From `docs/README.md`, `versions/v1.2/README.md` leads to v1.2. An image in a release note must be a file of that release.

To color a note, see [Styling](5-styling.md#one-version).

## Tag prefixes

If a repository holds more than 1 project with tags of its own, such as `api/v1.2.0` for a Go module in `api/`, set the tag prefix in `prefix`:

```yaml title="api/docs/README.md"
---
releases:
  from: v1.2
  prefix: api/
---
```

Only tags with that prefix are releases. The version menu still shows `v1.2`, and the release folder is `versions/v1.2/`.
