package docpages

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
)

// Files of each version for offline viewing.
const (
	offlineIndexName  = "offline.json"
	serviceWorkerName = "sw.js"
)

// Offline groups, in page order. pages hold every file but media copies.
const (
	offlinePages  = "pages"
	offlineImages = "images"
	offlineVideos = "videos"
)

// videoExts lists the video, audio and track extensions of [offlineVideos].
// Other media copies are images.
var videoExts = map[string]bool{
	".mp4": true, ".m4v": true, ".webm": true, ".ogv": true, ".mov": true,
	".mp3": true, ".m4a": true, ".aac": true, ".oga": true, ".ogg": true,
	".opus": true, ".wav": true, ".flac": true, ".vtt": true,
}

// offlineIndex is offline.json.
type offlineIndex struct {
	// Build hashes the files of the version.
	// sw.js holds it, so a new build installs a new worker.
	Build  string         `json:"build"`
	Groups []offlineGroup `json:"groups"`
}

// offlineGroup is a group of files that a reader can cache.
type offlineGroup struct {
	Name  string        `json:"name"`
	Size  int           `json:"size"` // Bytes of Files.
	Files []offlineFile `json:"files"`
}

// offlineFile is a file of an [offlineGroup].
type offlineFile struct {
	// URL is relative to the version root, without .html, as pages request it.
	URL  string `json:"url"`
	Size int    `json:"size"`
}

// offlineFiles returns offline.json and sw.js for version files,
// keyed by version-relative path. worker is sw.js without its build.
func offlineFiles(files map[string][]byte, worker []byte) (index, sw []byte, err error) {
	h := sha256.New()
	// Empty groups list [], not null.
	groups := map[string]*offlineGroup{
		offlinePages:  {Name: offlinePages, Files: []offlineFile{}},
		offlineImages: {Name: offlineImages, Files: []offlineFile{}},
		offlineVideos: {Name: offlineVideos, Files: []offlineFile{}},
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		data := files[name]
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", name, len(data))
		// A media copy's name holds a content hash,
		// so hashing the content again would only slow -serve.
		if !isMediaCopy(name) {
			h.Write(data)
		}
		g := groups[offlineGroupOf(name)]
		g.Files = append(g.Files, offlineFile{URL: offlineURL(name), Size: len(data)})
		g.Size += len(data)
	}
	idx := offlineIndex{Build: hex.EncodeToString(h.Sum(nil)[:8])}
	for _, name := range []string{offlinePages, offlineImages, offlineVideos} {
		g := groups[name]
		slices.SortFunc(g.Files,
			func(a, b offlineFile) int { return cmp.Compare(a.URL, b.URL) })
		idx.Groups = append(idx.Groups, *g)
	}
	if index, err = json.Marshal(idx); err != nil {
		return nil, nil, err
	}
	sw = append([]byte("const build = "+strconv.Quote(idx.Build)+";\n"), worker...)
	return index, sw, nil
}

// offlineGroupOf returns the offline group of version-relative file name.
func offlineGroupOf(name string) string {
	switch {
	case !isMediaCopy(name):
		return offlinePages
	case videoExts[strings.ToLower(path.Ext(name))]:
		return offlineVideos
	}
	return offlineImages
}

// offlineURL returns the URL of version-relative file name as page links
// name it, without .html or index.html.
func offlineURL(name string) string {
	switch {
	case name == "index.html":
		return "./"
	case strings.HasSuffix(name, "/index.html"):
		name = strings.TrimSuffix(name, "index.html")
	case strings.HasSuffix(name, ".html"):
		name = strings.TrimSuffix(name, ".html")
	}
	return (&url.URL{Path: name}).EscapedPath()
}

// addOfflineFiles adds offline.json and sw.js to each version in files.
func (s *Site) addOfflineFiles(files map[string][]byte) error {
	worker, err := static.ReadFile("static/sw.min.js")
	if err != nil {
		return err
	}
	for _, e := range s.editions {
		dir := e.outDir()
		version := make(map[string][]byte)
		for name, data := range files {
			if rest, ok := strings.CutPrefix(name, dir); ok {
				version[rest] = data
			}
		}
		index, sw, err := offlineFiles(version, worker)
		if err != nil {
			return err
		}
		files[dir+offlineIndexName], files[dir+serviceWorkerName] = index, sw
	}
	return nil
}
