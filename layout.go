package docpages

import (
	"bytes"
	"cmp"
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

// frontMatter is a docs Markdown file's optional YAML block between two "---" lines.
// GitHub shows it as a table above the text.
type frontMatter struct {
	// Title replaces the level 1 heading in the navigation and in <title>.
	Title string `yaml:"title"`
	// Suggested marks a page that the search offers while its field is empty.
	Suggested bool `yaml:"suggested"`
	// Source is a file-relative Markdown path shown instead of the page text.
	// It may be outside the docs directory.
	Source string `yaml:"source"`
	// URL makes the entry a link that leaves the site.
	URL string `yaml:"url"`

	// Note is a Markdown paragraph above every version page. Only the docs
	// README.md and that of a release folder accept it; empty means no note.
	Note string `yaml:"note"`

	// Site settings, accepted only in the docs README.md.

	// Name is the project name shown in <title> and llms.txt.
	// It defaults to the repository URL's last element.
	Name string `yaml:"name"`
	// Home is the logo URL relative to the output directory.
	// It defaults to the version's first page.
	Home string `yaml:"home"`
	// Repository is the repository URL, defaulting to Git remote origin.
	Repository string `yaml:"repository"`
	// Host describes where the repository is hosted, GitHub without it.
	Host *hostSettings `yaml:"host"`
	// Releases selects the tags that are releases; nil means no releases.
	Releases *releasesSettings `yaml:"releases"`
}

// releasesSettings selects the release tags: those of feature version From, as
// v1.2, and later, each with Prefix, as api/ for api/v1.2.3.
type releasesSettings struct {
	From   string `yaml:"from"`
	Prefix string `yaml:"prefix"`
}

// readmeName is the file GitHub shows for a directory.
// The docs README is the first page; a group README opens that group.
const readmeName = "README.md"

// orderPrefix matches a group or page name starting with its position,
// such as 1-get-started or 01-pages.md. URLs and titles omit the prefix.
var orderPrefix = regexp.MustCompile(`^([0-9]+)-(.+)$`)

// layout is what the docs directory holds, before the Markdown is converted.
type layout struct {
	pages  []page
	groups []group
	// srcs[i] is pages[i]'s Markdown without front matter, or nil for an external link.
	srcs     [][]byte
	settings frontMatter
}

// group is a group of the navigation, a directory of the docs directory.
type group struct {
	title string
	icon  string // SVG of icon.svg in the directory, empty without it.
}

// readLayout reads pages from docs directory dir in repository fsys.
//
// dir/README.md is first. Each directory beginning with a number and "-" is a group,
// ordered numerically in navigation. Its README.md comes first,
// followed numerically by Markdown files with the same prefix pattern.
// Markdown outside this pattern is an error because navigation would omit it.
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
		return layout{}, fmt.Errorf(
			"%s has no group directory, such as 1-get-started", dir,
		)
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
	// Omitting prefixes makes 1-setup.md and 2-setup.md share a file,
	// as do a group README.md and 1-index.md.
	written := make(map[string]string)
	for _, p := range l.pages {
		if p.external() {
			continue
		}
		file := cmp.Or(p.stub, p.file)
		if other, ok := written[p.outFile()]; ok {
			return layout{}, fmt.Errorf("%s and %s share the output file %s",
				other, file, p.outFile())
		}
		written[p.outFile()] = file
	}
	return l, nil
}

// readPageResult is a page read by [readPage], with Markdown and front matter.
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
		return readPageResult{}, fmt.Errorf(
			"%s: url and source exclude each other", file,
		)
	case fm.URL != "":
		if fm.Title == "" {
			return readPageResult{}, fmt.Errorf(
				"%s: a page with url needs a title", file,
			)
		}
		p.url = fm.URL
		return readPageResult{page: p, fm: fm}, nil
	case fm.Source != "":
		p.stub = file
		p.file = repoPath(file, fm.Source)
		if !fs.ValidPath(p.file) {
			return readPageResult{}, fmt.Errorf(
				"%s: source %q is outside the repository",
				file, fm.Source,
			)
		}
		data, err := fs.ReadFile(fsys, p.file)
		if err != nil {
			return readPageResult{}, fmt.Errorf("%s: source: %w", file, err)
		}
		// The source keeps its own front matter; the page does not inherit it.
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
		fm.Releases != nil
}

// orderedEntry is a group directory or a page file with its position.
type orderedEntry struct {
	n    int
	name string // Name in the directory.
	slug string // Name without the prefix.
}

// ordered returns entries' group directories when dirs, otherwise its page files,
// sorted by prefix. dir contains entries. A non-README Markdown file
// without a prefix or a prefix shared by two entries is an error.
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

// noNestedGroups rejects a positioned directory in group directory dir,
// which would nest a group.
func noNestedGroups(entries []fs.DirEntry, dir string) error {
	for _, e := range entries {
		if e.IsDir() && orderPrefix.MatchString(e.Name()) {
			return fmt.Errorf("%s: a group holds no groups", path.Join(dir, e.Name()))
		}
	}
	return nil
}

// groupTitle turns a group's unprefixed directory slug into a title,
// such as "Get started" from get-started.
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

// cutLine returns data's first line without its ending and the remaining data.
// The last line may lack an ending. ok is false when data is empty.
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

// httpsRepoURL converts Git remote URL remote to HTTPS, for example
// git@github.com:o/r.git to https://github.com/o/r.
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
