// A self-signed certificate for localhost, for the test-only HTTPS proxies:
// the sites' policies ask browsers to upgrade insecure requests, and WebKit
// does so even on loopback. Removed when the process exits.
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

export function localCertificate(): { key: Buffer; cert: Buffer } {
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
	return { key: readFileSync(key), cert: readFileSync(cert) };
}
