package docpages

import (
	"bytes"
	"fmt"
)

// llmsTxt returns llms.txt, which lists the Markdown copies of the pages for language
// models in the format of https://llmstxt.org. name is the name of the project,
// docs[i] is the doc of pages[i], and the first page introduces the project.
func llmsTxt(name string, pages []page, groups []group, docs []doc) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s\n\n> %s\n\n", name, docs[0].description)
	b.WriteString("Each link leads to the Markdown copy of a page of the " +
		"documentation. llms-full.txt holds all of them in 1 file.\n")
	group := -1
	for i, p := range pages {
		if p.group != group {
			group = p.group
			fmt.Fprintf(&b, "\n## %s\n\n", groups[group].title)
		}
		if p.external() {
			fmt.Fprintf(&b, "- [%s](%s)\n", p.title, p.url)
			continue
		}
		fmt.Fprintf(&b, "- [%s](%s): %s\n",
			p.title, p.markdownURL(), docs[i].description)
	}
	return b.Bytes()
}

// llmsFullTxt returns llms-full.txt, the Markdown copies of all pages in the
// order of the navigation with their links as seen from the root of the version.
func llmsFullTxt(pages []page, docs []doc) []byte {
	var b bytes.Buffer
	for i, p := range pages {
		if p.external() {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.Write(bytes.TrimSpace(docs[i].markdownFromRoot))
		b.WriteString("\n")
	}
	return b.Bytes()
}

// versionsLlmsTxt returns the llms.txt of the output directory, which links the
// llms.txt of each of editions, the releases from the latest followed by main.
// name is the name of the project and description introduces it.
func versionsLlmsTxt(editions []*edition, name, description string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s\n\n> %s\n\n", name, description)
	b.WriteString("Each link leads to the llms.txt of a version " +
		"of the documentation.\n\n")
	b.WriteString("## Versions\n\n")
	for i, e := range editions {
		what := "An earlier release"
		switch {
		case e.isMain():
			what = "The main branch, unreleased"
		case i == 0:
			what = "The latest release"
		}
		fmt.Fprintf(&b, "- [%s](%sllms.txt): %s\n", e.version, e.outDir(), what)
	}
	return b.Bytes()
}
