// Documentation menus, current-section tracking, and search.
// Pages load this module after parsing.

const root = new URL(document.body.dataset.siteRoot || "./", location.href);

// Navigation is a wide-screen column and narrow-screen overlay. Links close
// only the overlay after a jump; Morpheus's data-neo-sidebar-close would close both.
const nav = document.getElementById("docs-nav");
document.getElementById("docs-nav-toggle")?.addEventListener("click", () => nav.toggle());
nav?.addEventListener("click", (e) => {
	if (e.target.closest("a") && nav.matches(":state(overlay)")) {
		nav.hide();
	}
});

// openVersion opens href in another site version, or its first page at fallback
// when that version lacks the page, as when the page was added after release.
const openVersion = async (href, fallback) => {
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
const versions = document.querySelector("neo-select.version-select");
if (versions) {
	const current = versions.getAttribute("value");
	versions.addEventListener("neo-select-change", (e) => {
		const option = [...versions.querySelectorAll("neo-option")]
			.find((o) => o.getAttribute("value") === e.detail.value);
		if (option && e.detail.value !== current) {
			openVersion(option.dataset.href, option.dataset.fallback);
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
for (const details of document.querySelectorAll(".docs-content .details")) {
	const summary = details.querySelector(":scope > summary");
	const body = details.querySelector(":scope > .details-body");
	const closing = () => body.getAttribute("open") === "false";
	// collapse pins text at 0 immediately. A closed-details transition waits to open,
	// then starts at its previous height.
	const collapse = () => {
		body.style.transition = "none";
		body.setAttribute("open", "false");
		// <neo-elastic> pins the height in a pre-frame microtask.
		requestAnimationFrame(() => {
			void body.offsetHeight;
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
const tocLinks = [...document.querySelectorAll(".toc a")];
const sections = [...new Set(tocLinks.map((a) => a.hash))]
	.map((hash) => [
		tocLinks.filter((a) => a.hash === hash),
		document.getElementById(decodeURIComponent(hash.slice(1))),
	])
	.filter(([, heading]) => heading);
const currentTitle = document.querySelector(".toc-inline-current");
if (sections.length > 0) {
	let scheduled = false;
	const mark = () => {
		scheduled = false;
		const root = document.documentElement;
		// The sticky header and bar cover the scroll padding.
		const top = parseFloat(getComputedStyle(root).scrollPaddingTop) || 0;
		const end = scrollY + innerHeight >= root.scrollHeight - 1;
		const inView = (heading) => heading.getBoundingClientRect().top < innerHeight;
		let current = sections[0];
		for (const section of sections) {
			const heading = section[1];
			if (end ? !inView(heading) : heading.getBoundingClientRect().top > top + 96) {
				break;
			}
			current = section;
		}
		if (end) {
			current = sections.find(([links, heading]) =>
				links[0].hash === location.hash &&
				heading.getBoundingClientRect().top >= top &&
				inView(heading)) ?? current;
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
const bar = document.querySelector(".toc-inline");
if (bar) {
	const list = bar.querySelector(".toc");
	const stick = () => {
		const top = parseFloat(getComputedStyle(bar).top);
		bar.toggleAttribute("data-stuck", bar.getBoundingClientRect().top < top + 1);
	};
	addEventListener("scroll", stick, { passive: true });
	addEventListener("resize", stick);
	stick();
	const fit = () => {
		if (bar.open) {
			list.style.maxBlockSize = `${innerHeight - list.getBoundingClientRect().top - 8}px`;
		}
	};
	bar.addEventListener("toggle", fit);
	addEventListener("scroll", fit, { passive: true });
	addEventListener("resize", fit);
	list.addEventListener("click", (e) => {
		if (e.target.closest("a")) {
			bar.open = false;
		}
	});
	document.addEventListener("click", (e) => {
		if (bar.open && !bar.contains(e.target)) {
			bar.open = false;
		}
	});
	bar.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && bar.open) {
			bar.open = false;
			bar.querySelector("summary").focus();
		}
	});
}

// Search lists search.json sections containing every query word,
// ranked by matches in the heading, page title, then text.
const search = document.querySelector("neo-dialog.search");
if (search) {
	const trigger = search.querySelector("[data-neo-dialog-trigger]");
	const dialog = search.querySelector("dialog");
	const input = search.querySelector(".search-input");
	const list = search.querySelector(".search-results");
	const heading = search.querySelector(".search-heading");
	const status = search.querySelector(".search-status");
	// The template writes suggested pages into the list, shown for an empty field.
	const suggested = [...list.children];
	const limit = 20;

	// <neo-elastic> animates list height. A closed dialog has none, which would
	// regrow it on every open, so open at its height and adopt it next frame.
	const body = search.querySelector(".search-body");
	search.addEventListener("neo-dialog-open", () => {
		body.style.transition = "none";
		requestAnimationFrame(() => requestAnimationFrame(() => {
			body.style.transition = "";
		}));
	});

	// An on-screen keyboard covers the viewport bottom. iOS and default Android
	// keep the layout viewport and dvh behind it; only the visual viewport
	// stays visible. fit places the dialog there and sizes the list from its
	// top to the visual-viewport bottom, minus content below it and 1rem.
	const keys = search.querySelector(".search-keys");
	const viewport = window.visualViewport;
	const fit = () => {
		if (!dialog.open || !viewport) {
			return;
		}
		dialog.style.setProperty("--search-top", `${viewport.offsetTop}px`);
		dialog.style.setProperty("--search-height", `${viewport.height}px`);
		const below = status.offsetHeight + keys.offsetHeight + 1 + 16;
		const room = viewport.offsetTop + viewport.height -
			list.getBoundingClientRect().top - below;
		// Keep enough room to show at least one result.
		dialog.style.setProperty("--search-room", `${Math.max(room, 64)}px`);
	};
	viewport?.addEventListener("resize", fit);
	viewport?.addEventListener("scroll", fit);
	search.addEventListener("neo-dialog-open", fit);

	let index = null;
	const load = () => {
		index ??= fetch(new URL("search.json", root))
			.then((r) => {
				if (!r.ok) {
					throw new Error(r.statusText);
				}
				return r.json();
			})
			.then((entries) => entries.map((e) => ({
				...e,
				head: (e.s || e.p).toLowerCase(),
				page: e.p.toLowerCase(),
				text: e.t.toLowerCase(),
			})))
			.catch((err) => {
				index = null;
				throw err;
			});
		return index;
	};

	const words = (q) => q.toLowerCase().split(/\s+/).filter(Boolean);

	const score = (e, ws) => {
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

	const escapeRegExp = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

	// highlight appends s to el, wrapping every word match in <mark>.
	const highlight = (el, s, ws) => {
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
	const excerpt = (t, ws) => {
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
		return (start > 0 ? "..." : "") + t.slice(start, end) + (end < t.length ? "..." : "");
	};

	const span = (className) => {
		const s = document.createElement("span");
		s.className = className;
		return s;
	};

	let active = -1;

	const select = (i) => {
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

	const go = (a) => {
		dialog.close();
		location.href = a.href;
	};

	// hover selects option li at index i when the pointer moves over it.
	const hover = (li, i) => {
		li.addEventListener("mousemove", () => {
			if (active !== i) {
				select(i);
			}
		});
	};
	suggested.forEach(hover);

	const show = (options, label) => {
		list.replaceChildren(...options);
		list.setAttribute("aria-label", label);
		heading.hidden = options !== suggested || suggested.length === 0;
		input.setAttribute("aria-expanded", String(options.length > 0));
		// The heading above the list moves its top.
		fit();
	};

	const item = (e, i, ws) => {
		const li = document.createElement("li");
		// goldmark omits "_" from heading IDs, keeping this ID distinct.
		li.id = "search_result_" + i;
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
		let entries;
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
			.map((e) => [score(e, ws), e])
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
			go(list.children[active].querySelector("a"));
		} else if (e.key === "Escape") {
			// Search fields consume the first Escape to clear, hiding it from the dialog.
			e.preventDefault();
			dialog.close();
		}
	});
	list.addEventListener("click", (e) => {
		const a = e.target.closest("a");
		if (a) {
			e.preventDefault();
			go(a);
		}
	});

	// Preload on button hover to avoid waiting after the first key.
	trigger.addEventListener("pointerenter", () => load().catch(() => {}), { once: true });
	trigger.addEventListener("click", () => {
		load().catch(() => {});
		requestAnimationFrame(() => input.select());
	});

	document.addEventListener("keydown", (e) => {
		if (dialog.open || e.altKey) {
			return;
		}
		const typing = e.target.closest?.("input, textarea, select, [contenteditable]");
		const slash = e.key === "/" && !e.metaKey && !e.ctrlKey && !typing;
		const k = (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k";
		if (slash || k) {
			e.preventDefault();
			trigger.click();
		}
	});
}

// The renderer writes Mermaid blocks as <pre class="mermaid">.
// Mermaid replaces them with SVGs in the colors and font of the page and
// redraws on theme changes. Pages without diagrams do not load Mermaid.
const diagrams = [...document.querySelectorAll("pre.mermaid")];
if (diagrams.length > 0) {
	const sources = diagrams.map((d) => d.textContent);
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
	const hex = (css) => {
		pixel.clearRect(0, 0, 1, 1);
		pixel.fillStyle = css;
		pixel.fillRect(0, 0, 1, 1);
		const [r, g, b] = pixel.getImageData(0, 0, 1, 1).data;
		return `#${[r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")}`;
	};
	// color returns css in hex, or "" if it is no color. An undefined property
	// would fall back to the inherited color and hide a typo; left as written,
	// it fails the diagram.
	const color = (css) => {
		const name = /^var\((--[\w-]+)\)$/.exec(css)?.[1];
		if (name && !getComputedStyle(probe).getPropertyValue(name).trim()) {
			return "";
		}
		probe.style.color = "";
		probe.style.color = css;
		return probe.style.color ? hex(getComputedStyle(probe).color) : "";
	};
	const token = (name) => color(`var(${name})`);
	// The nearest opaque background behind the diagrams, as a template can
	// place the text on another color than --bg.
	const backdrop = () => {
		for (let e = diagrams[0]; e; e = e.parentElement) {
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
	const themed = (source) =>
		source.replace(frontMatter, (_, open, config, close) => {
			const resolved = config.replace(themedColor, (m, _q, css) => {
				const c = color(css);
				return c ? `"${c}"` : m;
			});
			return open + resolved + close;
		});
	const themeVariables = () => {
		const text = token("--text");
		const card = token("--bg-card");
		return {
			darkMode: dark(),
			// Mermaid's default shadow is light gray, a glow on dark pages.
			dropShadow: `drop-shadow(1px 2px 2px rgb(0 0 0 / ${dark() ? 0.6 : 0.15}))`,
			background: backdrop(),
			// The font of the text, as <pre> takes the font of code.
			fontFamily: getComputedStyle(diagrams[0].parentElement).fontFamily,
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
	import("https://cdn.jsdelivr.net/npm/mermaid@12.2.0/dist/mermaid.esm.min.mjs")
		.then(({ default: mermaid }) => {
			const draw = () => {
				diagrams[0].parentElement.append(probe);
				// The front matter of a diagram can still set its own config.
				mermaid.initialize({
					startOnLoad: false,
					theme: "base",
					themeVariables: themeVariables(),
					// Mermaid 12's 120px label width splits lines such as "events,
					// templates". Version 11's 200px fit every label in guide/.
					flowchart: { wrappingWidth: 200 },
				});
				diagrams.forEach((d, i) => {
					d.removeAttribute("data-processed");
					d.textContent = themed(sources[i]);
				});
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
