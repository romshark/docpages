package docpages

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// release is a feature version on the site and the tag of its latest patch.
type release struct {
	version string // As v1.2.
	tag     string // As v1.2.3, or api/v1.2.3 with a prefix.
}

// pickReleases returns the releases of tags that set selects, latest first:
// for each feature version from set.From on, the tag of its latest patch.
// Tags without set.Prefix and pre-releases, as v1.2.0-rc.1, are left out.
func pickReleases(tags []string, set releasesSettings) ([]release, error) {
	if set.From == "" {
		return nil, errors.New("releases: from is missing, as from: v1.2")
	}
	m := featureVersion.FindStringSubmatch(set.From)
	if m == nil {
		return nil, fmt.Errorf(
			"releases: from %q is no feature version, as v1.2", set.From)
	}
	first, err := versionNumbers(m[1:])
	if err != nil {
		return nil, fmt.Errorf("releases: from %q: %w", set.From, err)
	}
	prefix := set.Prefix
	tagRE := regexp.MustCompile(
		"^" + regexp.QuoteMeta(prefix) + `v([0-9]+)\.([0-9]+)\.([0-9]+)$`)
	type tagged struct {
		patch int
		tag   string
	}
	latest := make(map[[2]int]tagged)
	for _, t := range tags {
		m := tagRE.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		v, err := versionNumbers(m[1:])
		if err != nil {
			return nil, fmt.Errorf("tag %s: %w", t, err)
		}
		feature := [2]int{v[0], v[1]}
		if compareFeatures(feature, [2]int(first)) < 0 {
			continue
		}
		if cur, ok := latest[feature]; !ok || v[2] > cur.patch {
			latest[feature] = tagged{patch: v[2], tag: t}
		}
	}
	if _, ok := latest[[2]int(first)]; !ok {
		return nil, fmt.Errorf("releases: no tag %sv%d.%d.x, "+
			"which a clone without tags lacks, "+
			"as actions/checkout without fetch-depth: 0",
			prefix, first[0], first[1])
	}
	features := slices.SortedFunc(maps.Keys(latest),
		func(a, b [2]int) int { return compareFeatures(b, a) })
	rs := make([]release, len(features))
	for i, f := range features {
		rs[i] = release{version: fmt.Sprintf("v%d.%d", f[0], f[1]), tag: latest[f].tag}
	}
	return rs, nil
}

// versionNumbers returns the numbers of a version, as ["1", "2", "3"].
func versionNumbers(digits []string) ([]int, error) {
	r := make([]int, len(digits))
	for i, d := range digits {
		n, err := strconv.Atoi(d)
		if err != nil {
			return nil, err
		}
		r[i] = n
	}
	return r, nil
}

// compareFeatures compares feature versions a and b, as v1.2 before v1.10.
func compareFeatures(a, b [2]int) int {
	return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
}

// checkFolders reports a non-folder in versions/ of docs directory dir in fsys,
// and a folder of no release in rels, which set selected; nil set means none.
func checkFolders(fsys fs.FS, dir string, rels []release, set *releasesSettings) error {
	vdir := path.Join(dir, versionsDir)
	entries, err := fs.ReadDir(fsys, vdir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	for _, e := range entries {
		p := path.Join(vdir, e.Name())
		switch {
		case strings.HasPrefix(e.Name(), "."):
		case !e.IsDir():
			return fmt.Errorf("%s: %s holds only the folders of releases, as v1.2",
				p, vdir)
		case !featureVersion.MatchString(e.Name()):
			return fmt.Errorf("%s: the folder of a release is named "+
				"after its feature version, as v1.2", p)
		case set == nil:
			return fmt.Errorf("%s: the folder of a release needs releases "+
				"in the front matter of %s", p, path.Join(dir, readmeName))
		case !slices.ContainsFunc(rels,
			func(r release) bool { return r.version == e.Name() }):
			return fmt.Errorf("%s: no release %s among the tags from %s%s",
				p, e.Name(), set.Prefix, set.From)
		}
	}
	return nil
}

// readRelease reads release r of docs directory dir: the files of its tag in
// tags, replaced by the files of its folder in versions/ of fsys.
// Its links lead to files at the tag, or on main for replaced files.
func readRelease(
	fsys fs.FS, dir string, r release, tags tagStore, h *host,
) (*edition, error) {
	tree, err := tags.tree(r.tag, dir)
	if err != nil {
		return nil, err
	}
	folder := path.Join(dir, versionsDir, r.version)
	files := make(map[string][]byte)
	replaced := make(map[string]string)
	err = fs.WalkDir(fsys, folder, func(p string, d fs.DirEntry, err error) error {
		switch {
		case p == folder && errors.Is(err, fs.ErrNotExist):
			return fs.SkipAll
		case err != nil:
			return err
		case p != folder && strings.HasPrefix(d.Name(), "."):
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		case d.IsDir():
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		f := path.Join(dir, strings.TrimPrefix(p, folder+"/"))
		files[f], replaced[f] = data, p
		return nil
	})
	if err != nil {
		return nil, err
	}
	rfs := newOverlay(tree, files)
	l, err := readLayout(rfs, dir)
	if err != nil {
		return nil, err
	}
	// The README.md of the tag holds the settings and the note of main at the time.
	// A release takes neither.
	readme := path.Join(dir, readmeName)
	note := ""
	if file, ok := replaced[readme]; ok {
		if l.settings.hasSettings() {
			return nil, fmt.Errorf("%s: a release takes the settings of the site "+
				"from %s", file, readme)
		}
		note = strings.TrimSpace(l.settings.Note)
	}
	links := &site{ref: r.tag, host: h, fsys: rfs, replaced: replaced}
	docs, err := convertDocs(l.pages, l.srcs, links)
	if err != nil {
		return nil, err
	}
	return &edition{
		version:  r.version,
		tag:      r.tag,
		dir:      dir,
		pages:    l.pages,
		groups:   l.groups,
		docs:     docs,
		links:    links,
		note:     note,
		noteFile: path.Join(folder, readmeName),
	}, nil
}
