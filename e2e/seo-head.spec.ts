import { expect, test } from "./fixtures";

test("uses production canonical and localized alternate URLs", async ({
	page,
}) => {
	await page.goto("/fr-BE/pricelist/?campaign=ignored#booking");

	await expect(page.locator('link[rel="canonical"]')).toHaveAttribute(
		"href",
		"https://aestheticlab.be/fr-BE/pricelist/",
	);
	await expect(
		page.locator('link[rel="alternate"][hreflang="en-BE"]'),
	).toHaveAttribute("href", "https://aestheticlab.be/en-BE/pricelist/");
	await expect(
		page.locator('link[rel="alternate"][hreflang="fr-BE"]'),
	).toHaveAttribute("href", "https://aestheticlab.be/fr-BE/pricelist/");
	await expect(
		page.locator('link[rel="alternate"][hreflang="x-default"]'),
	).toHaveAttribute("href", "https://aestheticlab.be/en-BE/pricelist/");
	await expect(page.locator('link[rel="alternate"]')).toHaveCount(6);
});

test("names Leuven and the treatments in price-page titles", async ({
	page,
}) => {
	const titles = {
		"en-BE": /Prices in Leuven \| Aesthetic Lab$/,
		"nl-BE": /in Leuven \| Aesthetic Lab$/,
		"fr-BE": /à Louvain \| Aesthetic Lab$/,
	};
	for (const [locale, title] of Object.entries(titles)) {
		await page.goto(`/${locale}/pricelist/`);
		await expect(page).toHaveTitle(title);
		await expect(page.locator('meta[name="description"]')).toHaveAttribute(
			"content",
			/Leuven|Louvain/,
		);
	}
	await expect(page).toHaveTitle(/manucure, pédicure, sourcils & laser/);
});

test("publishes link-preview tags with a share image per language", async ({
	page,
}) => {
	await page.goto("/nl-BE/");

	await expect(page.locator('meta[property="og:image"]')).toHaveAttribute(
		"content",
		"https://aestheticlab.be/og/nl-BE.jpg",
	);
	await expect(page.locator('meta[name="twitter:image"]')).toHaveAttribute(
		"content",
		"https://aestheticlab.be/og/nl-BE.jpg",
	);
	await expect(page.locator('meta[property="og:locale"]')).toHaveAttribute(
		"content",
		"nl_BE",
	);
	await expect(
		page.locator('meta[property="og:locale:alternate"]'),
	).toHaveCount(4);
	await expect(page.locator('meta[property="og:title"]')).toHaveAttribute(
		"content",
		/Aesthetic Lab — Manicure · Pedicure · Wenkbrauwen/,
	);
	await expect(page.locator('meta[property="og:description"]')).toHaveAttribute(
		"content",
		/Boek je afspraak online/,
	);

	for (const locale of ["en-BE", "nl-BE", "fr-BE", "ru-BE", "uk-BE"]) {
		const image = await page.request.get(`/og/${locale}.jpg`);
		expect(image.ok(), locale).toBe(true);
		expect(image.headers()["content-type"], locale).toContain("image/jpeg");
	}
});

test("returns not found for unsupported or nested locale captures", async ({
	request,
}) => {
	for (const path of ["/de-BE/", "/en-BE/unknown-page/"]) {
		const response = await request.get(path);
		expect(response.status(), path).toBe(404);
	}
});

test("renders a branded, localized page for unknown links", async ({
	page,
}) => {
	const response = await page.goto("/nl-BE/oude-link/");

	expect(response?.status()).toBe(404);
	await expect(page.getByRole("heading", { level: 1 })).toHaveText(
		"Deze pagina is verhuisd",
	);
	await expect(
		page.getByRole("main").getByRole("button", { name: "Afspraak maken" }),
	).toBeVisible();
	const links = page
		.getByRole("navigation", { name: "Populaire pagina’s" })
		.getByRole("link");
	await expect(links).toHaveCount(3);
	await expect(links.nth(0)).toHaveAttribute("href", /^\/nl-BE\/#services$/);
	await expect(links.nth(1)).toHaveAttribute("href", /^\/nl-BE\/pricelist/);
	await expect(links.nth(2)).toHaveAttribute("href", /^\/nl-BE\/#contact$/);
	await expect(page.locator('meta[name="robots"]')).toHaveAttribute(
		"content",
		"noindex",
	);
});
