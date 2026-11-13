# Go library

The package `github.com/romshark/docpages` builds the docs from Go, as for a site with pages of your own around them:

```go
site, err := docpages.Read(docpages.Options{Root: ".", Dir: "docs"})
if err != nil {
	return err
}
return site.Write("public/docs")
```

`Options.Root` is the root of the repository, and `Options.Dir` the docs folder in it. `Options.NoOffline` leaves out [offline viewing](3-output.md#offline-viewing), as `-offline=false` does. `Read` of docs with [templates](../2-guide/6-templates.md) needs the go command, which builds them. It returns a `*VersionError` if the docs module requires a version of docpages that the running one isn't compatible with.

| Function | What it does |
| --- | --- |
| `Read(o)` | Reads the docs folder, and its releases from the Git tags. |
| `(*Site).Write(dir)` | Writes the site into `dir`. |
| `(*Site).Files()` | Returns the files of the site by their path. |
| `(*Site).Description()` | Returns the first paragraph of the first page. |
| `Handler(o)` | Serves the site over HTTP and reloads open pages on changes. |
| `GitRoot(dir)` | Returns the root of the Git repository that holds `dir`. |

The full API is on [pkg.go.dev](https://pkg.go.dev/github.com/romshark/docpages).
