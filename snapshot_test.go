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

// TestSnapshot tests that a snapshot copies the first page, the groups and the
// files that their links name into the directory of the release, without the
// chrome of the site and the settings of the README.md, with the text of each
// source in its page and the links that leave the docs directory leading to the
// page that shows their file or to GitHub at the release tag.
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

// TestSnapshotWrite tests that Snapshot writes the copy into the repository,
// from which the release reads.
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

// TestSnapshotFileWithSpace tests that a snapshot gives a link to a file of the
// repository whose name holds a space its escaped URL at the tag,
// which the release then shows as a link.
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

// TestRelativeLink tests the link from a file to another file or directory,
// which keeps the query and the fragment of the link.
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
