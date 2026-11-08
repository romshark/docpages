package docpages

// Generated files are committed so go install and go get build without running these.
//
// templ generates template Go. At go.mod's required versions, esbuild writes
// the page stylesheet—minified like docs style.css—and module script,
// while genmorpheus writes the bundle for rendered Morpheus components.
// A docs directory's template.templ is built by the site, not the module.

//go:generate go run github.com/a-h/templ/cmd/templ@v0.3.1100 generate -ignore-pattern=/template\.templ$
//go:generate go run github.com/evanw/esbuild/cmd/esbuild@v0.28.2 static/style.css --minify --log-level=warning --outfile=static/style.min.css
//go:generate go run github.com/evanw/esbuild/cmd/esbuild@v0.28.2 static/docs.js --minify --format=esm --log-level=warning --outfile=static/docs.min.js
//go:generate go run ./internal/genmorpheus
