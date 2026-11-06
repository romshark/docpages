package docpages

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"strings"
)

// host is where the repository is hosted: the name and the icon of the links
// to it, and the URLs of its files.
type host struct {
	repoURL string // The URL of the repository, which {repository} stands for.
	name    string // The name of the links to the repository, as "GitHub".
	icon    string // SVG markup of the icon of the links, empty for none.
	// github marks the default host, whose links show the mark of GitHub.
	github bool
	// file and dir are the URLs of a file and a directory of the repository,
	// in which {repository} stands for repoURL, {ref} for the Git ref and
	// {path} for the path from the repository root.
	file, dir string
}

// hostSettings is the host setting of the front matter, which describes a
// host other than GitHub. Icon is the path of an SVG file relative to the
// README.md that declares it, and Dir defaults to File.
type hostSettings struct {
	Name string `yaml:"name"`
	Icon string `yaml:"icon"`
	File string `yaml:"file"`
	Dir  string `yaml:"dir"`
}

// githubHost returns the host of the repository at repoURL on GitHub, which
// is the host without a host setting.
func githubHost(repoURL string) *host {
	return &host{
		repoURL: repoURL,
		name:    "GitHub",
		github:  true,
		file:    "{repository}/blob/{ref}/{path}",
		dir:     "{repository}/tree/{ref}/{path}/",
	}
}

// readHost returns the host of the repository at repoURL that set describes,
// [githubHost] without set. An icon is a file of the docs directory dir of fsys.
func readHost(fsys fs.FS, dir, repoURL string, set *hostSettings) (*host, error) {
	if set == nil {
		return githubHost(repoURL), nil
	}
	if set.Name == "" || set.File == "" {
		return nil, errors.New("a host has a name and a file URL")
	}
	h := &host{
		repoURL: repoURL,
		name:    set.Name,
		file:    set.File,
		dir:     cmp.Or(set.Dir, set.File),
	}
	if set.Icon != "" {
		svg, err := fs.ReadFile(fsys, path.Join(dir, set.Icon))
		if err != nil {
			return nil, fmt.Errorf("icon: %w", err)
		}
		h.icon = inlineSVG(svg, `aria-hidden="true"`)
	}
	return h, nil
}

// url returns the URL of file, a path from the repository root, at the Git ref,
// of a directory if dir. {path} takes file escaped, because a Markdown copy and
// a snapshot take the URL as it is, and a Markdown link ends at a space.
func (h *host) url(ref, file string, dir bool) string {
	t := h.file
	if dir {
		t = h.dir
	}
	return strings.NewReplacer(
		"{repository}", h.repoURL, "{ref}", ref,
		"{path}", (&url.URL{Path: file}).EscapedPath(),
	).Replace(t)
}
