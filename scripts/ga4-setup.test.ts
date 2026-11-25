import { describe, expect, test } from "bun:test";
import { dimensions, keyEvents, metrics, plan } from "./ga4-setup";

describe("plan", () => {
	test("creates only what the property lacks", () => {
		const todo = plan({
			dimensions: [{ parameterName: "placement" }],
			metrics: [],
			keyEvents: [{ eventName: "booking_opened" }],
		});
		expect(todo.dimensions.map((d) => d.parameterName)).not.toContain(
			"placement",
		);
		expect(todo.dimensions).toHaveLength(dimensions.length - 1);
		expect(todo.metrics).toEqual(metrics);
		expect(todo.keyEvents.map((k) => k.eventName)).not.toContain(
			"booking_opened",
		);
		expect(todo.keyEvents).toHaveLength(keyEvents.length - 1);
	});

	test("has nothing to do on a property that has it all", () => {
		const todo = plan({ dimensions, metrics, keyEvents });
		expect(todo).toEqual({ dimensions: [], metrics: [], keyEvents: [] });
	});
});

describe("the wanted setup", () => {
	test("uses parameter names GA4 accepts, each once", () => {
		const names = [...dimensions, ...metrics].map((d) => d.parameterName);
		expect(new Set(names).size).toBe(names.length);
		for (const name of names) expect(name).toMatch(/^[a-z][a-z0-9_]{0,39}$/);
	});

	test("keeps display names within GA4's limit", () => {
		for (const { displayName } of [...dimensions, ...metrics]) {
			expect(displayName).toMatch(/^[A-Za-z][A-Za-z0-9_ ]{0,81}$/);
		}
	});
});
