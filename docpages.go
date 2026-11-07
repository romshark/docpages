// Package docpages renders a Git repository's docs directory as a static site,
// with versions for main and each release. File links work as they do on GitHub.
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
// Numbers order groups and pages but are omitted from URLs and titles. Page
// front matter can set a title, suggest the page in search, show an external
// file with source, or create an external link with url. The docs README front
// matter also holds site settings; the module README lists its keys.
//
// The docs directory describes main. [Snapshot] makes an editable release copy
// under versions/.
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

// Site contains the docs for main and every release in the versions directory.
type Site struct {
	// editions are the releases, the latest first, followed by main.
	editions []*edition
	chrome   chrome
	// assets maps edition-relative paths to files each edition loads locally.
	assets map[string][]byte
	// served holds repository files that [Handler] serves instead of the host;
	// nil outside it. See [site].
	served map[string]bool
}

// chrome is what every page of the site shows around its text.
type chrome struct {
	name   string
	logo   string // SVG markup of logo.svg, empty without one.
	home   string // URL that the logo leads to, relative to the output directory.
	host   *host
	header []frontMatterLink
	// footer links sit between the repository and llms.txt;
	// each Icon contains its file's SVG markup.
	footer  []frontMatterLink
	license string // Last line of the footer, empty for none.
	favicon string // Name of the icon of the browser tab, empty without one.
	style   bool   // Whether the docs directory holds a style.css.
	// reload is [Handler]'s state for displayed files. Pages reload when it changes.
	// Empty outside [Handler].
	reload string
}

// faviconNames lists docs-directory tab icons in lookup order.
var faviconNames = []string{"favicon.svg", "favicon.png", "favicon.ico"}

// Read reads the documentation of o.Dir in o.Root and converts it.
func Read(o Options) (*Site, error) {
	return readOptions(o, false)
}

// readOptions is [Read], but when served, non-page repository links use files
// served by [Handler].
func readOptions(o Options, served bool) (*Site, error) {
	return read(os.DirFS(o.Root), path.Clean(filepath.ToSlash(o.Dir)),
		func() (string, error) { return gitRemote(o.Root) }, served)
}

// read reads docs directory dir from repository fsys. remote returns origin's URL,
// overridden by [frontMatter].Repository. When served,
// [Handler] serves non-page repository links.
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
		// [Handler] serves both stylesheets unminified for easier browser reading.
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
	// Rendering validates a note and its links, including links to other editions.
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
	// Pages, links, and settings have now named every file the site uses.
	settings := settingFiles(dir, c, slices.Concat(header, set.Footer), set.Host)
	if err := unusedFiles(fsys, s, settings); err != nil {
		return nil, err
	}
	return s, nil
}

// defaultHeader links the first page when the docs README sets no header.
var defaultHeader = []frontMatterLink{{Title: "Docs", URL: readmeName}}

// withIcons returns header or footer links, as part names them, with each Icon
// file from docs directory dir in fsys replaced by its SVG markup.
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

// defaultEdition returns the edition opened by output index.html:
// the latest release, or main when there are no releases.
func (s *Site) defaultEdition() *edition { return s.editions[0] }

// Description returns the text of the first paragraph of the first page of main.
func (s *Site) Description() string { return s.main().docs[0].description }

// Files returns site files by output-relative path. Each edition directory,
// main/ or a release such as v1.2/, contains its pages, Markdown copies,
// search.json, llms.txt, llms-full.txt, and page assets. Root index.html and
// llms.txt lead to the latest release, or main when there are none.
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

// Write writes the site to out. It first removes prior generated files,
// including edition directories, so deleted docs leave no stale pages.
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

// writeFiles writes files by slash-separated, dir-relative path, creating
// needed directories.
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
	// A first-page link represents all docs, so it is current when no other
	// header link reaches the page.
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

// chromeLink returns dest's URL from page i of e and the target page index in e, or -1.
// dest comes from main's first-page front matter.
// A main-page link uses the same path in e, falling back to main if e lacks it.
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
