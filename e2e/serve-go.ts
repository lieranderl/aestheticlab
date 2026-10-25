// Keep the production CSP intact: WebKit upgrades HTTP assets to HTTPS even
// on loopback. This test-only TLS proxy forwards the binary's bytes unchanged.
import { spawn } from "node:child_process";
import { request } from "node:http";
import { createServer } from "node:https";
import { localCertificate } from "./local-tls";

const child = spawn("./bin/web", [], {
	env: { ...process.env, PORT: "43122", FIRESTORE_DATABASE: "" },
	stdio: "inherit",
});
const server = createServer(localCertificate(), (req, res) => {
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
});

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
