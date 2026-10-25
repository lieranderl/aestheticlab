// Consent Mode v2 and Google Analytics for the Go site, as
// src/shared/cookie-consent.ts does them for the Qwik site. A classic
// deferred script, loaded before Datastar, so the pages' expressions can call
// window.aesthetic: CSP allows no inline script, so this replaces Qwik's
// inline bootstrap.
//
// Analytics storage stays denied until the visitor accepts; until then gtag
// sends only consent-aware pings (Consent Mode advanced).
(() => {
	const measurementId = "G-95QF984DPQ";
	const storageKey = "aestheticlab_cookie_consent_v2";
	const denied = {
		ad_storage: "denied",
		ad_user_data: "denied",
		ad_personalization: "denied",
		analytics_storage: "denied",
	};
	const granted = { ...denied, analytics_storage: "granted" };
	const config = {
		anonymize_ip: true,
		allow_google_signals: false,
		allow_ad_personalization_signals: false,
	};

	window.dataLayer = window.dataLayer || [];
	function gtag() {
		// biome-ignore lint/complexity/noArguments: gtag.js reads the arguments object.
		window.dataLayer.push(arguments);
	}
	window.gtag = window.gtag || gtag;
	window[`ga-disable-${measurementId}`] = false;

	/** The stored choice, or null when the visitor has not chosen. */
	function stored() {
		try {
			const parsed = JSON.parse(localStorage.getItem(storageKey) || "null");
			return parsed?.version === 1 &&
				typeof parsed.analytics === "boolean" &&
				typeof parsed.updatedAt === "string"
				? parsed
				: null;
		} catch {
			return null;
		}
	}

	function save(analytics) {
		try {
			localStorage.setItem(
				storageKey,
				JSON.stringify({
					version: 1,
					analytics,
					updatedAt: new Date().toISOString(),
				}),
			);
		} catch {
			// Private mode: the choice lasts for this page only.
		}
	}

	function page() {
		const { href, pathname, search, hash } = window.location;
		return {
			page_location: href,
			page_path: pathname + search + hash,
			page_title: document.title,
		};
	}

	/** Sends an event; consentRequired false sends it before any choice. */
	function track(name, params = {}, consentRequired = true) {
		if (consentRequired && stored()?.analytics !== true) return;
		const clean = {};
		for (const [key, value] of Object.entries(params)) {
			if (value !== null && value !== undefined && value !== "")
				clean[key] = value;
		}
		window.gtag("event", name, { ...page(), ...clean });
	}

	function choose(analytics) {
		save(analytics);
		window.gtag("consent", "update", analytics ? granted : denied);
		track(
			"cookie_consent_updated",
			{ analytics_consent: analytics ? "granted" : "denied" },
			false,
		);
	}

	window.gtag("consent", "default", { ...denied, wait_for_update: 500 });
	window.gtag("set", "ads_data_redaction", true);
	window.gtag("set", "allow_ad_personalization_signals", false);
	const choice = stored();
	if (choice) {
		window.gtag("consent", "update", choice.analytics ? granted : denied);
	}
	window.gtag("js", new Date());
	window.gtag("config", measurementId, config);

	const script = document.createElement("script");
	script.async = true;
	script.src = `https://www.googletagmanager.com/gtag/js?id=${measurementId}`;
	document.head.appendChild(script);

	/**
	 * A short silent clip (src/components/ui/in-view-video.tsx): plays once
	 * when 40% of it is in view, then rests on its last frame; pauses while
	 * off screen and resumes on return. Never plays by itself with reduced
	 * motion.
	 */
	function playInView(video) {
		if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
		let started = false;
		let pausedOffscreen = false;
		new IntersectionObserver(
			([entry]) => {
				if (entry?.isIntersecting && (!started || pausedOffscreen)) {
					started = true;
					pausedOffscreen = false;
					video.muted = true;
					// Autoplay can be refused (low-power mode); the poster then stays.
					video.play().catch(() => undefined);
				} else if (!entry?.isIntersecting && !video.paused) {
					pausedOffscreen = true;
					video.pause();
				}
			},
			{ threshold: 0.4 },
		).observe(video);
	}

	/** Pauses a playing clip, resumes a paused one, replays an ended one. */
	function toggleVideo(id) {
		const video = document.getElementById(id);
		if (!(video instanceof HTMLVideoElement)) return;
		if (!video.paused && !video.ended) {
			video.pause();
			return;
		}
		if (video.ended) video.currentTime = 0;
		video.muted = true;
		video.play().catch(() => undefined);
	}

	/** A clip's state for its control: "playing", "paused" or "ended". */
	function videoState(video) {
		if (video.ended) return "ended";
		return video.paused ? "paused" : "playing";
	}

	let sectionRequest;
	/**
	 * A fresh abort signal for the services section's requests, aborting the
	 * previous one: Datastar cancels requests per URL only, so a slow
	 * response for an earlier category could otherwise land after the
	 * latest one.
	 */
	function sectionSignal() {
		sectionRequest?.abort();
		sectionRequest = new AbortController();
		return sectionRequest;
	}

	window.aesthetic = {
		/** True when the cookie banner should show: no choice stored yet. */
		needsConsent: () => stored() === null,
		accept: () => choose(true),
		reject: () => choose(false),
		track,
		playInView,
		toggleVideo,
		videoState,
		sectionSignal,
	};
})();
