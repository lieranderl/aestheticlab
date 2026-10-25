import { expect, test } from "./fixtures";

test("pricelist shows one category at a time with expandable treatments", async ({
	page,
}) => {
	await page.goto("/en-BE/pricelist");

	await expect(page).toHaveURL(/\/en-BE\/pricelist\/?$/);
	await expect(
		page.getByRole("heading", { name: "Prices & time", level: 1 }),
	).toBeVisible({ timeout: 15000 });

	const tabs = page.getByRole("navigation", { name: "Service categories" });
	await expect(tabs.locator('a[aria-current="true"]')).toHaveText(/Manicure/);

	// Add-ons are listed apart from the treatments.
	await expect(page.locator("main")).toContainText("Add-ons");

	const firstRow = page
		.locator('main li button[aria-controls^="price-row-"]')
		.first();
	await expect(firstRow).toHaveAttribute("aria-expanded", "false");
	await firstRow.click();
	await expect(firstRow).toHaveAttribute("aria-expanded", "true");
	await expect(
		page.getByRole("button", { name: "Book this treatment" }).first(),
	).toBeVisible();

	// Works with mock (CI) and live (local) data alike: use the second tab.
	const second = tabs.getByRole("link").nth(1);
	const name = (await second.innerText()).replace(/^\d+\s*/, "").trim();
	await second.click();
	await expect(page).toHaveURL(/category=/);
	await expect(second).toHaveAttribute("aria-current", "true");
	await expect(
		page.getByRole("heading", { level: 2 }).filter({ hasText: name }),
	).toBeVisible();
});

test("every category is in the HTML and its link works without JavaScript", async ({
	browser,
}) => {
	const context = await browser.newContext({ javaScriptEnabled: false });
	const page = await context.newPage();
	await page.goto("/en-BE/pricelist/");

	const links = page
		.getByRole("navigation", { name: "Service categories" })
		.getByRole("link");
	const sections = page.locator('main section[aria-labelledby^="pricelist-"]');
	const count = await links.count();
	expect(count).toBeGreaterThan(1);
	await expect(sections).toHaveCount(count);
	// Search engines don't click tabs: each category ships its treatments.
	for (let index = 0; index < count; index++) {
		expect(
			await sections
				.nth(index)
				.locator('button[aria-controls^="price-row-"]')
				.count(),
		).toBeGreaterThan(0);
	}

	const second = links.nth(1);
	await expect(second).toHaveAttribute("href", /^\?category=/);
	await second.click();
	await expect(page).toHaveURL(/category=/);
	await expect(sections.nth(1)).toBeVisible();
	await expect(sections.nth(0)).toBeHidden();
	await context.close();
});
