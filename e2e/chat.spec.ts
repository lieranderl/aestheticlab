import { readFileSync } from "node:fs";
import { expect, test } from "./fixtures";

// The chat assistant runs with the scripted fake model and the made-up dev
// calendar (e2e/serve-go.ts): no network, no cost. Its tool calls and Book
// buttons are covered by the Go tests (internal/server/chat_test.go); these
// check the widget itself.

const consentStorageKey = "aestheticlab_cookie_consent_v2";

let visitor = 0;

test.beforeEach(async ({ page }) => {
	// Each test is its own visitor, so the chat's per-visitor rate limit
	// (six messages, then one every ten seconds) never spans tests.
	visitor += 1;
	await page.setExtraHTTPHeaders({
		"X-Forwarded-For": `198.51.100.${visitor}`,
	});
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
});

test("opens the chat, sends a message and shows the reply", async ({
	page,
}) => {
	await page.goto("/en-BE/");
	const open = page.getByRole("button", { name: "Chat with our assistant" });
	await expect(open).toBeVisible();
	await open.click();

	const panel = page.getByRole("dialog", { name: "Ask Aesthetic Lab" });
	await expect(panel).toBeVisible();
	await expect(open).toBeHidden();
	await expect(panel).toContainText(
		"Hi! Ask me about our treatments, prices, team or free times.",
	);
	await expect(panel).toContainText("AI assistant");

	const input = panel.getByRole("textbox", { name: "Your question" });
	await expect(input).toBeFocused();
	await input.fill("Do you do pedicures?");
	await input.press("Enter");

	await expect(panel.locator("#chat-log .chat-end")).toContainText(
		"Do you do pedicures?",
	);
	await expect(panel.locator("#chat-log .chat-start").last()).toContainText(
		"test assistant",
	);
	await expect(input).toHaveValue("");

	// The transcript goes back with the next message: two turns, then four.
	await input.fill("And manicures?");
	await panel.getByRole("button", { name: "Send" }).click();
	await expect(panel.locator("#chat-log .chat-end")).toHaveCount(2);
	await expect(panel.locator("#chat-log .chat-start")).toHaveCount(3);
});

test("closes with the close button or Escape and keeps the conversation", async ({
	page,
}) => {
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const panel = page.getByRole("dialog", { name: "Ask Aesthetic Lab" });
	const input = panel.getByRole("textbox", { name: "Your question" });
	await input.fill("Hello");
	await input.press("Enter");
	await expect(panel.locator("#chat-log .chat-end")).toContainText("Hello");

	await page.keyboard.press("Escape");
	await expect(panel).toBeHidden();
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	await expect(panel.locator("#chat-log .chat-end")).toContainText("Hello");

	await panel.getByRole("button", { name: "Close chat" }).click();
	await expect(panel).toBeHidden();
});

test("does not send an empty message", async ({ page }) => {
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const panel = page.getByRole("dialog", { name: "Ask Aesthetic Lab" });
	await expect(panel.getByRole("button", { name: "Send" })).toBeDisabled();
	await panel.getByRole("textbox", { name: "Your question" }).fill("   ");
	await expect(panel.getByRole("button", { name: "Send" })).toBeDisabled();
});

test("speaks the page's language", async ({ page }) => {
	await page.goto("/nl-BE/pricelist/");
	await page.getByRole("button", { name: "Chat met onze assistent" }).click();
	await expect(
		page.getByRole("dialog", { name: "Vraag het Aesthetic Lab" }),
	).toContainText("Hallo!");
});

test("fits a phone screen above the booking bar", async ({ page }) => {
	await page.setViewportSize({ width: 320, height: 640 });
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const panel = page.getByRole("dialog", { name: "Ask Aesthetic Lab" });
	await expect(panel).toBeVisible();
	const box = await panel.boundingBox();
	expect(box).not.toBeNull();
	if (box) {
		expect(box.x).toBeGreaterThanOrEqual(0);
		expect(box.x + box.width).toBeLessThanOrEqual(320);
		expect(box.y).toBeGreaterThanOrEqual(0);
		expect(box.y + box.height).toBeLessThanOrEqual(640);
	}
	const dimensions = await page.evaluate(() => ({
		clientWidth: document.documentElement.clientWidth,
		scrollWidth: document.documentElement.scrollWidth,
	}));
	expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
});

test("stays out of the way without JavaScript", async ({ browser }) => {
	const context = await browser.newContext({ javaScriptEnabled: false });
	const page = await context.newPage();
	await page.goto("/en-BE/");
	await expect(page.locator("#chat")).toBeHidden();
	await context.close();
});

test("keeps the conversation across reloads and pages in the same tab", async ({
	page,
}) => {
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const panel = page.getByRole("dialog", { name: "Ask Aesthetic Lab" });
	const input = panel.getByRole("textbox", { name: "Your question" });
	await input.fill("Remember me");
	await input.press("Enter");
	await expect(panel.locator("#chat-log .chat-start")).toHaveCount(2);

	// Reloaded while open: the panel reopens with the conversation.
	await page.reload();
	await expect(panel).toBeVisible();
	await expect(panel.locator("#chat-log .chat-end")).toHaveText("Remember me");
	await expect(panel.locator("#chat-log .chat-start")).toHaveCount(2);

	// Another page of the site, same tab.
	await page.goto("/en-BE/pricelist/");
	await expect(panel.locator("#chat-log .chat-end")).toHaveText("Remember me");

	// The next message carries the restored transcript to the server.
	const request = page.waitForRequest(
		(r) => r.method() === "POST" && r.url().endsWith("/en-BE/chat"),
	);
	await input.fill("Still there?");
	await input.press("Enter");
	const sent = (await request).postDataJSON();
	expect(sent.chatHistory).toHaveLength(2);
	expect(sent.chatHistory[0]).toEqual({ role: "visitor", text: "Remember me" });
	await expect(panel.locator("#chat-log .chat-end")).toHaveCount(2);
});

test("stays closed after a reload if it was closed", async ({ page }) => {
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const panel = page.getByRole("dialog", { name: "Ask Aesthetic Lab" });
	const input = panel.getByRole("textbox", { name: "Your question" });
	await input.fill("Hello");
	await input.press("Enter");
	await expect(panel.locator("#chat-log .chat-start")).toHaveCount(2);
	await panel.getByRole("button", { name: "Close chat" }).click();

	await page.reload();
	await expect(panel).toBeHidden();
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	await expect(panel.locator("#chat-log .chat-end")).toHaveText("Hello");
});

test("starts fresh in a new tab", async ({ page, context }) => {
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const input = page.getByRole("textbox", { name: "Your question" });
	await input.fill("Only in this tab");
	await input.press("Enter");
	await expect(page.locator("#chat-log .chat-end")).toHaveCount(1);

	const other = await context.newPage();
	await other.goto("/en-BE/");
	await expect(other.locator("#chat-log .chat-start")).toHaveCount(1);
	await expect(other.locator("#chat-log .chat-end")).toHaveCount(0);
	await other.close();
});

test("suggests common questions until the first message", async ({ page }) => {
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const panel = page.getByRole("dialog", { name: "Ask Aesthetic Lab" });
	const suggestions = panel.getByRole("group", { name: "Suggested questions" });
	await expect(suggestions.getByRole("button")).toHaveText([
		"What do your treatments cost?",
		"When can I book a manicure?",
		"Tell me about the studio",
		"How does laser hair removal work?",
	]);

	await suggestions
		.getByRole("button", { name: "When can I book a manicure?" })
		.click();
	await expect(panel.locator("#chat-log .chat-end")).toHaveText(
		"When can I book a manicure?",
	);
	await expect(panel.locator("#chat-log .chat-start")).toHaveCount(2);
	await expect(suggestions).toBeHidden();

	// A conversation that is under way keeps them hidden after a reload.
	await page.reload();
	await expect(panel).toBeVisible();
	await expect(panel.locator("#chat-log .chat-end")).toHaveCount(1);
	await expect(suggestions).toBeHidden();
});

test("writes several lines with Shift+Enter and sends with Enter", async ({
	page,
}) => {
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const panel = page.getByRole("dialog", { name: "Ask Aesthetic Lab" });
	const input = panel.getByRole("textbox", { name: "Your question" });
	await input.pressSequentially("Two questions:");
	await input.press("Shift+Enter");
	await input.pressSequentially("price and time");
	await expect(input).toHaveValue("Two questions:\nprice and time");
	await expect(panel.locator("#chat-log .chat-end")).toHaveCount(0);

	await input.press("Enter");
	await expect(panel.locator("#chat-log .chat-end")).toHaveText(
		"Two questions:\nprice and time",
	);
	await expect(input).toHaveValue("");
});

// The hint wraps in a text area in a smaller size; typed text stays 16px,
// since phones zoom into smaller fields.
test("shows the whole hint in a smaller size than typed text", async ({
	page,
}) => {
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const input = page.getByRole("textbox", { name: "Your question" });
	const sizes = await input.evaluate((el) => ({
		tag: el.tagName,
		text: Number.parseFloat(getComputedStyle(el).fontSize),
		hint: Number.parseFloat(getComputedStyle(el, "::placeholder").fontSize),
	}));
	expect(sizes.tag).toBe("TEXTAREA");
	expect(sizes.text).toBe(16);
	expect(sizes.hint).toBeLessThan(16);
	// Two lines of hint fit while the field is empty, also where it grows
	// with its typed content only (Safari).
	const room = await input.evaluate((el) => {
		const cs = getComputedStyle(el);
		return (
			el.clientHeight -
			Number.parseFloat(cs.paddingTop) -
			Number.parseFloat(cs.paddingBottom) -
			2 * Number.parseFloat(getComputedStyle(el, "::placeholder").lineHeight)
		);
	});
	expect(room).toBeGreaterThanOrEqual(0);
});

for (const [width, height] of [
	[375, 812],
	[1440, 900],
]) {
	test(`sits bottom right with the back-to-top arrow above it (${width}px)`, async ({
		page,
	}) => {
		await page.setViewportSize({ width, height });
		await page.goto("/en-BE/");
		await page.mouse.wheel(0, 3000);
		const chat = page.getByRole("button", { name: "Chat with our assistant" });
		const top = page.locator("button[data-signals\\:to-top]");
		await expect(top).toHaveCSS("opacity", "1");
		await expect(top).toHaveCSS("translate", "0px");
		const [c, t] = [await chat.boundingBox(), await top.boundingBox()];
		expect(c && t).toBeTruthy();
		if (c && t) {
			const edge = width < 1024 ? 16 : 24;
			expect(Math.round(width - (c.x + c.width))).toBe(edge);
			// Centred over the chat button, 12px above it.
			expect(Math.abs(t.x + t.width / 2 - (c.x + c.width / 2))).toBeLessThan(1);
			expect(Math.round(c.y - (t.y + t.height))).toBe(12);
		}
	});
}

test("glows with an aura that marks the AI assistant", async ({ page }) => {
	await page.goto("/en-BE/");
	const aura = page.locator("#chat-aura");
	await expect(aura).toHaveClass(/\baura\b/);
	await expect(aura).toHaveCSS("animation-name", "aura");
	// In the studio's colours: rosewood, blush and sage.
	const ring = await aura.evaluate(
		(el) => getComputedStyle(el).backgroundImage,
	);
	for (const colour of [
		"rgb(142, 90, 76)",
		"rgb(217, 179, 166)",
		"rgb(139, 150, 135)",
	]) {
		expect(ring).toContain(colour);
	}
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	await expect(aura).toBeHidden();
	await page.getByRole("button", { name: "Close chat" }).click();
	await expect(aura).toBeVisible();
});

test("keeps the aura still with reduced motion", async ({ page }) => {
	await page.emulateMedia({ reducedMotion: "reduce" });
	await page.goto("/en-BE/");
	const still = await page
		.locator("#chat-aura")
		.evaluate((el) => [
			getComputedStyle(el).animationName,
			getComputedStyle(el, "::before").animationName,
			getComputedStyle(el, "::after").animationName,
		]);
	expect(still).toEqual(["none", "none", "none"]);
});

test("is larger on a desktop screen and still fits it", async ({ page }) => {
	await page.setViewportSize({ width: 1440, height: 900 });
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const box = await page
		.getByRole("dialog", { name: "Ask Aesthetic Lab" })
		.boundingBox();
	expect(box).not.toBeNull();
	if (box) {
		expect(box.width).toBeGreaterThanOrEqual(440);
		expect(box.height).toBeGreaterThanOrEqual(640);
		expect(box.y).toBeGreaterThanOrEqual(0);
		expect(box.y + box.height).toBeLessThanOrEqual(900);
	}
});

// The fake model looks up times for the first treatment someone does; the
// site serves the embedded content snapshot (e2e/serve-go.ts).
const snapshot: { services: { staff_ids?: number[] }[] } = JSON.parse(
	readFileSync("internal/content/snapshot.json", "utf8"),
);
const bookable = snapshot.services.some((s) => s.staff_ids?.length);

// Free times are listed from the lookup itself, never from the model's
// text, with a Book button, and the transcript keeps them for a reload.
test("lists the free times it found with a Book button", async ({ page }) => {
	// biome-ignore lint/suspicious/noSkippedTests: runs once the snapshot has bookable treatments.
	test.skip(
		!bookable,
		"the content snapshot has no bookable treatment (staff_ids) until production is seeded and make snapshot refreshes it",
	);
	await page.goto("/en-BE/");
	await page.getByRole("button", { name: "Chat with our assistant" }).click();
	const panel = page.getByRole("dialog", { name: "Ask Aesthetic Lab" });
	await panel
		.getByRole("group", { name: "Suggested questions" })
		.getByRole("button", { name: "When can I book a manicure?" })
		.click();

	const reply = panel.locator("#chat-log .chat-start").last();
	const heading = reply.getByText(/^Free times for /);
	await expect(heading).toBeVisible();
	const list = reply.getByRole("list", {
		name: (await heading.textContent()) ?? "",
	});
	const times = list.locator("time");
	await expect(times.first()).toBeVisible();
	const count = await times.count();
	expect(count).toBeGreaterThan(0);
	expect(count).toBeLessThanOrEqual(12);
	for (const time of await times.all()) {
		const [, hhmm] =
			(await time.getAttribute("datetime"))?.match(/T(\d\d:\d\d)$/) ?? [];
		await expect(time).toHaveText(
			new RegExp(`^(Mon|Tue|Wed|Thu|Fri|Sat|Sun) \\d\\d/\\d\\d · ${hhmm}$`),
		);
	}
	await expect(reply).toContainText(
		"These times look free now; the booking page confirms them.",
	);
	await expect(reply.locator('button[id^="chat-book-"]')).toHaveCount(1);

	// Restored after a reload as text, times included.
	const first = (await times.first().textContent()) ?? "";
	await page.reload();
	await expect(panel.locator("#chat-log .chat-start").last()).toContainText(
		first,
	);
});
