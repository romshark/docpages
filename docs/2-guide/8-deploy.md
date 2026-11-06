# Deploy

`docpages -o site docs` writes a static site. Upload `site/` to any static host.

## Build in CI

Build on every push to `main`:

```sh
go run github.com/romshark/docpages/cmd/docpages@latest -o site docs
```

1 build writes every version, so nothing needs to carry over between builds.
A build first deletes what the last one wrote in `site/`: the version folders,
`index.html` and `llms.txt`. It leaves other files alone.

## URLs without .html

Pages link each other without `.html`, as `get-started/install` for
`get-started/install.html`. GitHub Pages serves these as they are. Other hosts
may need a rule that serves `/a/b` from `/a/b.html`. The rule must also cover a
name with a dot, as `/guide/v1.2-notes` for `/guide/v1.2-notes.html`.

## Below a landing page

To put the docs below a landing page, build them into a subfolder:

```sh
docpages -o public/docs docs
```

Then point the logo back to the landing page with `home`, relative to the
output folder:

```yaml title="docs/README.md"
---
home: ../
---
```
