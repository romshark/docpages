package docpages

// Generated files are committed so go install and go get build without running these.
//
// templ generates template Go. At go.mod's versions, esbuild minifies the page
// stylesheet, like docs style.css, and compiles the TypeScript of the page and
// the service worker. genmorpheus writes the Morpheus bundle of rendered
// components.
// A docs directory's template.templ is built by the site, not the module.

//go:generate go run github.com/a-h/templ/cmd/templ@v0.3.1100 generate -ignore-pattern=/template\.templ$
//go:generate go run github.com/evanw/esbuild/cmd/esbuild@v0.28.2 web/style.css --minify --log-level=warning --outfile=static/style.min.css
//go:generate go run github.com/evanw/esbuild/cmd/esbuild@v0.28.2 web/docs.ts --minify --format=esm --log-level=warning --outfile=static/docs.min.js
//go:generate go run github.com/evanw/esbuild/cmd/esbuild@v0.28.2 web/sw/sw.ts --minify --format=iife --log-level=warning --outfile=static/sw.min.js
//go:generate go run ./internal/genmorpheus
