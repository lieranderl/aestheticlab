import { defineConfig, devices } from "@playwright/test";

// The site's browser specs, against the Go binary behind a local TLS proxy
// (e2e/serve-go.ts) with no content database, so pages render the embedded
// snapshot.
export default defineConfig({
	testDir: "./e2e",
	fullyParallel: false,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	workers: 1,
	outputDir: "test-results",
	reporter: [["html", { outputFolder: "playwright-report", open: "never" }]],
	// CI runners (WebKit on Linux in particular) render more slowly.
	expect: { timeout: process.env.CI ? 10_000 : 5_000 },
	use: {
		baseURL: "https://localhost:43123",
		ignoreHTTPSErrors: true,
		trace: "on-first-retry",
	},
	projects: [
		{
			name: "chromium",
			use: { ...devices["Desktop Chrome"] },
		},
		{
			name: "firefox",
			use: { ...devices["Desktop Firefox"] },
		},
		{
			name: "webkit",
			use: { ...devices["Desktop Safari"] },
		},
	],
	webServer: {
		command: "make assets go-build && bun e2e/serve-go.ts",
		url: "https://localhost:43123/healthz",
		ignoreHTTPSErrors: true,
		reuseExistingServer: false,
		timeout: 180_000,
		gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
	},
});
