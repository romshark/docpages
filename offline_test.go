package docpages

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOfflineFiles checks the groups, URLs and sizes of offline.json,
// and that the build and sw.js change with the files and the worker,
// but not with the content of a media copy.
// media/codecs.html is a page of a docs group named media.
func TestOfflineFiles(t *testing.T) {
	files := map[string][]byte{
		"index.html":                       []byte("home"),
		"guides/index.html":                []byte("guides"),
		"guides/events.html":               []byte("events"),
		"guides/first steps.html":          []byte("steps"),
		"guides/events.md":                 []byte("# Events"),
		"media/codecs.html":                []byte("codecs"),
		"search.json":                      []byte("[]"),
		"media/0a1b2c3d4e5f6a7b/shot.webp": []byte("webp image"),
		"media/2c3d4e5f6a7b8c9d/demo.MP4":  []byte("video"),
		"media/4e5f6a7b8c9d0e1f/subs.vtt":  []byte("WEBVTT"),
		"morpheus/morpheus.min.js":         []byte("js"),
		"morpheus/icons/search.svg":        []byte("<svg/>"),
	}
	index, sw, err := offlineFiles(files, []byte("worker();\n"))
	require.NoError(t, err)
	var got offlineIndex
	require.NoError(t, json.Unmarshal(index, &got))
	require.Len(t, got.Build, 16)
	require.Equal(t, "const build = \""+got.Build+"\";\nworker();\n", string(sw))
	require.Equal(t, []offlineGroup{
		{Name: "pages", Size: 45, Files: []offlineFile{
			{URL: "./", Size: 4},
			{URL: "guides/", Size: 6},
			{URL: "guides/events", Size: 6},
			{URL: "guides/events.md", Size: 8},
			{URL: "guides/first%20steps", Size: 5},
			{URL: "media/codecs", Size: 6},
			{URL: "morpheus/icons/search.svg", Size: 6},
			{URL: "morpheus/morpheus.min.js", Size: 2},
			{URL: "search.json", Size: 2},
		}},
		{Name: "images", Size: 10, Files: []offlineFile{
			{URL: "media/0a1b2c3d4e5f6a7b/shot.webp", Size: 10},
		}},
		{Name: "videos", Size: 11, Files: []offlineFile{
			{URL: "media/2c3d4e5f6a7b8c9d/demo.MP4", Size: 5},
			{URL: "media/4e5f6a7b8c9d0e1f/subs.vtt", Size: 6},
		}},
	}, got.Groups)

	empty, _, err := offlineFiles(map[string][]byte{"index.html": []byte("home")}, nil)
	require.NoError(t, err)
	require.Contains(t, string(empty), `{"name":"videos","size":0,"files":[]}`)

	again, _, err := offlineFiles(files, nil)
	require.NoError(t, err)
	require.Equal(t, string(index), string(again))
	// A newer worker changes sw.js, even with the same files.
	_, newer, err := offlineFiles(files, []byte("worker(2);\n"))
	require.NoError(t, err)
	require.NotEqual(t, string(sw), string(newer))

	// Media copies hold a content hash in their names, which the build doesn't
	// hash again.
	files["media/0a1b2c3d4e5f6a7b/shot.webp"] = []byte("webp IMAGE")
	same, _, err := offlineFiles(files, nil)
	require.NoError(t, err)
	require.Equal(t, string(index), string(same))

	// A page in media/ has no hash in its name, so its content counts.
	files["media/codecs.html"] = []byte("CODECS")
	changed, _, err := offlineFiles(files, nil)
	require.NoError(t, err)
	var next offlineIndex
	require.NoError(t, json.Unmarshal(changed, &next))
	require.NotEqual(t, got.Build, next.Build)
}

// TestIsMediaCopy checks that only media/<16 hex digits>/<file> is a media copy,
// not a page of a docs group named media.
func TestIsMediaCopy(t *testing.T) {
	for name, want := range map[string]bool{
		"media/0a1b2c3d4e5f6a7b/shot.webp":  true,
		"media/codecs.html":                 false,
		"media/codecs.md":                   false,
		"media/index.html":                  false,
		"media/0a1b/shot.webp":              false, // Short hash.
		"media/0A1B2C3D4E5F6A7B/shot.webp":  false, // Upper case.
		"media/0a1b2c3d4e5f6a7g/shot.webp":  false, // Not hex.
		"media/0a1b2c3d4e5f6a7b/":           false,
		"media/0a1b2c3d4e5f6a7b/a/b.webp":   false,
		"guides/0a1b2c3d4e5f6a7b/shot.webp": false,
	} {
		require.Equal(t, want, isMediaCopy(name), name)
	}
}

// TestSiteOfflineFiles checks that each version holds sw.js and an
// offline.json that lists its own files.
func TestSiteOfflineFiles(t *testing.T) {
	fsys, tags := withReleases("v1.2", "v1.2.3")
	s, err := read(fsys, "docs", noRemote, tags, false)
	require.NoError(t, err)
	files, err := s.Files()
	require.NoError(t, err)
	for _, dir := range []string{"main/", "v1.2/"} {
		require.True(t,
			strings.HasPrefix(string(files[dir+"sw.js"]), "const build = "), dir)
		var index offlineIndex
		require.NoError(t, json.Unmarshal(files[dir+"offline.json"], &index))
		var urls []string
		for _, f := range index.Groups[0].Files {
			urls = append(urls, f.URL)
		}
		require.Contains(t, urls, "guides/events")
		require.Contains(t, urls, "docpages.js")
		require.NotContains(t, urls, "sw.js")
		require.NotContains(t, urls, "offline.json")
	}
}

// TestNoOffline checks that [Options].NoOffline omits offline viewing in a
// build and in [Handler].
func TestNoOffline(t *testing.T) {
	s, err := read(testRepoFS(), "docs", noRemote, nil, false)
	require.NoError(t, err)
	s.page = testPage
	s.options.NoOffline = true
	files, err := s.Files()
	require.NoError(t, err)
	require.NotContains(t, files, "main/sw.js")
	require.NotContains(t, files, "main/offline.json")
	require.NotContains(t, pageHTML(t, s, "guides/events"), `class="offline"`)

	h := Handler(Options{Root: writeTestRepo(t), Dir: "docs", NoOffline: true})
	code, _ := get(t, h, "/main/sw.js")
	require.Equal(t, http.StatusNotFound, code)
	code, body := get(t, h, "/main/guides/events")
	require.Equal(t, http.StatusOK, code)
	require.NotContains(t, body, `class="offline"`)
}

// TestHandlerOffline checks that [Handler] serves sw.js, the button and an
// offline.json of the assets it serves.
func TestHandlerOffline(t *testing.T) {
	h := Handler(Options{Root: writeTestRepo(t), Dir: "docs"})
	code, body := get(t, h, "/main/sw.js")
	require.Equal(t, http.StatusOK, code)
	require.True(t, strings.HasPrefix(body, "const build = "))
	code, body = get(t, h, "/main/guides/events")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `<div class="offline" data-offline-state="none" hidden>`)
	code, body = get(t, h, "/main/offline.json")
	require.Equal(t, http.StatusOK, code)
	var index offlineIndex
	require.NoError(t, json.Unmarshal([]byte(body), &index))
	js, err := static.ReadFile("static/docs.min.js")
	require.NoError(t, err)
	require.Contains(t,
		index.Groups[0].Files,
		offlineFile{URL: "docpages.js", Size: len(js)})
}

// TestOfflineControl checks that the button starts hidden,
// with every group unchecked and Apply disabled.
func TestOfflineControl(t *testing.T) {
	html := pageHTML(t, newTestSite(t), "guides/events")
	for _, want := range []string{
		`<div class="offline" data-offline-state="none" hidden>`,
		`aria-label="Offline viewing"`,
		`<div class="offline-option" data-group="pages" data-label="Pages"><label><neo-checkbox role="checkbox" aria-checked="false"`,
		`<div class="offline-option" data-group="images" data-label="Images"><label><neo-checkbox role="checkbox" aria-checked="false"`,
		`<div class="offline-option" data-group="videos" data-label="Videos and audio">`,
		`<neo-dialog class="offline-confirm">`,
		`Delete offline files?`,
		`Videos and audio`,
		`<neo-progress role="progressbar"`,
		`Cached for offline viewing`,
		`<neo-aura disabled class="offline-aura">`,
		`<neo-button role="button" size="sm" disabled aria-disabled="true" tabindex="-1" class="offline-apply">`,
	} {
		require.Contains(t, html, want)
	}
}
