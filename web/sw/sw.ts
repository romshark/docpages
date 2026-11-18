// Service worker of 1 docs version. It caches the offline.json groups that
// the reader picks and serves them offline. docpages prepends `const build`,
// so a new build installs a new worker, which saves the files of the new build
// and deletes those of the old one only then.

declare const self: ServiceWorkerGlobalScope;
declare const build: string;

// offline.json, as offline.go writes it.
interface OfflineFile {
	url: string;
	size: number;
	// The content, which a media copy holds in its URL instead.
	hash?: string;
}
interface OfflineIndex {
	build: string;
	groups: { name: string; size: number; files: OfflineFile[] }[];
}

// The state of the save survives a restarted worker. groups are the groups to hold,
// kept those complete before the save, which Cancel restores,
// and status is "saving" or "saved". error stops a save that a retry wouldn't
// complete, until a new save, Cancel or a new build.
interface SaveState {
	groups: string[];
	kept: string[];
	build: string;
	status: "saving" | "saved";
	error?: string;
}

const scope = self.registration.scope;
// GitHub Pages sites of 1 account share an origin.
// The scope keeps their caches apart.
const prefix = `docpages ${scope} `;
const filesCache = `${prefix}files ${build}`;
const stateCache = `${prefix}state`;
const stateKey = new URL("offline-state", scope).href;
const indexURL = new URL("offline.json", scope).href;
// Media copies hold a content hash in their names, so they never change.
// The pages of a docs group named media lie in media/ too, without the hash.
const mediaCopy = /^media\/[0-9a-f]{16}\/[^/]+$/;
// Pages load Mermaid and KaTeX from this CDN. A saved version caches what they load.
const cdn = [
	"https://cdn.jsdelivr.net/npm/mermaid@",
	"https://cdn.jsdelivr.net/npm/katex@",
];
// docs.ts listens on this channel.
const channel = new BroadcastChannel(prefix);

// caches.match doesn't create the cache, as open does.
const readState = async (): Promise<SaveState | null> => {
	const r = await caches.match(stateKey, { cacheName: stateCache });
	return r ? ((await r.json()) as SaveState) : null;
};
const writeState = async (state: SaveState) =>
	(await caches.open(stateCache)).put(
		stateKey,
		new Response(JSON.stringify(state), {
			headers: { "Content-Type": "application/json" },
		}),
	);

// The running save, its controller, its progress in bytes, and why the last one stopped.
let saving: Promise<void> | null = null;
let controller: AbortController | null = null;
let progress = { done: 0, total: 0 };
let failure = "";

const report = async () => {
	const state = await readState();
	channel.postMessage({
		status: state?.status ?? "none",
		groups: state?.groups ?? [],
		running: saving !== null,
		error: state?.error ?? failure,
		...progress,
	});
};

// At most 4 reports a second while files stream in.
let reportTimer = 0;
const reportSoon = () => {
	if (reportTimer) {
		return;
	}
	reportTimer = setTimeout(() => {
		reportTimer = 0;
		report();
	}, 250);
};

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

// serial runs messages and the activation 1 at a time.
// Each reads the state, awaits, then writes it,
// so a "cancel" beside a "save" could read the state before the save writes it.
let queue: Promise<unknown> = Promise.resolve();
const serial = (f: () => Promise<void>) => {
	const run = queue.then(f);
	queue = run.catch(() => {});
	return run;
};

self.addEventListener("install", () => self.skipWaiting());

self.addEventListener("activate", (e) => {
	e.waitUntil(
		serial(async () => {
			// Navigation requests start while the worker boots.
			await self.registration.navigationPreload?.enable();
			await self.clients.claim();
			const state = await readState();
			if (!state) {
				await deleteOldCaches();
			} else if (state.build !== build) {
				// The files of the old build stay until those of this one are saved.
				// Offline, pages come from them meanwhile.
				await writeState({
					groups: state.groups,
					kept: [],
					build,
					status: "saving",
				});
				resume();
			}
			await report();
		}),
	);
});

self.addEventListener("message", (e) => {
	switch (e.data?.type) {
		case "status":
			e.waitUntil(serial(() => status(true)));
			break;
		case "save":
			e.waitUntil(serial(() => start(e.data.groups)));
			break;
		case "keep":
			e.waitUntil(serial(() => keep(e.data.groups)));
			break;
		case "cancel":
			e.waitUntil(serial(cancel));
			break;
		case "remove":
			e.waitUntil(serial(remove));
			break;
		case "keepalive":
			// Pages send this during a save, as an idle worker stops after about
			// 30 s and 1 event keeps it up at most 5 min.
			// It also resumes a stopped save, after the backoff.
			e.waitUntil(
				Promise.race([
					serial(() => status(false)).then(() => saving),
					sleep(25_000),
				]),
			);
			break;
	}
});

// status reports the state and resumes a save that a retry may complete:
// now, as when a page loads or the network returns, or after the backoff.
// Evicted files, as Safari evicts after 7 days without a visit, turn the
// state back to saving.
const status = async (now: boolean) => {
	const state = await readState();
	if (state?.status === "saved") {
		const kept = await completeGroups(state.groups);
		if (kept.length < state.groups.length) {
			await writeState({ ...state, kept, status: "saving" });
		}
	}
	const next = await readState();
	if (
		next?.status === "saving" &&
		!next.error &&
		(now || Date.now() >= retryAt)
	) {
		resume();
	}
	await report();
};

// A save that a network error stopped waits 20 s before a keepalive resumes it,
// twice that after each further error, up to 5 min.
let retries = 0;
let retryAt = 0;
const backOff = () => {
	retryAt = Date.now() + Math.min(20_000 * 2 ** retries, 300_000);
	retries++;
};

// StatusError is a response of url with a status other than 2xx.
class StatusError extends Error {
	constructor(
		url: string,
		readonly status: number,
	) {
		super(`${url}: ${status}`);
	}
}

// permanent reports whether err stops a save that a retry wouldn't complete:
// a full storage, or a response of 4xx other than a timeout or a rate limit.
const permanent = (err: unknown) =>
	(err instanceof DOMException && err.name === "QuotaExceededError") ||
	(err instanceof StatusError &&
		err.status >= 400 &&
		err.status < 500 &&
		err.status !== 408 &&
		err.status !== 429);

// keptGroups returns the groups that Cancel restores in state.
const keptGroups = (state: SaveState | null) =>
	state?.status === "saved" ? state.groups : (state?.kept ?? []);

// start saves groups. The complete groups stay until the save completes, for Cancel.
// A running save stops first, as it would finish with its own groups.
const start = async (groups: string[]) => {
	await stop();
	const kept = keptGroups(await readState());
	await writeState({ groups, kept, build, status: "saving" });
	failure = "";
	retries = 0;
	retryAt = 0;
	resume();
	await report();
};

// stopped aborts a save without telling the page.
const stopped = "stopped";
const stop = async () => {
	controller?.abort(stopped);
	await saving;
};

// cancel stops a save and restores the groups complete before it, if any.
const cancel = async () => {
	await stop();
	const kept = keptGroups(await readState());
	const index = await caches.match(indexURL, { cacheName: filesCache });
	if (kept.length === 0 || !index) {
		return remove();
	}
	await prune((await index.json()) as OfflineIndex, kept);
	await writeState({ groups: kept, kept, build, status: "saved" });
	progress = { done: 0, total: 0 };
	failure = "";
	await report();
};

// keep deletes the saved files outside groups, without a download.
// Groups that aren't all saved need one, so they start a save.
const keep = async (groups: string[]) => {
	const state = await readState();
	const index = await caches.match(indexURL, { cacheName: filesCache });
	if (
		state?.status !== "saved" ||
		!index ||
		!groups.every((g) => state.groups.includes(g))
	) {
		return start(groups);
	}
	await prune((await index.json()) as OfflineIndex, groups);
	await writeState({ ...state, groups, kept: groups });
	await report();
};

// remove stops a save and deletes the saved files.
const remove = async () => {
	await stop();
	for (const name of await caches.keys()) {
		if (name.startsWith(prefix)) {
			await caches.delete(name);
		}
	}
	progress = { done: 0, total: 0 };
	failure = "";
	await report();
};

const resume = () => {
	saving ??= save().finally(() => {
		saving = null;
		controller = null;
		report();
	});
	return saving;
};

// files returns the files of groups in index.
const files = (index: OfflineIndex, groups: string[]) =>
	index.groups.filter((g) => groups.includes(g.name)).flatMap((g) => g.files);

// decode returns s with its escapes decoded, or s itself if one is invalid.
const decode = (s: string) => {
	try {
		return decodeURIComponent(s);
	} catch {
		return s;
	}
};

// key returns the cache key of url in scope, without query and fragment,
// and with each path segment escaped 1 way. offline.json escapes ' and (,
// which page links and the browser leave as they are. Servers decode either form.
const key = (url: string) => {
	const u = new URL(url, scope);
	u.search = "";
	u.hash = "";
	const path = u.href.slice(scope.length).split("/");
	return scope + path.map((s) => encodeURIComponent(decode(s))).join("/");
};

// oldCaches returns the names of the files caches of other builds.
const oldCaches = async () =>
	(await caches.keys()).filter(
		(name) => name.startsWith(`${prefix}files `) && name !== filesCache,
	);

const deleteOldCaches = async () => {
	for (const name of await oldCaches()) {
		await caches.delete(name);
	}
};

// cached returns the saved response for url, of this build,
// or of an older one while this build's save is incomplete.
const cached = async (url: string) => {
	for (const name of [filesCache, ...(await oldCaches())]) {
		const r = await caches.match(url, { cacheName: name, ignoreVary: true });
		if (r) {
			return r;
		}
	}
	return undefined;
};

// moveUnchanged moves into cache the files of groups in index that an older build
// saved with the same content, and the files of the CDN, whose URLs name their versions.
// Moving, not copying, keeps the storage that 2 builds take at about
// that of 1. The old files that remain serve offline reads until the save completes.
const moveUnchanged = async (
	cache: Cache,
	index: OfflineIndex,
	groups: string[],
) => {
	const have = new Set((await cache.keys()).map((r) => r.url));
	const move = async (old: Cache, url: string) => {
		const r = await old.match(url);
		if (r && !have.has(url)) {
			await cache.put(url, r);
			have.add(url);
		}
		await old.delete(url);
	};
	for (const name of await oldCaches()) {
		const old = await caches.open(name);
		const hashes = new Map<string, string>();
		const oldIndex = await old.match(indexURL);
		if (oldIndex) {
			const listed = (await oldIndex.json()) as OfflineIndex;
			for (const f of files(
				listed,
				listed.groups.map((g) => g.name),
			)) {
				if (f.hash) {
					hashes.set(key(f.url), f.hash);
				}
			}
		}
		for (const f of files(index, groups)) {
			const url = key(f.url);
			const same = f.hash
				? hashes.get(url) === f.hash
				: mediaCopy.test(url.slice(scope.length));
			if (same) {
				await move(old, url);
			}
		}
		for (const r of await old.keys()) {
			if (cdn.some((p) => r.url.startsWith(p))) {
				await move(old, r.url);
			}
		}
	}
};

// completeGroups returns the groups whose files are all cached.
const completeGroups = async (groups: string[]) => {
	const index = await caches.match(indexURL, { cacheName: filesCache });
	if (!index) {
		return [];
	}
	const listed = (await index.json()) as OfflineIndex;
	const cache = await caches.open(filesCache);
	const have = new Set((await cache.keys()).map((r) => r.url));
	return groups.filter((g) =>
		files(listed, [g]).every((f) => have.has(key(f.url))),
	);
};

// prune deletes the files outside groups, but keeps offline.json and the CDN.
const prune = async (index: OfflineIndex, groups: string[]) => {
	const cache = await caches.open(filesCache);
	const wanted = new Set(files(index, groups).map((f) => key(f.url)));
	for (const r of await cache.keys()) {
		if (r.url.startsWith(scope) && r.url !== indexURL && !wanted.has(r.url)) {
			await cache.delete(r);
		}
	}
};

const save = async () => {
	const abort = new AbortController();
	controller = abort;
	const { signal } = abort;
	failure = "";
	// Until offline.json arrives, report no progress, not the last save's.
	progress = { done: 0, total: 0 };
	try {
		const state = await readState();
		if (state?.status !== "saving") {
			return;
		}
		const response = await fetch(indexURL, { cache: "no-store", signal });
		if (!response.ok) {
			throw new StatusError("offline.json", response.status);
		}
		const index = (await response.clone().json()) as OfflineIndex;
		if (index.build !== build) {
			// A newer build's worker takes over. An older one, as a CDN can
			// serve after a deploy, is retried at the next keepalive.
			failure = "Waiting for the new version of the docs";
			await self.registration.update();
			return;
		}
		const cache = await caches.open(filesCache);
		await cache.put(indexURL, response);
		await moveUnchanged(cache, index, state.groups);
		const have = new Set((await cache.keys()).map((r) => r.url));
		const missing: OfflineFile[] = [];
		for (const f of files(index, state.groups)) {
			const url = key(f.url);
			progress.total += f.size;
			if (have.has(url)) {
				progress.done += f.size;
			} else {
				missing.push({ url, size: f.size });
			}
		}
		await report();
		const worker = async () => {
			for (let f = missing.shift(); f; f = missing.shift()) {
				await fetchInto(cache, f, signal);
			}
		};
		// 1 failed file stops the others, which would run beside a resumed save.
		await Promise.all([worker(), worker(), worker(), worker()]).catch((err) => {
			abort.abort(err);
			throw err;
		});
		// Unchecked groups go only now, so that Cancel keeps them.
		await prune(index, state.groups);
		await writeState({ ...state, kept: state.groups, build, status: "saved" });
		await deleteOldCaches();
		retries = 0;
	} catch (err) {
		if (signal.reason === stopped) {
			return;
		}
		failure = `Stopped: ${err instanceof Error ? err.message : err}`;
		const now = await readState();
		if (permanent(err) && now?.status === "saving") {
			await writeState({ ...now, error: failure });
		} else {
			backOff();
		}
	}
};

// fetchInto caches file f, counting its bytes as they arrive.
const fetchInto = async (cache: Cache, f: OfflineFile, signal: AbortSignal) => {
	const response = await fetch(f.url, { cache: "no-cache", signal });
	if (!response.ok || !response.body) {
		throw new StatusError(f.url, response.status);
	}
	let got = 0;
	const body = response.body.pipeThrough(
		new TransformStream<Uint8Array, Uint8Array>({
			transform(chunk, c) {
				got += chunk.byteLength;
				progress.done += chunk.byteLength;
				reportSoon();
				c.enqueue(chunk);
			},
		}),
	);
	// The body is decoded, which voids the encoding and length of the network.
	// Vary would block matches.
	const headers = new Headers(response.headers);
	for (const h of ["Content-Encoding", "Content-Length", "Vary"]) {
		headers.delete(h);
	}
	try {
		await cache.put(f.url, new Response(body, { status: 200, headers }));
	} catch (err) {
		progress.done -= got;
		throw err;
	}
};

self.addEventListener("fetch", (e) => {
	const { request } = e;
	if (request.method !== "GET") {
		return;
	}
	if (request.url.startsWith(scope)) {
		e.respondWith(serve(request, e.preloadResponse));
	} else if (cdn.some((p) => request.url.startsWith(p))) {
		e.respondWith(serveCDN(request));
	}
});

// serve answers from the network, so online readers get the deployed files,
// and from the cache offline, except media copies, which the cache answers first.
// preload is the navigation preload response, or undefined.
const serve = async (
	request: Request,
	preload: Promise<Response | undefined>,
) => {
	const url = key(request.url);
	if (mediaCopy.test(url.slice(scope.length))) {
		const response = await cached(url);
		if (response) {
			return ranged(request, response);
		}
	}
	try {
		return (await preload) ?? (await fetch(request));
	} catch (err) {
		const response = await cached(url);
		if (response) {
			return ranged(request, response);
		}
		throw err;
	}
};

const serveCDN = async (request: Request) => {
	if (!(await readState())) {
		return fetch(request);
	}
	const saved = await cached(request.url);
	if (saved) {
		return saved;
	}
	const response = await fetch(request);
	if (response.ok) {
		await (await caches.open(filesCache)).put(request.url, response.clone());
	}
	return response;
};

// ranged answers a Range request, as <video> makes, from the cached response.
// Safari plays no video without it.
const ranged = async (request: Request, response: Response) => {
	const m = /^bytes=(\d*)-(\d*)$/.exec(request.headers.get("Range") ?? "");
	if (!m || (m[1] === "" && m[2] === "")) {
		return response;
	}
	const body = await response.blob();
	const size = body.size;
	let start = Number(m[1]);
	let end = m[2] === "" ? size - 1 : Math.min(Number(m[2]), size - 1);
	if (m[1] === "") {
		start = Math.max(size - Number(m[2]), 0);
		end = size - 1;
	}
	if (start > end) {
		return new Response(null, {
			status: 416,
			headers: { "Content-Range": `bytes */${size}` },
		});
	}
	return new Response(body.slice(start, end + 1), {
		status: 206,
		headers: {
			"Content-Type": response.headers.get("Content-Type") ?? "",
			"Content-Range": `bytes ${start}-${end}/${size}`,
			"Content-Length": String(end - start + 1),
		},
	});
};
