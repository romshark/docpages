# Layout

The layout of your docs folder is the navigation.

```
docs/
  README.md               the first page
  logo.svg                optional, the logo in the header
  favicon.svg             optional, or favicon.png or favicon.ico
  style.css               optional, your styles
  template.templ          optional, your template, as any .templ file
  1-get-started/          a group, "Get started"
    icon.svg              optional, the icon of the group
    1-installation.md     a page, at get-started/installation
    2-upgrading.md        a page, at get-started/upgrading
  2-tutorial/
    README.md             opens the group, at tutorial/
    1-create-project.md   a page, at tutorial/create-project
  icons/                  other files, such as icons
  versions/               the docs of your releases
```

## Groups and pages

- A folder whose name starts with a number and `-` is a group. The number sets its place, and the rest of the name is its title: `1-get-started` becomes "Get started".
- A Markdown file in a group whose name starts with a number and `-` is a page. Leading zeros, as in `01-intro.md`, keep file listings in the same order.
- The `README.md` of a group opens the group.
- The `README.md` of the docs folder is the first page of the site and the first entry of the first group.

The numbers stay out of URLs and titles. The title of a page is the text of its level 1 heading, unless its [front matter](2-writing.md#front-matter) sets `title`.

## What fails the build

- A Markdown file in a group without a number, other than `README.md`
- 2 entries with the same number
- A group inside a group
- A group without pages, or a docs folder without groups

## Other files

The navigation leaves out folders without a number, such as `icons/`. Keep the files there that your pages and settings link to. `versions/` holds your [releases](7-versions.md).

## Icons

`logo.svg`, the `icon.svg` of a group and every other icon go into the page as inline SVG. Draw them with `currentColor` and they take the color of the text in both themes, as the icon of this group beside "Guide" in the navigation:

```svg title="docs/2-guide/icon.svg"
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"
  stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <path d="M3 5.5C5.5 4 8.5 4 12 6c3.5-2 6.5-2 9-.5v13c-2.5-1.5-5.5-1.5-9 .5-3.5-2-6.5-2-9-.5zM12 6v13"/>
</svg>
```
