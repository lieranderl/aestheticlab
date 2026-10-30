import { expect, test } from "bun:test";
import { execFile } from "node:child_process";
import { createServer } from "node:http";
import { promisify } from "node:util";

const exec = promisify(execFile);
const legacyPath = "/assets/B0kxtIDJ-lazer1.webp";

async function baseline(
	options: {
		oldImageExists?: boolean;
		candidateBytes?: string;
		renderedPath?: string;
	} = {},
) {
	const server = createServer((request, response) => {
		const path = request.url ?? "";
		if (path.endsWith("/en-BE/") || path.endsWith("/fr-BE/pricelist/")) {
			response.end(`<img src="${options.renderedPath ?? legacyPath}">`);
		} else if (path === `/previous${legacyPath}` && !options.oldImageExists) {
			response.writeHead(404).end("Not Found");
		} else {
			response.setHeader("Content-Type", "image/webp");
			response.end(
				path.startsWith("/candidate/")
					? (options.candidateBytes ?? "identical image bytes")
					: "identical image bytes",
			);
		}
	});
	await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
	const address = server.address();
	if (!address || typeof address === "string")
		throw new Error("Missing test server address");
	const origin = `http://127.0.0.1:${address.port}`;
	try {
		return (
			await exec("bash", [
				"scripts/legacy-image-baseline.sh",
				`${origin}/previous`,
				`${origin}/candidate`,
			])
		).stdout;
	} finally {
		await new Promise<void>((resolve, reject) =>
			server.close((error) => (error ? reject(error) : resolve())),
		);
	}
}

test("identifies only a pre-existing rendered defect repaired with identical image bytes", async () => {
	expect(await baseline()).toBe(`${legacyPath}\n`);
});

test("does not exempt a working old image from the cross-revision gate", async () => {
	expect(await baseline({ oldImageExists: true })).toBe("");
});

test("does not exempt a candidate serving different image bytes", async () => {
	expect(await baseline({ candidateBytes: "different bytes" })).toBe("");
});

test("does not exempt an asset absent from the old revision's rendered pages", async () => {
	expect(
		await baseline({ renderedPath: "/assets/B0kxtIDJ-universal.webp" }),
	).toBe("");
});
