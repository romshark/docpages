// The theme script, which every page inlines in <head>, minified, so that it
// sets the chosen theme before the first paint. No choice follows the system.
// It applies the choice of the theme switch or the theme menu and marks it in both.
//
// <picture> selects <source> by the prefers-color-scheme of the system, not by
// this choice. Each source's media instead states the chosen theme as an
// always or never condition, or returns to its original media without a choice.

const root = document.documentElement;
const stored = () => {
	try {
		return localStorage.getItem("theme");
	} catch {
		return null;
	}
};
const scheme = /\(\s*prefers-color-scheme\s*:\s*(light|dark)\s*\)/g;
const fit = (source: HTMLSourceElement) => {
	const media = source.dataset.media ?? source.getAttribute("media") ?? "";
	if (!media.includes("prefers-color-scheme")) {
		return;
	}
	source.dataset.media = media;
	const theme = root.dataset.theme;
	const next = theme
		? media.replace(scheme, (_, s) =>
				s === theme ? "(width >= 0)" : "(width < 0)",
			)
		: media;
	if (source.getAttribute("media") !== next) {
		source.setAttribute("media", next);
	}
};
const apply = (choice: string | null) => {
	if (choice === "light" || choice === "dark") {
		root.dataset.theme = choice;
	} else {
		delete root.dataset.theme;
	}
	const current = root.dataset.theme ?? "system";
	for (const b of document.querySelectorAll<HTMLElement>(
		"[data-theme-choice]",
	)) {
		b.setAttribute("aria-pressed", String(b.dataset.themeChoice === current));
	}
	// Use the attribute because the select may not be upgraded yet.
	for (const select of document.querySelectorAll("neo-select.theme-select")) {
		select.setAttribute("value", current);
	}
	for (const source of document.querySelectorAll<HTMLSourceElement>(
		"picture > source[media]",
	)) {
		fit(source);
	}
};
const choose = (choice: string) => {
	try {
		if (choice === "system") {
			localStorage.removeItem("theme");
		} else {
			localStorage.setItem("theme", choice);
		}
	} catch {}
	apply(choice);
};
apply(stored());
// Apply choices to parser-added sources immediately so the system-theme
// image doesn't load first.
const parsing = new MutationObserver((records) => {
	for (const r of records) {
		for (const n of r.addedNodes) {
			if (
				n instanceof HTMLSourceElement &&
				n.parentNode?.nodeName === "PICTURE"
			) {
				fit(n);
			}
		}
	}
});
if (root.dataset.theme) {
	parsing.observe(root, { childList: true, subtree: true });
}
document.addEventListener("DOMContentLoaded", () => {
	parsing.disconnect();
	apply(stored());
});
document.addEventListener("click", (e) => {
	const button =
		e.target instanceof Element
			? e.target.closest<HTMLElement>("[data-theme-choice]")
			: null;
	if (button?.dataset.themeChoice) {
		choose(button.dataset.themeChoice);
	}
});
document.addEventListener("neo-select-change", (e) => {
	const { target, detail } = e as CustomEvent<{ value: string }>;
	if (target instanceof Element && target.matches(".theme-select")) {
		choose(detail.value);
	}
});
