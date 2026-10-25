import type { Page } from "@playwright/test";
import { expect, test } from "./fixtures";

const localeRedirectPattern = /\/[a-z]{2}-[A-Z]{2}(?:\/|$)/;
const consentStorageKey = "aestheticlab_cookie_consent_v2";

test("redirects the root path to a locale-prefixed URL", async ({ page }) => {
	await page.goto("/");

	await expect(page).toHaveURL(localeRedirectPattern);
	await expect(page.locator("body")).toBeVisible();
});

test("keeps the current locale prefix in primary navigation links", async ({
	page,
}) => {
	await page.goto("/en-BE/");

	await expect(
		page.getByRole("link", { name: "Aesthetic Lab — Home" }),
	).toHaveAttribute("href", "/en-BE/#");
	await expect(
		page
			.getByRole("navigation", { name: "Primary navigation" })
			.getByRole("link", { name: "Services", exact: true }),
	).toHaveAttribute("href", "/en-BE/#services");
	await expect(
		page.getByRole("link", { name: "Privacy Policy", exact: true }),
	).toHaveAttribute("href", /^\/en-BE\/privacy-policy\/?$/);
});

test("uses the corporate light theme and keeps navigation aligned with page order", async ({
	page,
}) => {
	await page.goto("/en-BE/");

	await expect(page.locator("body")).toHaveAttribute("data-theme", "Aesthetic");
	await expect(
		page
			.getByRole("navigation", { name: "Primary navigation" })
			.getByRole("link"),
	).toHaveText(["Services", "Prices", "Our Work", "Team", "Visit"]);
});

test("keeps the mobile hero action-led and shows treatment imagery early", async ({
	page,
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto("/en-BE/");

	const hero = page.locator("#hero");
	await expect(
		hero.getByRole("heading", {
			name: "The art of natural beauty",
			level: 1,
		}),
	).toBeVisible();
	await expect(
		hero.getByRole("button", { name: "Book Appointment" }),
	).toBeVisible();
});

test("renders localized landing-page copy in every supported language", async ({
	page,
}) => {
	const locales = [
		{
			lang: "en-BE",
			hero: "The art of natural beauty",
			faq: "Good to know",
		},
		{
			lang: "nl-BE",
			hero: "De kunst van natuurlijke schoonheid",
			faq: "Goed om te weten",
		},
		{
			lang: "fr-BE",
			hero: "L'art de la beauté naturelle",
			faq: "Bon à savoir",
		},
		{
			lang: "ru-BE",
			hero: "Искусство естественной красоты",
			faq: "Полезно знать",
		},
		{
			lang: "uk-BE",
			hero: "Мистецтво природної краси",
			faq: "Корисно знати",
		},
	];

	await page.setViewportSize({ width: 390, height: 844 });

	for (const locale of locales) {
		await page.goto(`/${locale.lang}/`);
		await expect(page.locator("#hero h1")).toHaveText(locale.hero);
		await expect(
			page.locator("#faq").getByRole("heading", {
				name: locale.faq,
			}),
		).toBeVisible();

		const dimensions = await page.evaluate(() => ({
			clientWidth: document.documentElement.clientWidth,
			scrollWidth: document.documentElement.scrollWidth,
		}));
		expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
	}
});

test("supports the audited responsive widths without horizontal overflow", async ({
	page,
}) => {
	const viewports = [
		{ width: 320, height: 800 },
		{ width: 390, height: 844 },
		{ width: 430, height: 932 },
		{ width: 768, height: 900 },
		{ width: 1024, height: 768 },
		{ width: 1440, height: 900 },
	];

	for (const viewport of viewports) {
		await page.setViewportSize(viewport);
		await page.goto("/en-BE/");

		const dimensions = await page.evaluate(() => ({
			clientWidth: document.documentElement.clientWidth,
			scrollWidth: document.documentElement.scrollWidth,
		}));

		expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
	}
});

test("links to Instagram from the work section on phone and desktop", async ({
	page,
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto("/en-BE/#gallery");
	const phoneLink = page.getByTestId("instagram-link");
	await expect(phoneLink).toBeVisible();
	await expect(phoneLink).toHaveAttribute(
		"href",
		"https://www.instagram.com/aestheticlabbe",
	);

	await page.setViewportSize({ width: 1440, height: 900 });
	await expect(phoneLink).toBeHidden();
	await expect(
		page.locator("#gallery").getByRole("link", { name: "@aestheticlabbe" }),
	).toBeVisible();
});

test("shows honest starting prices that skip add-ons", async ({ page }) => {
	await page.goto("/en-BE/#services");

	const manicure = page
		.locator("#services")
		.getByRole("button", { name: /Manicure/ })
		.first();
	await expect(manicure).toContainText(/From €\d+/);
	await expect(manicure).not.toContainText(/From €5(?!\d)/);
});

test("shows the phone booking bar once the hero is scrolled past", async ({
	page,
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto("/en-BE/");

	const bookingBar = page.locator("#bottom-bar-book-btn").locator("..");
	await expect(bookingBar).toHaveAttribute("aria-hidden", "true");
	await page.mouse.wheel(0, 900);
	await expect(bookingBar).not.toHaveAttribute("aria-hidden", "true");
});

test("keeps the hero content inside the viewport at mobile and desktop widths", async ({
	page,
}) => {
	for (const viewport of [
		{ width: 320, height: 800 },
		{ width: 390, height: 844 },
		{ width: 1440, height: 900 },
	]) {
		await page.setViewportSize(viewport);
		await page.goto("/en-BE/");

		const result = await page.locator("#hero").evaluate((hero) => {
			const heading = hero.querySelector("h1");
			const primaryAction = hero.querySelector("button");
			if (!heading || !primaryAction) return null;

			const headingBox = heading.getBoundingClientRect();
			const actionBox = primaryAction.getBoundingClientRect();
			return {
				actionLeft: actionBox.left,
				actionRight: actionBox.right,
				headingLeft: headingBox.left,
				headingRight: headingBox.right,
				viewportWidth: document.documentElement.clientWidth,
			};
		});

		expect(result).not.toBeNull();
		expect(result?.headingLeft).toBeGreaterThanOrEqual(0);
		expect(result?.headingRight).toBeLessThanOrEqual(
			result?.viewportWidth ?? 0,
		);
		expect(result?.actionLeft).toBeGreaterThanOrEqual(0);
		expect(result?.actionRight).toBeLessThanOrEqual(result?.viewportWidth ?? 0);
	}
});

test("renders key sections on the landing page", async ({ page }) => {
	await page.goto("/en-BE/");

	await expect(page.locator("#hero")).toBeVisible();
	await expect(page.locator("#services")).toBeVisible();
	await expect(page.locator("#gallery")).toBeVisible();
});

test("renders the FAQ section with proper heading", async ({ page }) => {
	await page.goto("/en-BE/#faq");

	await expect(
		page.getByRole("heading", { name: "Good to know" }),
	).toBeVisible();
	await expect(page.locator("#faq .collapse-title").first()).toBeVisible();
});

test("opens treatments in-page", async ({ page }) => {
	// In the dev server the click handler pulls ~47 unbundled modules on demand,
	// which can take well over 15s on slow CI WebKit runners.
	test.setTimeout(60_000);
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto("/en-BE/#services");
	// The page smooth-scrolls to #services on load; tap once it has arrived.
	await expect
		.poll(() =>
			page.evaluate(() => {
				const services = document.getElementById("services");
				if (!services) return Number.POSITIVE_INFINITY;
				const margin = Number.parseFloat(
					getComputedStyle(services).scrollMarginTop,
				);
				return Math.abs(services.getBoundingClientRect().top - margin);
			}),
		)
		.toBeLessThan(2);

	// Click once: a dropped first interaction is a real UX regression. Only the
	// heading gets the long wait while the handler's modules load.
	await page
		.locator("#services")
		.getByRole("button", { name: /Manicure/ })
		.first()
		.click();

	await expect(page.locator("#service-details-heading")).toHaveText(
		"Manicure",
		{ timeout: 30_000 },
	);
	await expect(
		page.locator("#services").getByRole("button", { name: "Book Now" }).first(),
	).toBeVisible();
});

test("falls back to the treatment overview for invalid shared state", async ({
	page,
}) => {
	await page.goto("/en-BE/?treatment=unknown&treatmentArea=unknown#services");

	await expect(page.locator("#service-details-heading")).toHaveCount(0);
	await expect(page.locator('link[rel="canonical"]')).toHaveAttribute(
		"href",
		"https://aestheticlab.be/en-BE/",
	);
});

test("opens and closes the booking dialog", async ({ page }) => {
	await page.goto("/en-BE/");

	await page.getByRole("button", { name: "Book Appointment" }).first().click();

	const dialog = page.getByRole("dialog", { name: "Book Appointment" });
	await expect(dialog).toBeVisible();
	await expect(dialog).toHaveAttribute("aria-modal", "true");

	await dialog.getByRole("button", { name: "Close" }).first().click();
	await expect(dialog).toBeHidden();
});

test("language menu trigger is present", async ({ page }) => {
	await page.goto("/en-BE/");

	await expect(
		page.getByRole("button", { name: /select.*language/i }),
	).toBeVisible({ timeout: 15000 });
});

// Safari 27.0 closed the menu straight after opening it once the page was
// scrolled. Opening must hold, and closing stays where visitors expect it.
test("language menu opens on a scrolled page and closes on outside click or Escape", async ({
	page,
}) => {
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.addInitScript((key) => {
		window.localStorage.setItem(
			key,
			JSON.stringify({
				version: 1,
				analytics: false,
				updatedAt: new Date().toISOString(),
			}),
		);
	}, consentStorageKey);
	await page.goto("/en-BE/");
	await page.mouse.wheel(0, 1500);
	await expect
		.poll(() => page.evaluate(() => window.scrollY))
		.toBeGreaterThan(400);

	const trigger = page.getByRole("button", { name: /select.*language/i });
	const menu = page.getByRole("list", { name: /language options/i });

	await trigger.click();
	await expect(menu).toBeVisible();
	await page.waitForTimeout(300);
	await expect(menu).toBeVisible();
	const [button, list] = await Promise.all([
		trigger.boundingBox(),
		menu.boundingBox(),
	]);
	expect(list?.y ?? 0).toBeGreaterThanOrEqual(
		(button?.y ?? 0) + (button?.height ?? 0) - 1,
	);

	await page.mouse.click(200, 500);
	await expect(menu).toBeHidden();

	await trigger.click();
	await expect(menu).toBeVisible();
	await page.keyboard.press("Escape");
	await expect(menu).toBeHidden();

	await trigger.click();
	await expect(menu).toBeVisible();
	await trigger.click();
	await expect(menu).toBeHidden();
});

/** Scrolls framed photo `index`'s frame `fromCentre` px below the middle of
 * the screen and returns the photo's offset; it is drawn 60px above its
 * frame, so 0 means centred. */
async function photoOffset(page: Page, index: number, fromCentre: number) {
	return page.evaluate(
		async ({ index, fromCentre }) => {
			const photo = document.querySelectorAll<HTMLElement>(
				"figure > .\\-top-15",
			)[index];
			const frame = photo.parentElement as HTMLElement;
			const centre =
				frame.getBoundingClientRect().top +
				window.scrollY +
				frame.offsetHeight / 2;
			window.scrollTo({
				top: centre - window.innerHeight / 2 - fromCentre,
				behavior: "instant",
			});
			await new Promise((r) => setTimeout(r, 300));
			return (
				photo.getBoundingClientRect().top -
				frame.getBoundingClientRect().top +
				60
			);
		},
		{ index, fromCentre },
	);
}

/** Scrolls to `y` and returns the hero's motion: the video's and the text's
 * vertical shift, and the text's opacity. */
async function heroMotion(page: Page, y: number) {
	return page.evaluate(async (y) => {
		window.scrollTo({ top: y, behavior: "instant" });
		await new Promise((r) => setTimeout(r, 300));
		const video = document.querySelector(".parallax-top") as HTMLElement;
		const text = document.querySelector(".parallax-fade") as HTMLElement;
		return {
			video: Math.round(new DOMMatrix(getComputedStyle(video).transform).m42),
			text: Math.round(new DOMMatrix(getComputedStyle(text).transform).m42),
			opacity: Number(getComputedStyle(text).opacity),
		};
	}, y);
}

// The visit and closing photos lag behind the scroll: shifted up while their
// frame is below the middle of the screen, down once it is above. The Go site
// draws this with CSS scroll-driven animation, and a script where the browser
// lacks it (Firefox).
test("framed photos drift against the scroll", async ({ page }) => {
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.goto("/en-BE/");
	const count = await page.locator("figure > .\\-top-15").count();
	expect(count).toBe(2);
	for (let index = 0; index < count; index++) {
		expect(
			await photoOffset(page, index, 300),
			`photo ${index} below centre`,
		).toBeLessThan(-20);
		expect(
			await photoOffset(page, index, -300),
			`photo ${index} above centre`,
		).toBeGreaterThan(20);
	}
});

// Go site: the hero video lags at a quarter of the scroll speed for the first
// screen, and the text drifts up and fades out over the first 420px; the
// same curves in every browser, by CSS or by script.
test("the hero lags behind and its text fades as the page scrolls", async ({
	page,
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto("/en-BE/");
	// biome-ignore lint/suspicious/noSkippedTests: the Qwik site measures its hero from the frame, not the page; this pins the Go curves.
	test.skip(
		(await page.locator(".parallax-top").count()) === 0,
		"the Qwik site measures its hero differently",
	);

	const halfway = await heroMotion(page, 210);
	expect(halfway.video).toBeCloseTo(53, -1);
	expect(halfway.text).toBeCloseTo(-32, -1);
	expect(halfway.opacity).toBeCloseTo(0.5, 1);

	expect(await heroMotion(page, 0)).toEqual({ video: 0, text: 0, opacity: 1 });
});

test.describe("cookie consent banner", () => {
	test.beforeEach(async ({ page }) => {
		await page.addInitScript((key) => {
			window.localStorage.removeItem(key);
		}, consentStorageKey);
	});

	test("shows on first visit and persists accepted analytics consent", async ({
		page,
	}) => {
		await page.goto("/en-BE/");

		await expect(
			page.getByRole("button", { name: "Accept analytics" }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Reject analytics" }),
		).toBeVisible();
		await expect(
			page.getByRole("link", { name: "Read our Privacy Policy" }),
		).toHaveAttribute("href", /^\/en-BE\/privacy-policy\/?$/);

		await page.getByRole("button", { name: "Accept analytics" }).click();
		await expect(
			page.getByRole("button", { name: "Accept analytics" }),
		).toBeHidden();

		await expect(
			page.getByRole("button", { name: "Cookie settings" }),
		).toBeVisible();

		const storedConsent = await page.evaluate((key) => {
			return window.localStorage.getItem(key);
		}, consentStorageKey);

		expect(storedConsent).toContain('"analytics":true');
	});

	test("allows rejecting analytics and reopening the cookie settings panel", async ({
		page,
	}) => {
		await page.goto("/en-BE/");

		await page.getByRole("button", { name: "Reject analytics" }).click();
		await expect(
			page.getByRole("button", { name: "Reject analytics" }),
		).toBeHidden();

		await expect(
			page.getByRole("button", { name: "Cookie settings" }),
		).toBeVisible();

		const storedConsent = await page.evaluate((key) => {
			return window.localStorage.getItem(key);
		}, consentStorageKey);

		expect(storedConsent).toContain('"analytics":false');

		await page.getByRole("button", { name: "Cookie settings" }).click();

		await expect(
			page.getByRole("button", { name: "Accept analytics" }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Reject analytics" }),
		).toBeVisible();
	});
});

test.describe("mobile navigation", () => {
	test.use({
		viewport: {
			width: 390,
			height: 844,
		},
	});

	test("opens and closes the mobile menu from the keyboard", async ({
		page,
	}) => {
		await page.goto("/en-BE/");

		const openMenuControl = page.getByRole("button", {
			name: "Open navigation menu",
		});
		await expect(openMenuControl).toBeVisible();
		await openMenuControl.focus();
		await page.keyboard.press("Enter");

		await expect(openMenuControl).toHaveAttribute("aria-expanded", "true");
		await expect(
			page.getByRole("dialog", { name: "Aesthetic Lab" }),
		).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(openMenuControl).toHaveAttribute("aria-expanded", "false");
		await expect(openMenuControl).toBeFocused();
	});
});

test("operates the gallery lightbox with the keyboard and restores focus", async ({
	page,
}) => {
	await page.goto("/en-BE/#gallery");
	const firstImage = page.locator("#gallery-lightbox-trigger-0");
	await firstImage.scrollIntoViewIfNeeded();
	await firstImage.click();

	const dialog = page.getByRole("dialog", { name: "Gallery image viewer" });
	await expect(dialog).toBeVisible();
	await expect(dialog.getByRole("button", { name: "Close" })).toBeFocused();
	const image = dialog.locator("img");
	const firstImageSource = await image.getAttribute("src");
	await dialog.getByRole("button", { name: "Next" }).click();
	await expect(dialog).toContainText("2 / 8");
	await expect(image).not.toHaveAttribute("src", firstImageSource ?? "");
	await dialog.getByRole("button", { name: "Previous" }).click();
	await expect(dialog).toContainText("1 / 8");
	await page.keyboard.press("ArrowRight");
	await expect(dialog).toContainText("2 / 8");
	await page.keyboard.press("Escape");
	await expect(dialog).toBeHidden();
	await expect(firstImage).toBeFocused();
});

test("localizes gallery accessibility labels", async ({ page }) => {
	await page.goto("/fr-BE/");

	await expect(
		page.getByRole("button", { name: "Agrandir l’image" }).first(),
	).toBeVisible();
});

test("videos have a pause, play and replay control", async ({ page }) => {
	await page.goto("/en-BE/");
	const heroControl = page.locator('button[aria-controls="hero-video"]');
	await expect(heroControl).toBeVisible();
	await expect(heroControl).toHaveAttribute("aria-label", /\S/);
	await expect(
		page.locator('#gallery button[aria-controls^="work-clip-"]'),
	).toHaveCount(4);
});
