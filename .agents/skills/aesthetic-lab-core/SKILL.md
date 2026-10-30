---
name: aesthetic-lab-core
description: Project-specific guidance for Aesthetic Lab's Go, templ, Datastar and DaisyUI site, its texts, its Firestore content, and verification.
---

# Aesthetic Lab Core

Read `AGENTS.md` first, then the relevant `.github/*.md` guide for the files you are touching.

Use this skill for site work that needs a fast reminder of the repo's core rules:

- Go test-first at public seams (`httptest`, rendered components); `make ci` is the gate
- templ views take data as arguments and never fetch
- Datastar for interaction, DaisyUI components and theme tokens for styling, no inline styles or scripts
- English-keyed texts in all five `internal/i18n/locales/*.json` files, then `make i18n`
- content from Firestore through `internal/content`, read-only and fail-soft
- template and text changes take effect only after a deploy and promotion
