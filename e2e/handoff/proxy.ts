// Test-only: one origin in front of the Qwik and Go servers, as Cloud Run's
// URL is in front of both revisions during the cutover. PUT /__handoff/go
// moves every later request to the Go server, as a promotion does for a tab
// that is already open; PUT /__handoff/qwik moves it back.
import { request } from "node:http";
import { createServer } from "node:https";
import { localCertificate } from "../local-tls";

const upstreams = { qwik: 43131, go: 43132 } as const;
let current: keyof typeof upstreams = "qwik";

const server = createServer(localCertificate(), (req, res) => {
	const switchTo = /^\/__handoff\/(qwik|go)$/.exec(req.url ?? "")?.[1];
	if (switchTo && req.method === "PUT") {
		current = switchTo as keyof typeof upstreams;
		res.writeHead(204).end();
		return;
	}
	const upstream = request(
		{
			hostname: "127.0.0.1",
			port: upstreams[current],
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

function stop() {
	server.closeAllConnections();
	server.close();
	process.exit(0);
}
process.once("SIGTERM", stop);
process.once("SIGINT", stop);
server.listen(43130);
