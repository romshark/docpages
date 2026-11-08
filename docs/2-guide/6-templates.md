# Templates

To change the markup of the pages, as the links of the header and the footer, put [templ](https://templ.guide) files in the docs folder. The pages show `templ DocpagesMain` of these files, in place of the built-in page:

```templ title="docs/template.templ"
package docs

import "github.com/romshark/docpages"

templ DocpagesMain(p docpages.Page) {
	@docpages.Layout(p, docpages.Parts{
		HeaderLinks: docpages.HeaderLinks(p,
			docpages.Link{Title: "Docs", URL: "README.md"},
			docpages.Link{Title: "FAQ", URL: "4-more/1-faq.md", Icon: faqIcon()},
		),
		FooterLinks: docpages.FooterLinks(p,
			docpages.Link{Title: "Changelog", URL: "3-reference/2-changelog.md"},
		),
		FooterEnd: license(),
	})
}

templ faqIcon() {
	<svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor">
		<circle cx="12" cy="12" r="10"></circle>
	</svg>
}

templ license() {
	<span>MIT License © 2026 Jane Doe</span>
}
```

Every `.templ` file of the docs folder is `package docs`, and 1 of them has `templ DocpagesMain(p docpages.Page)`. Without `.templ` files, the pages show the built-in page, with 1 link in the header, "Docs", to the first page.

## The built-in page

`docpages.Layout` is the built-in page. It takes parts of your own:

| Part | What it adds |
| --- | --- |
| `Head` | Elements at the end of `<head>`, as fonts or a script. |
| `Banner` | A notice across the top of the page, above the header. |
| `HeaderLinks` | Links of the header, before the link to your repository. |
| `FooterLinks` | Links of the footer, between the link to your repository and `llms.txt`. |
| `FooterEnd` | The end of the footer, as a license. |

## Links

`docpages.HeaderLinks` and `docpages.FooterLinks` write links. Each `docpages.Link` has a `Title`, a `URL` and an optional `Icon`, a templ of an `<svg>`. Draw it with `currentColor` and it takes the color of the text in both themes.

A `URL` works like a link of `docs/README.md`. A Markdown file leads to its page, any other file of the repository to the file in the repository, and a full URL off the site. A page that a release lacks leads to the page of `main`. A link to a file or a heading that doesn't exist fails the build.

A header link is highlighted on the page it leads to. A link to the first page is highlighted on every page that no other link leads to.

For a link of your own, `p.URL` takes the same URLs:

```templ
<a href={ p.URL("2-guide/1-events.md#dispatch") }>Dispatch</a>
```

## Releases

The template of the docs folder renders the pages of `main` and of every release. `p.Version` is `main` or the release, as `v1.2`, so the template can change the pages of a release:

```templ
templ DocpagesMain(p docpages.Page) {
	@docpages.Layout(p, docpages.Parts{Banner: banner(p)})
}

func banner(p docpages.Page) templ.Component {
	if p.Version == "v0.9" {
		return oldBanner()
	}
	return nil
}

templ oldBanner() {
	<p class="old-banner">v0.9 has a known vulnerability. Upgrade to v1.0.</p>
}
```

Compare a version with `==`, since `<` sorts `v0.10` before `v0.9`. `p.Versions` lists the releases, the latest first, then `main`, and the `Href` of each leads to the same page in that version.

## A page of your own

`p` holds all that a page shows, as `p.Title`, `p.Body`, `p.Navigation` and `p.Versions`. The built-in page is made of parts that you can use on their own:

| Part | What it shows |
| --- | --- |
| `Head(p)` | `<head>` with the title, the styles and the scripts, then the elements inside it, as fonts. Every page needs it. |
| `Body(p)` | `<body>`, which the styles and the search need, with a link that skips to the text. |
| `Header(p)` | The logo, the version menu, the links in it, the search and the theme switch. |
| `Navigation(p)` | The navigation. |
| `Main(p)` | The note of the version, the text and the links to the previous and the next page. |
| `Contents(p)` | The contents of the page, beside the text on wide screens. |
| `Footer(p, end)` | The links in it and `end`. |
| `Search(p)`, `VersionSelect(p)`, `ThemeSwitch()` | The controls of `Header`. |

```templ title="docs/template.templ"
package docs

import "github.com/romshark/docpages"

templ DocpagesMain(p docpages.Page) {
	<!DOCTYPE html>
	<html lang="en">
		@docpages.Head(p)
		@docpages.Body(p) {
			@docpages.Header(p)
			<div class="docs">
				@docpages.Navigation(p)
				@docpages.Main(p)
			</div>
		}
	</html>
}
```

The [Go reference](https://pkg.go.dev/github.com/romshark/docpages) lists every field of `docpages.Page`.

## Build

A build with `.templ` files needs the [go command](https://go.dev/doc/install), and fails without it. docpages generates the Go of your files and of a renderer into a folder of the temporary directory, builds it and renders the pages with it. The first build takes a while, and later builds use the Go build cache. `-serve` builds again when you change a `.templ` file, and shows a mistake in the page.

Your files can import the Go standard library, docpages, templ and [Morpheus](https://github.com/romshark/morpheus), which docpages uses.

`templ generate` in your repository writes a `_templ.go` file next to each `.templ` file, which docpages ignores. Your Go module then needs docpages, or leave the docs folder out with `-ignore-pattern`.

A docs folder at the root of the repository takes no templates. Its `.templ` files belong to the repository.
