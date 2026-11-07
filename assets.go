package docpages

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/romshark/morpheus/neo"
)

// static holds page styles, scripts, icons, and the Morpheus bundle.
// generate.go writes style.min.css, docs.min.js, and morpheus/.
//
//go:embed static
var static embed.FS

// staticAssets maps version-relative asset paths to [static] files.
// A directory includes every file. Site and Morpheus icons join under morpheus/icons,
// the only directory <neo-icon> loads.
var staticAssets = map[string]string{
	"docpages.css":   "static/style.min.css",
	"docpages.js":    "static/docs.min.js",
	"morpheus":       "static/morpheus",
	"morpheus/icons": "static/icons",
}

// iconDirs hold site-drawn and genmorpheus-copied Morpheus page icons in [static].
// Their names do not overlap.
var iconDirs = []string{"static/icons", "static/morpheus/icons"}

// icons holds the icons of the pages, the files of [iconDirs].
var icons fs.FS = iconFS{}

// iconFS is the file system of [icons].
type iconFS struct{}

func (iconFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	for _, dir := range iconDirs {
		f, err := static.Open(path.Join(dir, name))
		if !errors.Is(err, fs.ErrNotExist) {
			return f, err
		}
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// <neo-icon> inlines icon SVGs, avoiding a browser request before drawing.
func init() { neo.SetIconFS(icons) }

// siteAssets returns [staticAssets] keyed by version-relative path.
func siteAssets() (map[string][]byte, error) {
	assets := make(map[string][]byte)
	for name, file := range staticAssets {
		err := fs.WalkDir(static, file, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := static.ReadFile(p)
			assets[name+strings.TrimPrefix(p, file)] = data
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	return assets, nil
}

// svgStart matches an SVG root start tag plus preceding XML or comments.
var svgStart = regexp.MustCompile(`(?s)^.*?<svg\b`)

// inlineSVG returns SVG data as HTML markup, dropping content before the root
// and adding name="value" attrs not already set there.
func inlineSVG(data []byte, attrs ...string) string {
	data = bytes.TrimSpace(data)
	loc := svgStart.FindIndex(data)
	if loc == nil {
		return ""
	}
	end := bytes.IndexByte(data[loc[1]:], '>')
	if end < 0 {
		return ""
	}
	tag := string(data[loc[1] : loc[1]+end])
	var b strings.Builder
	b.WriteString("<svg")
	for _, a := range attrs {
		name, _, _ := strings.Cut(a, "=")
		if !regexp.MustCompile(`\s` + regexp.QuoteMeta(name) + `\s*=`).MatchString(tag) {
			b.WriteByte(' ')
			b.WriteString(a)
		}
	}
	b.Write(data[loc[1]:])
	return b.String()
}
