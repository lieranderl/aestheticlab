import { expect, type Page, test } from "@playwright/test";

// A tab opened on the Qwik site before the cutover keeps its Qwik code. Each
// test loads a Qwik page through e2e/handoff/proxy.ts, then points the proxy
// at the Go server, as the promotion does.

const qwikPage = "html[q\\:container]";
const goPage = 'script[src^="/static/dist/datastar."]';

async function openQwikTab(page: Page, path: string) {
	const errors: Error[] = [];
	page.on("pageerror", (error) => errors.push(error));
	await page.request.put("/__handoff/qwik");
	await page.goto(path);
	await expect(page.locator(qwikPage)).toHaveCount(1);
	await page.request.put("/__handoff/go");
	return errors;
}

test("every Qwik asset URL a page references reaches its Go file", async ({
	page,
}) => {
	await page.request.put("/__handoff/qwik");
	const html = (
		await Promise.all(
			["/en-BE/", "/en-BE/pricelist/"].map(async (path) =>
				(await page.request.get(path)).text(),
			),
		)
	).join("\n");
	// Images, posters and clips; Qwik's own CSS and bundle graph have no
	// Go counterpart.
	const urls = [
		...new Set(html.match(/\/assets\/[\w-]+\.(?:webp|jpe?g|png|svg|mp4)/g)),
	];
	expect(urls.length).toBeGreaterThan(20);

	await page.request.put("/__handoff/go");
	for (const url of urls) {
		const response = await page.request.get(url);
		expect(response.status(), url).toBe(200);
		expect(response.url(), url).toContain("/static/dist/");
		expect(response.headers()["content-type"], url).toMatch(/^(image|video)\//);
	}
});

test("a link in an open tab leads to the Go site", async ({ page }) => {
	const errors = await openQwikTab(page, "/en-BE/");

	// The tab may already have reloaded into the Go page (a chunk it loaded
	// after the switch); either way, its links reach the Go site.
	await page.locator('a[href*="/pricelist"]:visible').first().click();
	await expect(page).toHaveURL(/\/en-BE\/pricelist\/$/);
	await expect(page.locator(goPage)).toHaveCount(1);
	await expect(page.locator(qwikPage)).toHaveCount(0);
	expect(errors).toEqual([]);
});

test("a Qwik chunk loaded after the cutover reloads the tab into the Go page", async ({
	page,
}) => {
	const errors = await openQwikTab(page, "/fr-BE/");

	// As Qwik loads a chunk it has not preloaded: with import().
	await page.evaluate(() => {
		void import(`${location.origin}/build/fr-BE/q-not-preloaded.js`);
	});
	await expect(page.locator(goPage)).toHaveCount(1);
	await expect(page.locator(qwikPage)).toHaveCount(0);
	await expect(page).toHaveURL(/\/fr-BE\/$/);
	expect(errors).toEqual([]);
});

test("the Qwik service worker removes itself", async ({
	page,
	browserName,
}) => {
	// biome-ignore lint/suspicious/noSkippedTests: Chromium cannot run it here.
	test.skip(
		browserName === "chromium",
		"Chromium registers no service worker over the self-signed certificate",
	);
	await page.request.put("/__handoff/qwik");
	await page.goto("/en-BE/");
	await page.evaluate(() => navigator.serviceWorker.ready);
	await page.request.put("/__handoff/go");

	// Browsers check the worker for updates on navigation and every day.
	// The update can settle as a failure once the new worker has
	// unregistered itself, so only the outcome counts.
	await page.evaluate(async () => {
		const registration = await navigator.serviceWorker.getRegistration();
		await registration?.update().catch(() => undefined);
	});
	await expect
		.poll(() =>
			page.evaluate(
				async () => (await navigator.serviceWorker.getRegistrations()).length,
			),
		)
		.toBe(0);
});
