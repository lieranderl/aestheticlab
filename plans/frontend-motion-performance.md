# Frontend Motion and Performance

## Goal

Improve perceived responsiveness, transitions and asset delivery in the Go site,
then release v5.0.1 through staging and the existing production promotion workflow.

## Scope and assumptions

- Preserve the design, five locales, native dialog accessibility and reduced motion.
- Preserve advanced Consent Mode v2 and its current analytics loading behavior.
- Keep one render-blocking stylesheet; optimize font discovery before introducing
  critical-CSS duplication or changing CSP. No infrastructure changes.
- The Lighthouse main-thread "Other" category needs a trace; do not claim that
  smaller assets resolve its entire 1.8 seconds or promise additive audit savings.
- Preserve the existing untracked `.claude/` directory.

## Phases and gates

1. Add failing regression tests for dimensioned, on-demand lightbox images,
   loading feedback and viewport-preserving service switches. Implement a decoded
   image handoff with stale-request protection, reduced-motion fade, and a bounded
   transition for service patches. Verify Go rendering and browser tests.
2. Shorten hero/reveal timings and make the primary CTA visible immediately.
   Animate native mobile-menu entry/exit without delaying focus restoration.
   Move continuous progress to CSS with an animation-frame fallback, keep header
   height stable, and batch parallax reads before writes. Verify all three browsers,
   keyboard behavior and reduced motion; make a manual visual check.
3. Convert the existing Qestero font to WOFF2, preload the heading/body fonts,
   remove decorative symbol-font demand where practical, lazy-load service
   thumbnails and encode the flagged gallery assets more efficiently. Compare
   bytes and image quality, rebuild generated manifests and templates.
4. Run `make ci`, `bun run verify`, Biome, Markdown lint and the Go cross-browser
   suite. Review the complete diff; resolve material findings before release.
5. Commit with acting-agent attribution, push the feature branch and open a PR
   targeting `staging`. Wait for required checks, squash merge and wait for the
   staging deployment. Publish v5.0.1 at that verified commit, monitor canary,
   smoke tests and promotion to completion; verify production assets and UI.

## Commit strategy

One coherent frontend/release commit after local verification, with generated
Go sources included. Any review fixes get a separate focused commit. Never commit
build output, reports, secrets or the untracked assistant directory.

## Verification record

- Planning: source and live frontend reviewed; branch starts at a4a742a.
- Initial browser regressions: all three failed on the original behavior, then
  passed after implementation. Home and motion suite: 84/84 passed across
  Chromium, Firefox and WebKit before the final review fix.
- Independent review found the category chip's vertical ancestor scrolling;
  a 390x320 regression reproduced it (heading at 27.8px). Horizontal-only chip
  scrolling passes with the heading below the fixed header.
- Qestero WOFF2 is 10,432 bytes versus 29,300 bytes for the retained legacy TTF.
  The distinct filename avoids a legacy asset-redirect stem collision.
- Flagged WebP variants shrink 9-10%: pink 800px 97,612 to 88,414 bytes;
  white 400px 34,816 to 31,480; lashes 400px 24,358 to 22,148. Other images
  retain their encoding quality. Visual checks preserve the existing typography.
- Decisions: retain all Qestero glyphs because no subsetting license was present;
  preserve analytics loading, the stylesheet/CSP and the existing video behavior.
  These avoid speculative measurement, rendering and content changes.
- Full Go browser suite: 114/114 passed, including gallery races, keyboard focus,
  short viewports, reduced motion and absent View Transition support. `make ci`,
  `bun run verify`, Biome and Markdown lint passed. Manual desktop visual check
  confirms the existing layout and font appearance.
- The header keeps its established scrolled height (64px) at all positions, so
  sticky treatment/price navigation and anchor offsets remain aligned.
- Deployment results follow in the PR and release records.
