// The GA4 property's key events, custom dimensions and custom metrics for
// the events the site sends (frontend/site.js, internal/views): the record
// of what the property has (set up by hand on 2026-11-25, as Google blocks
// gcloud's own sign-in from Analytics), and a way to recreate it. It finds
// the property by the site's measurement ID and creates only what is
// missing; it never changes or archives anything.
//
//   gcloud auth application-default login \
//     --scopes=https://www.googleapis.com/auth/analytics.edit,https://www.googleapis.com/auth/cloud-platform
//   bun scripts/ga4-setup.ts            # shows what it would create
//   bun scripts/ga4-setup.ts --apply    # creates it
//
// The Google Analytics Admin API must be enabled on the credentials' quota
// project (GA_QUOTA_PROJECT, else the one gcloud stored with them).
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { parseArgs } from "node:util";

const measurementId = "G-95QF984DPQ";
const api = "https://analyticsadmin.googleapis.com/v1beta";

export type Dimension = {
	parameterName: string;
	displayName: string;
	description: string;
};
export type Metric = Dimension & { measurementUnit: "STANDARD" | "SECONDS" };
export type KeyEvent = { eventName: string };

const dimension = (
	parameterName: string,
	displayName: string,
	description: string,
): Dimension => ({ parameterName, displayName, description });

/** Event-scoped dimensions for the parameters worth reporting on. */
export const dimensions: Dimension[] = [
	dimension(
		"message_number",
		"Chat message number",
		"Which message in the conversation was sent",
	),
	dimension(
		"placement",
		"Placement",
		"Where on the page the click or view happened",
	),
	dimension(
		"booking_id",
		"Booking button",
		"Which Book button opened the booking dialog",
	),
	dimension("service_id", "Service ID", "Treatment a Book button was for"),
	dimension("service_name", "Service name", "Treatment a Book button was for"),
	dimension(
		"service_category",
		"Service category",
		"Treatment category viewed or booked",
	),
	dimension("staff_id", "Staff ID", "Staff member a Book button was for"),
	dimension(
		"widget_loaded",
		"Booking widget loaded",
		"Whether Timely loaded before the dialog closed",
	),
	dimension(
		"time_open",
		"Booking dialog time open",
		"How long the booking dialog stayed open, bucketed",
	),
	dimension(
		"contact_method",
		"Contact method",
		"How the visitor chose to contact us",
	),
	dimension(
		"target_type",
		"Instagram target",
		"Instagram profile or message link",
	),
	dimension("parking_name", "Parking", "Car park whose directions were opened"),
	dimension(
		"from_locale",
		"Language from",
		"Page language before a language change",
	),
	dimension("to_locale", "Language to", "Language the visitor switched to"),
	dimension(
		"source",
		"Chat message source",
		"Typed, suggested question or treatment button",
	),
	dimension(
		"outcome",
		"Chat reply outcome",
		"How the chat assistant's reply went",
	),
	dimension(
		"metric_rating",
		"Web Vitals rating",
		"good, needs-improvement or poor",
	),
	dimension(
		"debug_target",
		"Web Vitals element",
		"Element behind a poor LCP, INP or CLS",
	),
];

/** Event-scoped metrics. */
export const metrics: Metric[] = [
	{
		...dimension(
			"seconds_open",
			"Booking dialog seconds open",
			"Seconds the booking dialog stayed open",
		),
		measurementUnit: "SECONDS",
	},
	{
		...dimension(
			"messages",
			"Chat messages at close",
			"Messages answered when the chat was closed",
		),
		measurementUnit: "STANDARD",
	},
	{
		...dimension(
			"booking_offers",
			"Chat booking offers",
			"Book buttons shown in a chat reply",
		),
		measurementUnit: "STANDARD",
	},
	{
		...dimension(
			"free_times",
			"Chat free time lists",
			"Lists of free times shown in a chat reply",
		),
		measurementUnit: "STANDARD",
	},
	{
		...dimension(
			"choices",
			"Chat treatment choices",
			"Treatment buttons offered in a chat reply",
		),
		measurementUnit: "STANDARD",
	},
];

/**
 * The actions that come closest to a booking or a contact. GA4's admin
 * screens can mark only events it has received; directions_clicked waits
 * for its first click.
 */
export const keyEvents: KeyEvent[] = [
	{ eventName: "booking_opened" },
	{ eventName: "contact_email_clicked" },
	{ eventName: "directions_clicked" },
];

type Existing = {
	dimensions: { parameterName: string }[];
	metrics: { parameterName: string }[];
	keyEvents: { eventName: string }[];
};

/** What to create so the property has everything above. */
export function plan(existing: Existing) {
	const has = (list: { parameterName: string }[]) =>
		new Set(list.map((d) => d.parameterName));
	const dims = has(existing.dimensions);
	const mets = has(existing.metrics);
	const events = new Set(existing.keyEvents.map((k) => k.eventName));
	return {
		dimensions: dimensions.filter((d) => !dims.has(d.parameterName)),
		metrics: metrics.filter((m) => !mets.has(m.parameterName)),
		keyEvents: keyEvents.filter((k) => !events.has(k.eventName)),
	};
}

function quotaProject(): string | undefined {
	if (process.env.GA_QUOTA_PROJECT) return process.env.GA_QUOTA_PROJECT;
	try {
		const file = join(
			homedir(),
			".config/gcloud/application_default_credentials.json",
		);
		return JSON.parse(readFileSync(file, "utf8")).quota_project_id;
	} catch {
		return undefined;
	}
}

async function main() {
	const { values } = parseArgs({ options: { apply: { type: "boolean" } } });
	const token = execFileSync(
		"gcloud",
		["auth", "application-default", "print-access-token"],
		{ encoding: "utf8" },
	).trim();
	const headers: Record<string, string> = {
		authorization: `Bearer ${token}`,
		"content-type": "application/json",
	};
	const quota = quotaProject();
	if (quota) headers["x-goog-user-project"] = quota;

	async function call<T>(path: string, body?: unknown): Promise<T> {
		const res = await fetch(`${api}/${path}`, {
			method: body ? "POST" : "GET",
			headers,
			body: body ? JSON.stringify(body) : undefined,
		});
		if (!res.ok) throw new Error(`${path}: ${res.status} ${await res.text()}`);
		return (await res.json()) as T;
	}

	async function list<T>(path: string, key: string): Promise<T[]> {
		const all: T[] = [];
		let pageToken = "";
		do {
			const page = await call<Record<string, unknown>>(
				`${path}?pageSize=200${pageToken ? `&pageToken=${pageToken}` : ""}`,
			);
			all.push(...((page[key] as T[]) ?? []));
			pageToken = (page.nextPageToken as string) ?? "";
		} while (pageToken);
		return all;
	}

	type Summary = { propertySummaries?: { property: string }[] };
	const summaries = await list<Summary>("accountSummaries", "accountSummaries");
	let property = "";
	for (const p of summaries.flatMap((s) => s.propertySummaries ?? [])) {
		type Stream = { webStreamData?: { measurementId?: string } };
		const streams = await list<Stream>(
			`${p.property}/dataStreams`,
			"dataStreams",
		);
		if (streams.some((s) => s.webStreamData?.measurementId === measurementId)) {
			property = p.property;
			break;
		}
	}
	if (!property)
		throw new Error(`no property you can see streams ${measurementId}`);

	const todo = plan({
		dimensions: await list(`${property}/customDimensions`, "customDimensions"),
		metrics: await list(`${property}/customMetrics`, "customMetrics"),
		keyEvents: await list(`${property}/keyEvents`, "keyEvents"),
	});
	console.log(`${property} (${measurementId})`);
	for (const d of todo.dimensions)
		console.log(`  dimension  ${d.parameterName}`);
	for (const m of todo.metrics) console.log(`  metric     ${m.parameterName}`);
	for (const k of todo.keyEvents) console.log(`  key event  ${k.eventName}`);
	const count =
		todo.dimensions.length + todo.metrics.length + todo.keyEvents.length;
	if (count === 0) return console.log("  nothing to create");
	if (!values.apply) return console.log(`${count} to create; run with --apply`);

	for (const d of todo.dimensions)
		await call(`${property}/customDimensions`, { ...d, scope: "EVENT" });
	for (const m of todo.metrics)
		await call(`${property}/customMetrics`, { ...m, scope: "EVENT" });
	for (const k of todo.keyEvents)
		await call(`${property}/keyEvents`, {
			...k,
			countingMethod: "ONCE_PER_EVENT",
		});
	console.log(`created ${count}`);
}

if (import.meta.main) await main();
