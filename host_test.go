package docpages

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// forgeHost replaces [testRoot]'s repository with non-GitHub forge settings.
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

// TestHost checks that host settings name and icon repository links, and build
// file URLs at main on the pages of main and at the tag on those of a release.
func TestHost(t *testing.T) {
	const (
		repo = "https://git.example.com/o/demo"
		icon = `<svg aria-hidden="true" viewBox="0 0 16 16"><path d="M3 3"/></svg>`
	)
	s, err := read(forgeRepoFS(), "docs", noRemote, nil, false)
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

	fsys := forgeRepoFS()
	fsys["docs/README.md"].Data = []byte(strings.Replace(
		string(fsys["docs/README.md"].Data),
		"suggested: true\n", "suggested: true\nreleases:\n  from: v1.2\n", 1,
	))
	s, err = read(fsys, "docs", noRemote, testTags{"v1.2.3": forgeRepoFS()}, false)
	require.NoError(t, err)
	html = editionPageHTML(t, s, "v1.2", "guides/events")
	require.Contains(t, html, `<a href="`+repo+`/src/v1.2.3/main.go">main.go</a>`)
	require.Contains(t, html,
		`<a class="icon-link" href="`+repo+`/src/v1.2.3/docs/2-guides/01-events.md">`)
}

// TestHostURL checks repository file and directory URLs, with directories using
// the file template when dir is unset.
func TestHostURL(t *testing.T) {
	h := githubHost("https://github.com/o/r")
	require.Equal(t,
		"https://github.com/o/r/blob/main/docs/a.md",
		h.url("main", "docs/a.md", false))
	require.Equal(t,
		"https://github.com/o/r/tree/v1.2.3/cmd/",
		h.url("v1.2.3", "cmd", true))
	require.Equal(t,
		"https://github.com/o/r/blob/main/docs/Design%20Doc%231.pdf",
		h.url("main", "docs/Design Doc#1.pdf", false))

	h, err := readHost(nil, "docs", "https://git.example.com/o/r",
		&hostSettings{Name: "Forge", File: "{repository}/-/blob/{ref}/{path}"})
	require.NoError(t, err)
	require.Equal(t,
		"https://git.example.com/o/r/-/blob/main/cmd",
		h.url("main", "cmd", true))
	require.Empty(t, h.icon)
	require.False(t, h.github)
}

// TestHostErrors checks required host names, file URLs, and existing icons.
func TestHostErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		host, want string
	}{
		"no name": {
			host: dedent(`
				host:
				  file: "{repository}/{path}"
			`),
			want: "docs/README.md: host: a host has a name and a file URL",
		},
		"no file": {
			host: dedent(`
				host:
				  name: Forge
			`),
			want: "docs/README.md: host: a host has a name and a file URL",
		},
		"missing icon": {
			host: dedent(`
				host:
				  name: Forge
				  file: "{repository}/{path}"
				  icon: icons/none.svg
			`),
			want: "docs/README.md: host: icon: open docs/icons/none.svg",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fsys := testRepoFS()
			fsys["docs/README.md"] = &fstest.MapFile{
				Data: []byte(strings.Replace(testRoot,
					"suggested: true\n", "suggested: true\n"+tt.host, 1)),
			}
			_, err := read(fsys, "docs", noRemote, nil, false)
			require.ErrorContains(t, err, tt.want)
		})
	}
}
