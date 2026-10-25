import { defineConfig, devices } from "@playwright/test";

const mockSupabaseUrl = "http://127.0.0.1:43121";
const webServers = [
	...(process.env.CI
		? [
				{
					command: "bun e2e/mock-supabase.ts",
					name: "Mock Supabase",
					reuseExistingServer: false,
					url: `${mockSupabaseUrl}/health`,
				},
			]
		: []),
	{
		command: "bun run dev",
		env: process.env.CI
			? {
					SUPABASE_KEY: "sb_publishable_ci_placeholder",
					SUPABASE_URL: mockSupabaseUrl,
				}
			: undefined,
		reuseExistingServer: !process.env.CI,
		url: "http://localhost:5173",
	},
];

export default defineConfig({
	testDir: "./e2e",
	// playwright.handoff.config.ts runs these against both sites at once.
	testIgnore: "handoff/**",
	fullyParallel: false,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	workers: 1,
	outputDir: "test-results/qwik",
	reporter: [
		["html", { outputFolder: "playwright-report/qwik", open: "never" }],
	],
	// CI runners (WebKit on Linux in particular) render the dev server more slowly.
	expect: { timeout: process.env.CI ? 10_000 : 5_000 },
	use: {
		baseURL: "http://localhost:5173",
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
	webServer: webServers,
});
