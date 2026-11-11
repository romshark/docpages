package docpages

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// tagStore holds the tags of a repository and its files at each tag.
type tagStore interface {
	// tags returns the names of the tags.
	tags() ([]string, error)
	// tree returns the files of the repository at tag. It may read the files
	// of docs directory dir at once, since a release reads them all.
	tree(tag, dir string) (fs.FS, error)
}

// gitTags is the [tagStore] of the Git repository in root.
type gitTags struct{ root string }

// git runs git with args in g's repository, with stdin as its input,
// and returns its output.
func (g gitTags) git(stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", g.root}, args...)...)
	cmd.Stdin = stdin
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return nil, fmt.Errorf(
				"git %s: %s", args[0], strings.TrimSpace(string(exit.Stderr)),
			)
		}
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

func (g gitTags) tags() ([]string, error) {
	out, err := g.git(nil, "tag", "--list")
	if err != nil {
		return nil, err
	}
	// Git forbids spaces in tag names.
	return strings.Fields(string(out)), nil
}

func (g gitTags) tree(tag, dir string) (fs.FS, error) {
	out, err := g.git(nil, "rev-parse", "--verify", "refs/tags/"+tag+"^{tree}")
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(string(out))
	key := g.root + "\x00" + id
	trees.Lock()
	t := trees.m[key]
	trees.Unlock()
	if t == nil {
		if t, err = g.readTree(id); err != nil {
			return nil, err
		}
		trees.Lock()
		trees.m[key] = t
		trees.Unlock()
	}
	var oids []string
	for name, f := range t.files {
		if docsFile(name, dir) {
			oids = append(oids, f.oid)
		}
	}
	if err := g.readBlobs(oids); err != nil {
		return nil, err
	}
	return t, nil
}

// docsFile reports whether repository-relative file belongs to docs directory
// dir, but not to its versions directory. For root-level docs, only README.md
// and groups belong to it, as in [unusedFiles].
func docsFile(file, dir string) bool {
	if dir != "." {
		return inDir(file, dir) && !inDir(file, path.Join(dir, versionsDir))
	}
	top, _, _ := strings.Cut(file, "/")
	return file == readmeName || orderPrefix.MatchString(top)
}

// trees caches the [treeFS] of each repository and tree ID. -serve rebuilds the
// site on every change, and a tag's tree doesn't change.
var trees = struct {
	sync.Mutex
	m map[string]*treeFS
}{m: make(map[string]*treeFS)}

// blobs caches file contents by blob ID. Releases share most of their files,
// which Git stores as 1 blob.
var blobs = struct {
	sync.Mutex
	m map[string][]byte
}{m: make(map[string][]byte)}

// readTree returns the files of tree id, without reading their blobs.
// A symbolic link to a file of the tree becomes that file. Other links, as to a
// directory or out of the tree, are left out.
func (g gitTags) readTree(id string) (*treeFS, error) {
	out, err := g.git(nil, "ls-tree", "-r", "-z", "-l", id)
	if err != nil {
		return nil, err
	}
	files := make(map[string]treeFile)
	var links []string
	for rec := range strings.SplitSeq(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		if rec == "" {
			continue
		}
		// <mode> <type> <object> <size>\t<path>
		meta, name, ok := strings.Cut(rec, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 4 {
			return nil, fmt.Errorf("git ls-tree: unexpected line %q", rec)
		}
		// A submodule is a commit, whose files the repository doesn't hold.
		if f[1] != "blob" {
			continue
		}
		size, err := strconv.ParseInt(f[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("git ls-tree: %s: %w", name, err)
		}
		files[name] = treeFile{oid: f[2], size: size}
		if f[0] == "120000" {
			links = append(links, name)
		}
	}
	targets := make([]string, len(links))
	for i, name := range links {
		targets[i] = files[name].oid
	}
	if err := g.readBlobs(targets); err != nil {
		return nil, err
	}
	resolved := make(map[string]treeFile, len(links))
	for _, name := range links {
		if f, ok := resolveLink(files, links, name); ok {
			resolved[name] = f
		}
	}
	for _, name := range links {
		if f, ok := resolved[name]; ok {
			files[name] = f
		} else {
			delete(files, name)
		}
	}
	// children maps each directory to its entries, true for directories.
	children := map[string]map[string]bool{".": {}}
	for name := range files {
		for child := name; child != "."; child = path.Dir(child) {
			dir := path.Dir(child)
			if children[dir] == nil {
				children[dir] = make(map[string]bool)
			}
			children[dir][path.Base(child)] = child != name
		}
	}
	t := &treeFS{g: g, files: files, dirs: make(map[string][]fs.DirEntry, len(children))}
	for dir, names := range children {
		var entries []fs.DirEntry
		for name, isDir := range names {
			info := fileInfo{name: name, dir: isDir}
			if !isDir {
				info.size = files[path.Join(dir, name)].size
			}
			entries = append(entries, info)
		}
		slices.SortFunc(entries, func(a, b fs.DirEntry) int {
			return strings.Compare(a.Name(), b.Name())
		})
		t.dirs[dir] = entries
	}
	return t, nil
}

// maxLinks is how many symbolic links [resolveLink] follows, as Linux's ELOOP.
const maxLinks = 40

// resolveLink returns the file that symbolic link name of files leads to, and
// whether it leads to a file of files. links are the links of files, whose
// targets [blobs] holds.
func resolveLink(files map[string]treeFile, links []string, name string) (treeFile, bool) {
	for range maxLinks {
		f, ok := files[name]
		if !ok || !slices.Contains(links, name) {
			return f, ok
		}
		blobs.Lock()
		target := string(blobs.m[f.oid])
		blobs.Unlock()
		if name = path.Join(path.Dir(name), target); strings.HasPrefix(target, "/") ||
			!fs.ValidPath(name) {
			return treeFile{}, false
		}
	}
	return treeFile{}, false
}

// readBlobs reads the blobs oids into [blobs] at once, skipping those it holds.
func (g gitTags) readBlobs(oids []string) error {
	blobs.Lock()
	var need []string
	for _, id := range oids {
		if _, ok := blobs.m[id]; !ok {
			need = append(need, id)
		}
	}
	blobs.Unlock()
	if len(need) == 0 {
		return nil
	}
	out, err := g.git(strings.NewReader(strings.Join(need, "\n")+"\n"), "cat-file", "--batch")
	if err != nil {
		return err
	}
	read := make(map[string][]byte, len(need))
	for len(out) > 0 {
		// <object> <type> <size>\n<contents>\n
		head, rest, _ := bytes.Cut(out, []byte("\n"))
		f := strings.Fields(string(head))
		if len(f) != 3 {
			return fmt.Errorf("git cat-file: unexpected line %q", head)
		}
		n, err := strconv.Atoi(f[2])
		if err != nil || n < 0 || len(rest) < n+1 {
			return fmt.Errorf("git cat-file: unexpected line %q", head)
		}
		read[f[0]], out = rest[:n:n], rest[n+1:]
	}
	blobs.Lock()
	for id, data := range read {
		blobs.m[id] = data
	}
	blobs.Unlock()
	return nil
}

// blob returns the contents of blob id.
func (g gitTags) blob(id string) ([]byte, error) {
	if err := g.readBlobs([]string{id}); err != nil {
		return nil, err
	}
	blobs.Lock()
	defer blobs.Unlock()
	data, ok := blobs.m[id]
	if !ok {
		return nil, fmt.Errorf("git cat-file: no blob %s", id)
	}
	return data, nil
}

// treeFS holds the files of a Git tree and reads each blob on first use.
type treeFS struct {
	g     gitTags
	files map[string]treeFile      // By path from the root of the tree.
	dirs  map[string][]fs.DirEntry // By path, "." for the root, sorted by name.
}

// treeFile is a file of a [treeFS].
type treeFile struct {
	oid  string // The ID of its blob.
	size int64
}

// file returns the file at name.
func (t *treeFS) file(op, name string) (treeFile, error) {
	if !fs.ValidPath(name) {
		return treeFile{}, &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	f, ok := t.files[name]
	if !ok {
		return treeFile{}, &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
	}
	return f, nil
}

func (t *treeFS) Open(name string) (fs.File, error) {
	if entries, ok := t.dirs[name]; ok {
		return &openDir{info: fileInfo{name: path.Base(name), dir: true}, entries: entries}, nil
	}
	data, err := t.read("open", name)
	if err != nil {
		return nil, err
	}
	info := fileInfo{name: path.Base(name), size: int64(len(data))}
	return &openFile{info: info, Reader: bytes.NewReader(data)}, nil
}

func (t *treeFS) ReadFile(name string) ([]byte, error) {
	data, err := t.read("read", name)
	return bytes.Clone(data), err
}

// read returns the shared contents of file name for operation op.
func (t *treeFS) read(op, name string) ([]byte, error) {
	f, err := t.file(op, name)
	if err != nil {
		return nil, err
	}
	data, err := t.g.blob(f.oid)
	if err != nil {
		return nil, &fs.PathError{Op: op, Path: name, Err: err}
	}
	return data, nil
}

func (t *treeFS) Stat(name string) (fs.FileInfo, error) {
	if _, ok := t.dirs[name]; ok {
		return fileInfo{name: path.Base(name), dir: true}, nil
	}
	f, err := t.file("stat", name)
	if err != nil {
		return nil, err
	}
	return fileInfo{name: path.Base(name), size: f.size}, nil
}

func (t *treeFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, ok := t.dirs[name]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return slices.Clone(entries), nil
}
