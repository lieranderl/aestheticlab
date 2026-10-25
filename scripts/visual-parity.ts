// Phase 6 parity check: full-page screenshots of the Qwik and Go production
// builds, every page × locale × viewport, and a pixel diff of each pair.
//
//   PORT=43200 bun server/entry.bun.js         # Qwik, reads Supabase from .env
//   PORT=43201 FIRESTORE_DATABASE= ./bin/web   # Go, embedded snapshot
//   bun scripts/visual-parity.ts [qwikURL] [goURL]
//
// PAGES=home,pricelist limits the run; results merge into summary.json.
// Output goes to test-results/visual-parity/ (summary.json, <stack>/*.png, diff/*.png).
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { type Browser, chromium, type Page } from "@playwright/test";
import sharp from "sharp";

const stacks = {
	qwik: process.argv[2] ?? "http://localhost:43200",
	go: process.argv[3] ?? "http://localhost:43201",
};
const locales = ["en-BE", "nl-BE", "fr-BE", "ru-BE", "uk-BE"];
const pages = {
	home: "/",
	pricelist: "/pricelist/",
	privacy: "/privacy-policy/",
	notice: "/notice/",
	"not-found": "/missing-page/",
};
const viewports = {
	desktop: { width: 1280, height: 800, isMobile: false },
	mobile: { width: 390, height: 844, isMobile: true },
};
// A channel difference above this counts; below it is font anti-aliasing.
const channelTolerance = 32;
const out = "test-results/visual-parity";
const thirdParty =
	/^https:\/\/(?:www\.googletagmanager\.com|(?:[\w-]+\.)?google-analytics\.com|bookings\.gettimely\.com)\//;

type Shot = { file: string; errors: string[] };

async function settle(page: Page) {
	// Walk the page so lazy images and in-view effects run, then return.
	await page.evaluate(async () => {
		const step = window.innerHeight / 2;
		for (let y = 0; y < document.documentElement.scrollHeight; y += step) {
			window.scrollTo({ top: y, behavior: "instant" });
			await new Promise((r) => setTimeout(r, 60));
		}
		window.scrollTo({ top: 0, behavior: "instant" });
	});
	await page.waitForLoadState("networkidle", { timeout: 10_000 }).catch(() => {});
	await page.evaluate(async () => {
		await document.fonts.ready;
		// decode() never settles for an image without a source.
		const timeout = new Promise((r) => setTimeout(r, 5000));
		await Promise.race([
			timeout,
			Promise.all(
				[...document.images]
					.filter((image) => image.currentSrc && !image.complete)
					.map((image) => image.decode().catch(() => null)),
			),
		]);
		for (const video of document.querySelectorAll("video")) {
			video.pause();
		}
	});
	await page.waitForTimeout(400);
}

async function capture(
	browser: Browser,
	stack: keyof typeof stacks,
	viewport: keyof typeof viewports,
	locale: string,
	name: keyof typeof pages,
): Promise<Shot> {
	const { width, height, isMobile } = viewports[viewport];
	const context = await browser.newContext({
		viewport: { width, height },
		isMobile,
		hasTouch: isMobile,
		deviceScaleFactor: 1,
		reducedMotion: "reduce",
		colorScheme: "light",
		locale,
	});
	await context.route(thirdParty, (route) =>
		route.fulfill({ status: 200, body: "" }),
	);
	const page = await context.newPage();
	const errors: string[] = [];
	page.on("pageerror", (error) => errors.push(`pageerror: ${error.message}`));
	page.on("console", (message) => {
		if (message.type() === "error") errors.push(message.text());
	});
	await page.goto(`${stacks[stack]}/${locale}${pages[name]}`, {
		waitUntil: "load",
	});
	await settle(page);
	const file = join(out, stack, `${name}-${locale}-${viewport}.png`);
	await page.screenshot({ path: file, fullPage: true, animations: "disabled" });
	await context.close();
	return { file, errors };
}

async function diff(a: string, b: string, target: string) {
	const [left, right] = await Promise.all(
		[a, b].map((file) =>
			sharp(file).ensureAlpha().raw().toBuffer({ resolveWithObject: true }),
		),
	);
	const width = Math.min(left.info.width, right.info.width);
	const height = Math.min(left.info.height, right.info.height);
	const marked = Buffer.alloc(width * height * 4);
	let changed = 0;
	const rows: number[] = [];
	for (let y = 0; y < height; y++) {
		let rowChanged = false;
		for (let x = 0; x < width; x++) {
			const i = (y * left.info.width + x) * 4;
			const j = (y * right.info.width + x) * 4;
			const o = (y * width + x) * 4;
			let delta = 0;
			for (let c = 0; c < 3; c++) {
				delta = Math.max(delta, Math.abs(left.data[i + c] - right.data[j + c]));
			}
			const gray = (left.data[i] + left.data[i + 1] + left.data[i + 2]) / 3;
			if (delta > channelTolerance) {
				changed++;
				rowChanged = true;
				marked.set([255, 0, 0, 255], o);
			} else {
				const faded = 200 + gray / 5;
				marked.set([faded, faded, faded, 255], o);
			}
		}
		if (rowChanged) rows.push(y);
	}
	if (changed > 0) {
		await sharp(marked, { raw: { width, height, channels: 4 } })
			.png()
			.toFile(target);
	}
	// Collapse changed rows into bands, merging gaps under 20 px.
	const bands: [number, number][] = [];
	for (const y of rows) {
		const last = bands.at(-1);
		if (last && y - last[1] < 20) last[1] = y;
		else bands.push([y, y]);
	}
	return {
		qwikSize: [left.info.width, left.info.height],
		goSize: [right.info.width, right.info.height],
		changedPercent: +((changed / (width * height)) * 100).toFixed(3),
		bands,
	};
}

for (const directory of ["qwik", "go", "diff"]) {
	mkdirSync(join(out, directory), { recursive: true });
}
const browser = await chromium.launch();
const selected = (process.env.PAGES?.split(",") ?? Object.keys(pages)) as (keyof typeof pages)[];
const summaryFile = join(out, "summary.json");
const previous: { id: string }[] = existsSync(summaryFile)
	? JSON.parse(readFileSync(summaryFile, "utf8"))
	: [];
const summary = previous.filter(
	(entry) => !selected.some((name) => entry.id.startsWith(`${name}-`)),
);
for (const name of selected) {
	for (const locale of locales) {
		for (const viewport of Object.keys(viewports) as (keyof typeof viewports)[]) {
			const qwik = await capture(browser, "qwik", viewport, locale, name);
			const go = await capture(browser, "go", viewport, locale, name);
			const id = `${name}-${locale}-${viewport}`;
			const result = await diff(qwik.file, go.file, join(out, "diff", `${id}.png`));
			summary.push({ id, ...result, qwikErrors: qwik.errors, goErrors: go.errors });
			console.log(
				id.padEnd(30),
				`${result.changedPercent}%`.padStart(9),
				`h ${result.qwikSize[1]} / ${result.goSize[1]}`,
				go.errors.length ? `go errors: ${go.errors.length}` : "",
			);
		}
	}
}
await browser.close();
writeFileSync(summaryFile, JSON.stringify(summary, null, 2));
