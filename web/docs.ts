// Documentation menus, current-section tracking, and search.
// Pages load this module after parsing.

// The Morpheus elements that this module drives.
interface NeoSidebar extends HTMLElement {
	toggle(): void;
	hide(): void;
}
interface NeoSelect extends HTMLElement {
	value: string;
}
interface NeoCheckbox extends HTMLElement {
	checked: boolean;
}

// find returns the element of parent that selector names,
// which the templates render with parent.
const find = <E extends Element = HTMLElement>(
	parent: ParentNode,
	selector: string,
): E => {
	const el = parent.querySelector<E>(selector);
	if (!el) {
		throw new Error(`docpages: no ${selector}`);
	}
	return el;
};

const root = new URL(document.body.dataset.siteRoot || "./", location.href);

// Navigation is a wide-screen column and narrow-screen overlay. Links close
// only the overlay after a jump; Morpheus's data-neo-sidebar-close would close both.
const nav = document.getElementById("docs-nav") as NeoSidebar | null;
document
	.getElementById("docs-nav-toggle")
	?.addEventListener("click", () => nav?.toggle());
nav?.addEventListener("click", (e) => {
	if (
		e.target instanceof Element &&
		e.target.closest("a") &&
		nav.matches(":state(overlay)")
	) {
		nav.hide();
	}
});

// openVersion opens href in another site version, or its first page at fallback
// when that version lacks the page, as when the page was added after release.
const openVersion = async (href: string, fallback: string) => {
	let target = new URL(href, location.href).href;
	try {
		const r = await fetch(target, { method: "HEAD" });
		if (!r.ok) {
			target = new URL(fallback, location.href).href;
		}
	} catch {
		// With no response, open the original link.
	}
	location.href = target;
};

// The header's version menu opens this page in the selected version.
const versions = document.querySelector<NeoSelect>("neo-select.version-select");
if (versions) {
	const current = versions.getAttribute("value") ?? "";
	versions.addEventListener("neo-select-change", (e) => {
		const { value } = (e as CustomEvent<{ value: string }>).detail;
		const option = [
			...versions.querySelectorAll<HTMLElement>("neo-option"),
		].find((o) => o.getAttribute("value") === value);
		if (option && value !== current) {
			openVersion(option.dataset.href ?? "", option.dataset.fallback ?? "");
		}
	});
	// On return, the browser restores this page with the chosen version.
	addEventListener("pageshow", (e) => {
		if (e.persisted) {
			versions.value = current;
		}
	});
}

// Each page <details> wraps its text in a height-animating <neo-elastic>.
// open="false" pins it at 0; no open attribute follows the text height.
// Closed details start pinned, then grow when opened by click, in-page search, or a
// heading link. Clicking an open summary shrinks text before the browser hides it;
// clicking again while it shrinks reopens it.
for (const details of document.querySelectorAll<HTMLDetailsElement>(
	".docs-content .details",
)) {
	const summary = find(details, ":scope > summary");
	const body = find(details, ":scope > .details-body");
	const closing = () => body.getAttribute("open") === "false";
	// collapse pins text at 0 immediately. A closed-details transition waits to open,
	// then starts at its previous height.
	const collapse = () => {
		body.style.transition = "none";
		body.setAttribute("open", "false");
		// <neo-elastic> pins the height in a pre-frame microtask.
		requestAnimationFrame(() => {
			// Reading the height applies the pinned height before the transition returns.
			body.getBoundingClientRect();
			body.style.transition = "";
		});
	};
	details.addEventListener("toggle", () => {
		if (details.open) {
			body.removeAttribute("open");
		} else if (!closing()) {
			collapse();
		}
	});
	summary.addEventListener("click", (e) => {
		if (!details.open) {
			return;
		}
		e.preventDefault();
		if (closing()) {
			body.removeAttribute("open");
			return;
		}
		body.setAttribute("open", "false");
		// No transitionend fires without a transition, such as at
		// --neo-duration-scale: 0 or zero height. getAnimations() starts any due.
		requestAnimationFrame(() => {
			if (closing() && body.getAnimations().length === 0) {
				details.open = false;
			}
		});
	});
	// A nested <details> animation also bubbles here.
	body.addEventListener("neo-elastic-end", (e) => {
		if (e.target === body && closing()) {
			details.open = false;
		}
	});
}

// Both contents lists mark the last heading scrolled past; the bar also shows
// its title. The first entry represents text above the first section and stays
// marked until a heading passes. At page end, mark the URL section if visible,
// otherwise the last visible section. Each section has one link in each list.
type Section = [links: HTMLAnchorElement[], heading: HTMLElement];
const tocLinks = [...document.querySelectorAll<HTMLAnchorElement>(".toc a")];
const sections: Section[] = [];
for (const hash of new Set(tocLinks.map((a) => a.hash))) {
	const heading = document.getElementById(decodeURIComponent(hash.slice(1)));
	if (heading) {
		sections.push([tocLinks.filter((a) => a.hash === hash), heading]);
	}
}
const currentTitle = document.querySelector(".toc-inline-current");
if (sections.length > 0) {
	let scheduled = false;
	const mark = () => {
		scheduled = false;
		const root = document.documentElement;
		// The sticky header and bar cover the scroll padding.
		const top = Number.parseFloat(getComputedStyle(root).scrollPaddingTop) || 0;
		const end = scrollY + innerHeight >= root.scrollHeight - 1;
		const inView = (heading: HTMLElement) =>
			heading.getBoundingClientRect().top < innerHeight;
		let current = sections[0];
		for (const section of sections) {
			const heading = section[1];
			if (
				end ? !inView(heading) : heading.getBoundingClientRect().top > top + 96
			) {
				break;
			}
			current = section;
		}
		if (end) {
			current =
				sections.find(
					([links, heading]) =>
						links[0].hash === location.hash &&
						heading.getBoundingClientRect().top >= top &&
						inView(heading),
				) ?? current;
		}
		for (const section of sections) {
			for (const a of section[0]) {
				if (section === current) {
					a.setAttribute("aria-current", "true");
				} else {
					a.removeAttribute("aria-current");
				}
			}
		}
		if (currentTitle) {
			currentTitle.textContent = current[0][0].textContent;
		}
	};
	const schedule = () => {
		if (!scheduled) {
			scheduled = true;
			requestAnimationFrame(mark);
		}
	};
	addEventListener("scroll", schedule, { passive: true });
	// Linking to an end-of-page section already in view does not scroll.
	addEventListener("hashchange", schedule);
	mark();
}

// The bar list ends 8px above the viewport and scrolls if longer.
// Its room changes while scrolling because the bar begins below its sticky position.
// The bar closes after a section jump, outside click, or Escape. data-stuck
// marks its sticky state, which CSS cannot detect in every browser.
const tocBar = document.querySelector<HTMLDetailsElement>(".toc-inline");
if (tocBar) {
	const list = find(tocBar, ".toc");
	const stick = () => {
		const top = Number.parseFloat(getComputedStyle(tocBar).top);
		tocBar.toggleAttribute(
			"data-stuck",
			tocBar.getBoundingClientRect().top < top + 1,
		);
	};
	addEventListener("scroll", stick, { passive: true });
	addEventListener("resize", stick);
	stick();
	const fit = () => {
		if (tocBar.open) {
			list.style.maxBlockSize = `${innerHeight - list.getBoundingClientRect().top - 8}px`;
		}
	};
	tocBar.addEventListener("toggle", fit);
	addEventListener("scroll", fit, { passive: true });
	addEventListener("resize", fit);
	list.addEventListener("click", (e) => {
		if (e.target instanceof Element && e.target.closest("a")) {
			tocBar.open = false;
		}
	});
	document.addEventListener("click", (e) => {
		if (tocBar.open && !tocBar.contains(e.target as Node)) {
			tocBar.open = false;
		}
	});
	tocBar.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && tocBar.open) {
			tocBar.open = false;
			find(tocBar, "summary").focus();
		}
	});
}

// An entry of search.json: the URL, the page title, the section heading if
// any, and the text of the section.
interface SearchEntry {
	u: string;
	p: string;
	s?: string;
	t: string;
}
// A SearchEntry with lowercase heading, page title and text.
interface IndexedEntry extends SearchEntry {
	head: string;
	page: string;
	text: string;
}

// Search lists search.json sections containing every query word,
// ranked by matches in the heading, page title, then text.
const search = document.querySelector("neo-dialog.search");
if (search) {
	const trigger = find(search, "[data-neo-dialog-trigger]");
	const dialog = find<HTMLDialogElement>(search, "dialog");
	const input = find<HTMLInputElement>(search, ".search-input");
	const list = find(search, ".search-results");
	const heading = find(search, ".search-heading");
	const status = find(search, ".search-status");
	// The template writes suggested pages into the list, shown for an empty field.
	const suggested = [...list.children];
	const limit = 20;

	// <neo-elastic> animates list height. A closed dialog has none, which would
	// regrow it on every open, so open at its height and adopt it next frame.
	const body = find(search, ".search-body");
	search.addEventListener("neo-dialog-open", () => {
		body.style.transition = "none";
		requestAnimationFrame(() =>
			requestAnimationFrame(() => {
				body.style.transition = "";
			}),
		);
	});

	// An on-screen keyboard covers the viewport bottom. iOS and default Android
	// keep the layout viewport and dvh behind it; only the visual viewport
	// stays visible. fit places the dialog there and sizes the list from its
	// top to the visual-viewport bottom, minus content below it and 1rem.
	const keys = find(search, ".search-keys");
	const viewport = window.visualViewport;
	const fit = () => {
		if (!dialog.open || !viewport) {
			return;
		}
		dialog.style.setProperty("--search-top", `${viewport.offsetTop}px`);
		dialog.style.setProperty("--search-height", `${viewport.height}px`);
		const below = status.offsetHeight + keys.offsetHeight + 1 + 16;
		const room =
			viewport.offsetTop +
			viewport.height -
			list.getBoundingClientRect().top -
			below;
		// Keep enough room to show at least one result.
		dialog.style.setProperty("--search-room", `${Math.max(room, 64)}px`);
	};
	viewport?.addEventListener("resize", fit);
	viewport?.addEventListener("scroll", fit);
	search.addEventListener("neo-dialog-open", fit);

	let index: Promise<IndexedEntry[]> | null = null;
	const load = () => {
		index ??= fetch(new URL("search.json", root))
			.then((r) => {
				if (!r.ok) {
					throw new Error(r.statusText);
				}
				return r.json() as Promise<SearchEntry[]>;
			})
			.then((entries) =>
				entries.map((e) => ({
					...e,
					head: (e.s || e.p).toLowerCase(),
					page: e.p.toLowerCase(),
					text: e.t.toLowerCase(),
				})),
			)
			.catch((err) => {
				index = null;
				throw err;
			});
		return index;
	};

	const words = (q: string) => q.toLowerCase().split(/\s+/).filter(Boolean);

	const score = (e: IndexedEntry, ws: string[]) => {
		let s = 0;
		for (const w of ws) {
			if (e.head.includes(w)) {
				s += e.head.startsWith(w) ? 12 : 8;
			} else if (e.page.includes(w)) {
				s += 5;
			} else if (e.text.includes(w)) {
				s += 1;
			} else {
				return 0;
			}
		}
		if (ws.length > 1 && e.head.includes(ws.join(" "))) {
			s += 10;
		}
		return s;
	};

	const escapeRegExp = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

	// highlight appends s to el, wrapping every word match in <mark>.
	const highlight = (el: HTMLElement, s: string, ws: string[]) => {
		const re = new RegExp(ws.map(escapeRegExp).join("|"), "gi");
		let last = 0;
		for (const m of s.matchAll(re)) {
			el.append(s.slice(last, m.index));
			const mark = document.createElement("mark");
			mark.textContent = m[0];
			el.append(mark);
			last = m.index + m[0].length;
		}
		el.append(s.slice(last));
	};

	// excerpt returns about 160 characters around t's first word match.
	const excerpt = (t: string, ws: string[]) => {
		const lower = t.toLowerCase();
		let at = -1;
		for (const w of ws) {
			const i = lower.indexOf(w);
			if (i >= 0 && (at < 0 || i < at)) {
				at = i;
			}
		}
		const start = Math.max(0, at - 40);
		const end = Math.min(t.length, start + 160);
		return (
			(start > 0 ? "..." : "") +
			t.slice(start, end) +
			(end < t.length ? "..." : "")
		);
	};

	const span = (className: string) => {
		const s = document.createElement("span");
		s.className = className;
		return s;
	};

	let active = -1;

	const select = (i: number) => {
		active = i;
		for (const [j, li] of [...list.children].entries()) {
			li.setAttribute("aria-selected", String(j === i));
		}
		if (i < 0) {
			input.removeAttribute("aria-activedescendant");
			return;
		}
		input.setAttribute("aria-activedescendant", list.children[i].id);
		list.children[i].scrollIntoView({ block: "nearest" });
	};

	const go = (a: HTMLAnchorElement) => {
		dialog.close();
		location.href = a.href;
	};

	// hover selects option li at index i when the pointer moves over it.
	const hover = (li: Element, i: number) => {
		li.addEventListener("mousemove", () => {
			if (active !== i) {
				select(i);
			}
		});
	};
	for (const [i, li] of suggested.entries()) {
		hover(li, i);
	}

	const show = (options: Element[], label: string) => {
		list.replaceChildren(...options);
		list.setAttribute("aria-label", label);
		heading.hidden = options !== suggested || suggested.length === 0;
		input.setAttribute("aria-expanded", String(options.length > 0));
		// The heading above the list moves its top.
		fit();
	};

	const item = (e: IndexedEntry, i: number, ws: string[]) => {
		const li = document.createElement("li");
		// goldmark omits "_" from heading IDs, keeping this ID distinct.
		li.id = `search_result_${i}`;
		li.setAttribute("role", "option");
		const a = document.createElement("a");
		a.href = new URL(e.u, root).href;
		a.tabIndex = -1;
		if (e.s) {
			const page = span("result-page");
			page.textContent = e.p;
			a.append(page);
		}
		const head = span("result-section");
		highlight(head, e.s || e.p, ws);
		a.append(head);
		if (e.t) {
			const text = span("result-text");
			highlight(text, excerpt(e.t, ws), ws);
			a.append(text);
		}
		li.append(a);
		hover(li, i);
		return li;
	};

	const render = async () => {
		const ws = words(input.value);
		if (ws.length === 0) {
			show(suggested, "Suggested pages");
			status.textContent = "";
			select(-1);
			return;
		}
		let entries: IndexedEntry[];
		try {
			entries = await load();
		} catch {
			status.textContent = "The search index did not load. Try again.";
			return;
		}
		// The query changed while the index loaded.
		if (words(input.value).join(" ") !== ws.join(" ")) {
			return;
		}
		const results = entries
			.map((e) => [score(e, ws), e] as const)
			.filter(([s]) => s > 0)
			.sort((a, b) => b[0] - a[0])
			.slice(0, limit)
			.map(([, e], i) => item(e, i, ws));
		show(results, "Results");
		status.textContent = results.length === 0 ? "No results." : "";
		select(results.length > 0 ? 0 : -1);
	};

	input.addEventListener("input", render);
	input.addEventListener("keydown", (e) => {
		const n = list.children.length;
		if (e.key === "ArrowDown" && n > 0) {
			e.preventDefault();
			select(Math.min(active + 1, n - 1));
		} else if (e.key === "ArrowUp" && n > 0) {
			e.preventDefault();
			select(Math.max(active - 1, 0));
		} else if (e.key === "Enter" && active >= 0) {
			e.preventDefault();
			const a = list.children[active]?.querySelector("a");
			if (a) {
				go(a);
			}
		} else if (e.key === "Escape") {
			// Search fields consume the first Escape to clear, hiding it from the dialog.
			e.preventDefault();
			dialog.close();
		}
	});
	list.addEventListener("click", (e) => {
		const a = e.target instanceof Element ? e.target.closest("a") : null;
		if (a) {
			e.preventDefault();
			go(a);
		}
	});

	// Preload on button hover to avoid waiting after the first key.
	trigger.addEventListener("pointerenter", () => load().catch(() => {}), {
		once: true,
	});
	trigger.addEventListener("click", () => {
		load().catch(() => {});
		requestAnimationFrame(() => input.select());
	});

	document.addEventListener("keydown", (e) => {
		if (dialog.open || e.altKey) {
			return;
		}
		const typing =
			e.target instanceof Element &&
			e.target.closest("input, textarea, select, [contenteditable]");
		const slash = e.key === "/" && !e.metaKey && !e.ctrlKey && !typing;
		const k = (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k";
		if (slash || k) {
			e.preventDefault();
			trigger.click();
		}
	});
}

// The part of Mermaid that this module calls.
interface Mermaid {
	initialize(config: object): void;
	run(options: { nodes: HTMLElement[] }): Promise<void>;
}

// The URL of Mermaid. A variable keeps tsc from resolving the import.
const mermaidURL =
	"https://cdn.jsdelivr.net/npm/mermaid@12.2.0/dist/mermaid.esm.min.mjs";

// The renderer writes Mermaid blocks as <pre class="mermaid">.
// Mermaid replaces them with SVGs in the colors and font of the page and
// redraws on theme changes. Pages without diagrams do not load Mermaid.
const diagrams = [...document.querySelectorAll<HTMLElement>("pre.mermaid")];
if (diagrams.length > 0) {
	const sources = diagrams.map((d) => d.textContent ?? "");
	const holder = diagrams[0].parentElement ?? document.body;
	const system = matchMedia("(prefers-color-scheme: dark)");
	const dark = () => {
		const theme = document.documentElement.dataset.theme;
		return theme ? theme === "dark" : system.matches;
	};
	// Mermaid parses only hex, rgb() and hsl(), and a token can hold
	// light-dark() or any other CSS color. A probe in the text resolves a color
	// in the current theme, and a canvas pixel turns the result into hex.
	const probe = document.createElement("span");
	probe.hidden = true;
	const pixel = document.createElement("canvas").getContext("2d", {
		willReadFrequently: true,
	});
	const hex = (css: string) => {
		if (!pixel) {
			return "";
		}
		pixel.clearRect(0, 0, 1, 1);
		pixel.fillStyle = css;
		pixel.fillRect(0, 0, 1, 1);
		const [r, g, b] = pixel.getImageData(0, 0, 1, 1).data;
		return `#${[r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")}`;
	};
	// color returns css in hex, or "" if it is no color. An undefined property
	// would fall back to the inherited color and hide a typo; left as written,
	// it fails the diagram.
	const color = (css: string) => {
		const name = /^var\((--[\w-]+)\)$/.exec(css)?.[1];
		if (name && !getComputedStyle(probe).getPropertyValue(name).trim()) {
			return "";
		}
		probe.style.color = "";
		probe.style.color = css;
		return probe.style.color ? hex(getComputedStyle(probe).color) : "";
	};
	const token = (name: string) => color(`var(${name})`);
	// The nearest opaque background behind the diagrams, as a template can
	// place the text on another color than --bg.
	const backdrop = () => {
		for (let e: HTMLElement | null = diagrams[0]; e; e = e.parentElement) {
			const c = getComputedStyle(e).backgroundColor;
			if (c !== "transparent" && !/^rgba\(.*,\s*0\)$/.test(c)) {
				return hex(c);
			}
		}
		return token("--bg");
	};
	const frontMatter = /^(\s*---\r?\n)([\s\S]*?)(\r?\n---[ \t]*(?:\r?\n|$))/;
	const themedColor =
		/(["']?)(light-dark\((?:[^()]|\([^()]*\))*\)|var\(--[\w-]+\))\1/g;
	// themed replaces each var() and light-dark() in the front matter of
	// source with its color in the current theme, as Mermaid takes neither.
	// The value is quoted because YAML reads an unquoted "#" as a comment.
	const themed = (source: string) =>
		source.replace(
			frontMatter,
			(_: string, open: string, config: string, close: string) => {
				const resolved = config.replace(
					themedColor,
					(m: string, _q: string, css: string) => {
						const c = color(css);
						return c ? `"${c}"` : m;
					},
				);
				return open + resolved + close;
			},
		);
	const themeVariables = () => {
		const text = token("--text");
		const card = token("--bg-card");
		return {
			darkMode: dark(),
			// Mermaid's default shadow is light gray, a glow on dark pages.
			dropShadow: `drop-shadow(1px 2px 2px rgb(0 0 0 / ${dark() ? 0.6 : 0.15}))`,
			background: backdrop(),
			// The font of the text, as <pre> takes the font of code.
			fontFamily: getComputedStyle(holder).fontFamily,
			primaryColor: card,
			primaryTextColor: text,
			primaryBorderColor: token("--accent"),
			lineColor: token("--text-muted"),
			textColor: text,
			noteBkgColor: card,
			noteTextColor: text,
			noteBorderColor: token("--border"),
		};
	};
	(import(mermaidURL) as Promise<{ default: Mermaid }>)
		.then(({ default: mermaid }) => {
			const draw = () => {
				holder.append(probe);
				// The front matter of a diagram can still set its own config.
				mermaid.initialize({
					startOnLoad: false,
					theme: "base",
					themeVariables: themeVariables(),
					// Mermaid 12's 120px label width splits lines such as "events,
					// templates". Version 11's 200px fit every label in guide/.
					flowchart: { wrappingWidth: 200 },
				});
				for (const [i, d] of diagrams.entries()) {
					d.removeAttribute("data-processed");
					d.textContent = themed(sources[i]);
				}
				probe.remove();
				return mermaid.run({ nodes: diagrams });
			};
			draw();
			new MutationObserver(draw).observe(document.documentElement, {
				attributes: true,
				attributeFilter: ["data-theme"],
			});
			system.addEventListener("change", draw);
		})
		.catch((err) => console.error("loading Mermaid:", err));
}

// offline.json, as offline.go writes it.
interface OfflineIndex {
	build: string;
	groups: {
		name: string;
		size: number;
		files: { url: string; size: number }[];
	}[];
}
// A report of sw.js.
interface OfflineReport {
	status: "none" | "saving" | "saved";
	groups: string[];
	done: number;
	total: number;
	running: boolean;
	error: string;
}
// A message to sw.js.
type OfflineMessage =
	| { type: "status" | "cancel" | "remove" | "keepalive" }
	| { type: "save"; groups: string[] };

// Offline viewing. sw.js caches the offline.json groups that the reader checks,
// reports on a BroadcastChannel and serves the files offline.
// Pages keep it up during a save and resume a save that a closed tab stopped.
const offline = document.querySelector<HTMLElement>(".offline");
if (offline && "serviceWorker" in navigator && "BroadcastChannel" in window) {
	const options = [...offline.querySelectorAll<HTMLElement>(".offline-option")];
	const box = (name: string) =>
		options
			.find((o) => o.dataset.group === name)
			?.querySelector<NeoCheckbox>("neo-checkbox") ?? undefined;
	const checked = (name: string) => box(name)?.checked ?? false;
	const bar = find(offline, ".offline-saving neo-progress");
	const progress = find(offline, ".offline-progress-text");
	const applyButton = find(offline, ".offline-apply");
	const failed = find(offline, ".offline-error");
	const aura = find(offline, "neo-aura");
	// offline.json, null until fetched.
	let index: OfflineIndex | null = null;
	// The last report of the worker.
	let state: OfflineReport = {
		status: "none",
		groups: [],
		done: 0,
		total: 0,
		running: false,
		error: "",
	};

	// size formats bytes in powers of 1000, as browsers count storage.
	const size = (bytes: number) => {
		const units = ["B", "KB", "MB", "GB", "TB"];
		let n = bytes;
		let i = 0;
		for (; n >= 1000 && i < units.length - 1; i++) {
			n /= 1000;
		}
		return `${i > 0 && n < 10 ? n.toFixed(1) : Math.round(n)} ${units[i]}`;
	};
	const groupSize = (name: string) =>
		index?.groups.find((g) => g.name === name)?.size ?? 0;
	// chosen returns the checked groups.
	const chosen = () =>
		options.map((o) => o.dataset.group ?? "").filter((name) => checked(name));
	const same = (a: string[], b: string[]) =>
		a.length === b.length && a.every((g) => b.includes(g));

	// Free storage at the last estimate, infinite without one.
	let free = Number.POSITIVE_INFINITY;
	const estimate = async () => {
		try {
			const { quota, usage } = await navigator.storage.estimate();
			free = (quota ?? Number.POSITIVE_INFINITY) - (usage ?? 0);
		} catch {
			return;
		}
		fit();
	};

	// fit shows the group sizes and disables a group that doesn't fit with the
	// pages and the other checked groups, and every group without pages.
	// Apply works once the checked groups differ from the saved ones.
	// fit runs at once on a click, with the last estimate.
	const fit = () => {
		let room = free;
		// Saved groups already take their space.
		if (state.status === "saved") {
			for (const g of state.groups) {
				room += groupSize(g);
			}
		}
		const media = ["images", "videos"];
		for (const o of options) {
			const name = o.dataset.group ?? "";
			const bytes = groupSize(name);
			// Without offline.json, show the groups without sizes.
			o.hidden = index !== null && bytes === 0;
			find(o, ".offline-size").textContent = index ? size(bytes) : "";
			let need = groupSize("pages");
			if (name !== "pages") {
				need += bytes;
				for (const other of media) {
					if (other !== name && checked(other)) {
						need += groupSize(other);
					}
				}
			}
			const tooBig = need > room;
			const b = box(name);
			const off = tooBig || (name !== "pages" && !checked("pages"));
			if (b) {
				if (off) {
					b.checked = false;
				}
				b.toggleAttribute("disabled", off);
			}
			const info = find(o, ".offline-too-big");
			info.hidden = !tooBig;
			find(info, "[data-neo-tooltip-content]").textContent =
				`${size(bytes)} is too big to cache for offline viewing. ` +
				`This browser has ${size(Math.max(room, 0))} free.`;
		}
		applyButton.toggleAttribute(
			"disabled",
			same(chosen(), state.status === "saved" ? state.groups : []),
		);
	};

	// synced names what the boxes show: the saved groups, or "none" with all unchecked.
	// A checked box means a saved group.
	let synced = "";
	const sync = (view: string) => {
		const key = view === "saved" ? `saved ${state.groups}` : view;
		if (key === synced || (view !== "saved" && view !== "none")) {
			return;
		}
		synced = key;
		for (const o of options) {
			const name = o.dataset.group ?? "";
			const b = box(name);
			if (b) {
				b.checked = view === "saved" && state.groups.includes(name);
			}
		}
	};

	// The fetch of offline.json, null before it starts.
	let indexing: Promise<void> | null = null;

	const render = () => {
		let view: string = state.status;
		// Offline and nothing saved, offline.json fails.
		if (view === "none" && !index && indexing && !navigator.onLine) {
			view = "unavailable";
		}
		offline.dataset.offlineState = view;
		const stopped = !state.running && state.error !== "";
		aura.toggleAttribute("disabled", view !== "saving" || stopped);
		if (view === "saving") {
			// The size is known once the worker has read offline.json.
			// Until then, the bar is indeterminate.
			const known = state.total > 0;
			bar.toggleAttribute("indeterminate", !known && !stopped);
			bar.setAttribute("max", String(Math.max(state.total, 1)));
			bar.setAttribute("value", String(state.done));
			if (!stopped) {
				progress.textContent = known
					? `${size(state.done)} of ${size(state.total)}`
					: "Starting";
			} else if (!navigator.onLine) {
				progress.textContent = "Waiting for the network";
			} else {
				progress.textContent = state.error;
			}
		}
		if (view === "none" || view === "saved") {
			sync(view);
			fit();
			estimate();
		} else {
			// After a save or a cancel, the boxes show what is cached.
			synced = "";
		}
	};

	const loadIndex = () => {
		indexing ??= (async () => {
			try {
				const r = await fetch(new URL("offline.json", root), {
					cache: "no-cache",
				});
				if (r.ok) {
					index = (await r.json()) as OfflineIndex;
				}
			} catch {
				// Offline and not saved.
			}
			render();
		})();
		return indexing;
	};

	// The worker of the version, only once the reader saves, as a worker starts
	// before every page load in its scope.
	let registration: ServiceWorkerRegistration | null = null;
	// existing returns the registration of the version,
	// which a page in another tab may have made after this page loaded.
	const existing = async () => {
		const r = await navigator.serviceWorker.getRegistration(root.href);
		return r?.scope === root.href ? r : null;
	};
	const register = async () => {
		registration ??= await navigator.serviceWorker.register(
			new URL("sw.js", root),
			{
				scope: root.href,
			},
		);
	};
	// active returns the worker of registration once it activates.
	// navigator.serviceWorker.ready resolves only once, too early for a new registration.
	const active = async () => {
		registration ??= await existing();
		const r = registration;
		const worker = r?.installing ?? r?.waiting;
		if (r && !r.active && worker) {
			await new Promise<void>((resolve) => {
				worker.addEventListener("statechange", () => {
					if (worker.state === "activating" || worker.state === "activated") {
						resolve();
					}
				});
			});
		}
		return r?.active ?? null;
	};
	const post = (worker: ServiceWorker | null, message: OfflineMessage) =>
		worker?.postMessage(message);
	const send = async (message: OfflineMessage) => post(await active(), message);
	// pending marks a save not reported yet,
	// as a new worker reports "none" on activation.
	let pending = false;

	// During a save, a message every 20 s keeps the worker up.
	let keepalive = 0;
	const keepAlive = () => {
		if (state.status === "saving" && !keepalive) {
			keepalive = setInterval(() => send({ type: "keepalive" }), 20_000);
		} else if (state.status !== "saving" && keepalive) {
			clearInterval(keepalive);
			keepalive = 0;
		}
	};

	new BroadcastChannel(`docpages ${root.href} `).addEventListener(
		"message",
		(e) => {
			const report = e.data as OfflineReport;
			if (pending && report.status === "none") {
				return;
			}
			pending = false;
			state = report;
			render();
			keepAlive();
			// With nothing saved, the worker would only slow page loads.
			if (state.status === "none" && !state.running && registration) {
				registration.unregister();
				registration = null;
			}
		},
	);

	// starting holds the state before a save that the page hasn't posted yet,
	// which Cancel restores. Each save makes a new one.
	let starting: { before: OfflineReport } | null = null;

	const save = async (groups: string[]) => {
		const start = { before: state };
		starting = start;
		pending = true;
		failed.textContent = "";
		state = {
			...state,
			status: "saving",
			done: 0,
			total: 0,
			running: true,
			error: "",
		};
		render();
		// Without persistence, the browser may evict the files.
		// Firefox asks the reader, and the save doesn't wait for the answer.
		navigator.storage?.persist?.().catch(() => {});
		let worker: ServiceWorker | null = null;
		let error = "";
		try {
			await register();
			worker = await active();
		} catch (err) {
			error = `This browser can't save the docs: ${(err as Error).message}`;
		}
		if (starting !== start) {
			return;
		}
		starting = null;
		if (error) {
			pending = false;
			state = start.before;
			render();
			failed.textContent = error;
			return;
		}
		post(worker, { type: "save", groups });
	};
	// cancel stops a save and keeps the groups saved before it.
	// Before the page posts the save, it only restores the state.
	const cancel = () => {
		pending = false;
		if (starting) {
			state = starting.before;
			starting = null;
			render();
			return;
		}
		send({ type: "cancel" });
	};
	const remove = () => {
		pending = false;
		send({ type: "remove" });
	};

	offline.addEventListener("neo-checkbox-change", () => fit());
	// Apply saves the checked groups, then drops the others.
	// Without pages, it drops everything.
	applyButton.addEventListener("click", () => {
		if (applyButton.hasAttribute("disabled")) {
			return;
		}
		const groups = chosen();
		if (groups.length > 0) {
			save(groups);
		} else {
			remove();
		}
	});
	find(offline, ".offline-cancel").addEventListener("click", cancel);
	// The popover can open before the page has loaded.
	find(offline, ".offline-button").addEventListener("click", () => loadIndex());

	// The button shows at once; its requests wait for the load event.
	offline.hidden = false;
	const loaded =
		document.readyState === "complete"
			? Promise.resolve()
			: new Promise((resolve) =>
					addEventListener("load", resolve, { once: true }),
				);
	loaded.then(async () => {
		const r = await existing();
		if (r) {
			registration = r;
			// A new build changes sw.js, and its worker replaces the saved files.
			if (navigator.onLine) {
				r.update().catch(() => {});
			}
			send({ type: "status" });
		}
		loadIndex();
	});
	// A new build replaces the saved files once the network returns.
	addEventListener("online", async () => {
		if (registration) {
			await registration.update().catch(() => {});
			send({ type: "status" });
		}
		if (!index) {
			indexing = null;
			loadIndex();
		}
	});
	addEventListener("offline", render);
} else if ("serviceWorker" in navigator) {
	// Without the control, as with -offline=false, remove the worker and files
	// of an earlier build.
	navigator.serviceWorker.getRegistration(root.href).then((r) => {
		if (r?.scope === root.href) {
			r.unregister();
		}
	});
	caches
		.keys()
		.then((names) =>
			Promise.all(
				names
					.filter((n) => n.startsWith(`docpages ${root.href} `))
					.map((n) => caches.delete(n)),
			),
		)
		.catch(() => {});
}

// Offline, an image, video or audio that wasn't saved shows a placeholder of
// its size until the network returns.
const placeholders = new Map<HTMLElement, HTMLElement>();
const cloudOff =
	'<svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
	'stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
	'<path d="m2 2 20 20"/><path d="M5.782 5.782A7 7 0 0 0 9 19h8.5a4.5 4.5 0 0 0 1.307-.193"/>' +
	'<path d="M21.532 16.5A4.5 4.5 0 0 0 17.5 10h-1.79A7.008 7.008 0 0 0 10 5.07"/></svg>';
const unavailable = (el: Element) => {
	const media = (el.closest("picture") ??
		(el.matches("source, track")
			? el.parentElement
			: el)) as HTMLElement | null;
	if (!media?.isConnected || !media.closest(".docs-content")) {
		return;
	}
	const sized = media.matches("picture") ? media.querySelector("img") : media;
	const width = Number(sized?.getAttribute("width")) || media.offsetWidth;
	const height = Number(sized?.getAttribute("height")) || media.offsetHeight;
	const p = document.createElement("span");
	p.className = "offline-placeholder";
	p.setAttribute("role", "img");
	p.setAttribute("aria-label", "Not available offline");
	if (media.matches("audio")) {
		p.classList.add("offline-placeholder-audio");
	} else if (width && height) {
		p.style.inlineSize = `${width}px`;
		p.style.aspectRatio = `${width} / ${height}`;
	}
	p.innerHTML = `${cloudOff}<span>Not available offline</span>`;
	media.replaceWith(p);
	placeholders.set(p, media);
};
document.addEventListener(
	"error",
	(e) => {
		const el = e.target;
		// A <video> tries its next <source> first.
		if (
			!navigator.onLine &&
			el instanceof Element &&
			el.matches("img, video, audio, source") &&
			!(el.matches("source") && el.nextElementSibling?.matches("source"))
		) {
			unavailable(el);
		}
	},
	true,
);
// Media that failed before this module ran.
if (!navigator.onLine) {
	for (const img of document.querySelectorAll<HTMLImageElement>(
		".docs-content img",
	)) {
		if (img.complete) {
			img.decode().catch(() => unavailable(img));
		}
	}
	for (const m of document.querySelectorAll<HTMLMediaElement>(
		".docs-content :is(video, audio)",
	)) {
		if (m.error || m.networkState === HTMLMediaElement.NETWORK_NO_SOURCE) {
			unavailable(m);
		}
	}
}
addEventListener("online", () => {
	for (const [p, media] of placeholders) {
		p.replaceWith(media);
		if (media instanceof HTMLMediaElement) {
			media.load();
		}
		const images =
			media instanceof HTMLImageElement
				? [media]
				: media.querySelectorAll("img");
		for (const img of images) {
			// Setting src loads the image again.
			img.setAttribute("src", img.getAttribute("src") ?? "");
		}
	}
	placeholders.clear();
});
