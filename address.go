package docpages

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// siteAddress returns setting address, the URL that the output directory
// deploys to, ending in "/". It's empty without one.
func siteAddress(address string) (string, error) {
	if address == "" {
		return "", nil
	}
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("address %q isn't an absolute http or https URL "+
			"without query and fragment, as https://o.github.io/r/", address)
	}
	return strings.TrimSuffix(u.String(), "/") + "/", nil
}

// canonicalEdition returns the edition whose page of path search engines show
// in place of that of e: the latest release, or main without releases,
// if it has the page, and e otherwise. It returns nil for main with releases,
// which search engines don't index.
func (s *Site) canonicalEdition(e *edition, path string) *edition {
	latest := s.latest()
	switch {
	case latest == nil:
		return e
	case e.isMain():
		return nil
	case slices.ContainsFunc(latest.pages,
		func(q page) bool { return !q.external() && q.path == path }):
		return latest
	}
	return e
}

// canonical returns the URL of the page that search engines show for page i of e,
// from that page, or "" for none. With an address, it's absolute.
func (s *Site) canonical(e *edition, i int) string {
	p := e.pages[i]
	c := s.canonicalEdition(e, p.path)
	if c == nil {
		return ""
	}
	u := c.outDir() + p.urlPath()
	if s.chrome.address != "" {
		return s.chrome.address + u
	}
	return p.base() + "../" + u
}

// sitemapXML returns sitemap.xml, the absolute URLs of the pages that search
// engines show: each page whose canonical URL is its own.
func (s *Site) sitemapXML() []byte {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, e := range s.editions {
		for _, p := range e.pages {
			if p.external() || s.canonicalEdition(e, p.path) != e {
				continue
			}
			b.WriteString("<url><loc>")
			_ = xml.EscapeText(&b, []byte(s.chrome.address+e.outDir()+p.urlPath()))
			b.WriteString("</loc></url>\n")
		}
	}
	b.WriteString("</urlset>\n")
	return b.Bytes()
}
