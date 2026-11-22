// The chat assistant's test set: real questions in all five languages
// (prices, team, policies, care, free times, off-topic, medical and trick
// prompts), sent to a running site with a real model. It saves every
// transcript with what a good answer says, for a person (or Claude) to
// grade. Every message is billed, about €0.003 each: run it after big
// content changes or a model change, not on every commit.
//
//   make go-dev or the go-web-chat launch config (real model, local ledger)
//   bun run chat-test-set [--base http://localhost:3013] [--only id,id]
//
// Run it against a local server: it gives each case its own visitor
// (X-Forwarded-For), which a deployed site ignores, so there the
// per-visitor limit (40 a day) stops the set halfway.
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { parseArgs } from "node:util";

export type Case = {
	id: string;
	locale: string;
	messages: string[];
	expect: string;
};

type Turn = { role: string; text: string };

export type Reply = { text: string; buttons: string[]; history: Turn[] };

const entities: Record<string, string> = {
	amp: "&",
	lt: "<",
	gt: ">",
	quot: '"',
	"#39": "'",
};

function decodeEntities(s: string): string {
	return s.replace(/&(amp|lt|gt|quot|#39);/g, (_, e) => entities[e]);
}

// parseReply reads the chat's Datastar response: the assistant's bubble
// and Book buttons from the last element patch, and the transcript from
// the signals.
export function parseReply(sse: string, history: Turn[]): Reply {
	let elements = "";
	let next = history;
	for (const event of sse.split("\n\n")) {
		const data = event
			.split("\n")
			.filter((l) => l.startsWith("data: "))
			.map((l) => l.slice(6));
		const html = data.filter((l) => l.startsWith("elements "));
		if (html.length > 0) {
			elements = html.map((l) => l.slice(9)).join("\n");
		}
		const signals = data.filter((l) => l.startsWith("signals "));
		if (signals.length > 0) {
			const s = JSON.parse(signals.map((l) => l.slice(8)).join("\n"));
			next = s.chatHistory ?? next;
		}
	}
	const bubble = elements.match(
		/<div class="chat-bubble bg-base-200[^"]*">([\s\S]*?)<\/div>/,
	);
	const buttons = [
		...elements.matchAll(
			/<button[^>]*id="chat-book-[^"]*"[^>]*>([\s\S]*?)<\/button>/g,
		),
	].map((m) => decodeEntities(m[1].replace(/<[^>]+>/g, "")).trim());
	return {
		text: bubble ? decodeEntities(bubble[1]) : "",
		buttons,
		history: next,
	};
}

async function send(
	base: string,
	c: Case,
	visitor: string,
	message: string,
	history: Turn[],
): Promise<Reply> {
	const response = await fetch(`${base}/${c.locale}/chat`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			"Datastar-Request": "true",
			"X-Forwarded-For": visitor,
		},
		body: JSON.stringify({ chatInput: message, chatHistory: history }),
		signal: AbortSignal.timeout(60_000),
	});
	if (!response.ok) {
		throw new Error(`${c.id}: HTTP ${response.status}`);
	}
	return parseReply(await response.text(), history);
}

// pause keeps under the site's limit of one message every 2 s.
const pause = 2_500;

async function main() {
	const { values } = parseArgs({
		options: {
			base: { type: "string", default: "http://localhost:3013" },
			only: { type: "string" },
			out: { type: "string" },
		},
	});
	const all: Case[] = JSON.parse(
		readFileSync(new URL("./chat-test-set.json", import.meta.url), "utf8"),
	);
	const only = values.only?.split(",");
	const cases = only ? all.filter((c) => only.includes(c.id)) : all;
	const out =
		values.out ??
		`tmp/chat-test-set/${new Date().toISOString().replace(/[:.]/g, "-")}.json`;
	mkdirSync(out.replace(/\/[^/]*$/, ""), { recursive: true });

	const results = [];
	for (const [i, c] of cases.entries()) {
		const visitor = `203.0.113.${(i % 250) + 1}`;
		let history: Turn[] = [];
		const turns = [];
		for (const message of c.messages) {
			const start = performance.now();
			const reply = await send(values.base, c, visitor, message, history);
			history = reply.history;
			const seconds = Math.round((performance.now() - start) / 100) / 10;
			turns.push({ visitor: message, ...reply, history: undefined, seconds });
			console.log(
				`${c.id} ${seconds}s ${JSON.stringify(reply.text.slice(0, 80))} ${reply.buttons.join(" | ")}`,
			);
			await Bun.sleep(pause);
		}
		results.push({ ...c, turns });
		writeFileSync(out, JSON.stringify(results, null, "\t"));
	}
	console.log(`${results.length} cases saved to ${out}`);
}

if (import.meta.main) {
	await main();
}
