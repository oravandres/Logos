# AGENTS.md

This repository is a production-grade Go service deployed to Kubernetes and backed by PostgreSQL.

## Primary goals
- correctness
- maintainability
- operational safety
- observability
- secure defaults
- stable public behavior

## Required working style
- Make the smallest safe change that fully solves the task.
- Follow existing package and constructor patterns before introducing new ones.
- Prefer standard library solutions unless there is a strong reason not to.
- Keep handlers thin and business logic outside transport code.
- Pass context through request, database, and outbound call boundaries.
- Preserve backward compatibility unless explicitly asked to break it.

## Go expectations
- Avoid `any` unless absolutely necessary.
- Avoid panic for normal flow.
- Return wrapped errors with useful context.
- Prefer explicit SQL and clear transaction boundaries.
- Do not create interfaces before a consuming boundary needs one.
- Prefer table-driven tests where they improve clarity.

## Kubernetes expectations
- Readiness reflects ability to safely serve traffic.
- Startup probes protect slow initialization.
- Liveness is for unrecoverable unhealthy states, not general slowness.
- Respect graceful shutdown and in-flight request draining.
- Never use `:latest` in production manifests.
- Do not weaken security context to make something pass.

## PostgreSQL expectations
- No `SELECT *` in critical query paths.
- Use explicit columns and query shapes.
- Keep transactions short.
- Do not hold transactions across network calls.
- Consider constraints and indexes as part of correctness, not an afterthought.
- For important query changes, require `EXPLAIN` or `EXPLAIN ANALYZE`.

## Observability expectations
- Use structured logs.
- Add metrics and traces for critical flows.
- Include request and trace correlation where available.
- Never log secrets, tokens, passwords, DSNs, or raw credentials.

## Change checklist
Before finishing:
- types and imports are correct
- tests were added or updated where needed
- no obvious security regression was introduced
- no obvious shutdown, migration, or rollout risk was introduced
- docs were updated if behavior changed