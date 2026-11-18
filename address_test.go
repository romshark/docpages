package docpages

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// TestSiteAddress checks that the setting address takes an absolute http or
// https URL, which gets a trailing "/", and rejects anything else.
func TestSiteAddress(t *testing.T) {
	for address, want := range map[string]string{
		"":                       "",
		"https://o.github.io/r":  "https://o.github.io/r/",
		"https://o.github.io/r/": "https://o.github.io/r/",
		"http://localhost:8080":  "http://localhost:8080/",
	} {
		got, err := siteAddress(address)
		require.NoError(t, err, address)
		require.Equal(t, want, got, address)
	}
	for _, address := range []string{
		"o.github.io/r",
		"/r/",
		"ftp://o.github.io/r/",
		"https://o.github.io/r/?v=1",
		"https://o.github.io/r/#top",
	} {
		_, err := siteAddress(address)
		require.ErrorContains(t, err, "isn't an absolute http or https URL", address)
	}
}

// testEditions returns a site of releases v1.2 and v1.1 and main. v1.1 has a
// page that v1.2 lacks, and main one that no release has.
func testEditions(address string) *Site {
	pages := func(paths ...string) []page {
		var r []page
		for _, p := range paths {
			r = append(r, page{file: "docs/" + p + ".md", path: p})
		}
		return r
	}
	return &Site{
		chrome: chrome{address: address},
		editions: []*edition{
			{version: "v1.2", pages: pages("", "guides/events")},
			{version: "v1.1", pages: pages("", "guides/events", "guides/old notes")},
			{version: MainVersion, pages: pages("", "guides/events", "guides/new")},
		},
	}
}

// TestCanonical checks that a page of an older release leads search engines to
// the same page of the latest release, that a page that the latest release
// lacks leads to itself, and that main, which noindex keeps out of search,
// leads nowhere. With an address, the URLs are absolute.
func TestCanonical(t *testing.T) {
	for name, tt := range map[string]struct {
		edition, page int
		relative, abs string
	}{
		"latest": {
			0, 1, "../../v1.2/guides/events", "https://o.github.io/r/v1.2/guides/events",
		},
		"older": {
			1, 1, "../../v1.2/guides/events", "https://o.github.io/r/v1.2/guides/events",
		},
		"first page of an older release": {
			1, 0, "./../v1.2/", "https://o.github.io/r/v1.2/",
		},
		"only in an older release": {
			1, 2,
			"../../v1.1/guides/old%20notes",
			"https://o.github.io/r/v1.1/guides/old%20notes",
		},
		"main": {2, 1, "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			s := testEditions("")
			require.Equal(t, tt.relative, s.canonical(s.editions[tt.edition], tt.page))
			s = testEditions("https://o.github.io/r/")
			require.Equal(t, tt.abs, s.canonical(s.editions[tt.edition], tt.page))
		})
	}

	// Without releases, main leads to itself.
	s := testEditions("")
	s.editions = s.editions[2:]
	require.Equal(t, "../../main/guides/new", s.canonical(s.editions[0], 2))
}

// TestSitemap checks that sitemap.xml lists the pages that lead search engines
// to themselves, with escaped absolute URLs.
func TestSitemap(t *testing.T) {
	require.Equal(t, dedent(`
		<?xml version="1.0" encoding="UTF-8"?>
		<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
		<url><loc>https://o.github.io/r/v1.2/</loc></url>
		<url><loc>https://o.github.io/r/v1.2/guides/events</loc></url>
		<url><loc>https://o.github.io/r/v1.1/guides/old%20notes</loc></url>
		</urlset>
	`), string(testEditions("https://o.github.io/r/").sitemapXML()))
}

// TestAddressFiles checks the canonical link and Open Graph tags of a page
// without and with the setting address, which adds og:url, sitemap.xml and
// a 404.html with absolute links.
func TestAddressFiles(t *testing.T) {
	build := func(address string) (*Site, map[string][]byte) {
		t.Helper()
		fsys := testRepoFS()
		if address != "" {
			fsys["docs/README.md"] = &fstest.MapFile{
				Data: []byte(strings.Replace(testRoot,
					"suggested: true\n", "suggested: true\naddress: "+address+"\n", 1)),
			}
		}
		s, err := read(fsys, "docs", noRemote, nil, false)
		require.NoError(t, err)
		s.page = testPage
		files, err := s.Files()
		require.NoError(t, err)
		return s, files
	}

	s, files := build("")
	html := pageHTML(t, s, "guides/events")
	for _, want := range []string{
		`<link rel="canonical" href="../../main/guides/events">`,
		`<meta property="og:type" content="website">`,
		`<meta property="og:site_name" content="Demo">`,
		`<meta property="og:title" content="Events">`,
		`<meta property="og:description" content="Events update tabs.">`,
	} {
		require.Contains(t, html, want)
	}
	require.NotContains(t, html, "og:url")
	require.NotContains(t, files, "sitemap.xml")
	require.NotContains(t, files, "404.html")

	s, files = build("https://o.github.io/r")
	html = pageHTML(t, s, "guides/events")
	require.Contains(t, html,
		`<link rel="canonical" href="https://o.github.io/r/main/guides/events">`)
	require.Contains(t, html,
		`<meta property="og:url" content="https://o.github.io/r/main/guides/events">`)
	require.Contains(t, string(files["sitemap.xml"]),
		"<url><loc>https://o.github.io/r/main/guides/events</loc></url>")
	notFound := string(files["404.html"])
	for _, want := range []string{
		`<meta name="robots" content="noindex">`,
		`<link rel="stylesheet" href="https://o.github.io/r/main/docpages.css">`,
		`<a href="https://o.github.io/r/">Open the documentation</a>`,
	} {
		require.Contains(t, notFound, want)
	}

	_, err := read(fstest.MapFS{"docs/README.md": &fstest.MapFile{Data: []byte(strings.Replace(
		testRoot, "suggested: true\n", "suggested: true\naddress: o.github.io/r\n", 1))}},
		"docs", noRemote, nil, false)
	require.EqualError(t, err,
		`docs/README.md: address "o.github.io/r" isn't an absolute `+
			"http or https URL without query and fragment, as https://o.github.io/r/")
}
