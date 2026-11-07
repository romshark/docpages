package docpages

import (
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// TestSnapshot checks that a release copy contains the first page, groups,
// and linked files; omits site chrome and README settings; substitutes source text;
// and sends outside links to a page showing the file or its release-tag GitHub URL.
func TestSnapshot(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte("# Testing\n\nRead the [notes](../notes.txt).\n"),
	}
	fsys["docs/notes.txt"] = &fstest.MapFile{Data: []byte("Not a page.")}
	target, files, err := snapshot(fsys, "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	require.Equal(t, "docs/versions/v1.2", target)
	const v = "docs/versions/v1.2/"
	require.Equal(t, []string{
		v + "1-get-started/1-installation.md",
		v + "1-get-started/2-upgrading.md",
		v + "1-get-started/icon.svg",
		v + "2-guides/01-events.md",
		v + "2-guides/10-testing.md",
		v + "2-guides/README.md",
		v + "3-reference/1-specification.md",
		v + "3-reference/2-changelog.md",
		v + "3-reference/3-go-api.md",
		v + "4-more/1-examples.md",
		v + "README.md",
		v + "notes.txt",
	}, slices.Sorted(maps.Keys(files)))
	for file, want := range map[string]string{
		"README.md": "---\nsuggested: true\n---\n\n" +
			"# Introduction\n\nDemo is a web framework.\n",
		"1-get-started/2-upgrading.md": "# Upgrading\n\n" +
			"Read the [changelog](../3-reference/2-changelog.md).\n",
		"2-guides/01-events.md": "# Events\n\nEvents update tabs.\n\n" +
			"## Dispatch\n\nSee [actions](../3-reference/1-specification.md#actions), " +
			"[the spec](../3-reference/1-specification.md) and " +
			"[main.go](https://github.com/o/demo/blob/v1.2.3/main.go).\n\n" +
			"```go\nfunc f() {}\n```\n",
		"2-guides/10-testing.md": "# Testing\n\nRead the [notes](../notes.txt).\n",
		"3-reference/1-specification.md": "---\ntitle: Specification\n---\n\n" +
			"# Demo Specification\n\n## Pages\n\n### Actions\n",
		"3-reference/2-changelog.md": "# Changelog\n\n## v1.0.0\n\n### Added\n\n" +
			"## v0.9.0\n\n### Added\n",
		"3-reference/3-go-api.md": string(fsys["docs/3-reference/3-go-api.md"].Data),
		"notes.txt":               "Not a page.",
	} {
		require.Equal(t, want, string(files[v+file]), file)
	}

	_, _, err = snapshot(fsys, "docs", "v1.2", noRemote)
	require.ErrorContains(t, err, `"v1.2" is not a release tag, as v1.2.3`)
	_, _, err = snapshot(
		withReleases(t, testRepoFS(), "v1.2.0"), "docs", "v1.2.3", noRemote,
	)
	require.ErrorContains(t,
		err,
		"docs/versions/v1.2 exists; remove it to snapshot v1.2.3 again")
}

// TestSnapshotWrite checks that Snapshot writes the repository copy read by the release.
func TestSnapshotWrite(t *testing.T) {
	root := writeTestRepo(t)
	o := Options{Root: root, Dir: "docs"}
	target, err := Snapshot(o, "v1.2.3")
	require.NoError(t, err)
	require.Equal(t, "docs/versions/v1.2", target)
	_, err = os.Stat(filepath.Join(root, "docs", "versions", "v1.2", "README.md"))
	require.NoError(t, err)
	s, err := Read(o)
	require.NoError(t, err)
	require.Equal(t, "v1.2", s.latest().version)
}

// TestSnapshotRootDocs checks that root-level docs snapshot only README.md and
// groups, matching unused-file checks. Other repository links use tag URLs;
// outside images move under the copy's media/.
func TestSnapshotRootDocs(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md": {Data: []byte("---\nrepository: https://github.com/o/r\n---\n\n" +
			"# Intro\n\nSee [A](1-guide/1-a.md), [main.go](cmd/app/main.go) and " +
			"[the license](LICENSE).\n\n![Logo](img/logo.png)\n")},
		"1-guide/1-a.md":  {Data: []byte("# A\n\nText.\n")},
		"cmd/app/main.go": {Data: []byte("package main\n")},
		"LICENSE":         {Data: []byte("MIT License\n")},
		"img/logo.png":    {Data: []byte("logo")},
	}
	_, files, err := snapshot(fsys, ".", "v1.2.3", noRemote)
	require.NoError(t, err)
	require.Equal(t, []string{
		"versions/v1.2/1-guide/1-a.md",
		"versions/v1.2/README.md",
		"versions/v1.2/media/img/logo.png",
	}, slices.Sorted(maps.Keys(files)))
	require.Equal(t, "# Intro\n\nSee [A](1-guide/1-a.md), "+
		"[main.go](https://github.com/o/r/blob/v1.2.3/cmd/app/main.go) and "+
		"[the license](https://github.com/o/r/blob/v1.2.3/LICENSE).\n\n"+
		"![Logo](media/img/logo.png)\n",
		string(files["versions/v1.2/README.md"]))

	for name, data := range files {
		fsys[name] = &fstest.MapFile{Data: data}
	}
	_, err = read(fsys, ".", noRemote, false)
	require.NoError(t, err)
}

// TestSnapshotRootedLink checks that snapshots make root-based docs links and
// media paths relative to release copies instead of main files.
func TestSnapshotRootedLink(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte("# Testing\n\n" +
		"See [events](/docs/2-guides/01-events.md#dispatch)" +
		" and [notes](/docs/notes.txt).\n\n" +
		"![Logo](/docs/img/logo.png)\n\n<img src=\"/docs/img/logo.png\" width=\"10\">\n")}
	fsys["docs/notes.txt"] = &fstest.MapFile{Data: []byte("Notes.")}
	fsys["docs/img/logo.png"] = &fstest.MapFile{Data: []byte("logo")}
	_, files, err := snapshot(fsys, "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	const v = "docs/versions/v1.2/"
	require.Equal(t, "# Testing\n\n"+
		"See [events](01-events.md#dispatch) and [notes](../notes.txt).\n\n"+
		"![Logo](../img/logo.png)\n\n<img src=\"../img/logo.png\" width=\"10\">\n",
		string(files[v+"2-guides/10-testing.md"]))
	require.Equal(t, "Notes.", string(files[v+"notes.txt"]))
	require.Equal(t, "logo", string(files[v+"img/logo.png"]))

	_, err = read(withReleases(t, fsys, "v1.2.3"), "docs", noRemote, false)
	require.NoError(t, err)
}

// TestSnapshotDirectoryLink checks that links to directories whose README is a
// page become file URLs without trailing "/". It covers an outside source
// directory and a group linked by a source.
func TestSnapshotDirectoryLink(t *testing.T) {
	fsys := testRepoFS()
	fsys["examples/README.md"] = &fstest.MapFile{Data: []byte("# Examples of Demo\n")}
	fsys["docs/4-more/2-samples.md"] = &fstest.MapFile{
		Data: []byte("---\nsource: ../../examples/README.md\n---\n"),
	}
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte("# Testing\n\nSee [the examples](../../examples/).\n"),
	}
	fsys["SPECIFICATION.md"] = &fstest.MapFile{
		Data: []byte("# Demo Specification\n\nSee [the guides](docs/2-guides/).\n\n" +
			"## Pages\n\n### Actions\n"),
	}
	_, files, err := snapshot(fsys, "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	const v = "docs/versions/v1.2/"
	require.Equal(t, "# Testing\n\nSee [the examples](../4-more/2-samples.md).\n",
		string(files[v+"2-guides/10-testing.md"]))
	require.Contains(t, string(files[v+"3-reference/1-specification.md"]),
		"See [the guides](../2-guides/README.md).")

	_, err = read(withReleases(t, fsys, "v1.2.3"), "docs", noRemote, false)
	require.NoError(t, err)
}

// TestSnapshotFileWithSpace checks that spaced repository files get escaped
// tag URLs, which releases display as links.
func TestSnapshotFileWithSpace(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte("# Testing\n\nRead the [design](../../Design%20Doc.pdf).\n"),
	}
	fsys["Design Doc.pdf"] = &fstest.MapFile{Data: []byte("PDF")}
	const design = "https://github.com/o/demo/blob/v1.2.3/Design%20Doc.pdf"
	_, files, err := snapshot(fsys, "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	require.Equal(t, "# Testing\n\nRead the [design]("+design+").\n",
		string(files["docs/versions/v1.2/2-guides/10-testing.md"]))

	s, err := read(withReleases(t, fsys, "v1.2.3"), "docs", noRemote, false)
	require.NoError(t, err)
	require.Contains(t,
		editionPageHTML(t, s, "v1.2", "guides/testing"),
		`<a href="`+design+`">design</a>`)
}

// TestRelativeLink checks file-to-file or directory links preserving query and fragment.
func TestRelativeLink(t *testing.T) {
	for name, tt := range map[string]struct {
		from, to, link, want string
	}{
		"sibling":      {"docs/a/b.md", "docs/a/c.md", "x.md", "c.md"},
		"cousin":       {"docs/a/b.md", "docs/c/d.md", "x.md#h", "../c/d.md#h"},
		"up":           {"docs/a/b.md", "docs/c.md", "x.md?q=1", "../c.md?q=1"},
		"own dir":      {"docs/a/b.md", "docs/a", "x/", "./"},
		"directory":    {"docs/a/b.md", "docs/c", "x/", "../c/"},
		"from the top": {"README.md", "docs/a.md", "x.md", "docs/a.md"},
	} {
		t.Run(name, func(t *testing.T) {
			u, err := url.Parse(tt.link)
			require.NoError(t, err)
			require.Equal(t, tt.want, relativeLink(tt.from, tt.to, u))
		})
	}
}
