import { test as base, expect } from "@playwright/test";

// Exercise our consent and booking controls without waiting for Google or
// Timely. Both stacks use the same isolated third-party boundary.
export const test = base.extend<{
	externalServices: undefined;
	browserErrors: undefined;
	checkJavaScriptErrors: boolean;
}>({
	// The Go migration adds a JavaScript-error gate. Qwik's dev server can
	// report lazy-import errors during repeated full-page navigation;
	// both stacks still check every CSP violation and functional assertion.
	checkJavaScriptErrors: [false, { option: true }],
	externalServices: [
		async ({ context }, use) => {
			await context.route(
				/^https:\/\/(?:www\.googletagmanager\.com|(?:[\w-]+\.)?google-analytics\.com|bookings\.gettimely\.com)\//,
				(route) =>
					route.fulfill({
						status: 200,
						contentType: route
							.request()
							.url()
							.startsWith("https://bookings.gettimely.com/")
							? "text/html"
							: "application/javascript",
						body: "",
					}),
			);
			await use(undefined);
		},
		{ auto: true },
	],
	browserErrors: [
		async ({ page, checkJavaScriptErrors }, use) => {
			const errors: string[] = [];
			if (checkJavaScriptErrors) {
				page.on("pageerror", (error) => errors.push(error.message));
			}
			page.on("console", (message) => {
				if (/content.security.policy|violates.*policy/i.test(message.text())) {
					errors.push(message.text());
				}
			});
			await use(undefined);
			expect(errors, "JavaScript errors or CSP violations").toEqual([]);
		},
		{ auto: true },
	],
});

export { expect };
