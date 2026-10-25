import { defineConfig } from "@playwright/test";
import shared from "./playwright.config";

export default defineConfig<{ checkJavaScriptErrors: boolean }>({
	...shared,
	outputDir: "test-results/go",
	reporter: [["html", { outputFolder: "playwright-report/go", open: "never" }]],
	use: {
		...shared.use,
		baseURL: "https://localhost:43123",
		ignoreHTTPSErrors: true,
		checkJavaScriptErrors: true,
	},
	webServer: {
		command: "make assets go-build && bun e2e/serve-go.ts",
		url: "https://localhost:43123/healthz",
		ignoreHTTPSErrors: true,
		reuseExistingServer: false,
		timeout: 180_000,
		gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
	},
});
