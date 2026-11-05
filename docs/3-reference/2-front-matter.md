# Front matter

A key that docpages doesn't know fails the build.

## Any page

| Key | Value | What it does |
| --- | --- | --- |
| `title` | Text | Replaces the level 1 heading in the navigation and the browser tab. |
| `suggested` | `true` | Offers the page in the search before you type. |
| `source` | Path | Shows this Markdown file in place of the page. Links to the file lead to the page. |
| `url` | URL | Makes the entry a link off the site. Needs `title`, and can't go with `source`. |

## The README.md of a version

In `docs/README.md`, and in the `README.md` of each release in `versions/`:

| Key | Value | What it does |
| --- | --- | --- |
| `note` | Markdown | The [note](../2-guide/6-versions.md#notes) above every page of the version. None without it. |

## Site settings

Only in `docs/README.md`:

| Key | Value | What it does |
| --- | --- | --- |
| `name` | Text | The name of the project. Default: the last part of the repository URL. |
| `home` | URL | Where the logo leads, relative to the output folder. Default: the first page. |
| `repository` | URL | Your repository. Default: the URL of the Git remote `origin`. |
| `host` | Map | Where the repository is hosted: `name`, `icon`, `file` and `dir`. See [Repository host](../2-guide/7-repository-host.md). |
| `header` | List | The links of the header, each with `title`, `url` and an optional `icon`. Default: "Docs", to the first page. |
| `footer` | List | More links in the footer, each with `title`, `url` and an optional `icon`. |
| `license` | Text | The last line of the footer. `""` leaves it out. Default: the first line and the copyright holder of the `LICENSE` file. |
