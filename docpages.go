// Package docpages renders a Git repository's docs directory as a static site,
// with versions for main and each release. File links work as they do on GitHub.
//
// The layout of the docs directory makes the navigation:
//
//	README.md              the first page
//	logo.svg               the logo of the header, optional
//	favicon.svg            the icon of the browser tab, optional
//	style.css              loaded after the stylesheet of docpages, optional
//	template.templ         an optional page template, like any .templ file
//	go.mod                 the Go module of the templates, which requires docpages
//	1-get-started/         a group, "Get started"
//	  icon.svg             the icon of the group, optional
//	  README.md            the first page of the group, optional
//	  1-installation.md    a page, at get-started/installation
//	  2-upgrading.md
//	2-tutorial/
//	versions/              fixes of releases, optional
//	  v1.2/                files in place of those at the tag of v1.2.x
//
// Numbers order groups and pages but are omitted from URLs and titles.
// Page front matter can set a title, suggest the page in search, show an external
// file with source, or create an external link with url. The docs README front
// matter also holds site settings; the module README lists its keys.
//
// Docs .templ files use package docs and replace [DefaultPage] with templ
// DocpagesMain(p [Page]). The go command builds them in the Go module of the
// docs directory, with the running docpages, which must be compatible with the
// version that the module requires. See [VersionError].
//
// The docs directory describes main. The releases setting, as from: v1.2,
// makes the Git tag of the latest patch of each feature version from v1.2 on
// a release, read from the docs directory at that tag.
package docpages

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"maps"
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
	// NoOffline omits sw.js, offline.json and [Offline].
	NoOffline bool
}

// Site contains the docs for main and every release.
type Site struct {
	// editions are the releases, the latest first, followed by main.
	editions []*edition
	chrome   chrome
	// assets maps edition-relative paths to files each edition loads locally.
	assets map[string][]byte
	// served holds repository files that [Handler] serves instead of the host;
	// nil outside it. See [site].
	served map[string]bool
	// templates are the docs .templ files.
	templates []templateFile
	// module is the Go module of the docs directory, nil without templates.
	module *docsModule
	// page renders each page. Nil uses the renderer built from templates.
	page func(Page) templ.Component
	// templateGlob names the active template files, such as "docs/*.templ";
	// empty when their renderer is inactive.
	templateGlob string
	// options are shared by [readRepo] and the template renderer.
	options Options
}

// chrome is what every page of the site shows around its text.
type chrome struct {
	name    string
	logo    string // SVG markup of logo.svg, empty without one.
	home    string // URL that the logo leads to, relative to the output directory.
	host    *host
	favicon string // Name of the icon of the browser tab, empty without one.
	style   bool   // Whether the docs directory holds a style.css.
	// reload is [Handler]'s state for displayed files. Pages reload when it changes.
	// Empty outside [Handler].
	reload string
}

// faviconNames lists docs-directory tab icons in lookup order.
var faviconNames = []string{"favicon.svg", "favicon.png", "favicon.ico"}

// Read reads the documentation of o.Dir in o.Root and converts it.
// Releases need the git command and the tags of the repository.
func Read(o Options) (*Site, error) {
	return readOptions(o, false)
}

// readOptions is [Read], but served mode makes non-page repository links use
// [Handler]-served files. Any docs .templ files render pages and require go,
// and a docs module that requires a compatible docpages.
func readOptions(o Options, served bool) (*Site, error) {
	s, err := readRepo(o, served)
	if err != nil {
		return nil, err
	}
	if len(s.templates) > 0 {
		if err := checkVersion(s.module); err != nil {
			return nil, err
		}
		if _, err := exec.LookPath("go"); err != nil {
			return nil, fmt.Errorf("%s: the .templ files need the go command: %w",
				s.main().dir, err)
		}
		s.page = nil
	}
	return s, nil
}

// readRepo reads o's docs from disk like [readOptions] but uses [DefaultPage].
func readRepo(o Options, served bool) (*Site, error) {
	s, err := read(
		os.DirFS(o.Root),
		path.Clean(filepath.ToSlash(o.Dir)),
		func() (string, error) { return gitRemote(o.Root) },
		gitTags{root: o.Root},
		served,
	)
	if err != nil {
		return nil, err
	}
	s.options = o
	return s, nil
}

// read reads docs directory dir from repository fsys. remote returns origin's URL,
// overridden by [frontMatter].Repository. tags holds the releases that
// [frontMatter].Releases selects; nil means a repository without tags.
// When served, [Handler] serves non-page repository links.
func read(
	fsys fs.FS, dir string, remote func() (string, error), tags tagStore, served bool,
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
	c := chrome{name: set.Name, home: set.Home}
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
	templates, err := readTemplates(fsys, dir)
	if err != nil {
		return nil, err
	}
	var mod *docsModule
	if len(templates) > 0 {
		if mod, err = readModule(fsys, dir); err != nil {
			return nil, err
		}
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
		c.style, assets["style.css"] = true, min
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	s := &Site{
		chrome: c, assets: assets, templates: templates, module: mod, page: DefaultPage,
	}
	if served {
		s.served = make(map[string]bool)
	}
	var rels []release
	if set.Releases != nil {
		var names []string
		if tags != nil {
			if names, err = tags.tags(); err != nil {
				return nil, err
			}
		}
		if rels, err = pickReleases(names, *set.Releases); err != nil {
			return nil, fmt.Errorf("%s: %w", path.Join(dir, readmeName), err)
		}
	}
	if err := checkFolders(fsys, dir, rels, set.Releases); err != nil {
		return nil, err
	}
	for _, r := range rels {
		e, err := readRelease(fsys, dir, r, tags, c.host)
		if err != nil {
			return nil, fmt.Errorf("release %s: %w", r.tag, err)
		}
		s.editions = append(s.editions, e)
	}
	main, err := readMain(fsys, dir, c.host, s.served)
	if err != nil {
		return nil, err
	}
	s.editions = append(s.editions, main)
	// Rendering validates a note and its links, including links to other editions.
	for _, e := range s.editions {
		if _, err := e.noteHTML(0, s.editions); err != nil {
			return nil, fmt.Errorf("%s: note: %w", e.noteFile, err)
		}
	}
	// Pages, links, and settings have now named every file the site uses.
	settings := settingFiles(dir, c, set.Host, templates, mod)
	if err := unusedFiles(fsys, s, settings); err != nil {
		return nil, err
	}
	return s, nil
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

// Files returns the site's files by output-relative path, a directory per version,
// as main/ or v1.2/, plus index.html and llms.txt,
// which lead to the latest release or main.
func (s *Site) Files() (map[string][]byte, error) {
	var index bytes.Buffer
	err := docsIndex(s.defaultEdition().outDir(), &s.chrome).
		Render(context.Background(), &index)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{
		"index.html": index.Bytes(),
		"llms.txt":   versionsLlmsTxt(s.editions, s.chrome.name, s.Description()),
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
			if !p.external() {
				files[dir+p.markdownFile()] = e.docs[i].markdown
			}
		}
		search, err := searchJSON(e.docs)
		if err != nil {
			return nil, err
		}
		files[dir+"search.json"] = search
	}
	pages, err := s.renderPages()
	if err != nil {
		return nil, err
	}
	maps.Copy(files, pages)
	if !s.options.NoOffline {
		if err := s.addOfflineFiles(files); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// renderPages returns every edition's HTML by output path.
// If [Site.page] is nil, the .templ renderer handles them.
func (s *Site) renderPages() (map[string][]byte, error) {
	if s.page == nil {
		return s.renderTemplates()
	}
	pages := make(map[string][]byte)
	for _, e := range s.editions {
		for i, p := range e.pages {
			if p.external() {
				continue
			}
			name := e.outDir() + p.outFile()
			var b bytes.Buffer
			err := s.page(s.pageView(e, i)).Render(context.Background(), &b)
			if err != nil {
				return nil, s.renderError(name, err)
			}
			pages[name] = b.Bytes()
		}
	}
	return pages, nil
}

// renderError wraps err for output path name. For docs templates, it names the
// files and gives an expression's ending file, line, and column.
func (s *Site) renderError(name string, err error) error {
	if s.templateGlob == "" {
		return fmt.Errorf("%s: %w", name, err)
	}
	// templ uses 1-based lines and 0-based columns.
	var te templ.Error
	if errors.As(err, &te) && strings.HasSuffix(te.FileName, ".templ") {
		return fmt.Errorf("%s:%d:%d: %w", te.FileName, te.Line, te.Col+1, te.Err)
	}
	return fmt.Errorf("%s: %w", s.templateGlob, err)
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

// writeFiles writes files by slash-separated,
// dir-relative path, creating needed directories.
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

// pageView returns page i of e as a [Page].
func (s *Site) pageView(e *edition, i int) Page {
	p, d := e.pages[i], e.docs[i]
	base := p.base()
	prev, next := neighbors(e.pages, i)
	v := Page{
		Title:       p.title,
		Heading:     d.title,
		Description: d.description,
		Body:        d.body,
		Site:        s.chrome.name,
		Version:     e.version,
		Root:        base,
		Home:        base,
		Logo:        s.chrome.logo,
		Markdown:    base + p.markdownFile(),
		Source:      e.links.sourceURL(p.file),
		Repository:  s.chrome.host.repoURL,
		Host:        s.chrome.host.name,
		Navigation:  navigation(e.pages, e.groups, i),
		Contents:    contents(d),
		Prev:        prev,
		Next:        next,
		Suggested:   suggestions(e.pages, e.groups, i),
		Indexed:     !e.isMain() || s.latest() == nil,
		site:        s,
		edition:     e,
		index:       i,
	}
	switch h := s.chrome.home; {
	case isAbsURL(h):
		v.Home = h
	case h != "":
		v.Home = base + "../" + h
	}
	for _, x := range s.editions {
		v.Versions = append(v.Versions, x.versionLink(base, p.path, x == e))
	}
	// read checked the note, which leaves no error here.
	v.Note, _ = e.noteHTML(i, s.editions)
	return v
}

// chromeLink returns dest's URL from page i of e and the target page index in e, or -1.
// dest is a template link resolved from main's first page.
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
