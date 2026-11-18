# CLAUDE.md

This file holds operational rules for working in the repo. The user documentation is in [docs/](docs/README.md), and the docs-directory layout is in the package doc of [docpages.go](docpages.go).

## Repository Structure

- `*.go`, `*.templ` at the root: the `docpages` package, which renders a docs directory as a static site. `docs.templ` holds the built-in page layout.
- `cmd/docpages`: the `docpages` command. `-o` writes a site, `-serve` previews one.
- `web`: TypeScript and CSS of the page (`docs.ts`, `style.css`) and the service worker (`sw/sw.ts`), which esbuild compiles into `static/`.
- `static`: files embedded in every site. `static/icons` holds the icons that Morpheus lacks. `static/*.min.*` and `static/morpheus` are generated.
- `internal/genmorpheus`: writes the Morpheus bundle to `static/morpheus`.
- `docs`: the docpages documentation, itself a docpages site with its own Go module for `template.templ`.
- `example/cli`, `example/library`: example sites, deployed under `example/` of the docs site. `example/library` builds releases from its tags with the prefix `example/library/`.
- `testdata`: images for the image-size tests.
- `site`: ignored, the output of `go run ./cmd/docpages -o site docs`.

## Build

Generated files are committed, because `go install` and `go get` don't run `go generate`. CI fails when they are stale. After a change to a `.templ` file, `web/` or the Morpheus version in `go.mod`, run `make gen` (`go generate ./...`) and commit the output. Don't edit `*_templ.go`, `static/*.min.*` or `static/morpheus` by hand.

- `go test ./...` runs the tests. Some tests build `example/library` from its Git tags, which needs a full clone.
- `make fmt` formats Go with gofumpt. CI checks the format and `go mod tidy -diff`.
- `make check` runs Biome and `tsc` on `web/`, as CI does. `pnpm -C web run lint-apply` and `pnpm -C web run format` autofix.
- `go run ./cmd/docpages -serve 127.0.0.1:7340 docs` previews the docs and reloads on changes.


# Writing

Applies to chat replies, code comments, documentation and commit messages. Write like an engineer reporting findings to another engineer: plain, specific, with facts, non-verbose.

Styles:

- BLUF (bottom line up front), US military staff writing.
- Inverted pyramid, newswire reporting.
- Plain English: Gowers' `Plain Words`, Cutts' `Oxford Guide to Plain English`.
- Orwell's 6 rules from `Politics and the English Language`.
- Strunk & White: omit needless words, use the active voice.
- IMRaD Results sections: every claim carries a measurement or a citation.
- SRE postmortems: timeline, root cause, action items, no blame, no drama.
- Aviation and maritime logs: 1 fact per line, interpretation kept separate.
- Unix man pages and RFCs: terse, imperative, everything named exactly.

Rules:

- Prefer simple (near primitive) technical English.
- Lead with the conclusion, then the facts that support it.
- Name concrete things, such as files, lines, symbols, values and error codes. Write `parser.go:142`, not "where the parser reads it".
- Write quantities as digits, in code comments, docs, test names and error messages too. Write `8 instances`, `2 times` and `1/5`, not `eight instances`, `twice` and `a fifth`. Idioms and prose written for tone keep their words, as in `one by one` and `halfway through`.
- End with what to do about it, or say plainly that nothing needs doing.
- Don't paraphrase code changes, the diff already shows them. Name the file and what it does now. Prose is for the non-obvious what the diff can't show: why, what was left out, what could break.
- Say what was verified and how: "`mage test` passes", "not run". Never imply a check that didn't happen.
- Report failures, dead ends and skipped work as plainly as successes.
- Answer at the length the question needs. "Is `Cache.Get` safe for concurrent use?" gets "No, it writes `c.entries` without `c.lock`." and nothing else. Don't pad a short answer, and don't compress a real explanation into 3 bullets.
- Stop when the information is delivered. No preamble, no restating the request, no closing summary of what was just said.

Avoid:

- Suspense and buildup: "here's where it gets interesting", "and this is the kicker", "the third one is the most instructive".
- Hype and intensifiers: "not just X, it's THE Y", "load-bearing", "crucial", "powerful", "seamless", "robust", "comprehensive", "deep dive".
- Counting the items instead of naming them. "Two things: X and Y" is "X and Y". A teaser count with no items after it ("three things jumped out at me") is worse.
- Colons in running text, whether they join 2 clauses ("It stays on 1 line: slog quotes it."), hang a list off a sentence ("It tests 2 cases: A and B.") or follow a verbless lead-in ("One dispatch, three streams: ..."). Write 2 sentences, or join the clauses with "which" or "because". Labels such as `TODO:` and `BREAKING:`, the label that starts a list item, and a line that introduces a code block or a list are exempt.
- Figurative language where a plain word fits, such as "buys", "drives", "unlocks", "wins", "kills", "shines" and "leaves the reader hunting". The test is whether the sentence says what literally happens. Write "prevents mutation", not "buys immutability", and "sends it requests", not "drives it".
- Restating a general principle the facts already show. Start with the example.
- Paraphrasing a standard or another document the reader can look up. Write "invalid according to RFC 3986", not "contains a character RFC 3986 forbids, such as `{`".
- Rhetorical questions as headings: "So what does this mean?".
- Filler transitions: "let's dive in", "at the end of the day", "it's worth noting that", "as we can see".
- Praise of the user or of the question: "great question", "you're absolutely right".
- Typographic drama: spaced-out words, all-caps emphasis, exclamation marks, emoji, bold scattered over 1/2 the sentences.
- Em-dashes. Use a full stop or "which ...".
- ", so ..." as a reflex, chained ("X, so Y, so Z") or obvious ("It returns an error, so it can fail."). Keep it where the consequence is the point, as in "The flag is unset, so the default applies."
- Non-ASCII characters where ASCII exists: curly quotes, ellipsis, arrows, non-breaking spaces. Write `'`, `"`, `...`, `->` and a plain space.
- Stating what did not change, stayed, or was already correct.
- Hedging where a check would settle it. Check, then state the answer.
- Apologies and post-mortems after a mistake. Correct it and continue.

Bad:

> Here's where it gets interesting: the retry logic isn't just a nice-to-have, it's the load-bearing assumption of the entire sync pipeline. Three things jumped out at me, and the third one is the most instructive yet. [...] And third, and this is the kicker, the dedupe key includes a timestamp, which means retries are never actually deduplicated.

Good:

> The sync pipeline's retry logic has 3 bugs. `syncQueue.ts:142` swallows `ETIMEDOUT` instead of re-queuing the job. `backoff.ts:31` caps the delay at 2s, under the 8s p99 LTE reconnect time in `bench/network.json`. `dedupe.ts:77` puts a timestamp in the key, which means retries never deduplicate. All 3 reproduce in `syncQueue_test.ts` against the network stub. Fix: re-throw the timeout, raise the cap to 30s and strip the timestamp from the key.

## Code comments

Write a comment only if it adds value to the reader by providing non-obvious context information that the reader cannot easily infer from the code itself.

Don't write:

- Comments that repeat the next line: `// increment i` above `i++`, `// loop over users`, `// return the result`, `// error handling`.
- Doc comments that only spell out the name: `// UserID is the user ID.`
- Banners and dividers: `// --- helpers ---`, `// BEGIN`, `// END`.
- History: `// added in v2`, `// fixed #142`, `// was int64 before`. Git has it. Explain the current code, unless it makes no sense without its history.
- Commented-out code. Delete it, git has it.
- Comments written for the reviewer of the diff instead of the reader of the code: `// Note: now also handles nil`, `// Changed to a map for speed`, `// For simplicity we just skip this`.
- Comments that teach Go or the standard library: `// mu guards concurrent access` above a plain mutex, `// defer closes the file`, `// ok is false when the key is missing`. Assume the reader knows Go well.
- File and line references: `// see decoder.go:212`. They break as soon as the code moves and nothing checks them. Name the symbol with a doc link instead: `[Decoder.Next]`.
- Positional references: `// here and not above`, `// unlike the check below`, `// as mentioned earlier`. The reader cannot tell what is meant, and the words stop being true when the code moves. Name the symbol, or state the fact on its own.

Do write:

- Why the code works this way, when that isn't clear from reading it: why this order, this algorithm, this lock, what breaks without it.
- Where a value comes from: a constant, a timeout, a buffer size. Name the spec section, benchmark, RFC or issue and link it.
- Rules the types can't state: what the caller must guarantee, what the function assumes.
- Why the obvious approach wasn't used: name it and say what went wrong with it.
- `TODO: ...` with what to do and what unblocks it, not a bare `// TODO`.

Form:

- Respect [writing](#writing).
- Plain sentences, present tense, within the line limit.
- Go doc comments on exported names start with the name. This is the only place where repeating the name is required.
- Put the comment above the code, not after it. Comments after the code are for short notes on struct fields, enum values and table-test rows.
- Reference other code with Go doc links in square brackets: `[Build]`, `[Node.ShortName]`, `[github.com/romshark/morpheus/neo.Button]`.
- A doc comment on a symbol says what the symbol does: `// Close ends the open requests and is safe to call repeatedly.`
- A doc comment on a test explains what scenario is tested: `// TestCloseIdempotent tests that repeated Close calls all return nil.`

Bad:

```go
// Timeout for the request.
const timeout = 8 * time.Second

// Loop over the items and check each one.
for _, it := range items {
	// Skip if nil.
	if it == nil {
		continue // skip
	}
}
```

Good:

```go
// timeout covers the p99 LTE reconnect time measured in bench/network.json.
// Below 8s the sync queue retries before the radio is back up.
const timeout = 8 * time.Second

for _, it := range items {
	// [Decoder.Next] emits nil for unknown tags.
	if it == nil {
		continue
	}
}
```

### Git commits

Follow [Conventional Commits 1.0](https://www.conventionalcommits.org/en/v1.0.0/): `<type>[optional scope]: <description>` (e.g. `feat: ...`, `fix: ...`, `chore(deps): ...`).

- Title (first line) 50 characters max, sentence case (capitalize the first word), no trailing period, imperative mood: it completes "If applied, this commit will ...". Use `fix: Escape page URLs`, not `Escaped` (past) or `Escapes` (present).
- Body wraps at 72 characters per line. State the non-obvious _why_, never paraphrase the _what_. Describe the current state in present tense and the fix in imperative, not past: `Focus leaves the open dialog. Trap it in the panel`, not `Focus left the open dialog and was trapped`.
- Reserve `fix`/`feat` for what users get: the `docpages` package, `cmd/docpages`, `web` and `static`. `docs`, `example` and `README.md` use `docs`; tooling (`internal/genmorpheus`, `Makefile`, CI) uses `chore`; test-only changes use `test`. GoReleaser writes the release notes from the commit titles and leaves out `docs:`, `test:` and `chore:` ([.goreleaser.yaml](.goreleaser.yaml)).

### Releases

There is no changelog file. A release is a `vX.Y.Z` tag on main, and pushing it runs GoReleaser ([release.yml](.github/workflows/release.yml)).

### Comment style

Prefer one or two lines over a paragraph and a sentence fragment over a full sentence when it's unambiguous. State the non-obvious _why_ (invariant, constraint, browser quirk, gotcha). Never paraphrase the code's _what_. Use precise terms (e.g. "top layer", "capture phase", "passive listener", "cascade", "containing block") rather than narrating in plain English. If a comment grows past ~3 lines, the reason it needs that much prose is usually the signal to split or rename, not to keep writing.

No history in comments: describe the current state, not what the code used to do or what changed. That belongs in commit messages.

If a reader could infer the comment from the code itself, remove it as it adds no value.

#### Templ and Go comments

An exported Go or Templ symbol takes a doc comment that opens with its name, per Go convention:

```
// Layout is the built-in page with template parts.
templ Layout(p Page, parts Parts) { ... }
```

When a comment names another symbol, write it as a doc link, not unlinked text: `[Layout]` in the same package, `[neo.Select]` for an imported one. Keep runnable example code literal and tab-indented so godoc renders it as a code block, not a link.

```
// DefaultPage is [Layout] with a "Docs" header link to the first page,
// used when docs have no template.templ.
```
