# Repository host

The pages link your repository: in the header, in the footer, as "View this page
on GitHub" and wherever a page links a file that isn't a page. By default these
are GitHub links.

For another host, set `host` in the front matter of `docs/README.md`:

```yaml title="docs/README.md"
---
repository: https://git.example.com/me/project
host:
  name: Forgejo
  icon: icons/forgejo.svg
  file: "{repository}/src/{ref}/{path}"
  dir: "{repository}/src/{ref}/{path}/"
---
```

| Key | What it does |
| --- | --- |
| `name` | The name in the links, as "View this page on Forgejo". Required. |
| `icon` | An SVG file before the name. Without it the links have no icon. |
| `file` | The URL of a file. Required. |
| `dir` | The URL of a folder. Defaults to `file`. |

In `file` and `dir`:

- `{repository}` is the URL of `repository`.
- `{ref}` is `main`, or the release tag in a snapshot.
- `{path}` is the path from the root of the repository, escaped as a URL path,
  as `Design%20Doc.pdf`.

Without `host`, the links show the GitHub icon and use the URLs of GitHub:
`{repository}/blob/{ref}/{path}` and `{repository}/tree/{ref}/{path}/`.
