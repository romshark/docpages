package docpages

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// forgeHost is the front matter of a repository hosted on a forge other than
// GitHub, which replaces the repository of [testRoot].
const forgeHost = `repository: https://git.example.com/o/demo
host:
  name: Forge
  icon: icons/forge.svg
  file: "{repository}/src/{ref}/{path}"
  dir: "{repository}/src/{ref}/{path}/"
`

// forgeRepoFS returns [testRepoFS] with the repository on the forge of [forgeHost].
func forgeRepoFS() fstest.MapFS {
	fsys := testRepoFS()
	fsys["docs/README.md"] = &fstest.MapFile{Data: []byte(strings.Replace(testRoot,
		"repository: https://github.com/o/demo\n", forgeHost, 1))}
	fsys["docs/icons/forge.svg"] = &fstest.MapFile{
		Data: []byte(`<svg viewBox="0 0 16 16"><path d="M3 3"/></svg>`),
	}
	return fsys
}

// TestHost tests that the host setting names the links to the repository,
// gives them its icon, and makes the URLs of the files of the repository,
// at main on the pages and at the release tag in a snapshot.
func TestHost(t *testing.T) {
	const (
		repo = "https://git.example.com/o/demo"
		icon = `<svg aria-hidden="true" viewBox="0 0 16 16"><path d="M3 3"/></svg>`
	)
	s, err := read(forgeRepoFS(), "docs", noRemote, false)
	require.NoError(t, err)
	html := pageHTML(t, s, "guides/events")
	for _, want := range []string{
		`<a href="` + repo + `/src/main/main.go">main.go</a>`,
		`<a class="icon-link" href="` + repo + `/src/main/docs/2-guides/01-events.md">` +
			icon + ` View this page on Forge</a>`,
		`<a class="header-repository icon-link" href="` +
			repo + `">` + icon + ` Forge</a>`,
		`<a class="icon-link" href="` + repo + `">` + icon + ` Forge</a>`,
	} {
		require.Contains(t, html, want)
	}
	require.NotContains(t, html, "GitHub")

	_, files, err := snapshot(forgeRepoFS(), "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	require.Contains(t, string(files["docs/versions/v1.2/2-guides/01-events.md"]),
		"[main.go]("+repo+"/src/v1.2.3/main.go)")
}

// TestHostURL tests the URLs of a file and a directory of the repository, the
// URL of a directory being that of a file without a dir setting.
func TestHostURL(t *testing.T) {
	h := githubHost("https://github.com/o/r")
	require.Equal(t,
		"https://github.com/o/r/blob/main/docs/a.md",
		h.url("main", "docs/a.md", false))
	require.Equal(t,
		"https://github.com/o/r/tree/v1.2.3/cmd/",
		h.url("v1.2.3", "cmd", true))

	h, err := readHost(nil, "docs", "https://git.example.com/o/r",
		&hostSettings{Name: "Forge", File: "{repository}/-/blob/{ref}/{path}"})
	require.NoError(t, err)
	require.Equal(t,
		"https://git.example.com/o/r/-/blob/main/cmd",
		h.url("main", "cmd", true))
	require.Empty(t, h.icon)
	require.False(t, h.github)
}

// TestHostErrors tests that a host setting has a name and a file URL, and that
// its icon exists.
func TestHostErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		host, want string
	}{
		"no name": {
			host: "host:\n  file: \"{repository}/{path}\"\n",
			want: "docs/README.md: host: a host has a name and a file URL",
		},
		"no file": {
			host: "host:\n  name: Forge\n",
			want: "docs/README.md: host: a host has a name and a file URL",
		},
		"missing icon": {
			host: "host:\n  name: Forge\n  file: \"{repository}/{path}\"" +
				"\n  icon: icons/none.svg\n",
			want: "docs/README.md: host: icon: open docs/icons/none.svg",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fsys := testRepoFS()
			fsys["docs/README.md"] = &fstest.MapFile{
				Data: []byte(strings.Replace(testRoot,
					"suggested: true\n", "suggested: true\n"+tt.host, 1)),
			}
			_, err := read(fsys, "docs", noRemote, false)
			require.ErrorContains(t, err, tt.want)
		})
	}
}
