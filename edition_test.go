package docpages

import (
	"fmt"
	"runtime/debug"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/require"
)

// TestPickReleases checks that the releases setting selects the latest patch of
// each feature version from the first, latest first, with or without a tag prefix.
func TestPickReleases(t *testing.T) {
	tags := []string{
		"v0.9.0", "v1.0.0", "v1.0.2", "v1.0.10", "v1.2.0", "v1.3.0-rc.1",
		"v1.10.0", "api/v1.0.3", "api/v2.0.0", "release-1", "v2",
	}
	for name, tt := range map[string]struct {
		set  releasesSettings
		want []release
	}{
		"from v1.0": {
			set: releasesSettings{From: "v1.0"},
			want: []release{
				{version: "v1.10", tag: "v1.10.0"},
				{version: "v1.2", tag: "v1.2.0"},
				{version: "v1.0", tag: "v1.0.10"},
			},
		},
		"from v1.2": {
			set: releasesSettings{From: "v1.2"},
			want: []release{
				{version: "v1.10", tag: "v1.10.0"},
				{version: "v1.2", tag: "v1.2.0"},
			},
		},
		"prefix": {
			set: releasesSettings{From: "v1.0", Prefix: "api/"},
			want: []release{
				{version: "v2.0", tag: "api/v2.0.0"},
				{version: "v1.0", tag: "api/v1.0.3"},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := pickReleases(tags, tt.set)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestPickReleasesErrors checks a setting without from, a from that is no feature
// version, as one with the prefix, and a from without a tag, as in a clone without tags.
func TestPickReleasesErrors(t *testing.T) {
	tags := []string{"v1.2.0", "v1.3.0-rc.1"}
	for name, tt := range map[string]struct {
		set  releasesSettings
		want string
	}{
		"no from": {
			set:  releasesSettings{Prefix: "api/"},
			want: "releases: from is missing, as from: v1.2",
		},
		"patch": {
			set:  releasesSettings{From: "v1.2.0"},
			want: `releases: from "v1.2.0" is no feature version, as v1.2`,
		},
		"prefix in from": {
			set:  releasesSettings{From: "api/v1.2"},
			want: `releases: from "api/v1.2" is no feature version, as v1.2`,
		},
		"no tag": {
			set: releasesSettings{From: "v1.3"},
			want: "releases: no tag v1.3.x, which a clone without tags lacks, " +
				"as actions/checkout without fetch-depth: 0",
		},
		"no tag with the prefix": {
			set: releasesSettings{From: "v1.2", Prefix: "api/"},
			want: "releases: no tag api/v1.2.x, which a clone without tags lacks, " +
				"as actions/checkout without fetch-depth: 0",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pickReleases(tags, tt.set)
			require.EqualError(t, err, tt.want)
		})
	}
}

// TestEditionPages checks edition UI: same-page version links latest-first, no version
// menu without releases, no note when unset, and noindex on main when a release exists.
func TestEditionPages(t *testing.T) {
	const noindex = `<meta name="robots" content="noindex">`
	option := func(version string) string {
		return `value="` + version + `" data-neo-value="` + version +
			`" data-fallback="../../` + version + `/" data-href="../../` + version +
			`/guides/events">` + version + `</neo-option>`
	}
	fsys, tags := withReleases("v1.1", "v1.1.0", "v1.2.0")
	releases, err := read(fsys, "docs", noRemote, tags, false)
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
			want: []string{`<header class="site-header">`},
			wantNot: []string{
				`version-select`, `aria-label="Documentation version"`, noindex,
			},
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

// TestReleaseLinks checks links to release pages, tag URLs for outside files,
// page sources at the tag, and main fallbacks for missing header pages.
func TestReleaseLinks(t *testing.T) {
	fsys, tags := withReleases("v1.2", "v1.2.3")
	fsys["docs/4-more/2-faq.md"] = &fstest.MapFile{Data: []byte("# FAQ\n")}
	s, err := read(fsys, "docs", noRemote, tags, false)
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
		`<a class="icon-link" href="https://github.com/o/demo/blob/v1.2.3/` +
			`docs/2-guides/01-events.md">`,
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

// TestReleaseFolder checks that the files of a release folder replace and add
// to those of the tag, and that their source and links lead to main.
func TestReleaseFolder(t *testing.T) {
	fsys, tags := withReleases("v1.2", "v1.2.3")
	fsys["docs/versions/v1.2/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte(dedent(`
			# Testing

			Fixed after the release. See [the notes](notes.txt) and [Mocks](11-mocks.md).
		`)),
	}
	fsys["docs/versions/v1.2/2-guides/notes.txt"] = &fstest.MapFile{
		Data: []byte("Notes."),
	}
	fsys["docs/versions/v1.2/2-guides/11-mocks.md"] = &fstest.MapFile{
		Data: []byte("# Mocks\n"),
	}
	s, err := read(fsys, "docs", noRemote, tags, false)
	require.NoError(t, err)
	html := editionPageHTML(t, s, "v1.2", "guides/testing")
	for _, want := range []string{
		`Fixed after the release.`,
		`<a href="https://github.com/o/demo/blob/main/` +
			`docs/versions/v1.2/2-guides/notes.txt">the notes</a>`,
		`<a href="../guides/mocks">Mocks</a>`,
		`<a class="icon-link" href="https://github.com/o/demo/blob/main/` +
			`docs/versions/v1.2/2-guides/10-testing.md">`,
	} {
		require.Contains(t, html, want)
	}
	require.Contains(t, editionPageHTML(t, s, "v1.2", "guides/mocks"), "Mocks")
	require.NotContains(t, pageHTML(t, s, "guides/testing"), "Fixed after the release.")
}

// TestReleaseTagReadme checks that a release drops the site settings and the
// note of main that the README.md of its tag holds.
func TestReleaseTagReadme(t *testing.T) {
	fsys, tags := withReleases("v1.2", "v1.2.3")
	readme := strings.Replace(testRoot, "name: Demo\n", "name: Old\n", 1)
	tags["v1.2.3"]["docs/README.md"] = &fstest.MapFile{
		Data: []byte(strings.Replace(readme,
			"suggested: true\n", "suggested: true\nnote: Not released yet.\n", 1)),
	}
	s, err := read(fsys, "docs", noRemote, tags, false)
	require.NoError(t, err)
	html := editionPageHTML(t, s, "v1.2", "guides/events")
	require.NotContains(t, html, "Not released yet.")
	require.NotContains(t, html, "Old")
}

// TestReleaseErrors checks the errors of release folders and of the pages of
// a release, which name its tag.
func TestReleaseErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		releases, file, data, want string
	}{
		"settings": {
			releases: "v1.2",
			file:     "docs/versions/v1.2/README.md",
			data: dedent(`
				---
				name: Old
				---

				# Introduction
			`),
			want: "release v1.2.3: docs/versions/v1.2/README.md: a release takes " +
				"the settings of the site from docs/README.md",
		},
		"page": {
			releases: "v1.2",
			file:     "docs/versions/v1.2/2-guides/10-testing.md",
			data: dedent(`
				# Testing

				See [none](none.md).
			`),
			want: `release v1.2.3: docs/2-guides/10-testing.md: ` +
				`link "none.md": no file docs/2-guides/none.md`,
		},
		"folder name": {
			releases: "v1.2",
			file:     "docs/versions/latest/README.md",
			data:     "# Introduction\n",
			want: "docs/versions/latest: the folder of a release is named " +
				"after its feature version, as v1.2",
		},
		"folder of no release": {
			releases: "v1.2",
			file:     "docs/versions/v1.1/README.md",
			data:     "# Introduction\n",
			want:     "docs/versions/v1.1: no release v1.1 among the tags from v1.2",
		},
		"folder without releases": {
			file: "docs/versions/v1.2/README.md",
			data: "# Introduction\n",
			want: "docs/versions/v1.2: the folder of a release needs releases " +
				"in the front matter of docs/README.md",
		},
		"file in versions": {
			releases: "v1.2",
			file:     "docs/versions/README.md",
			data:     "# Versions\n",
			want: "docs/versions/README.md: docs/versions holds only " +
				"the folders of releases, as v1.2",
		},
		"unused file": {
			releases: "v1.2",
			file:     "docs/versions/v1.2/img/old.png",
			data:     "old",
			want: "docs/versions/v1.2/img/old.png: no page shows this file, " +
				"and no link or setting names it",
		},
		"no tag": {
			releases: "v1.0",
			want:     "docs/README.md: releases: no tag v1.0.x",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fsys, tags := withReleases(tt.releases, "v1.2.3")
			if tt.file != "" {
				fsys[tt.file] = &fstest.MapFile{Data: []byte(tt.data)}
			}
			_, err := read(fsys, "docs", noRemote, tags, false)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// TestDocsIndex checks that root index.html opens its edition's first page with
// a script before the stylesheets, or a refresh without scripts, and shows the
// project in the stored theme and the colors of the edition.
func TestDocsIndex(t *testing.T) {
	c := &chrome{name: "Demo", favicon: "favicon.svg", style: true}
	html := render(t, docsIndex("v1.2/", c))
	for _, want := range []string{
		`<script data-dir="v1.2/">`,
		`location.replace(document.currentScript.dataset.dir);`,
		`<meta http-equiv="refresh" content="0; url=v1.2/">`,
		`<meta name="color-scheme" content="light dark">`,
		`<title>Demo documentation</title>`,
		`<link rel="icon" href="v1.2/favicon.svg" type="image/svg+xml">`,
		`localStorage.getItem("theme")`,
		`<link rel="stylesheet" href="v1.2/docpages.css">`,
		`<link rel="stylesheet" href="v1.2/style.css">`,
		`<body class="docs-index"><a class="docs-index-link" href="v1.2/">Demo</a></body>`,
	} {
		require.Contains(t, html, want)
	}
	require.Less(t,
		strings.Index(html, "location.replace"), strings.Index(html, `rel="stylesheet"`))

	c.style, c.logo = false, `<svg class="logo-svg"></svg>`
	html = render(t, docsIndex("main/", c))
	require.NotContains(t, html, "style.css")
	require.Contains(t, html,
		`<a class="docs-index-link" href="main/"><svg class="logo-svg"></svg></a>`)
}

// TestDefaultEdition checks that index.html opens the latest release, or main if none.
func TestDefaultEdition(t *testing.T) {
	fsys, tags := withReleases("v1.1", "v1.1.0", "v1.2.0")
	s, err := read(fsys, "docs", noRemote, tags, false)
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
	fsys, tags := withReleases("v1.1", "v1.1.0", "v1.2.0")
	s, err := read(fsys, "docs", noRemote, tags, false)
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

// TestVersionNote checks edition README notes above every page, links that
// resolve from the README.md holding the note, including links to other editions,
// and page-version naming in the body.
func TestVersionNote(t *testing.T) {
	fsys, tags := withReleases("v1.1", "v1.1.0", "v1.2.0")
	fsys["docs/README.md"] = &fstest.MapFile{Data: []byte(strings.Replace(testRoot,
		"suggested: true\n", dedent(`
			suggested: true
			releases:
			  from: v1.1
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
	s, err := read(fsys, "docs", noRemote, tags, false)
	require.NoError(t, err)
	for name, tt := range map[string]struct {
		version, path string
		want          []string
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
		})
	}
}

// TestVersionNoteImage checks note-image copy links and sizes from every page,
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
	const svg = `<svg width="16" height="12"></svg>`
	fsys["docs/img/new.svg"] = &fstest.MapFile{Data: []byte(svg)}
	s, err := read(fsys, "docs", noRemote, nil, false)
	require.NoError(t, err)
	img := mediaName(svg, "new.svg")
	require.Contains(t, pageHTML(t, s, "guides/events"),
		`<p class="version-note" role="note"><img src="../`+
			img+`" alt="New" width="16" height="12"> Try v2.</p>`)
	require.Contains(t, pageHTML(t, s, ""),
		`<img src="./`+img+`" alt="New" width="16" height="12">`)
	files, err := s.Files()
	require.NoError(t, err)
	require.Equal(t, svg, string(files["main/"+img]))

	releaseNote := func(note string) fstest.MapFS {
		fsys, _ := withReleases("v1.2")
		fsys["docs/versions/v1.2/img/old.svg"] = &fstest.MapFile{Data: []byte(svg)}
		fsys["docs/versions/v1.2/README.md"] = &fstest.MapFile{Data: []byte(dedent(`
			---
			note: "` + note + `"
			---

			# Introduction

			Demo.
		`))}
		return fsys
	}
	tags := testTags{"v1.2.0": testRepoFS()}
	s, err = read(releaseNote("![Old](img/old.svg)"), "docs", noRemote, tags, false)
	require.NoError(t, err)
	require.Contains(t, editionPageHTML(t, s, "v1.2", "guides/events"),
		`<img src="../`+mediaName(svg, "old.svg")+`" alt="Old" width="16" height="12">`)
	fsys = releaseNote("![New](../../img/new.svg)")
	fsys["docs/img/new.svg"] = &fstest.MapFile{Data: []byte(svg)}
	_, err = read(fsys, "docs", noRemote, tags, false)
	require.ErrorContains(t, err, `media "../../img/new.svg" is outside `+
		`docs/versions/v1.2, which holds the files of the release`)
}

// TestVersionNoteErrors checks that notes are one paragraph, link to existing
// files and headings, and appear only in the README.md of the docs directory or
// of a release folder.
func TestVersionNoteErrors(t *testing.T) {
	// main sets its note in the test docs front matter.
	main := func(note string) string {
		return strings.Replace(testRoot, "suggested: true\n", fmt.Sprintf(dedent(`
			suggested: true
			releases:
			  from: v1.2
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
			want: `docs/README.md: note: ` +
				`link "2-guides/01-events.md#missing" names no heading`,
		},
		"missing file": {
			file: "docs/versions/v1.2/README.md",
			data: dedent(`
				---
				note: "[x](../../../NONE.md)"
				---

				# Introduction
			`),
			want: `docs/versions/v1.2/README.md: ` +
				`note: link "../../../NONE.md": no file NONE.md`,
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
			fsys, tags := withReleases("v1.2", "v1.2.3")
			fsys[tt.file] = &fstest.MapFile{Data: []byte(tt.data)}
			_, err := read(fsys, "docs", noRemote, tags, false)
			require.ErrorContains(t, err, tt.want)
		})
	}
}
