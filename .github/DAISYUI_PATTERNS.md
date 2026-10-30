# DaisyUI Patterns

DaisyUI 5 on Tailwind CSS 4, configured in `frontend/app.css` and built by `make assets`. For the full component catalogue see `.agents/skills/daisyui-expert.md`.

## Theme

One light theme, `Aesthetic`, declared with `@plugin "daisyui/theme"` in `frontend/app.css`:

| Token | Use |
| --- | --- |
| `base-100` / `base-200` / `base-300` | Page, raised and border surfaces (warm off-whites) |
| `base-content` | Body text |
| `primary` | Sage accents, focus and selected states |
| `secondary` | Terracotta calls to action on light surfaces |
| `accent` | Blush highlights |
| `neutral` | Dark bands, the footer and dark buttons |
| `info` / `success` / `warning` / `error` | Status only |

Use the tokens (`bg-base-200`, `text-secondary`, `btn-neutral`), never hex values. Radii and borders come from the theme (`--radius-*`, `--border`), so don't override them per component.

## Components in Use

| Need | Pattern |
| --- | --- |
| Buttons and calls to action | `btn` with `btn-neutral`, `btn-accent`, `btn-secondary`, `btn-outline` or `btn-ghost`; `btn-sm`, `btn-square` for icon buttons (with `aria-label`) |
| Text links | `link`, `link-hover` |
| Navigation and lists of links | `menu`, `menu-title`, `menu-sm` |
| Language switcher | `dropdown menu menu-sm` |
| Booking dialog, lightbox | native `<dialog>` with `modal`, `modal-box`, `modal-backdrop`; `modal-bottom` on phones, `modal-middle` above |
| Care notes (laser, nails) | `tabs`, `tabs-border`, `tab`, `tab-content` |
| FAQ | `collapse`, `collapse-plus`, `collapse-title`, `collapse-content` |
| Footer | `footer` |
| Loading and notices | `loading loading-spinner`, `alert` |

## Conventions

- Prefer a DaisyUI component over hand-built utility stacks; add Tailwind utilities for layout and spacing only.
- Interactive elements keep at least a 44 px target (`min-h-11`), a visible focus style and a text or `aria-label` name.
- Dialogs open with `showModal()` from a `data-on:click` expression and close with a `<form method="dialog">` button or Escape; the browser returns focus.
- Motion uses CSS (keyframes, scroll-driven animation) and stops under `prefers-reduced-motion`.
- A new DaisyUI component needs nothing more than its classes: the plugin ships them all, and Tailwind keeps only the classes the templates use.
