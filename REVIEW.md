# Review Guidelines

- Apply `AGENTS.md`; report only actionable findings introduced by the change.
- Prioritize correctness, security, regressions, missing tests, and deployment safety over style.
- Start with findings ordered by severity; cite file and tight line range for each finding.
- State “No findings” when appropriate, then list residual test gaps or risks.
- Verify Go changes came with tests at a public seam and that `make ci` passes.
- Verify new visible strings go through `tr`/`trf`/`trc`, exist in all five locale files, and `messages_gen.go` was regenerated with `make i18n`.
- Verify views take data as arguments and never fetch; content reads stay in `internal/content`, bounded and fail-soft.
- Verify UI uses Datastar and existing DaisyUI components, remains keyboard-accessible, uses semantic tokens, and adds no inline styles or scripts that would break the CSP.
- Verify dependency changes are necessary, reflected in `go.sum` or `bun.lock`, and free of avoidable high/critical advisories.
- Verify delivery builds once, uses immutable Artifact Registry digests, promotes the same artifact, and authenticates through OIDC/WIF.
- Verify Cloud Run/IAM/secret/monitoring changes are declared in `infra/`, least-privilege, and rollback-safe.
- Reject any leaked `.env`, secret value, key, sensitive plan output, or credential in logs.
- Skip formatting-only noise and non-contradictory Markdown nits.
