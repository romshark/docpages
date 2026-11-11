package docpages

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// overlayFS is base with files replaced or added.
type overlayFS struct {
	base  fs.FS
	files map[string][]byte // By path.
	dirs  map[string]bool   // The directories of files, with ".".
}

// newOverlay returns base with files, by path, in place of its own.
func newOverlay(base fs.FS, files map[string][]byte) *overlayFS {
	o := &overlayFS{base: base, files: files, dirs: map[string]bool{".": true}}
	for name := range files {
		for d := path.Dir(name); d != "."; d = path.Dir(d) {
			o.dirs[d] = true
		}
	}
	return o
}

func (o *overlayFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if data, ok := o.files[name]; ok {
		info := fileInfo{name: path.Base(name), size: int64(len(data))}
		return &openFile{info: info, Reader: bytes.NewReader(data)}, nil
	}
	info, err := o.Stat(name)
	if err != nil || !info.IsDir() {
		return o.base.Open(name)
	}
	entries, err := o.ReadDir(name)
	if err != nil {
		return nil, err
	}
	return &openDir{
		info:    fileInfo{name: path.Base(name), dir: true},
		entries: entries,
	}, nil
}

func (o *overlayFS) ReadFile(name string) ([]byte, error) {
	if data, ok := o.files[name]; ok {
		return bytes.Clone(data), nil
	}
	return fs.ReadFile(o.base, name)
}

func (o *overlayFS) Stat(name string) (fs.FileInfo, error) {
	if data, ok := o.files[name]; ok {
		return fileInfo{name: path.Base(name), size: int64(len(data))}, nil
	}
	if o.dirs[name] {
		return fileInfo{name: path.Base(name), dir: true}, nil
	}
	return fs.Stat(o.base, name)
}

// ReadDir lists the entries of base and of the files in directory name.
// A file of the overlay replaces the entry of base with its name.
func (o *overlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(o.base, name)
	if err != nil && (!errors.Is(err, fs.ErrNotExist) || !o.dirs[name]) {
		return nil, err
	}
	add := func(e fs.DirEntry) {
		i := slices.IndexFunc(entries,
			func(x fs.DirEntry) bool { return x.Name() == e.Name() })
		if i < 0 {
			entries = append(entries, e)
		} else if !e.IsDir() {
			entries[i] = e
		}
	}
	for f, data := range o.files {
		if path.Dir(f) == name {
			add(fileInfo{name: path.Base(f), size: int64(len(data))})
		}
	}
	for d := range o.dirs {
		if d != "." && path.Dir(d) == name {
			add(fileInfo{name: path.Base(d), dir: true})
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries, nil
}

// fileInfo describes a file or a directory of [treeFS] and [overlayFS],
// and is its directory entry too.
type fileInfo struct {
	name string
	size int64
	dir  bool
}

func (i fileInfo) Name() string       { return i.name }
func (i fileInfo) Size() int64        { return i.size }
func (i fileInfo) ModTime() time.Time { return time.Time{} }
func (i fileInfo) IsDir() bool        { return i.dir }
func (i fileInfo) Sys() any           { return nil }
func (i fileInfo) Type() fs.FileMode  { return i.Mode().Type() }

func (i fileInfo) Info() (fs.FileInfo, error) { return i, nil }

func (i fileInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}

// openFile is an open file of [treeFS] and [overlayFS].
type openFile struct {
	info fileInfo
	*bytes.Reader
}

func (f *openFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *openFile) Close() error               { return nil }

// openDir is an open directory of [treeFS] and [overlayFS].
type openDir struct {
	info    fileInfo
	entries []fs.DirEntry
	read    int // How many entries ReadDir returned.
}

func (d *openDir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *openDir) Close() error               { return nil }

func (d *openDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{
		Op:   "read",
		Path: d.info.name,
		Err:  errors.New("is a directory"),
	}
}

func (d *openDir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.entries[d.read:]
	if n > 0 {
		if len(rest) == 0 {
			return nil, io.EOF
		}
		rest = rest[:min(n, len(rest))]
	}
	d.read += len(rest)
	return slices.Clone(rest), nil
}
