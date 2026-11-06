package docpages

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// frontMatter is the YAML block that may open a Markdown file of the docs
// directory, between 2 lines of "---". GitHub shows it as a table above the text.
type frontMatter struct {
	// Title replaces the level 1 heading in the navigation and in <title>.
	Title string `yaml:"title"`
	// Suggested marks a page that the search offers while its field is empty.
	Suggested bool `yaml:"suggested"`
	// Source is the Markdown file that the page shows in place of its own text,
	// relative to the file. It includes a file outside the docs directory.
	Source string `yaml:"source"`
	// URL makes the entry a link that leaves the site.
	URL string `yaml:"url"`

	// Note is the note above every page of the version, a paragraph of
	// Markdown. Only the README.md of a docs directory, that of main or of a
	// release, takes it. Without it, the pages show no note.
	Note string `yaml:"note"`

	// The settings of the site, which only the README.md of the docs
	// directory takes.

	// Name is the name of the project, which <title> and llms.txt show.
	// The default is the last element of the repository URL.
	Name string `yaml:"name"`
	// Home is the URL that the logo leads to, relative to the output directory.
	// The default is the first page of the version.
	Home string `yaml:"home"`
	// Repository is the URL of the repository. The default is the URL of the
	// Git remote origin.
	Repository string `yaml:"repository"`
	// Host describes where the repository is hosted, GitHub without it.
	Host *hostSettings `yaml:"host"`
	// Header are the links of the header before the repository. Without it the
	// header links the first page as "Docs". An empty list leaves the header
	// without links.
	Header []frontMatterLink `yaml:"header"`
	// Footer are the links of the footer between the repository and llms.txt.
	Footer []frontMatterLink `yaml:"footer"`
	// License is the line that ends the footer, which has none without it.
	License string `yaml:"license"`
}

// frontMatterLink is a link of the header or the footer. URL is resolved as a
// link of the README.md that declares it. Icon is the path of an SVG file
// relative to that README.md, which the link shows before its title.
type frontMatterLink struct {
	Title string `yaml:"title"`
	URL   string `yaml:"url"`
	Icon  string `yaml:"icon"`
}

// readmeName is the file that GitHub shows for a directory. The README.md of the
// docs directory is the first page, and that of a group opens the group.
const readmeName = "README.md"

// orderPrefix matches the name of a group directory or a page file, which
// starts with its position, as 1-get-started or 01-pages.md. The site leaves
// the prefix out of URLs and titles.
var orderPrefix = regexp.MustCompile(`^([0-9]+)-(.+)$`)

// layout is what the docs directory holds, before the Markdown is converted.
type layout struct {
	pages  []page
	groups []group
	// srcs[i] is the Markdown of pages[i] without its front matter,
	// nil for a link that leaves the site.
	srcs     [][]byte
	settings frontMatter
}

// group is a group of the navigation, a directory of the docs directory.
type group struct {
	title string
	icon  string // SVG of icon.svg in the directory, empty without it.
}

// readLayout reads the pages of the docs directory dir of fsys, the repository.
//
// The README.md of dir is the first page. Each directory of dir whose name
// starts with a number and "-" is a group, which the navigation lists in the
// order of the numbers. Its README.md opens it, followed by its Markdown files
// whose names start with a number and "-", in the order of the numbers.
// A Markdown file or a directory with Markdown files outside this pattern is an
// error, since the navigation would leave it out.
func readLayout(fsys fs.FS, dir string) (layout, error) {
	var l layout
	root, err := readPage(fsys, path.Join(dir, readmeName), "", -1)
	if err != nil {
		return layout{}, err
	}
	l.settings = root.fm
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return layout{}, err
	}
	dirs, err := ordered(entries, dir, true)
	if err != nil {
		return layout{}, err
	}
	if len(dirs) == 0 {
		return layout{}, fmt.Errorf("%s has no group directory, such as 1-get-started", dir)
	}
	l.add(root)
	for _, d := range dirs {
		gdir := path.Join(dir, d.name)
		g := group{title: groupTitle(d.slug)}
		if svg, err := fs.ReadFile(fsys, path.Join(gdir, "icon.svg")); err == nil {
			g.icon = inlineSVG(svg, `aria-hidden="true"`)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return layout{}, err
		}
		l.groups = append(l.groups, g)
		gi := len(l.groups) - 1
		// The README.md of the docs directory opens the first group.
		if gi == 0 {
			l.pages[0].group = 0
		}
		readme, err := readPage(fsys, path.Join(gdir, readmeName), d.slug+"/", gi)
		switch {
		case err == nil:
			l.add(readme)
		case !errors.Is(err, fs.ErrNotExist):
			return layout{}, err
		}
		entries, err := fs.ReadDir(fsys, gdir)
		if err != nil {
			return layout{}, err
		}
		if err := noNestedGroups(entries, gdir); err != nil {
			return layout{}, err
		}
		files, err := ordered(entries, gdir, false)
		if err != nil {
			return layout{}, err
		}
		for _, f := range files {
			p, err := readPage(fsys, path.Join(gdir, f.name),
				d.slug+"/"+strings.TrimSuffix(f.slug, ".md"), gi)
			if err != nil {
				return layout{}, err
			}
			l.add(p)
		}
		if len(l.pages) == 1 || l.pages[len(l.pages)-1].group != gi {
			return layout{}, fmt.Errorf("group %s has no page", gdir)
		}
	}
	return l, nil
}

// readPageResult is a page that [readPage] read, with its Markdown and its
// front matter.
type readPageResult struct {
	page page
	src  []byte
	fm   frontMatter
}

// readPage reads the file of the page with the URL path urlPath in group gi.
// A page whose front matter has a source reads that file instead.
func readPage(fsys fs.FS, file, urlPath string, gi int) (readPageResult, error) {
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return readPageResult{}, err
	}
	fm, src, err := parseFrontMatter(data)
	if err != nil {
		return readPageResult{}, fmt.Errorf("%s: %w", file, err)
	}
	// The URL path of the first page alone is empty.
	if urlPath != "" && fm.hasSettings() {
		return readPageResult{}, fmt.Errorf(
			"%s: only the README.md of the docs directory takes site settings", file)
	}
	if urlPath != "" && fm.Note != "" {
		return readPageResult{}, fmt.Errorf(
			"%s: only the README.md of a docs directory takes a note", file)
	}
	p := page{
		file:      file,
		path:      urlPath,
		title:     fm.Title,
		group:     gi,
		suggested: fm.Suggested,
	}
	switch {
	case fm.URL != "" && fm.Source != "":
		return readPageResult{}, fmt.Errorf("%s: url and source exclude each other", file)
	case fm.URL != "":
		if fm.Title == "" {
			return readPageResult{}, fmt.Errorf("%s: a page with url needs a title", file)
		}
		p.url = fm.URL
		return readPageResult{page: p, fm: fm}, nil
	case fm.Source != "":
		p.stub = file
		p.file = repoPath(file, fm.Source)
		if !fs.ValidPath(p.file) {
			return readPageResult{}, fmt.Errorf("%s: source %q is outside the repository",
				file, fm.Source)
		}
		data, err := fs.ReadFile(fsys, p.file)
		if err != nil {
			return readPageResult{}, fmt.Errorf("%s: source: %w", file, err)
		}
		// The front matter of the source is its own, which the page does not take.
		if _, src, err = parseFrontMatter(data); err != nil {
			return readPageResult{}, fmt.Errorf("%s: %w", p.file, err)
		}
	}
	return readPageResult{page: p, src: src, fm: fm}, nil
}

// add appends the page of r.
func (l *layout) add(r readPageResult) {
	l.pages = append(l.pages, r.page)
	l.srcs = append(l.srcs, r.src)
}

// hasSettings reports whether fm sets a setting of the site.
func (fm frontMatter) hasSettings() bool {
	return fm.Name != "" || fm.Home != "" || fm.Repository != "" || fm.Host != nil ||
		fm.Header != nil || fm.Footer != nil || fm.License != ""
}

// orderedEntry is a group directory or a page file with its position.
type orderedEntry struct {
	n    int
	name string // Name in the directory.
	slug string // Name without the prefix.
}

// ordered returns the group directories of entries if dirs, or else the
// page files, in the order of their prefixes. dir is the directory of entries.
// A Markdown file without a prefix other than README.md is an error, as is a
// prefix that 2 entries share.
func ordered(entries []fs.DirEntry, dir string, dirs bool) ([]orderedEntry, error) {
	var r []orderedEntry
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() != dirs || strings.HasPrefix(name, ".") {
			continue
		}
		if !dirs && (path.Ext(name) != ".md" || name == readmeName) {
			continue
		}
		m := orderPrefix.FindStringSubmatch(name)
		if m == nil {
			if !dirs {
				return nil, fmt.Errorf(
					"%s: the name of a page starts with its position, as 1-%s",
					path.Join(dir, name), name,
				)
			}
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path.Join(dir, name), err)
		}
		r = append(r, orderedEntry{n: n, name: name, slug: m[2]})
	}
	slices.SortFunc(r, func(a, b orderedEntry) int { return a.n - b.n })
	for i := 1; i < len(r); i++ {
		if r[i].n == r[i-1].n {
			return nil, fmt.Errorf("%s and %s share the position %d",
				path.Join(dir, r[i-1].name), path.Join(dir, r[i].name), r[i].n)
		}
	}
	return r, nil
}

// noNestedGroups reports an error if entries, those of the group directory dir,
// hold a directory with a position, which would be a group in a group.
func noNestedGroups(entries []fs.DirEntry, dir string) error {
	for _, e := range entries {
		if e.IsDir() && orderPrefix.MatchString(e.Name()) {
			return fmt.Errorf("%s: a group holds no groups", path.Join(dir, e.Name()))
		}
	}
	return nil
}

// groupTitle returns the title of the group whose directory has the name slug
// after its prefix, as "Get started" for get-started.
func groupTitle(slug string) string {
	t := strings.ReplaceAll(slug, "-", " ")
	r, n := utf8.DecodeRuneInString(t)
	return string(unicode.ToTitle(r)) + t[n:]
}

// frontMatterDelim is the line that opens and closes front matter.
const frontMatterDelim = "---"

// parseFrontMatter returns the front matter of data and the Markdown after it.
// data without front matter is all Markdown.
func parseFrontMatter(data []byte) (frontMatter, []byte, error) {
	var fm frontMatter
	first, rest, ok := cutLine(data)
	if !ok || first != frontMatterDelim {
		return fm, data, nil
	}
	start := rest
	var block []byte
	for {
		line, after, ok := cutLine(rest)
		if !ok {
			return fm, nil, errors.New("front matter has no closing ---")
		}
		if line == frontMatterDelim {
			block = start[:len(start)-len(rest)]
			rest = after
			break
		}
		rest = after
	}
	dec := yaml.NewDecoder(bytes.NewReader(block))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil && !errors.Is(err, io.EOF) {
		return fm, nil, fmt.Errorf("front matter: %w", err)
	}
	return fm, rest, nil
}

// cutLine returns the first line of data without its line ending and the data
// after it. The last line may lack a line ending. ok is false for empty data.
func cutLine(data []byte) (line string, rest []byte, ok bool) {
	if len(data) == 0 {
		return "", nil, false
	}
	i := bytes.IndexByte(data, '\n')
	if i < 0 {
		return strings.TrimSuffix(string(data), "\r"), nil, true
	}
	return strings.TrimSuffix(string(data[:i]), "\r"), data[i+1:], true
}

// httpsRepoURL returns the HTTPS URL of the repository of the Git remote
// URL remote, as https://github.com/o/r for git@github.com:o/r.git.
func httpsRepoURL(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	if rest, ok := strings.CutPrefix(remote, "git@"); ok {
		host, p, ok := strings.Cut(rest, ":")
		if !ok {
			return "", fmt.Errorf("remote %q has no path", remote)
		}
		remote = "https://" + host + "/" + p
	}
	u, err := url.Parse(remote)
	if err != nil {
		return "", fmt.Errorf("remote %q: %w", remote, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("remote %q has no host", remote)
	}
	u.Scheme, u.User = "https", nil
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), ".git")
	return u.String(), nil
}
