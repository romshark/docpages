package docpages

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// testGit runs git with args in dir, without the user's Git configuration,
// and returns its output.
func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

// writeTestFiles writes files, by slash-separated path, into dir.
func writeTestFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	data := make(map[string][]byte, len(files))
	for name, s := range files {
		data[name] = []byte(s)
	}
	require.NoError(t, writeFiles(dir, data))
}

// testGitRepo returns a Git repository with files on main.
func testGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	testGit(t, dir, "init", "-q", "-b", "main")
	writeTestFiles(t, dir, files)
	testGit(t, dir, "add", "-A")
	testGit(t, dir, "commit", "-q", "-m", "init")
	return dir
}

// TestGitTags checks the tags of a repository and its files at a tag:
// blobs, directories, symbolic links in the tree, a link out of the tree,
// which is left out, and missing files.
func TestGitTags(t *testing.T) {
	dir := testGitRepo(t, map[string]string{
		"README.md":         "# Repo\n",
		"docs/README.md":    "# Docs\n",
		"docs/1-a/1-b.md":   "# B at v1.0\n",
		"docs/img/shot.png": "png",
	})
	require.NoError(t, os.Symlink("1-a/1-b.md", filepath.Join(dir, "docs/link.md")))
	require.NoError(t, os.Symlink("../README.md", filepath.Join(dir, "docs/up.md")))
	require.NoError(t, os.Symlink("../../outside.md", filepath.Join(dir, "docs/out.md")))
	testGit(t, dir, "add", "-A")
	testGit(t, dir, "commit", "-q", "-m", "links")
	testGit(t, dir, "tag", "v1.0.0")
	testGit(t, dir, "tag", "-a", "-m", "API", "api/v1.0.0")
	writeTestFiles(t, dir, map[string]string{"docs/1-a/1-b.md": "# B on main\n"})

	g := gitTags{root: dir}
	tags, err := g.tags()
	require.NoError(t, err)
	require.Equal(t, []string{"api/v1.0.0", "v1.0.0"}, tags)

	for _, tag := range tags {
		tree, err := g.tree(tag, "docs")
		require.NoError(t, err)
		require.NoError(t, fstest.TestFS(tree, "README.md", "docs/README.md",
			"docs/1-a/1-b.md", "docs/img/shot.png", "docs/link.md", "docs/up.md"))
		data, err := fs.ReadFile(tree, "docs/1-a/1-b.md")
		require.NoError(t, err)
		require.Equal(t, "# B at v1.0\n", string(data))
		data, err = fs.ReadFile(tree, "docs/link.md")
		require.NoError(t, err)
		require.Equal(t, "# B at v1.0\n", string(data))
		data, err = fs.ReadFile(tree, "docs/up.md")
		require.NoError(t, err)
		require.Equal(t, "# Repo\n", string(data))
		_, err = fs.ReadFile(tree, "docs/out.md")
		require.ErrorIs(t, err, fs.ErrNotExist)
		_, err = fs.Stat(tree, "docs/none.md")
		require.ErrorIs(t, err, fs.ErrNotExist)
		_, err = fs.Stat(tree, "../x")
		require.ErrorIs(t, err, fs.ErrInvalid)
	}
	_, err = g.tree("v9.9.9", "docs")
	require.ErrorContains(t, err, "git rev-parse:")
}

// TestReadGitReleases checks a site whose releases come from Git tags: the
// latest patch of each feature version, files of release folders on main in
// place of those of the tag, and pages of main from the working tree.
func TestReadGitReleases(t *testing.T) {
	readme := dedent(`
		---
		repository: https://github.com/o/demo
		releases:
		  from: v1.0
		---

		# Docs

		See [B](1-a/1-b.md).
	`)
	dir := testGitRepo(t, map[string]string{
		"docs/README.md":  readme,
		"docs/1-a/1-b.md": "# B at v1.0.0\n",
	})
	testGit(t, dir, "tag", "v1.0.0")
	writeTestFiles(t, dir, map[string]string{"docs/1-a/1-b.md": "# B at v1.0.1\n"})
	testGit(t, dir, "commit", "-q", "-am", "v1.0.1")
	testGit(t, dir, "tag", "v1.0.1")
	writeTestFiles(t, dir, map[string]string{"docs/1-a/1-b.md": "# B at v1.1.0\n"})
	testGit(t, dir, "commit", "-q", "-am", "v1.1.0")
	testGit(t, dir, "tag", "v1.1.0")
	writeTestFiles(t, dir, map[string]string{
		"docs/1-a/1-b.md":               "# B on main\n",
		"docs/versions/v1.0/1-a/1-b.md": "# B fixed in v1.0\n",
	})

	s, err := readRepo(Options{Root: dir, Dir: "docs"}, false)
	require.NoError(t, err)
	var versions []string
	for _, e := range s.editions {
		versions = append(versions, e.version)
	}
	require.Equal(t, []string{"v1.1", "v1.0", "main"}, versions)
	for version, want := range map[string]string{
		"v1.1": "B at v1.1.0",
		"v1.0": "B fixed in v1.0",
		"main": "B on main",
	} {
		html := editionPageHTML(t, s, version, "a/b")
		require.Contains(t, html, "<h1 id=", version)
		require.Contains(t, html, want, version)
	}
	require.Contains(t, editionPageHTML(t, s, "v1.1", "a/b"),
		`href="https://github.com/o/demo/blob/v1.1.0/docs/1-a/1-b.md"`)
	require.Contains(t, editionPageHTML(t, s, "v1.0", "a/b"),
		`href="https://github.com/o/demo/blob/main/docs/versions/v1.0/1-a/1-b.md"`)
}

// TestOverlayFS checks an overlay that replaces and adds files and directories.
func TestOverlayFS(t *testing.T) {
	base := fstest.MapFS{
		"a.md":     {Data: []byte("a")},
		"d/b.md":   {Data: []byte("b")},
		"d/c.md":   {Data: []byte("c")},
		"e/f/g.md": {Data: []byte("g")},
	}
	o := newOverlay(base, map[string][]byte{
		"d/b.md":   []byte("new b"),
		"d/x.md":   []byte("x"),
		"n/m/y.md": []byte("y"),
	})
	require.NoError(t, fstest.TestFS(o,
		"a.md", "d/b.md", "d/c.md", "d/x.md", "e/f/g.md", "n/m/y.md"))
	data, err := fs.ReadFile(o, "d/b.md")
	require.NoError(t, err)
	require.Equal(t, "new b", string(data))
	entries, err := fs.ReadDir(o, "d")
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.Equal(t, []string{"b.md", "c.md", "x.md"}, names)
	entries, err = fs.ReadDir(o, ".")
	require.NoError(t, err)
	require.True(t, slices.ContainsFunc(entries, func(e fs.DirEntry) bool {
		return e.Name() == "n" && e.IsDir()
	}))
	_, err = fs.ReadDir(o, "none")
	require.ErrorIs(t, err, fs.ErrNotExist)
	_, err = o.Open("../x")
	require.ErrorIs(t, err, fs.ErrInvalid)
}
