import { defineConfig, devices } from "@playwright/test";

// A tab opened on the Qwik site, then served by the Go site (Phase 7 of
// plans/go-datastar-scale-to-zero.md): the Qwik production build and the Go
// binary behind e2e/handoff/proxy.ts. Runs until Qwik is removed.
const mockSupabaseUrl = "http://127.0.0.1:43121";

export default defineConfig({
	testDir: "./e2e/handoff",
	fullyParallel: false,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	workers: 1,
	outputDir: "test-results/handoff",
	reporter: [
		["html", { outputFolder: "playwright-report/handoff", open: "never" }],
	],
	use: {
		baseURL: "https://localhost:43130",
		ignoreHTTPSErrors: true,
		trace: "on-first-retry",
	},
	projects: [
		{ name: "chromium", use: { ...devices["Desktop Chrome"] } },
		{ name: "firefox", use: { ...devices["Desktop Firefox"] } },
		{ name: "webkit", use: { ...devices["Desktop Safari"] } },
	],
	webServer: [
		{
			command: "bun e2e/mock-supabase.ts",
			name: "Mock Supabase",
			url: `${mockSupabaseUrl}/health`,
			reuseExistingServer: false,
		},
		{
			command: "bun run build && bun server/entry.bun.js",
			name: "Qwik",
			env: {
				PORT: "43131",
				SUPABASE_KEY: "sb_publishable_ci_placeholder",
				SUPABASE_URL: mockSupabaseUrl,
			},
			url: "http://127.0.0.1:43131/healthz",
			reuseExistingServer: false,
			timeout: 180_000,
		},
		{
			command: "make assets go-build && ./bin/web",
			name: "Go",
			env: { PORT: "43132", FIRESTORE_DATABASE: "" },
			url: "http://127.0.0.1:43132/healthz",
			reuseExistingServer: false,
			timeout: 180_000,
		},
		{
			command: "bun e2e/handoff/proxy.ts",
			name: "Proxy",
			url: "https://localhost:43130/healthz",
			ignoreHTTPSErrors: true,
			reuseExistingServer: false,
		},
	],
});
