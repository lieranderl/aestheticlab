import { expect, test } from "./fixtures";

// biome-ignore lint/suspicious/noSkippedTests: these regressions target only the Go frontend.
test.skip(({ baseURL }) => !baseURL?.includes(":43123"), "Go frontend");

test("lightbox loads on demand and preserves each photo's dimensions", async ({
	page,
}) => {
	await page.goto("/en-BE/");
	const dialog = page.locator("#gallery-lightbox");
	const image = dialog.locator("img");
	await expect(image).not.toHaveAttribute("src");
	await page.locator("#gallery-lightbox-trigger-0").click();
	await expect(image).toHaveAttribute("src", /pink-shimmer/);
	await expect(image).toHaveAttribute("width", /^[1-9]\d*$/);
	await expect(image).toHaveAttribute("height", /^[1-9]\d*$/);
	await dialog.getByRole("button", { name: "Next" }).click();
	await expect(image).toHaveAttribute("src", /chrome-manicure/);
	await expect
		.poll(() =>
			image.evaluate(
				(img: HTMLImageElement) => img.complete && img.naturalWidth > 0,
			),
		)
		.toBe(true);
	await page.keyboard.press("Escape");
	await expect(image).not.toHaveAttribute("src");
});

test("services expose pending state and keep the selected content until it arrives", async ({
	page,
}) => {
	await page.emulateMedia({ reducedMotion: "reduce" });
	await page.goto("/en-BE/?treatment=635112#services");
	const { promise: gate, resolve: release } = Promise.withResolvers<void>();
	await page.route("**/en-BE/?treatment=**", async (route) => {
		if (route.request().headers()["datastar-request"] === "true") await gate;
		await route.continue();
	});
	await page
		.locator("#service-category-nav")
		.getByRole("button", { name: "Pedicure", exact: true })
		.click();
	await expect(page.locator("#services")).toHaveAttribute("aria-busy", "true");
	await expect(page.locator("#service-details-heading")).toHaveText("Manicure");
	release();
	await expect(page.locator("#service-details-heading")).toHaveText("Pedicure");
	await expect(page.locator("#services")).toHaveAttribute("aria-busy", "false");
});

test("primary action is immediate and reduced motion disables decorative animation", async ({
	page,
}) => {
	await page.goto("/en-BE/");
	const action = page.locator("#hero-book-btn");
	const styles = await action.evaluate((el) => {
		if (!el.parentElement) throw new Error("Missing action container");
		const parent = getComputedStyle(el.parentElement);
		return { animation: parent.animationName, opacity: parent.opacity };
	});
	expect(styles).toEqual({ animation: "none", opacity: "1" });
	await page.emulateMedia({ reducedMotion: "reduce" });
	await expect
		.poll(() =>
			page
				.locator(".fade-up")
				.first()
				.evaluate((el) => getComputedStyle(el).opacity),
		)
		.toBe("1");
});

test("centering a category chip never scrolls its heading behind the header", async ({
	page,
}) => {
	await page.setViewportSize({ width: 390, height: 320 });
	await page.emulateMedia({ reducedMotion: "reduce" });
	await page.goto("/en-BE/?treatment=635112#services");
	await page
		.locator("#service-category-nav")
		.getByRole("button", { name: "Pedicure", exact: true })
		.click();
	await expect(page.locator("#service-details-heading")).toHaveText("Pedicure");
	await expect
		.poll(() =>
			page
				.locator("#service-details-card")
				.evaluate((el) => el.getBoundingClientRect().top),
		)
		.toBeGreaterThanOrEqual(90);
});

test("a slow gallery image cannot overwrite a newer selection or a closed dialog", async ({
	page,
}) => {
	const { promise: gate, resolve: release } = Promise.withResolvers<void>();
	await page.route("**/cherry-gloss-*.webp", async (route) => {
		await gate;
		await route.continue();
	});
	await page.goto("/en-BE/");
	await page.locator("#gallery-lightbox-trigger-0").click();
	const dialog = page.locator("#gallery-lightbox");
	const image = dialog.locator("img");
	await expect(image).toHaveAttribute("src", /pink-shimmer/);
	await dialog.getByRole("button", { name: "Next" }).click();
	await expect(image).toHaveAttribute("src", /chrome-manicure/);
	await dialog.getByRole("button", { name: "Next" }).click();
	await expect(dialog).toHaveAttribute("aria-busy", "true");
	await dialog.getByRole("button", { name: "Previous" }).click();
	await expect(dialog).toHaveAttribute("aria-busy", "false");
	await expect(image).toHaveAttribute("src", /chrome-manicure/);
	await page.keyboard.press("Escape");
	release();
	await expect(image).not.toHaveAttribute("src");
});

test("services still switch without the View Transition API", async ({
	page,
}) => {
	await page.addInitScript(() =>
		Reflect.deleteProperty(Document.prototype, "startViewTransition"),
	);
	await page.goto("/en-BE/?treatment=635112#services");
	await page
		.locator("#service-category-nav")
		.getByRole("button", { name: "Pedicure", exact: true })
		.click();
	await expect(page.locator("#service-details-heading")).toHaveText("Pedicure");
	await expect(page.locator("#services")).toHaveAttribute("aria-busy", "false");
});
