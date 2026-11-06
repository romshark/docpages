// Package docpages renders the Markdown files of a docs directory of a Git
// repository as a static documentation site with a version for main and a
// version for each release. Links between the files work on the site as they
// do on GitHub.
//
// The layout of the docs directory makes the navigation:
//
//	README.md              the first page
//	logo.svg               the logo of the header, optional
//	favicon.svg            the icon of the browser tab, optional
//	style.css              loaded after the stylesheet of docpages, optional
//	1-get-started/         a group, "Get started"
//	  icon.svg             the icon of the group, optional
//	  README.md            the first page of the group, optional
//	  1-installation.md    a page, at get-started/installation
//	  2-upgrading.md
//	2-tutorial/
//	versions/              the docs of the releases, optional
//	  v1.2/                a docs directory of the same layout, for v1.2.x
//
// The numbers order the groups and the pages, and the site leaves them out of
// URLs and titles. The front matter of a page sets its title, marks it as
// suggested by the search, shows a file outside the docs directory with source,
// or makes it a link that leaves the site with url. The front matter of the
// README.md of the docs directory also holds the settings of the site. The
// README of the module lists the keys.
//
// The docs directory documents main. [Snapshot] copies it into versions/ for
// a release, where its pages stay editable.
package docpages

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/a-h/templ"
)

// Options select the documentation that [Read] reads.
type Options struct {
	// Root is the root directory of the repository.
	Root string
	// Dir is the docs directory, relative to Root, as "guide" or ".".
	Dir string
}

// Site is the documentation of main and of every release in the versions
// directory of the docs directory.
type Site struct {
	// editions are the releases, the latest first, followed by main.
	editions []*edition
	chrome   chrome
	// assets are the files that the pages of each edition load from its own
	// directory, by their path relative to it.
	assets map[string][]byte
	// served are the files of the repository that [Handler] serves in place
	// of the host, nil outside it. See [site].
	served map[string]bool
}

// chrome is what every page of the site shows around its text.
type chrome struct {
	name   string
	logo   string // SVG markup of logo.svg, empty without one.
	home   string // URL that the logo leads to, relative to the output directory.
	host   *host
	header []frontMatterLink
	// footer are the links of the footer between the repository and llms.txt,
	// their Icon replaced by the SVG markup of its file.
	footer  []frontMatterLink
	license string // Last line of the footer, empty for none.
	favicon string // Name of the icon of the browser tab, empty without one.
	style   bool   // Whether the docs directory holds a style.css.
	// reload is the state of the files that the pages show, as [Handler]
	// reports it. The pages reload when the state changes. Empty outside [Handler].
	reload string
}

// faviconNames are the names of the icon of the browser tab in the docs
// directory, in the order in which they are looked up.
var faviconNames = []string{"favicon.svg", "favicon.png", "favicon.ico"}

// Read reads the documentation of o.Dir in o.Root and converts it.
func Read(o Options) (*Site, error) {
	return readOptions(o, false)
}

// readOptions is [Read], whose links to the files of the repository that no
// page shows lead to the files that [Handler] serves if served.
func readOptions(o Options, served bool) (*Site, error) {
	return read(os.DirFS(o.Root), path.Clean(filepath.ToSlash(o.Dir)),
		func() (string, error) { return gitRemote(o.Root) }, served)
}

// read reads the documentation of the docs directory dir of the repository fsys.
// remote returns the URL of the Git remote origin, which [frontMatter]
// Repository replaces. If served, the links to the files of the repository that
// no page shows lead to the files that [Handler] serves.
func read(
	fsys fs.FS, dir string, remote func() (string, error), served bool,
) (*Site, error) {
	root, err := readPage(fsys, path.Join(dir, readmeName), "", -1)
	if err != nil {
		return nil, err
	}
	set := root.fm
	repoURL := set.Repository
	if repoURL == "" {
		r, err := remote()
		if err != nil {
			return nil, fmt.Errorf("repository of %s: %w "+
				"(set repository in the front matter of %s)",
				dir, err, path.Join(dir, readmeName))
		}
		if repoURL, err = httpsRepoURL(r); err != nil {
			return nil, err
		}
	}
	repoURL = strings.TrimSuffix(repoURL, "/")
	c := chrome{
		name:    set.Name,
		home:    set.Home,
		license: strings.TrimSpace(set.License),
	}
	if c.name == "" {
		c.name = path.Base(repoURL)
	}
	if c.host, err = readHost(fsys, dir, repoURL, set.Host); err != nil {
		return nil, fmt.Errorf("%s: host: %w", path.Join(dir, readmeName), err)
	}
	if svg, err := fs.ReadFile(fsys, path.Join(dir, "logo.svg")); err == nil {
		c.logo = inlineSVG(svg, `class="logo-svg"`, `role="img"`,
			`aria-label="`+html.EscapeString(c.name)+`"`)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	header := set.Header
	if header == nil {
		header = defaultHeader
	}
	if c.header, err = withIcons(fsys, dir, "header", header); err != nil {
		return nil, err
	}
	if c.footer, err = withIcons(fsys, dir, "footer", set.Footer); err != nil {
		return nil, err
	}
	assets, err := siteAssets()
	if err != nil {
		return nil, err
	}
	for _, name := range faviconNames {
		data, err := fs.ReadFile(fsys, path.Join(dir, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		c.favicon, assets[name] = name, data
		break
	}
	if data, err := fs.ReadFile(fsys, path.Join(dir, "style.css")); err == nil {
		min, err := minifyCSS(path.Join(dir, "style.css"), data)
		if err != nil {
			return nil, err
		}
		// [Handler] serves the stylesheet as written, as it serves that of
		// docpages, which is easier to read in a browser.
		if !served {
			data = min
		}
		c.style, assets["style.css"] = true, data
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	s := &Site{chrome: c, assets: assets}
	if served {
		s.served = make(map[string]bool)
	}
	versions, err := releases(fsys, dir)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		e, err := readEdition(fsys, path.Join(dir, versionsDir, v), v, c.host, s.served)
		if err != nil {
			return nil, err
		}
		s.editions = append(s.editions, e)
	}
	main, err := readEdition(fsys, dir, MainVersion, c.host, s.served)
	if err != nil {
		return nil, err
	}
	s.editions = append(s.editions, main)
	// Rendering a note checks it and its links, which may lead to the pages
	// of the other editions.
	for _, e := range s.editions {
		if _, err := e.noteHTML(0, s.editions); err != nil {
			return nil, fmt.Errorf("%s: note: %w", path.Join(e.dir, readmeName), err)
		}
	}
	for _, l := range slices.Concat(c.header, c.footer) {
		if _, err := main.links.resolve(0, l.URL, "", page.urlPath); err != nil {
			return nil, fmt.Errorf("%s: link %q of the front matter: %w",
				path.Join(dir, readmeName), l.Title, err)
		}
	}
	// The pages, the links and the settings have named every file that the site uses.
	settings := settingFiles(dir, c, slices.Concat(header, set.Footer), set.Host)
	if err := unusedFiles(fsys, s, settings); err != nil {
		return nil, err
	}
	return s, nil
}

// defaultHeader is the header of a docs directory whose README.md sets none:
// a link to the first page.
var defaultHeader = []frontMatterLink{{Title: "Docs", URL: readmeName}}

// withIcons returns links, the links of the header or the footer as part
// names it, with the Icon of each, a file of the docs directory dir of fsys,
// replaced by its SVG markup.
func withIcons(
	fsys fs.FS, dir, part string, links []frontMatterLink,
) ([]frontMatterLink, error) {
	var r []frontMatterLink
	for _, l := range links {
		if l.Icon != "" {
			svg, err := fs.ReadFile(fsys, path.Join(dir, l.Icon))
			if err != nil {
				return nil, fmt.Errorf("icon of the %s link %q: %w", part, l.Title, err)
			}
			l.Icon = inlineSVG(svg, `aria-hidden="true"`)
		}
		r = append(r, l)
	}
	return r, nil
}

// main returns the edition of main.
func (s *Site) main() *edition { return s.editions[len(s.editions)-1] }

// latest returns the latest release, or nil without releases.
func (s *Site) latest() *edition {
	if len(s.editions) == 1 {
		return nil
	}
	return s.editions[0]
}

// defaultEdition returns the edition that the index.html of the output
// directory opens: the latest release, or main without releases.
func (s *Site) defaultEdition() *edition { return s.editions[0] }

// Description returns the text of the first paragraph of the first page of main.
func (s *Site) Description() string { return s.main().docs[0].description }

// Files returns the files of the site by their path relative to the output directory.
// The directory of each edition, main/ or that of a release, as v1.2/, holds its pages,
// their Markdown copies, search.json, llms.txt, llms-full.txt and the assets of
// the pages. index.html and llms.txt of the output directory lead to the latest release,
// or to main without releases.
func (s *Site) Files() (map[string][]byte, error) {
	pages := map[string]templ.Component{
		"index.html": docsIndex(s.defaultEdition().outDir(), &s.chrome),
	}
	files := map[string][]byte{
		"llms.txt": versionsLlmsTxt(s.editions, s.chrome.name, s.Description()),
	}
	for _, e := range s.editions {
		dir := e.outDir()
		files[dir+"llms.txt"] = llmsTxt(s.chrome.name, e.pages, e.groups, e.docs)
		files[dir+"llms-full.txt"] = llmsFullTxt(e.pages, e.docs)
		for name, content := range s.assets {
			files[dir+name] = content
		}
		for _, m := range e.links.media {
			files[dir+m.name] = m.data
		}
		for i, p := range e.pages {
			if p.external() {
				continue
			}
			pages[dir+p.outFile()] = docsPage(s.view(e, i))
			files[dir+p.markdownFile()] = e.docs[i].markdown
		}
		search, err := searchJSON(e.docs)
		if err != nil {
			return nil, err
		}
		files[dir+"search.json"] = search
	}
	for name, page := range pages {
		var b bytes.Buffer
		if err := page.Render(context.Background(), &b); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		files[name] = b.Bytes()
	}
	return files, nil
}

// Write writes the files of the site into the directory out. It first removes
// the files that a run writes, the directories of the editions among them,
// which leaves no page that the docs directory no longer has.
func (s *Site) Write(out string) error {
	files, err := s.Files()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(out)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		edition := e.IsDir() && (name == MainVersion || featureVersion.MatchString(name))
		if edition || !e.IsDir() && (name == "index.html" || name == "llms.txt") {
			if err := os.RemoveAll(filepath.Join(out, name)); err != nil {
				return err
			}
		}
	}
	return writeFiles(out, files)
}

// writeFiles writes files, by their slash-separated path relative to dir,
// into dir, creating the directories that they need.
func writeFiles(dir string, files map[string][]byte) error {
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// view returns the view of the page with index i of e.
func (s *Site) view(e *edition, i int) docView {
	p := e.pages[i]
	prev, next := neighbors(e.pages, i)
	v := docView{
		doc:       e.docs[i],
		chrome:    &s.chrome,
		nav:       navigation(e.pages, e.groups, i),
		toc:       contents(e.docs[i]),
		prev:      prev,
		next:      next,
		edition:   e,
		indexed:   !e.isMain() || s.latest() == nil,
		suggested: suggestions(e.pages, e.groups, i),
	}
	for _, x := range s.editions {
		v.versions = append(v.versions, x.versionLink(p.base(), p.path, x == e))
	}
	// read checked the note, which leaves no error here.
	v.note, _ = e.noteHTML(i, s.editions)
	// A link to the first page stands for the whole documentation: it's the
	// current one on every page that no other link of the header leads to.
	var first []int
	for _, l := range s.chrome.header {
		href, to := s.chromeLink(e, i, l.URL)
		if to == 0 {
			first = append(first, len(v.header))
		}
		v.header = append(v.header,
			headerEntry{Title: l.Title, Href: href, Icon: l.Icon, Current: to == i})
	}
	if !slices.ContainsFunc(v.header, func(h headerEntry) bool { return h.Current }) {
		for _, k := range first {
			v.header[k].Current = true
		}
	}
	for _, l := range s.chrome.footer {
		href, _ := s.chromeLink(e, i, l.URL)
		v.footer = append(v.footer,
			footerEntry{Title: l.Title, Href: href, Icon: l.Icon})
	}
	return v
}

// chromeLink returns the URL on the page with index i of e of dest, a link of
// the front matter of the first page of main, and the index of the page of e
// that it leads to, or -1. A link to a page of main leads to the page at the
// same path in e, or to the page of main if e lacks it.
func (s *Site) chromeLink(e *edition, i int, dest string) (string, int) {
	main, base := s.main(), e.pages[i].base()
	to := main.links.target(0, dest)
	if to < 0 {
		// read checked the links, which leaves no error here.
		href, _ := main.links.resolve(0, dest, base, page.urlPath)
		return href, -1
	}
	p := main.pages[to].path
	href := base + p
	j := slices.IndexFunc(e.pages,
		func(q page) bool { return !q.external() && q.path == p })
	if j < 0 {
		href = base + "../" + main.outDir() + p
	}
	if u, err := url.Parse(dest); err == nil && u.Fragment != "" {
		href += "#" + u.Fragment
	}
	return href, j
}

// gitRemote returns the URL of the Git remote origin of the repository in root.
func gitRemote(root string) (string, error) {
	out, err := exec.Command("git", "-C", root, "remote", "get-url", "origin").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return "", fmt.Errorf("git remote get-url origin: %s",
				strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GitRoot returns the root directory of the Git repository that holds dir.
func GitRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("root of the Git repository of %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}
