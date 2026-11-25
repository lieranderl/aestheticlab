import type { Page } from "@playwright/test";
import { expect, test } from "./fixtures";

const consentStorageKey = "aestheticlab_cookie_consent_v2";
const vitals = ["CLS", "FCP", "INP", "LCP", "TTFB"];

async function consent(page: Page, analytics: boolean) {
	await page.addInitScript(
		([key, value]) => {
			window.localStorage.setItem(
				key as string,
				JSON.stringify({
					version: 1,
					analytics: value,
					updatedAt: new Date().toISOString(),
				}),
			);
		},
		[consentStorageKey, analytics],
	);
}

/** Google Analytics events so far whose name passes keep. */
async function events(page: Page, keep: string[]) {
	return page.evaluate(
		(names) =>
			(window as unknown as { dataLayer: IArguments[] }).dataLayer
				.map((entry) => Array.from(entry))
				.filter(
					([command, name]) =>
						command === "event" && names.includes(String(name)),
				)
				.map(([, name, params]) => ({ name: String(name), params })),
		keep,
	);
}

/** Hides the page as a tab switch would: web-vitals reports then. */
async function hide(page: Page) {
	await page.evaluate(() => {
		Object.defineProperty(document, "visibilityState", {
			value: "hidden",
			configurable: true,
		});
		document.dispatchEvent(new Event("visibilitychange"));
	});
}

test("reports Web Vitals after analytics consent", async ({ page }) => {
	await consent(page, true);
	await page.goto("/en-BE/");
	await expect(page.locator("script[src*='web-vitals']")).toHaveCount(1);
	await page.locator("h1").first().click();
	await hide(page);

	await expect
		.poll(async () => (await events(page, vitals)).map((e) => e.name))
		.toEqual(expect.arrayContaining(["FCP", "TTFB"]));
	for (const { name, params } of await events(page, vitals)) {
		expect(params, name).toMatchObject({
			metric_id: expect.any(String),
			metric_value: expect.any(Number),
			metric_delta: expect.any(Number),
			metric_rating: expect.stringMatching(/^(good|needs-improvement|poor)$/),
			value: expect.any(Number),
		});
		expect(Number.isInteger(params.value), name).toBe(true);
	}
});

test("loads no Web Vitals code and reports nothing without consent", async ({
	page,
}) => {
	await consent(page, false);
	await page.goto("/en-BE/");
	await page.locator("h1").first().click();
	await hide(page);

	await expect(page.locator("script[src*='web-vitals']")).toHaveCount(0);
	expect(await events(page, vitals)).toEqual([]);
});

test("starts reporting Web Vitals once the visitor accepts", async ({
	page,
}) => {
	await page.goto("/en-BE/");
	await expect(page.locator("script[src*='web-vitals']")).toHaveCount(0);
	await page.getByRole("button", { name: "Accept analytics" }).click();
	await expect(page.locator("script[src*='web-vitals']")).toHaveCount(1);
	await hide(page);

	await expect
		.poll(async () => (await events(page, vitals)).map((e) => e.name))
		.toContain("TTFB");
});

test("records errors in the site's own scripts, without query strings", async ({
	page,
}) => {
	await consent(page, true);
	await page.goto("/en-BE/");
	await page.evaluate(() => {
		const raise = (message: string, filename: string) =>
			window.dispatchEvent(
				new ErrorEvent("error", {
					message,
					filename,
					lineno: 12,
					colno: 3,
					error: new Error(message),
				}),
			);
		const ours = `${location.origin}/static/dist/site.abc.js?v=1`;
		raise("TypeError: x is undefined at https://a.test/?email=me@x.be", ours);
		raise("TypeError: x is undefined at https://a.test/?email=me@x.be", ours);
		raise("Script error.", "");
		raise("from an extension", "chrome-extension://abc/content.js");
		window.dispatchEvent(
			new PromiseRejectionEvent("unhandledrejection", {
				promise: Promise.resolve(),
				reason: new Error("load failed for /api/chat?token=1"),
			}),
		);
		for (let i = 0; i < 10; i++) raise(`Error ${i}`, ours);
	});

	const exceptions = await events(page, ["exception"]);
	expect(exceptions[0]?.params).toMatchObject({
		description:
			"TypeError: x is undefined at https://a.test/ @ /static/dist/site.abc.js:12:3",
		fatal: false,
	});
	expect(exceptions[1]?.params).toMatchObject({
		description: "unhandledrejection: load failed for /api/chat",
		fatal: false,
	});
	expect(exceptions).toHaveLength(5);
	for (const { params } of exceptions) {
		expect(params.description.length).toBeLessThanOrEqual(100);
		expect(params.description).not.toContain("?");
	}
});

test("records no errors without analytics consent", async ({ page }) => {
	await consent(page, false);
	await page.goto("/en-BE/");
	await page.evaluate(() =>
		window.dispatchEvent(
			new ErrorEvent("error", {
				message: "boom",
				filename: `${location.origin}/static/dist/site.js`,
			}),
		),
	);
	expect(await events(page, ["exception"])).toEqual([]);
});
