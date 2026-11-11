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

// host describes a repository host: link name, icon, and file URLs.
type host struct {
	repoURL string // The URL of the repository, which {repository} stands for.
	name    string // The name of the links to the repository, as "GitHub".
	icon    string // SVG markup of the icon of the links, empty for none.
	// github marks the default host, whose links show the mark of GitHub.
	github bool
	// file and dir are repository URL templates: {repository} is repoURL,
	// {ref} the Git ref, and {path} the repository-relative path.
	file, dir string
}

// hostSettings configures a non-GitHub host.
// Icon is an SVG path relative to the declaring README.md; Dir defaults to File.
type hostSettings struct {
	Name string `yaml:"name"`
	Icon string `yaml:"icon"`
	File string `yaml:"file"`
	Dir  string `yaml:"dir"`
}

// githubHost returns repoURL's default, unconfigured GitHub host.
func githubHost(repoURL string) *host {
	return &host{
		repoURL: repoURL,
		name:    "GitHub",
		github:  true,
		file:    "{repository}/blob/{ref}/{path}",
		dir:     "{repository}/tree/{ref}/{path}/",
	}
}

// readHost returns set's host for repoURL, or [githubHost] without set.
// Icons come from docs directory dir in fsys.
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

// url returns repository-relative file's URL at ref, using the directory
// template when dir. {path} uses escaped file because Markdown copies
// preserve it, and spaces end Markdown links.
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
