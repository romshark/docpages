# docpages review

Reviewed on 2026-11-18 at commit a0563b3.

Fix the code-execution risk on shared Linux machines and the 3 bugs that delete readers' saved offline files first. After those, the biggest gap is GitHub Markdown that docpages renders as plain text or rejects, which breaks the promise that a page that reads well on GitHub reads well on the site. `go vet ./...`, `go test ./...`, `go test -race .` and `pnpm check-all` all pass, so none of the problems below is caught by the current tests.

Each item ends with how it was checked. "repro" means reproduced on a test site, "output" means seen in the built `docs/` site, and "read" means found by reading the source only.

## Security

1. [template.go:451-455](template.go#L451-L455) builds and runs the template renderer in `/tmp/docpages-<hash>`. It creates that folder with mode 0755 and never checks who owns it. On a shared Linux machine, another user can create the folder first, and docpages then builds and runs their code as you. Build under `os.UserCacheDir()`, or check that you own the folder and that its mode is 0700. (read)

## Offline viewing loses saved files

2. [web/sw/sw.ts:106-115](web/sw/sw.ts#L106-L115) deletes the old saved copy before it downloads the new build. Every push to main makes a new build, because [offline.go](offline.go) hashes every non-media file. If the connection drops during the download, the reader is left with a partial copy and no old one. Delete the old copy only after the status reaches "saved", and copy over the files that didn't change. (read)
3. A save that fails on a full disk or a 4xx error stays in "saving". The 20 s keepalive in [web/docs.ts:949-958](web/docs.ts#L949-L958) then starts the download again, so a video that doesn't fit downloads again every 20 s. Retry only network errors, with backoff. (read)
4. [web/docs.ts:1127-1145](web/docs.ts#L1127-L1145) treats any page without `.offline` as built with `-offline=false`. It then removes the service worker and deletes the saved files, so a custom template that leaves out `Offline(p)` on some pages wipes them. Mark `-offline=false` explicitly in the build instead. (read)
5. [web/sw/sw.ts:395-417](web/sw/sw.ts#L395-L417) tries the network first with no timeout, and it returns a 5xx response instead of the saved copy. A link that ends in `.html`, such as `main/index.html`, misses the cache because the keys in `offline.json` have no extension, and the reader gets the browser's error page. (output)

## GitHub Markdown gaps

6. Footnotes (`[^1]`), `$...$` and `$$...$$` math, and emoji shortcodes come out as literal text, and a `math` code block fails the build. GitHub renders all 4. (repro)
7. ✅ ~~`<kbd>`, `<sup>`, `<sub>`, `<br>` and `<ins>` fail the build, and all 5 are common in READMEs shown through `source:`. (repro)~~
8. ✅ ~~[layout.go:326](layout.go#L326) misses front matter in a file that starts with a UTF-8 byte order mark, and it reports no error. In `docs/README.md` that silently drops `releases` and `repository`. (repro)~~

## Correctness

9. `-serve` keeps showing an error after you fix it, even when you reload the page by hand ([serve.go:132-151](serve.go#L132-L151)). This happens with a link to a repository file that was missing and now exists, and with a broken `source:`. It doesn't watch linked files. (repro)
10. Running `docpages` with no arguments inside `docs/` writes `docs/site/`. The next build then fails with 28 "no page shows this file" errors ([cmd/docpages/main.go:94](cmd/docpages/main.go#L94)). Reject an `-o` folder inside the docs folder. (repro)
11. A docs file that only a template links to, such as `files/cheatsheet.pdf` in `HeaderLinks`, fails the build as unused. `unusedFiles` runs before the templates render ([docpages.go:259](docpages.go#L259)). (repro)
12. A `.templ` file in a subpackage of the docs module fails as unused ([lint.go:259](lint.go#L259)), although `docs/2-guide/6-templates.md:161` allows such imports. (repro)
13. `url:` on `docs/README.md` builds without an error but writes no `main/index.html`, so the root redirect leads to a 404 ([layout.go:212](layout.go#L212)). Reject it. (repro)
14. Page URLs aren't escaped ([docs.go:185](docs.go#L185), [docs.go:460](docs.go#L460)). For example, `2-c#-sharp.md` gets the link `./guide/c#-sharp`, which requests the wrong file. Escape the URLs or reject such names. (repro)
15. A link to a folder without a trailing `/` uses the host's `file` URL instead of `dir` ([docs.go:455](docs.go#L455)). (repro)

## Performance

16. Each `/_repository/` request in `-serve` reads the whole site and all tags again ([serve.go:226](serve.go#L226)). On a 2000-page site a request takes 4.6 s, against 9 ms for a cached page. (repro)
17. `pageOf` scans every page for each link, 3 times per link ([docs.go:494](docs.go#L494)). A 2000-page site with 20 links per page builds in 5.9 s, against 1.2 s without links. A map from file to page fixes it. (repro)
18. With the docs folder at the repository root, `-serve` walks the whole repository every 250 ms per open tab, including `node_modules` ([serve.go:166](serve.go#L166)). (read)
19. Each page carries about 33 KB of markup that is the same on every page:
    - The logo is inlined as a 6.2 KB SVG.
    - The theme script is inlined unminified, at 2.6 KB.
    - Icons are repeated.

    Asset URLs carry no version, and GitHub Pages caches files for 10 minutes. New HTML can therefore load an old `docpages.js`. If the old script then misses a selector, `find()` in [web/docs.ts:22-31](web/docs.ts#L22-L31) throws, which stops search, Mermaid and offline viewing on that page. Add `?v=<build>` to asset URLs. (output)

## Accessibility and UX

20. [blocks.go:24](blocks.go#L24) puts the section anchor inside the heading, so screen readers read "Front matter Link to this section". The anchor is also invisible on touch screens ([web/style.css:1431](web/style.css#L1431)). (output)
21. Pressing Enter to confirm a Japanese or Chinese input conversion in the search box opens the top result ([web/docs.ts:517-536](web/docs.ts#L517-L536)). The Ctrl+K shortcut checks `e.key`, which misses it on non-Latin keyboard layouts. (read)
22. Search matches substrings, so "go" ranks "Logo and favicon" above the Go pages. It doesn't fold accents either, so "cafe" misses "café" ([web/docs.ts:357-411](web/docs.ts#L357-L411)). (checked with a Node copy of the search code on the built `search.json`)
23. Cmd- or Ctrl-click on a search result opens it in the same tab ([web/docs.ts:537](web/docs.ts#L537)). (read)
24. During a save, the progress text in the `aria-live` region changes 4 times a second, and screen readers announce each change. New search results aren't announced. (read)
25. On narrow screens, the contents list stays open when keyboard focus tabs out of it ([web/docs.ts:227-265](web/docs.ts#L227-L265)). (read)

## Site output

26. The build writes no `404.html` or `sitemap.xml`, and pages have no canonical link or Open Graph tags. Search engines index every release, so the copies of a page in each version compete, and no canonical link points to the latest. (output)

## Repository

27. The structure and build sections of [CLAUDE.md](CLAUDE.md) describe morpheus (`web/lib`, `neo`, `datastar`), and it points to `DESIGN.md` and `CHANGELOG.md`, which don't exist here. (checked with `ls`)
28. CI runs the tests only on Ubuntu, but releases ship Windows and macOS binaries. Add an OS matrix to the `test` job. The Windows cross-compile builds. (checked `.github/workflows` and `GOOS=windows go build`)

## Next steps

Items 8, 9, 11, 12, 14 and 15, and the folder ownership in item 1, have no tests. Fix items 1-5 first, as `fix:` commits with a test each.
