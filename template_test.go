package docpages

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// TestMain builds renderers in a temporary directory and removes it.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "docpages-renderers-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rendererRoot = func() string { return dir }
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// templatePrefix is lines 1-4 of every test .templ file.
var templatePrefix = dedent(`
	package docs

	import "github.com/romshark/docpages"

	`)

// templateGoMod is the go.mod of a test docs module.
// Tests run docpages from source, which replaces the version.
var templateGoMod = dedent(`
	module example.com/docs

	go 1.27.3

	require github.com/romshark/docpages v0.0.0-00010101000000-000000000000
	`)

// TestReadTemplates checks package docs, one templ DocpagesMain, a docs module
// that requires docpages, templ-generated neighbor files and module files
// counting as used, and no templates for root-level docs.
func TestReadTemplates(t *testing.T) {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	main := templatePrefix + dedent(`
		templ DocpagesMain(p docpages.Page) {
			<p>x</p>
		}
	`)
	icons := dedent(`
		package docs

		templ icon() {
			<svg></svg>
		}
	`)
	for name, tt := range map[string]struct {
		files map[string]string
		want  string // The error, empty for none.
	}{
		"main in 1 of 2 files": {
			files: map[string]string{
				"docs/main.templ":     main,
				"docs/main_templ.go":  "package docs\n",
				"docs/main_templ.txt": "x",
				"docs/icons.templ":    icons,
				"docs/go.mod":         templateGoMod,
			},
		},
		"module files": {
			files: map[string]string{
				"docs/main.templ":      main,
				"docs/icons.templ":     icons,
				"docs/go.mod":          templateGoMod,
				"docs/go.sum":          "example.com/x v1.0.0 h1:x=\n",
				"docs/helper.go":       "package docs\n",
				"docs/lib/lib.go":      "package lib\n",
				"docs/lib/lib_test.go": "package lib\n",
			},
		},
		"no go.mod": {
			files: map[string]string{"docs/main.templ": main},
			want: "docs: the .templ files need a Go module that requires " +
				"github.com/romshark/docpages\n" +
				"run: cd docs && go mod init docs && " +
				"go get -tool github.com/romshark/docpages/cmd/docpages@latest",
		},
		"no requirement": {
			files: map[string]string{
				"docs/main.templ": main,
				"docs/go.mod":     "module example.com/docs\n",
			},
			want: "docs/go.mod: no requirement of github.com/romshark/docpages\n" +
				"run: cd docs && " +
				"go get -tool github.com/romshark/docpages/cmd/docpages@latest",
		},
		"bad go.mod": {
			files: map[string]string{
				"docs/main.templ": main,
				"docs/go.mod":     "module example.com/docs\nrequire (\n",
			},
			want: "docs/go.mod:3: syntax error " +
				"(unterminated block started at docs/go.mod:2:1)",
		},
		"no main": {
			files: map[string]string{"docs/icons.templ": icons},
			want:  "docs: no templ DocpagesMain(p docpages.Page) in the .templ files",
		},
		"package": {
			files: map[string]string{
				"docs/main.templ": strings.Replace(
					main, "package docs", "package site", 1,
				),
			},
			want: "docs/main.templ:1:1: package site, not package docs",
		},
		"syntax": {
			files: map[string]string{"docs/main.templ": templatePrefix + dedent(`
				templ DocpagesMain(p docpages.Page) {
					<div>
						<p>x
					</div>
				}
			`)},
			want: "docs/main.templ:8:2: <p>: close tag not found",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fsys := testRepoFS()
			for name, data := range tt.files {
				fsys[name] = file(data)
			}
			s, err := read(fsys, "docs", noRemote, false)
			if tt.want != "" {
				require.EqualError(t, err, tt.want)
				return
			}
			require.NoError(t, err)
			require.Len(t, s.templates, 2)
		})
	}

	fsys := fstest.MapFS{
		"README.md": file(dedent(`
			---
			repository: https://github.com/o/r
			---

			# A
		`)),
		"1-a/1-b.md": file("# B\n"),
		"page.templ": file(dedent(`
			package site

			templ page() {
				<p>x</p>
			}
		`)),
		"page_templ.go": file("package site\n"),
	}
	s, err := read(fsys, ".", noRemote, false)
	require.NoError(t, err)
	require.Empty(t, s.templates)
}

// writeTemplateRepo writes [testRepoFS], the docs module of [templateGoMod] and named
// docs files to and returns a new directory. It skips tests unable to build a renderer.
func writeTemplateRepo(t *testing.T, templates map[string]string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("building a renderer takes a go build")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	root := writeTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "go.mod"),
		[]byte(templateGoMod), 0o644))
	for name, src := range templates {
		require.NoError(t, os.WriteFile(filepath.Join(root, "docs", name),
			[]byte(src), 0o644))
	}
	return root
}

// TestTemplate checks that a renderer built from two .templ files and a Go file
// of the docs module renders template parts and links in a static build and [Handler].
// Go that templ generate left for a deleted .templ file doesn't count.
func TestTemplate(t *testing.T) {
	root := writeTemplateRepo(t, map[string]string{
		"helper.go": dedent(`
			package docs

			func label() string { return "from helper.go" }
		`),
		"deleted_templ.go": "package docs\n\nfunc broken( {\n",
		"main.templ": templatePrefix + dedent(`
			templ DocpagesMain(p docpages.Page) {
				@docpages.Layout(p, docpages.Parts{
					Head: head(),
					HeaderLinks: docpages.HeaderLinks(p,
						docpages.Link{Title: "Examples", URL: "4-more/1-examples.md", Icon: icon()},
					),
					FooterEnd: end(p),
				})
			}

			templ head() {
				<meta name="template" content="test"/>
			}

			templ end(p docpages.Page) {
				<a class="dispatch" href={ p.URL("2-guides/01-events.md#dispatch") }>Dispatch</a>
				<span class="helper">{ label() }</span>
			}
		`),
		"icons.templ": dedent(`
			package docs

			templ icon() {
				<svg aria-hidden="true"></svg>
			}
		`),
	})
	o := Options{Root: root, Dir: "docs"}
	s, err := Read(o)
	require.NoError(t, err)
	files, err := s.Files()
	require.NoError(t, err)
	html := string(files["main/guides/events.html"])
	for _, want := range []string{
		`<meta name="template" content="test"></head>`,
		`<a class="header-link icon-link" href="../more/examples">` +
			`<svg aria-hidden="true"></svg> Examples</a>`,
		`<a class="dispatch" href="../guides/events#dispatch">Dispatch</a> ` +
			`<span class="helper">from helper.go</span></footer>`,
	} {
		require.Contains(t, html, want)
	}
	require.NotContains(t, html, "_reload")

	code, body := get(t, Handler(o), "/main/guides/events")
	require.Equal(t, 200, code)
	require.Contains(t, body, `<meta name="template" content="test"></head>`)
	require.Contains(t, body, `<script data-url="../../_reload"`)
}

// TestTemplateErrors checks that renderer build and run errors name the .templ
// file and any templ- or Go-reported line and column.
func TestTemplateErrors(t *testing.T) {
	layout := templatePrefix + dedent(`
		templ DocpagesMain(p docpages.Page) {
			@docpages.Layout(p, docpages.Parts{FooterEnd: end(p)})
		}
	`)
	for name, tt := range map[string]struct {
		templates map[string]string
		want      string
	}{
		"Go": {
			templates: map[string]string{
				"main.templ": layout,
				"end.templ": templatePrefix + dedent(`
					templ end(p docpages.Page) {
						<p>{ undefinedThing }</p>
					}
				`),
			},
			want: "docs/end.templ:6:7: undefined: undefinedThing",
		},
		"main signature": {
			templates: map[string]string{"main.templ": templatePrefix + dedent(`
				templ DocpagesMain(title string) {
					<p>{ title }</p>
				}

				var _ docpages.Page
			`)},
			want: "docs: templ DocpagesMain(p docpages.Page): cannot use docs.DocpagesMain",
		},
		"Go file of the module": {
			templates: map[string]string{
				"main.templ": layout,
				"end.templ": templatePrefix + dedent(`
					templ end(p docpages.Page) {
						<p>{ label() }</p>
					}
				`),
				"helper.go": dedent(`
					package docs

					func label() string { return undefinedThing }
				`),
			},
			want: "docs/helper.go:3:30: undefined: undefinedThing",
		},
		"link": {
			templates: map[string]string{
				"main.templ": layout,
				"end.templ": templatePrefix + dedent(`
					templ end(p docpages.Page) {
						<a href={ p.URL("4-more/1-examples.md#missing") }>Examples</a>
					}
				`),
			},
			want: `docs/end.templ:6:49: ` +
				`link "4-more/1-examples.md#missing" names no heading`,
		},
		"header link": {
			templates: map[string]string{"main.templ": templatePrefix + dedent(`
				templ DocpagesMain(p docpages.Page) {
					@docpages.Layout(p, docpages.Parts{
						HeaderLinks: docpages.HeaderLinks(p,
							docpages.Link{Title: "Examples", URL: "4-more/1-examples.md#missing"},
						),
					})
				}
			`)},
			want: `docs/*.templ: header link "Examples": ` +
				`link "4-more/1-examples.md#missing" names no heading`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := writeTemplateRepo(t, tt.templates)
			s, err := Read(Options{Root: root, Dir: "docs"})
			require.NoError(t, err)
			_, err = s.Files()
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// TestTemplateNoGo checks that only docs with .templ files require go.
func TestTemplateNoGo(t *testing.T) {
	root := writeTestRepo(t)
	t.Setenv("PATH", t.TempDir())
	_, err := Read(Options{Root: root, Dir: "docs"})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "main.templ"),
		[]byte(templatePrefix+dedent(`
			templ DocpagesMain(p docpages.Page) {
			}
		`)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "go.mod"),
		[]byte(templateGoMod), 0o644))
	_, err = Read(Options{Root: root, Dir: "docs"})
	require.ErrorContains(t, err, "docs: the .templ files need the go command: ")
}

// TestCompatible tests which docpages releases build the templates of
// a module that requires another release.
func TestCompatible(t *testing.T) {
	for _, tt := range []struct {
		running, required string
		want              bool
	}{
		{"v1.2.0", "v1.2.0", true},
		{"v1.3.1", "v1.2.0", true},
		{"v1.2.0", "v1.3.0", false},
		{"v2.0.0", "v1.9.0", false},
		{"v1.9.0", "v2.0.0", false},
		{"v0.3.2", "v0.3.0", true},
		{"v0.3.0", "v0.3.2", false},
		{"v0.4.0", "v0.3.0", false},
		{"v1.0.0", "v0.9.0", false},
		{"v0.3.0", "v0.3.0-20260101000000-abcdefabcdef", true},
	} {
		t.Run(tt.running+" "+tt.required, func(t *testing.T) {
			require.Equal(t, tt.want, compatible(tt.running, tt.required))
		})
	}
}

// TestCheckVersion checks that [Read] rejects a docs module that requires a
// docpages that the running release isn't compatible with, and that
// development builds, pseudo-versions and a module that replaces docpages with
// a directory skip the check.
func TestCheckVersion(t *testing.T) {
	root := writeTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "main.templ"),
		[]byte(templatePrefix+dedent(`
			templ DocpagesMain(p docpages.Page) {
			}
		`)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "go.mod"),
		[]byte(dedent(`
			module example.com/docs

			require github.com/romshark/docpages v0.4.0
		`)), 0o644))
	for _, tt := range []struct {
		running string
		want    error
	}{
		{running: "v0.3.2", want: &VersionError{
			GoMod: "docs/go.mod", Required: "v0.4.0", Running: "v0.3.2",
		}},
		{running: "v0.5.0", want: &VersionError{
			GoMod: "docs/go.mod", Required: "v0.4.0", Running: "v0.5.0",
		}},
		{running: "v0.4.1"},
		{running: ""},
		{running: "v0.3.3-0.20260101000000-abcdefabcdef"},
	} {
		t.Run(tt.running, func(t *testing.T) {
			old := runningVersion
			runningVersion = func() string { return tt.running }
			t.Cleanup(func() { runningVersion = old })
			_, err := Read(Options{Root: root, Dir: "docs"})
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.Equal(t, tt.want, err)
		})
	}
	require.EqualError(t, &VersionError{
		GoMod: "docs/go.mod", Required: "v0.4.0", Running: "v0.3.2",
	}, "docs/go.mod requires github.com/romshark/docpages v0.4.0, "+
		"incompatible with the running v0.3.2")

	// A docs module that replaces docpages with a directory, as the docs of
	// docpages do, names no release.
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "go.mod"),
		[]byte(dedent(`
			module example.com/docs

			require github.com/romshark/docpages v0.0.0-00010101000000-000000000000

			replace github.com/romshark/docpages => ../
		`)), 0o644))
	old := runningVersion
	runningVersion = func() string { return "v0.3.2" }
	t.Cleanup(func() { runningVersion = old })
	_, err := Read(Options{Root: root, Dir: "docs"})
	require.NoError(t, err)
}

// TestRendererModFile tests that the go.mod of a renderer drops the docs
// module's replace of docpages and uses the running docpages:
// its source, or else its version.
func TestRendererModFile(t *testing.T) {
	mod := &docsModule{file: "docs/go.mod", goMod: []byte(dedent(`
		module example.com/docs

		go 1.27.3

		require github.com/romshark/docpages v0.3.0

		replace github.com/romshark/docpages => ../docpages
	`))}
	for name, tt := range map[string]struct {
		src, version, want string
	}{
		"source": {
			src: "/src/docpages",
			want: dedent(`
				module example.com/docs

				go 1.27.3

				require github.com/romshark/docpages v0.3.0

				replace github.com/romshark/docpages => /src/docpages
			`),
		},
		"version": {
			version: "v0.3.2",
			want: dedent(`
				module example.com/docs

				go 1.27.3

				require github.com/romshark/docpages v0.3.2
			`),
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := rendererModFile(mod, tt.src, tt.version)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got))
		})
	}
	_, err := rendererModFile(mod, "", "")
	require.ErrorContains(t, err, "needs the source or the version of docpages")
}
