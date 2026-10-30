# Prompt Pack

Reusable prompts for this repository. Keep prompts short; let `AGENTS.md` carry durable policy.

Read first:

- `AGENTS.md`
- `plans/README.md` for complex work
- `REVIEW.md` for review-only work

## Planning Prompt

```text
Read AGENTS.md and plans/README.md first.

Goal:
[Describe the change.]

Scope:
- [files or areas in scope]
- [user-visible behavior in scope]

Out of scope:
- [things that must not change]

Constraints:
- Follow AGENTS.md exactly.
- Preserve Go, Datastar, DaisyUI, i18n, content, and deployment boundaries.
- Avoid duplicate policy or unnecessary new docs.

Deliverable:
Create or update plans/[task-slug].md with:
- goal
- scope / non-goals
- assumptions
- ordered tasks
- verification
- risks or follow-ups

Do not implement unless explicitly asked.
```

## Plan Review Prompt

```text
Read AGENTS.md, REVIEW.md, and plans/README.md first.

Review this plan against the actual codebase:
plans/[task-slug].md

Rules:
- Inspect real files before judging the plan.
- Do not rewrite approved phases unless they are unsafe or stale.
- Append a section titled "Review Findings".
- Add only missing steps, risks, sequencing fixes, verification gaps, or convention conflicts.
- Flag conflicts with AGENTS.md, current repo structure, or CI workflows.

Output:
- update the plan file
- summarize the highest-risk findings
```

## Implementation Prompt

```text
Read AGENTS.md, plans/[task-slug].md, and any relevant .github guide first.

Execute the next approved phase from:
plans/[task-slug].md

Constraints:
- Stay within scope.
- Do not silently expand the task.
- If assumptions break, update the plan before continuing.
- If workflow files change, keep AGENTS.md, README.md, REVIEW.md, plans/README.md, and docs/prompt-pack.md aligned.

Deliverable:
- implement only the requested phase
- summarize files changed, completed work, verification, and residual risks
- provide fresh verification evidence before claiming success
```

## Verification Prompt

```text
Read AGENTS.md, REVIEW.md, and plans/[task-slug].md first.

Verify the implementation against:
plans/[task-slug].md

Check:
- required files changed
- repo conventions preserved
- no duplicated or contradictory workflow guidance introduced
- docs stayed aligned if workflow/config files changed
- implementation matches the approved plan

Verification:
- run the smallest relevant checks from AGENTS.md
- run make ci for site, dependency, build, or deployment changes
- report only what fresh command output proves

Output:
- pass/fail status
- concrete findings with file references
- residual risks if no blocking issue exists
```

## Small Bugfix Prompt

```text
Read AGENTS.md first.

Fix this issue:
[bug description]

Scope:
- likely files: [path1], [path2]
- keep the change minimal

Constraints:
- preserve current architecture
- no unrelated refactors
- follow the Go, Datastar, DaisyUI, i18n, and content rules from AGENTS.md

Deliverable:
- fix the bug
- explain root cause briefly
- run the smallest relevant verification, then make ci
```

## UI Change Prompt

```text
Read AGENTS.md, .github/DAISYUI_PATTERNS.md, and .github/COMPONENT_GUIDE.md first.

Goal:
[describe the UI change]

Scope:
- views: [internal/views paths]
- handlers: [internal/server paths]

Constraints:
- DaisyUI 5 first
- theme tokens only
- Datastar for interaction, no inline styles or scripts
- visible strings remain translatable
- preserve mobile-first behavior
- reuse existing component patterns when they fit

Deliverable:
- implement the UI change
- summarize reused patterns, translation updates, accessibility, and verification
```

## Content Change Prompt

```text
Read AGENTS.md, .github/DATA_LOADING.md and docs/CONTENT.md first.

Goal:
[describe the content or data-model change]

Scope:
- internal/content files: [paths]
- views that render it: [paths]

Constraints:
- tests first, over documents, in internal/content
- views take data as arguments and never fetch
- localized fields keep the <name>_<lang> pattern with English fallback
- reads stay bounded and fail soft (last good content, then the snapshot)
- the runtime stays read-only with no keys

Deliverable:
- implement the change and refresh the snapshot with make snapshot
- summarize the field changes, fallback behavior, and verification
```

## Docs Alignment Prompt

```text
Read AGENTS.md, REVIEW.md, plans/README.md, and docs/prompt-pack.md first.

Task:
Audit and align the repo's workflow and agent-facing docs.

Check:
- AGENTS.md is canonical and concise
- README.md is onboarding-oriented
- REVIEW.md contains review-only guidance
- plans/README.md contains planning workflow only
- thin entry points do not duplicate policy

Deliverable:
- make the smallest edits needed to reduce duplication and contradiction
- run Markdown linting for changed Markdown files
```

## New Feature Prompt

```text
Read AGENTS.md and plans/README.md first.

Goal:
Add [feature name].

Scope:
- in scope: [handlers/views/docs/config]
- out of scope: [unrelated pages, infra, redesigns]

Constraints:
- create or update a plan first
- follow repo conventions exactly
- prefer existing patterns over new abstractions
- document only what is truly new

Deliverable:
1. Write plans/[feature-slug].md.
2. Wait for review before implementation.
```

## Refactor Prompt

```text
Read AGENTS.md and the relevant .github guides first.

Goal:
Refactor [target] to improve [clarity / duplication / maintainability].

Constraints:
- no behavior change unless explicitly required
- no broad cleanup outside touched scope
- preserve current architectural boundaries
- reduce duplication, not abstraction for its own sake

Deliverable:
- explain the current problem briefly
- make the refactor
- note any doc updates required
- run the relevant verification from AGENTS.md
```

## Good Habits

- Name the deliverable file.
- Say what is out of scope.
- Tell the agent whether it is planning, implementing, reviewing, or verifying.
- Require fresh verification.
