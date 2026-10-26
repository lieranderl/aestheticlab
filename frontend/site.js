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

	/** Keep old treatments visible until the newest request completes. */
	function loadSection(fetchSection) {
		const section = document.getElementById("services");
		section?.setAttribute("aria-busy", "true");
		const pending = fetchSection();
		const request = sectionRequest;
		return pending.finally(() => {
			if (sectionRequest === request)
				section?.setAttribute("aria-busy", "false");
		});
	}

	function revealTreatments(id) {
		const target = document.getElementById(id);
		if (!target) return;
		const { top } = target.getBoundingClientRect();
		if (top < 90 || top > window.innerHeight * 0.7) {
			target.scrollIntoView({
				behavior: reducedMotion() ? "instant" : "smooth",
			});
		}
		// Scroll the chips horizontally without moving any outer ancestor.
		const nav = document.getElementById("service-category-nav");
		const active = nav?.querySelector('[aria-pressed="true"]');
		if (!active) return;
		const frame = nav.getBoundingClientRect();
		const chip = active.getBoundingClientRect();
		if (chip.left < frame.left || chip.right > frame.right) {
			nav.scrollTo({
				left:
					nav.scrollLeft +
					chip.left -
					frame.left -
					(frame.width - chip.width) / 2,
				behavior: reducedMotion() ? "instant" : "smooth",
			});
		}
	}

	const reducedMotion = () =>
		window.matchMedia("(prefers-reduced-motion: reduce)").matches;
	const galleryRequests = new WeakMap();
	/** Decode before replacing the visible photo; obsolete loads never win. */
	async function showGallery(dialog, index) {
		const request = {};
		galleryRequests.set(dialog, request);
		const photo = JSON.parse(dialog.dataset.images)[index];
		const image = dialog.querySelector("img");
		dialog.setAttribute("aria-busy", "true");
		const next = new Image();
		next.sizes = image.sizes;
		next.srcset = photo.srcset;
		next.src = photo.src;
		try {
			await next.decode();
		} catch {
			/* The image's alt remains available on failure. */
		}
		if (galleryRequests.get(dialog) !== request || !dialog.open) return;
		image.width = photo.width;
		image.height = photo.height;
		image.alt = photo.alt;
		image.srcset = photo.srcset;
		image.src = photo.src;
		dialog.setAttribute("aria-busy", "false");
		for (const animation of image.getAnimations()) animation.cancel();
		if (!reducedMotion())
			image.animate([{ opacity: 0 }, { opacity: 1 }], {
				duration: 180,
				easing: "ease-out",
			});
	}

	function closeGallery(dialog) {
		galleryRequests.delete(dialog);
		const image = dialog.querySelector("img");
		image.removeAttribute("srcset");
		image.removeAttribute("src");
		dialog.setAttribute("aria-busy", "false");
	}

	/** One frame-aligned observer for navigation; read geometry before writes. */
	function navigation(root) {
		const hero = document.getElementById("hero");
		const sections = [
			...document.querySelectorAll(
				"#services, #gallery, #team, #faq, #contact",
			),
		];
		const progress = document.getElementById("reading-progress");
		const cssProgress = CSS.supports("animation-timeline", "scroll()");
		let scheduled = false;
		function update() {
			scheduled = false;
			const y = window.scrollY;
			const height = window.innerHeight;
			const heroBottom = hero?.getBoundingClientRect().bottom ?? 0;
			const section =
				sections.find((el) => {
					const rect = el.getBoundingClientRect();
					return rect.top < height * 0.5 && rect.bottom > height * 0.45;
				})?.id ?? "";
			const fraction =
				y / Math.max(1, document.documentElement.scrollHeight - height);
			root.dispatchEvent(
				new CustomEvent("navigation-state", {
					detail: {
						scrolled: y > 24,
						overHero: heroBottom > 64,
						bar: heroBottom < height * 0.35,
						section,
					},
				}),
			);
			if (!cssProgress && progress)
				progress.style.transform = `scaleX(${Math.min(1, fraction)})`;
		}
		function schedule() {
			if (scheduled) return;
			scheduled = true;
			requestAnimationFrame(update);
		}
		window.addEventListener("scroll", schedule, { passive: true });
		window.addEventListener("resize", schedule, { passive: true });
		schedule();
	}

	/**
	 * Scroll-linked motion where CSS scroll-driven animations are missing
	 * (Firefox): the same curves app.css draws with animation-timeline, as
	 * src/shared/scroll-motion.ts wrote them for the Qwik site. Written in
	 * requestAnimationFrame, and only when a value changes. Nothing moves
	 * with reduced motion.
	 */
	function scrollMotion() {
		if (
			CSS.supports("animation-timeline", "view()") ||
			window.matchMedia("(prefers-reduced-motion: reduce)").matches
		) {
			return;
		}
		const items = [
			...document.querySelectorAll(
				".parallax-center, .parallax-top, .parallax-fade",
			),
		].map((el) => ({ el, last: "" }));
		if (items.length === 0) return;

		function motion(el) {
			const scrolled = window.scrollY;
			if (el.classList.contains("parallax-top")) {
				return {
					y: Math.min(scrolled, window.innerHeight) * 0.25,
				};
			}
			if (el.classList.contains("parallax-fade")) {
				const progress = Math.min(1, scrolled / 420);
				return { y: -63 * progress, opacity: 1 - progress };
			}
			// .parallax-center: 15% of the scroll, at most 60px either way of
			// the frame's centre.
			const frame = el.parentElement.getBoundingClientRect();
			const fromCentre = frame.top + frame.height / 2 - window.innerHeight / 2;
			return { y: Math.max(-60, Math.min(60, -fromCentre * 0.15)) };
		}

		let ticking = false;
		function update() {
			ticking = false;
			const positions = items.map((item) => motion(item.el));
			for (const [i, item] of items.entries()) {
				const { y, opacity } = positions[i];
				const next = `${y.toFixed(1)}|${opacity ?? ""}`;
				if (next === item.last) continue;
				item.last = next;
				item.el.style.transform = `translate3d(0, ${y.toFixed(1)}px, 0)`;
				if (opacity !== undefined) {
					item.el.style.opacity = String(Math.round(opacity * 1000) / 1000);
				}
			}
		}
		function onScroll() {
			if (ticking) return;
			ticking = true;
			requestAnimationFrame(update);
		}
		update();
		window.addEventListener("scroll", onScroll, { passive: true });
		window.addEventListener("resize", onScroll, { passive: true });
	}
	scrollMotion();

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
		loadSection,
		revealTreatments,
		showGallery,
		closeGallery,
		navigation,
	};
})();
