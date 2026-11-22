import { expect, test } from "./fixtures";

// The chat assistant runs with the scripted fake model (e2e/serve-go.ts):
// no network, no cost. Its tool calls and Book buttons are covered by the
// Go tests (internal/server/chat_test.go); these check the widget itself.

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
