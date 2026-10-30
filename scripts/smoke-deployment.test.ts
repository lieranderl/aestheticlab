import { test } from "bun:test";
import { execFile } from "node:child_process";
import { createServer } from "node:http";
import { promisify } from "node:util";

test("keeps the page's revision affinity cookie when validating its image assets", async () => {
	const server = createServer((request, response) => {
		if (request.url?.startsWith("/assets/")) {
			if (!request.headers.cookie?.includes("revision=candidate")) {
				response.writeHead(404).end("Asset absent on previous revision");
				return;
			}
			response.setHeader("Content-Type", "image/webp");
			response.end("candidate image");
		} else if (request.url === "/readyz" || request.url === "/dependencyz") {
			response.end("OK");
		} else {
			response.setHeader("Set-Cookie", "revision=candidate; Path=/");
			response.setHeader("X-Content-Type-Options", "nosniff");
			response.end('Aesthetic Lab <img src="/assets/candidate.webp">');
		}
	});
	await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
	const address = server.address();
	if (!address || typeof address === "string")
		throw new Error("Missing test server address");
	try {
		await promisify(execFile)("bash", [
			"scripts/smoke-deployment.sh",
			`http://127.0.0.1:${address.port}`,
		]);
	} finally {
		await new Promise<void>((resolve, reject) =>
			server.close((error) => (error ? reject(error) : resolve())),
		);
	}
}, 15_000);
