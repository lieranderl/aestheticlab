/**
 * Renders the 1200 × 630 link-preview image for each locale into
 * `frontend/public/og/<locale>.jpg`. Run after changing the slogan or the photo:
 *
 *   bun run share-images
 */
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { chromium } from "@playwright/test";

const root = resolve(import.meta.dir, "..");
const fileUrl = (path: string) => pathToFileURL(join(root, path)).href;

/** Copy that only appears on the image; the slogan comes from the locale file. */
const LOCALE_COPY: Record<string, { services: string; city: string }> = {
	"en-BE": { services: "Nails · Brows · Lashes · Laser", city: "Leuven" },
	"nl-BE": {
		services: "Nagels · Wenkbrauwen · Wimpers · Laser",
		city: "Leuven",
	},
	"fr-BE": { services: "Ongles · Sourcils · Cils · Laser", city: "Louvain" },
	"ru-BE": { services: "Ногти · Брови · Ресницы · Лазер", city: "Лёвен" },
	"uk-BE": { services: "Нігті · Брови · Вії · Лазер", city: "Левен" },
};

const bird = readFileSync(join(root, "frontend/media/Bird.svg"), "utf8")
	.replace(/<title>.*?<\/title>/s, "")
	.replace("<svg ", '<svg class="bird" aria-hidden="true" ');

function slogan(locale: string): string {
	const app = JSON.parse(
		readFileSync(join(root, `internal/i18n/locales/${locale}.json`), "utf8"),
	);
	return app.app.hero.slogan;
}

function page(locale: string): string {
	const { services, city } = LOCALE_COPY[locale];
	return `<!doctype html>
<html lang="${locale.slice(0, 2)}">
<head>
<meta charset="utf-8">
<style>
@font-face{font-family:qestero;src:url("${fileUrl("frontend/public/fonts/QESTERO-Regular.ttf")}")}
@font-face{font-family:Cormorant;src:url("${fileUrl("frontend/public/fonts/cormorant-garamond-500.woff2")}")}
@font-face{font-family:Cormorant;src:url("${fileUrl("frontend/public/fonts/cormorant-garamond-500-cyrillic.woff2")}");unicode-range:U+0400-04FF}
@font-face{font-family:Onest;font-weight:100 900;src:url("${fileUrl("node_modules/@fontsource-variable/onest/files/onest-latin-wght-normal.woff2")}")}
@font-face{font-family:Onest;font-weight:100 900;src:url("${fileUrl("node_modules/@fontsource-variable/onest/files/onest-cyrillic-wght-normal.woff2")}");unicode-range:U+0400-04FF}
*{box-sizing:border-box;margin:0}
body{width:1200px;height:630px;display:grid;grid-template-columns:640px 1fr;background:#8b9687;color:#1c211b;font-family:Onest,sans-serif}
.photo{width:640px;height:630px;object-fit:cover;object-position:45% center;display:block}
.panel{padding:60px 56px 56px;display:flex;flex-direction:column;justify-content:space-between}
.bird{align-self:flex-start;width:58px;height:69px}
.bird path{fill:#1c211b}
.name{font-family:qestero,serif;font-size:84px;line-height:.95;letter-spacing:-.005em}
.slogan{margin-top:20px;font-family:Cormorant,serif;font-size:34px;line-height:1.1;color:#2b3328}
.meta{padding-top:20px;border-top:1px solid rgba(28,33,27,.45);display:grid;gap:8px;font-size:20px;font-weight:500;line-height:1.3}
</style>
</head>
<body>
<img class="photo" src="${fileUrl("frontend/media/hero/taupe-manicure.jpg")}" alt="">
<div class="panel">
${bird}
<div><p class="name">Aesthetic Lab</p><p class="slogan">${slogan(locale)}</p></div>
<div class="meta"><p>${services}</p><p>Diestsestraat 174 · ${city}</p></div>
</div>
</body>
</html>`;
}

const work = mkdtempSync(join(tmpdir(), "share-images-"));
const browser = await chromium.launch();
try {
	const tab = await browser.newPage({
		viewport: { width: 1200, height: 630 },
		deviceScaleFactor: 1,
	});
	for (const locale of Object.keys(LOCALE_COPY)) {
		const html = join(work, `${locale}.html`);
		writeFileSync(html, page(locale));
		await tab.goto(pathToFileURL(html).href, { waitUntil: "load" });
		await tab.evaluate(() => document.fonts.ready);
		const out = join(root, `frontend/public/og/${locale}.jpg`);
		await tab.screenshot({ path: out, type: "jpeg", quality: 86 });
		console.log(`wrote ${out}`);
	}
} finally {
	await browser.close();
	rmSync(work, { recursive: true, force: true });
}
