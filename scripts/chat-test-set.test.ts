import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { type Case, parseReply } from "./chat-test-set";

test("reads the assistant's reply, Book buttons and transcript from the chat's response", () => {
	const sse = [
		"event: datastar-patch-elements",
		"data: selector #chat-log",
		"data: mode append",
		'data: elements <div class="chat chat-end"><div class="chat-bubble chat-bubble-primary">Hi</div></div>',
		"",
		"event: datastar-patch-elements",
		"data: selector #chat-log",
		"data: mode append",
		'data: elements <div class="chat chat-start">',
		'data: elements <div class="chat-bubble bg-base-200 whitespace-pre-line">Zara is free:',
		"data: elements • 10:00 &amp; 10:15</div>",
		'data: elements <button id="chat-book-2-4847871-642297" type="button">Book <span>Pedicure</span> with Zara</button>',
		"data: elements </div>",
		"",
		"event: datastar-patch-signals",
		'data: signals {"chatHistory":[{"role":"visitor","text":"Hi"},{"role":"assistant","text":"Zara is free"}],"chatBusy":false}',
		"",
	].join("\n");
	const reply = parseReply(sse, []);
	expect(reply.text).toBe("Zara is free:\n• 10:00 & 10:15");
	expect(reply.buttons).toEqual(["Book Pedicure with Zara"]);
	expect(reply.history).toHaveLength(2);
});

test("every case has an id, a site locale, messages and what a good answer says", () => {
	const cases: Case[] = JSON.parse(
		readFileSync(new URL("./chat-test-set.json", import.meta.url), "utf8"),
	);
	const locales = ["en-BE", "nl-BE", "fr-BE", "ru-BE", "uk-BE"];
	expect(new Set(cases.map((c) => c.id)).size).toBe(cases.length);
	for (const c of cases) {
		expect(locales).toContain(c.locale);
		expect(c.messages.length).toBeGreaterThan(0);
		expect(c.expect.length).toBeGreaterThan(0);
	}
	for (const locale of locales) {
		expect(cases.some((c) => c.locale === locale)).toBe(true);
	}
});
