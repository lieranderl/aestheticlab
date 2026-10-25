// Keep the production CSP intact: WebKit upgrades HTTP assets to HTTPS even
// on loopback. This test-only TLS proxy forwards the binary's bytes unchanged.
import { spawn, spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { request } from "node:http";
import { createServer } from "node:https";
import { tmpdir } from "node:os";
import { join } from "node:path";

const directory = mkdtempSync(join(tmpdir(), "aestheticlab-e2e-"));
process.on("exit", () => rmSync(directory, { recursive: true, force: true }));
const key = join(directory, "key.pem");
const cert = join(directory, "cert.pem");
const certificate = spawnSync(
	"openssl",
	[
		"req",
		"-x509",
		"-newkey",
		"rsa:2048",
		"-nodes",
		"-keyout",
		key,
		"-out",
		cert,
		"-days",
		"1",
		"-subj",
		"/CN=localhost",
		"-addext",
		"subjectAltName=DNS:localhost,IP:127.0.0.1",
	],
	{ stdio: "pipe" },
);
if (certificate.status !== 0) {
	console.error(
		"Could not create the local test certificate",
		certificate.error,
	);
	process.exit(1);
}

const child = spawn("./bin/web", [], {
	env: { ...process.env, PORT: "43122", FIRESTORE_DATABASE: "" },
	stdio: "inherit",
});
const server = createServer(
	{ key: readFileSync(key), cert: readFileSync(cert) },
	(req, res) => {
		const upstream = request(
			{
				hostname: "127.0.0.1",
				port: 43122,
				path: req.url,
				method: req.method,
				headers: req.headers,
			},
			(response) => {
				res.writeHead(response.statusCode ?? 502, response.headers);
				response.pipe(res);
			},
		);
		upstream.on("error", () => {
			if (!res.headersSent) res.writeHead(502);
			res.end();
		});
		res.on("close", () => upstream.destroy());
		req.pipe(upstream);
	},
);

let stopping = false;
function stop(code: number) {
	if (stopping) return;
	stopping = true;
	server.closeAllConnections();
	server.close();
	child.kill("SIGTERM");
	process.exit(code);
}
process.once("SIGTERM", () => stop(0));
process.once("SIGINT", () => stop(0));
child.once("error", (error) => {
	console.error("Could not start the Go test server", error);
	stop(1);
});
child.once("exit", (code) => stop(code || 1));
server.once("error", (error) => {
	console.error("Could not start the local HTTPS proxy", error);
	stop(1);
});
server.listen(43123);
