package docpages

import (
	"io/fs"
	"runtime/debug"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// TestReleases tests that the releases are the directories of the versions
// directory, the latest first by their numbers, and that a directory named
// otherwise fails the read.
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

// TestEditionPages tests what a docs page shows of its edition: the version
// menu with the same page in each edition, the latest release first, no note
// without one in the front matter, and noindex on the pages of main next to a
// release.
func TestEditionPages(t *testing.T) {
	const noindex = `<meta name="robots" content="noindex">`
	option := func(version string) string {
		return `value="` + version + `" data-neo-value="` + version +
			`" data-fallback="../../` + version + `/" data-href="../../` + version +
			`/guides/events">` + version + `</neo-option>`
	}
	releases, err := read(withReleases(t, testRepoFS(), "v1.1.0", "v1.2.0"), "docs", noRemote, false)
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
	require.Less(t, strings.Index(html, option("v1.2")), strings.Index(html, option("v1.1")))
	require.Less(t, strings.Index(html, option("v1.1")), strings.Index(html, option("main")))
}

// TestReleaseLinks tests that the pages of a release lead to its own pages,
// link the files that their snapshot took from outside the docs directory at
// the release tag and their own file on main, and that a link of the header
// to a page that the release lacks leads to the page of main.
func TestReleaseLinks(t *testing.T) {
	fsys := withReleases(t, testRepoFS(), "v1.2.3")
	fsys["docs/4-more/2-faq.md"] = &fstest.MapFile{Data: []byte("# FAQ\n")}
	fsys["docs/README.md"] = &fstest.MapFile{Data: []byte(strings.Replace(testRoot,
		"url: 4-more/1-examples.md", "url: 4-more/2-faq.md", 1))}
	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	html := editionPageHTML(t, s, "v1.2", "guides/events")
	for _, want := range []string{
		`<a href="../reference/specification#actions">actions</a>`,
		`<a href="https://github.com/o/demo/blob/v1.2.3/main.go">main.go</a>`,
		`<a class="icon-link" href="https://github.com/o/demo/blob/main/` +
			`docs/versions/v1.2/2-guides/01-events.md">`,
		`<a class="header-link" href="../../main/more/faq">Examples</a>`,
		`<a class="icon-link" href="../reference/changelog"> Changelog</a>`,
	} {
		require.Contains(t, html, want)
	}
	require.Contains(t, pageHTML(t, s, "guides/events"),
		`<a class="header-link" href="../more/faq">Examples</a>`)
	require.Contains(t, editionPageHTML(t, s, "v1.2", "more/examples"),
		`<a class="header-link" href="../../main/more/faq">Examples</a>`)
}

// TestReleaseErrors tests that a release takes no settings of the site and
// keeps its links and sources within its directory.
func TestReleaseErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		file, data, want string
	}{
		"settings": {
			file: "docs/versions/v1.2/README.md",
			data: "---\nname: Old\n---\n\n# Introduction\n",
			want: "docs/versions/v1.2/README.md: a release takes the settings of the site " +
				"from the README.md of the docs directory",
		},
		"link out of the release": {
			file: "docs/versions/v1.2/2-guides/10-testing.md",
			data: "# Testing\n\nSee [main.go](../../../../main.go).\n",
			want: `link "../../../../main.go" leads out of docs/versions/v1.2`,
		},
		"source out of the release": {
			file: "docs/versions/v1.2/3-reference/2-changelog.md",
			data: "---\nsource: ../../../../CHANGELOG.md\n---\n\n# Changelog\n",
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

// TestDocsIndex tests that the index.html of the output directory opens the
// first page of the edition that it names.
func TestDocsIndex(t *testing.T) {
	html := render(t, docsIndex("v1.2/", &chrome{name: "Demo", favicon: "favicon.svg"}))
	require.Contains(t, html, `<meta http-equiv="refresh" content="0; url=v1.2/">`)
	require.Contains(t, html, `<title>Demo documentation</title>`)
	require.Contains(t, html, `<a href="v1.2/">`)
	require.Contains(t, html, `<link rel="icon" href="v1.2/favicon.svg" type="image/svg+xml">`)
}

// TestDefaultEdition tests that index.html opens the latest release, or main
// without releases.
func TestDefaultEdition(t *testing.T) {
	s, err := read(withReleases(t, testRepoFS(), "v1.1.0", "v1.2.0"), "docs", noRemote, false)
	require.NoError(t, err)
	files, err := s.Files()
	require.NoError(t, err)
	require.Contains(t, string(files["index.html"]), `url=v1.2/`)
	files, err = newTestSite(t).Files()
	require.NoError(t, err)
	require.Contains(t, string(files["index.html"]), `url=main/`)
}

// TestVersionsLlmsTxt tests that the llms.txt of the output directory links
// the llms.txt of each edition, the latest release first.
func TestVersionsLlmsTxt(t *testing.T) {
	s, err := read(withReleases(t, testRepoFS(), "v1.1.0", "v1.2.0"), "docs", noRemote, false)
	require.NoError(t, err)
	require.Equal(t, "# Demo\n\n> Demo is a web framework.\n\n"+
		"Each link leads to the llms.txt of a version of the documentation.\n\n"+
		"## Versions\n\n"+
		"- [v1.2](v1.2/llms.txt): The latest release\n"+
		"- [v1.1](v1.1/llms.txt): An earlier release\n"+
		"- [main](main/llms.txt): The main branch, unreleased\n",
		string(versionsLlmsTxt(s.editions, "Demo", s.Description())))
	require.NotContains(t,
		string(versionsLlmsTxt(newTestSite(t).editions, "Demo", "Intro.")), "latest release")
}

// TestMorpheusAssets tests that a docs page loads the Morpheus bundle of its
// version and points <neo-icon> at the icons of the version after
// morpheus.min.css, whose /static/icons it replaces.
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

// TestMorpheusBundleVersion tests that the embedded Morpheus bundle is that of
// the version of Morpheus that go.mod requires, whose templ components the
// pages render. go generate writes the bundle.
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

// TestVersionNote tests that the note of the README.md of an edition shows
// above its pages, with its links as seen from each page, including those to
// the pages of other editions, that the body names the version of the page,
// and that a snapshot leaves the note of main behind.
func TestVersionNote(t *testing.T) {
	const intro = "\n\n# Introduction\n\nDemo is a web framework.\n"
	fsys := withReleases(t, testRepoFS(), "v1.1.0", "v1.2.0")
	fsys["docs/README.md"] = &fstest.MapFile{Data: []byte(strings.Replace(testRoot,
		"suggested: true\n", "suggested: true\nnote: Main moves fast. "+
			"See [Testing](2-guides/10-testing.md) and the [intro](README.md#introduction), "+
			"or [v1.2](versions/v1.2/2-guides/01-events.md#dispatch).\n", 1))}
	fsys["docs/versions/v1.1/README.md"] = &fstest.MapFile{Data: []byte("---\n" +
		"note: v1.1 gets no fixes. See [Upgrading](1-get-started/2-upgrading.md), " +
		"[v1.2](../v1.2/) and [main](../../README.md#introduction).\n---" + intro)}
	fsys["docs/versions/v1.2/README.md"] = &fstest.MapFile{Data: []byte("---\n" +
		"note: \"**Beta.**\"\n---" + intro)}
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

// TestVersionNoteImage tests that an image of a note leads to the copy of its file,
// as seen from each page, which the note alone makes used, and that the
// note of a release takes its images from the release.
func TestVersionNoteImage(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/README.md"] = &fstest.MapFile{
		Data: []byte(strings.Replace(testRoot,
			"suggested: true\n",
			"suggested: true\nnote: \"![New](img/new.svg) Try v2.\"\n", 1,
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
	fsys["docs/versions/v1.2/README.md"] = &fstest.MapFile{Data: []byte(
		"---\nnote: \"![New](../../img/new.svg)\"\n---\n\n# Introduction\n\nDemo.\n")}
	_, err = read(fsys, "docs", noRemote, false)
	require.ErrorContains(t,
		err, `media "../../img/new.svg" is outside docs/versions/v1.2`)
}

// TestVersionNoteErrors tests that a note is 1 paragraph whose links lead to
// headings that exist and stay within a release or lead to the pages of
// another edition, and that only the README.md of a docs directory takes one.
func TestVersionNoteErrors(t *testing.T) {
	// main sets the note of main, in the front matter of the test docs.
	main := func(note string) string {
		return strings.Replace(testRoot, "suggested: true\n", "suggested: true\nnote: "+note+"\n", 1)
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
			data: "---\nnote: \"[x](../../../CHANGELOG.md)\"\n---\n\n# Introduction\n",
			want: `link "../../../CHANGELOG.md" leads out of docs/versions/v1.2`,
		},
		"dead fragment of another edition": {
			file: "docs/versions/v1.2/README.md",
			data: "---\nnote: \"[x](../../2-guides/01-events.md#missing)\"\n---\n\n# Introduction\n",
			want: `docs/versions/v1.2/README.md: note: ` +
				`link "../../2-guides/01-events.md#missing" names no heading`,
		},
		"note of another page": {
			file: "docs/2-guides/10-testing.md",
			data: "---\nnote: A.\n---\n\n# Testing\n",
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
