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

// static holds the stylesheet, the script, the icons and the Morpheus bundle of
// the pages. generate.go writes style.min.css and morpheus/.
//
//go:embed static
var static embed.FS

// staticAssets maps the paths of the assets of a version to their files in [static].
// A directory maps every file in it. The icons of the site join those of Morpheus
// in morpheus/icons, the one directory that <neo-icon> loads icons from.
var staticAssets = map[string]string{
	"docpages.css":   "static/style.min.css",
	"docpages.js":    "static/docs.js",
	"morpheus":       "static/morpheus",
	"morpheus/icons": "static/icons",
}

// iconDirs are the directories of [static] that hold the icons of the pages:
// those drawn for the site and those of Morpheus, which genmorpheus copies.
// No name is in both.
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

// <neo-icon> inlines the SVG of the icon, which spares the browser a request
// before it draws the icon.
func init() { neo.SetIconFS(icons) }

// siteAssets returns the assets of [staticAssets] by their path relative to
// the directory of a version.
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

// svgStart matches the start tag of the root element of an SVG file and what
// precedes it, such as an XML declaration or a comment.
var svgStart = regexp.MustCompile(`(?s)^.*?<svg\b`)

// inlineSVG returns the SVG file data as markup for an HTML page, without
// what precedes its root element and with attrs added to that element,
// each as name="value", unless the element sets the attribute already.
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
