# Site settings

The front matter of `docs/README.md` sets up the whole site:

```yaml title="docs/README.md"
---
name: My Project
home: ../
repository: https://github.com/me/project
---
```

The links of the header and the footer belong to a [template](6-templates.md).

## Name and repository

`name` shows in the browser tab and in `llms.txt`. It defaults to the last part of the repository URL.

`repository` is the URL of your repository. It defaults to the URL of the Git remote `origin`. If your repository isn't on GitHub, set [`host`](8-repository-host.md) too.

## Logo and favicon

Put `logo.svg` in the docs folder to show it in place of the name. The logo leads to the first page, or to `home`: a URL relative to the output folder, as `../` for a landing page above the docs.

`favicon.svg`, `favicon.png` or `favicon.ico` becomes the icon of the browser tab.
