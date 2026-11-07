package docpages

// The generated files are committed, which lets go install and go get build
// the package without running these.
//
// templ generates the Go code of the templates. esbuild writes the stylesheet
// that the pages load, minified as the style.css of a docs directory, and the script,
// which the pages load as a module, at the version that go.mod requires.
// genmorpheus writes the Morpheus bundle of the components that the templates render,
// at the version that go.mod requires.

//go:generate go run github.com/a-h/templ/cmd/templ@v0.3.1100 generate
//go:generate go run github.com/evanw/esbuild/cmd/esbuild@v0.28.2 static/style.css --minify --log-level=warning --outfile=static/style.min.css
//go:generate go run github.com/evanw/esbuild/cmd/esbuild@v0.28.2 static/docs.js --minify --format=esm --log-level=warning --outfile=static/docs.min.js
//go:generate go run ./internal/genmorpheus
