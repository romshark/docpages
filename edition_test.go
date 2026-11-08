package docpages

import (
	"fmt"
	"io/fs"
	"runtime/debug"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/require"
)

// TestReleases checks numeric latest-first version directories and rejection of
// other directory names.
func TestReleases(t *testing.T) {
	dir := &fstest.MapFile{Mode: fs.ModeDir}
	got, err := releases(fstest.MapFS{
		"docs/versions/v0.9":      dir,
		"docs/versions/v1.10":     dir,
		"docs/versions/v1.2":      dir,
		"docs/versions/.git":      dir,
		"docs/versions/README.md": {Data: []byte("# Versions\n")},
	}, "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"v1.10", "v1.2", "v0.9"}, got)

	got, err = releases(fstest.MapFS{"docs/README.md": {}}, "docs")
	require.NoError(t, err)
	require.Nil(t, got)

	_, err = releases(fstest.MapFS{"docs/versions/v1.2.3": dir}, "docs")
	require.ErrorContains(t, err, "docs/versions/v1.2.3: the directory of a release "+
		"is named after its feature version, as v1.2")
}

// TestEditionPages checks edition UI: same-page version links latest-first,
// no note when unset, and noindex on main when a release exists.
func TestEditionPages(t *testing.T) {
	const noindex = `<meta name="robots" content="noindex">`
	option := func(version string) string {
		return `value="` + version + `" data-neo-value="` + version +
			`" data-fallback="../../` + version + `/" data-href="../../` + version +
			`/guides/events">` + version + `</neo-option>`
	}
	releases, err := read(withReleases(t,
		testRepoFS(), "v1.1.0", "v1.2.0"), "docs", noRemote, false)
	require.NoError(t, err)
	for name, tt := range map[string]struct {
		s             *Site
		version       string
		want, wantNot []string
	}{
		"main next to releases": {
			s: releases, version: "main",
			want: []string{
				`<neo-select value="main" size="sm" aria-label="Documentation version"`,
				option("v1.2"), option("v1.1"), option("main"),
				noindex,
			},
		},
		"latest release": {
			s: releases, version: "v1.2",
			want:    []string{`<neo-select value="v1.2" size="sm"`, option("main")},
			wantNot: []string{noindex},
		},
		"earlier release": {
			s: releases, version: "v1.1",
			want:    []string{`<neo-select value="v1.1" size="sm"`},
			wantNot: []string{noindex},
		},
		"main alone": {
			s: newTestSite(t), version: "main",
			want:    []string{`<neo-select value="main" size="sm"`, option("main")},
			wantNot: []string{`data-neo-value="v`, noindex},
		},
	} {
		t.Run(name, func(t *testing.T) {
			html := editionPageHTML(t, tt.s, tt.version, "guides/events")
			require.NotContains(t, html, `class="version-note"`)
			for _, want := range tt.want {
				require.Contains(t, html, want)
			}
			for _, notWant := range tt.wantNot {
				require.NotContains(t, html, notWant)
			}
		})
	}
	html := editionPageHTML(t, releases, "main", "guides/events")
	require.Less(t,
		strings.Index(html, option("v1.2")),
		strings.Index(html, option("v1.1")))
	require.Less(t,
		strings.Index(html, option("v1.1")),
		strings.Index(html, option("main")))
}

// TestReleaseLinks checks links to release pages, release-tag URLs for snapshotted
// outside files, source files on main, and main fallbacks for missing header pages.
func TestReleaseLinks(t *testing.T) {
	fsys := withReleases(t, testRepoFS(), "v1.2.3")
	fsys["docs/4-more/2-faq.md"] = &fstest.MapFile{Data: []byte("# FAQ\n")}
	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	s.page = func(p Page) templ.Component {
		return Layout(p, Parts{
			HeaderLinks: HeaderLinks(p, Link{Title: "Examples", URL: "4-more/2-faq.md"}),
			FooterLinks: FooterLinks(p,
				Link{Title: "Changelog", URL: "3-reference/2-changelog.md"}),
		})
	}
	html := editionPageHTML(t, s, "v1.2", "guides/events")
	for _, want := range []string{
		`<a href="../reference/specification#actions">actions</a>`,
		`<a href="https://github.com/o/demo/blob/v1.2.3/main.go">main.go</a>`,
		`<a class="icon-link" href="https://github.com/o/demo/blob/main/` +
			`docs/versions/v1.2/2-guides/01-events.md">`,
		`<a class="header-link" href="../../main/more/faq">Examples</a>`,
		`<a class="icon-link" href="../reference/changelog">Changelog</a>`,
	} {
		require.Contains(t, html, want)
	}
	require.Contains(t, pageHTML(t, s, "guides/events"),
		`<a class="header-link" href="../more/faq">Examples</a>`)
	require.Contains(t, editionPageHTML(t, s, "v1.2", "more/examples"),
		`<a class="header-link" href="../../main/more/faq">Examples</a>`)
}

// TestReleaseErrors checks that releases reject
// site settings and outside links or sources.
func TestReleaseErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		file, data, want string
	}{
		"settings": {
			file: "docs/versions/v1.2/README.md",
			data: dedent(`
				---
				name: Old
				---

				# Introduction
			`),
			want: "docs/versions/v1.2/README.md: a release takes the settings of " +
				"the site from the README.md of the docs directory",
		},
		"link out of the release": {
			file: "docs/versions/v1.2/2-guides/10-testing.md",
			data: dedent(`
				# Testing

				See [main.go](../../../../main.go).
			`),
			want: `link "../../../../main.go" leads out of docs/versions/v1.2`,
		},
		"source out of the release": {
			file: "docs/versions/v1.2/3-reference/2-changelog.md",
			data: dedent(`
				---
				source: ../../../../CHANGELOG.md
				---

				# Changelog
			`),
			want: "docs/versions/v1.2/3-reference/2-changelog.md: " +
				"source CHANGELOG.md is outside docs/versions/v1.2",
		},
		"directory name": {
			file: "docs/versions/latest/README.md",
			data: "# Introduction\n",
			want: "docs/versions/latest: the directory of a release is named " +
				"after its feature version",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fsys := withReleases(t, testRepoFS(), "v1.2.3")
			fsys[tt.file] = &fstest.MapFile{Data: []byte(tt.data)}
			_, err := read(fsys, "docs", noRemote, false)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// TestDocsIndex checks that root index.html opens its edition's first page.
func TestDocsIndex(t *testing.T) {
	html := render(t, docsIndex("v1.2/", &chrome{name: "Demo", favicon: "favicon.svg"}))
	require.Contains(t, html,
		`<meta http-equiv="refresh" content="0; url=v1.2/">`)
	require.Contains(t, html,
		`<title>Demo documentation</title>`)
	require.Contains(t, html,
		`<a href="v1.2/">`)
	require.Contains(t, html,
		`<link rel="icon" href="v1.2/favicon.svg" type="image/svg+xml">`)
}

// TestDefaultEdition checks that index.html opens the latest release, or main if none.
func TestDefaultEdition(t *testing.T) {
	s, err := read(
		withReleases(t, testRepoFS(), "v1.1.0", "v1.2.0"), "docs", noRemote, false,
	)
	require.NoError(t, err)
	files, err := s.Files()
	require.NoError(t, err)
	require.Contains(t, string(files["index.html"]), `url=v1.2/`)
	files, err = newTestSite(t).Files()
	require.NoError(t, err)
	require.Contains(t, string(files["index.html"]), `url=main/`)
}

// TestVersionsLlmsTxt checks root llms.txt links to every edition, latest first.
func TestVersionsLlmsTxt(t *testing.T) {
	s, err := read(
		withReleases(t, testRepoFS(), "v1.1.0", "v1.2.0"), "docs", noRemote, false,
	)
	require.NoError(t, err)
	require.Equal(t, dedent(`
		# Demo

		> Demo is a web framework.

		Each link leads to the llms.txt of a version of the documentation.

		## Versions

		- [v1.2](v1.2/llms.txt): The latest release
		- [v1.1](v1.1/llms.txt): An earlier release
		- [main](main/llms.txt): The main branch, unreleased
	`),
		string(versionsLlmsTxt(s.editions, "Demo", s.Description())))
	require.NotContains(t,
		string(versionsLlmsTxt(
			newTestSite(t).editions, "Demo", "Intro.")), "latest release")
}

// TestMorpheusAssets checks that pages load their version's Morpheus bundle and,
// after morpheus.min.css, point <neo-icon> to version icons instead of /static/icons.
func TestMorpheusAssets(t *testing.T) {
	html := pageHTML(t, newTestSite(t), "guides/events")
	for _, want := range []string{
		`<link rel="stylesheet" href="../morpheus/morpheus.min.css">`,
		`<script type="module" src="../morpheus/morpheus.min.js"></script>`,
	} {
		require.Contains(t, html, want)
	}
	require.NotContains(t, html, "cdn.jsdelivr.net")
	base := strings.Index(html,
		`<style>:root{--neo-icon-base:"../morpheus/icons"}</style>`)
	require.Positive(t, base, "no icon base")
	require.Less(t, strings.Index(html, `morpheus.min.css">`), base)
}

// TestMorpheusBundleVersion checks that go generate's embedded bundle matches
// go.mod's Morpheus version used by rendered templ components.
func TestMorpheusBundleVersion(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	require.True(t, ok)
	var version string
	for _, dep := range info.Deps {
		if dep.Path == "github.com/romshark/morpheus" {
			version = dep.Version
		}
	}
	require.NotEmpty(t, version)
	js, err := static.ReadFile("static/morpheus/morpheus.min.js")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(js), "/*! Morpheus "+version+" "),
		"run go generate")
}

// TestVersionNote checks edition README notes above every page, page-relative
// links including other editions, page-version naming in the body, and snapshots
// omitting main's note.
func TestVersionNote(t *testing.T) {
	fsys := withReleases(t, testRepoFS(), "v1.1.0", "v1.2.0")
	fsys["docs/README.md"] = &fstest.MapFile{Data: []byte(strings.Replace(testRoot,
		"suggested: true\n", dedent(`
			suggested: true
			note: Main moves fast. See [Testing](2-guides/10-testing.md) and the
			  [intro](README.md#introduction), or
			  [v1.2](versions/v1.2/2-guides/01-events.md#dispatch).
		`), 1))}
	fsys["docs/versions/v1.1/README.md"] = &fstest.MapFile{Data: []byte(dedent(`
		---
		note: v1.1 gets no fixes. See [Upgrading](1-get-started/2-upgrading.md),
		  [v1.2](../v1.2/) and [main](../../README.md#introduction).
		---

		# Introduction

		Demo is a web framework.
	`))}
	fsys["docs/versions/v1.2/README.md"] = &fstest.MapFile{Data: []byte(dedent(`
		---
		note: "**Beta.**"
		---

		# Introduction

		Demo is a web framework.
	`))}
	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	for name, tt := range map[string]struct {
		version, path string
		want          []string
		wantNot       []string
	}{
		"main": {
			version: "main", path: "guides/events",
			want: []string{
				`<body class="docs-page" data-site-root="../" data-version="main">`,
				`<p class="version-note" role="note">Main moves fast. ` +
					`See <a href="../guides/testing">Testing</a> and the ` +
					`<a href="../#introduction">intro</a>, ` +
					`or <a href="../../v1.2/guides/events#dispatch">v1.2</a>.</p>`,
			},
		},
		"first page of main": {
			version: "main", path: "",
			want: []string{`and the <a href="#introduction">intro</a>, ` +
				`or <a href="../v1.2/guides/events#dispatch">v1.2</a>.</p>`},
		},
		"earlier release": {
			version: "v1.1", path: "guides/events",
			want: []string{
				`data-version="v1.1">`,
				`<p class="version-note" role="note">v1.1 gets no fixes. ` +
					`See <a href="../get-started/upgrading">Upgrading</a>, ` +
					`<a href="../../v1.2/">v1.2</a> and ` +
					`<a href="../../main/#introduction">main</a>.</p>`,
			},
		},
		"latest release": {
			version: "v1.2", path: "guides/events",
			want: []string{
				`<p class="version-note" role="note"><strong>Beta.</strong></p>`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			html := editionPageHTML(t, s, tt.version, tt.path)
			for _, want := range tt.want {
				require.Contains(t, html, want)
			}
			for _, notWant := range tt.wantNot {
				require.NotContains(t, html, notWant)
			}
		})
	}

	_, files, err := snapshot(fsys, "docs", "v1.3.0", noRemote)
	require.NoError(t, err)
	require.NotContains(t, string(files["docs/versions/v1.3/README.md"]), "note")
}

// TestVersionNoteImage checks note-image copy links from every page,
// note-only usage, and release notes using release images.
func TestVersionNoteImage(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/README.md"] = &fstest.MapFile{
		Data: []byte(strings.Replace(testRoot,
			"suggested: true\n",
			dedent(`
				suggested: true
				note: "![New](img/new.svg) Try v2."
			`), 1,
		)),
	}
	fsys["docs/img/new.svg"] = &fstest.MapFile{Data: []byte("<svg></svg>")}
	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	img := mediaName("<svg></svg>", "new.svg")
	require.Contains(t, pageHTML(t, s, "guides/events"),
		`<p class="version-note" role="note"><img src="../`+
			img+`" alt="New"> Try v2.</p>`)
	require.Contains(t, pageHTML(t, s, ""), `<img src="./`+img+`" alt="New">`)
	files, err := s.Files()
	require.NoError(t, err)
	require.Equal(t, "<svg></svg>", string(files["main/"+img]))

	fsys = withReleases(t, testRepoFS(), "v1.2.0")
	fsys["docs/img/new.svg"] = &fstest.MapFile{Data: []byte("<svg></svg>")}
	fsys["docs/versions/v1.2/README.md"] = &fstest.MapFile{Data: []byte(dedent(`
		---
		note: "![New](../../img/new.svg)"
		---

		# Introduction

		Demo.
	`))}
	_, err = read(fsys, "docs", noRemote, false)
	require.ErrorContains(t,
		err, `media "../../img/new.svg" is outside docs/versions/v1.2`)
}

// TestVersionNoteErrors checks that notes are one paragraph, link to existing
// headings within a release or pages in another edition, and appear only in a
// docs-directory README.md.
func TestVersionNoteErrors(t *testing.T) {
	// main sets its note in the test docs front matter.
	main := func(note string) string {
		return strings.Replace(testRoot, "suggested: true\n", fmt.Sprintf(dedent(`
			suggested: true
			note: %s
		`), note), 1)
	}
	for name, tt := range map[string]struct {
		file, data, want string
	}{
		"2 paragraphs": {
			file: "docs/README.md",
			data: main(`"A.\n\nB."`),
			want: "docs/README.md: note: a note is 1 paragraph",
		},
		"heading": {
			file: "docs/README.md",
			data: main(`"# A"`),
			want: "docs/README.md: note: a note is 1 paragraph",
		},
		"dead fragment": {
			file: "docs/README.md",
			data: main(`"[x](2-guides/01-events.md#missing)"`),
			want: `docs/README.md: note: link "2-guides/01-events.md#missing" names no heading`,
		},
		"link out of a release": {
			file: "docs/versions/v1.2/README.md",
			data: dedent(`
				---
				note: "[x](../../../CHANGELOG.md)"
				---

				# Introduction
			`),
			want: `link "../../../CHANGELOG.md" leads out of docs/versions/v1.2`,
		},
		"dead fragment of another edition": {
			file: "docs/versions/v1.2/README.md",
			data: dedent(`
				---
				note: "[x](../../2-guides/01-events.md#missing)"
				---

				# Introduction
			`),
			want: `docs/versions/v1.2/README.md: note: ` +
				`link "../../2-guides/01-events.md#missing" names no heading`,
		},
		"note of another page": {
			file: "docs/2-guides/10-testing.md",
			data: dedent(`
				---
				note: A.
				---

				# Testing
			`),
			want: "docs/2-guides/10-testing.md: only the README.md of a docs directory " +
				"takes a note",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fsys := withReleases(t, testRepoFS(), "v1.2.3")
			fsys[tt.file] = &fstest.MapFile{Data: []byte(tt.data)}
			_, err := read(fsys, "docs", noRemote, false)
			require.ErrorContains(t, err, tt.want)
		})
	}
}
