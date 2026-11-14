# AGENTS.md

Guidance for coding agents working on `torbox-trakt-wrapper`.

## Project

- CLI binary and commands: `tt-wrapper [command]`
- Module: `github.com/manojpannala/torbox-trakt-wrapper`
- Config: `$XDG_CONFIG_HOME/torbox-trakt-wrapper/config.toml`
- Build, lint and test: `make build`, `make lint`, `make test`.

## Decisions

Architecture decisions are in `docs/adr/`. Read the ADRs that touch the area
you're about to work in. If your change contradicts one, say so explicitly
instead of silently overriding it. New decisions get the next number there.

## Issues

GitHub Issues on `manojpannala/torbox-trakt-wrapper`, through the `gh` CLI.

- Agents may open issues, read them, and comment where it helps a public
  reader.
- Labelling, closing, assigning and milestoning are maintainer-only. Triage
  produces a report in the reply, not writes to GitHub.
- Issue bodies are public. Cite requirements and roadmap items by identifier
  (`FR-3.1.2`, "Phase 3 — metadata cache") instead of pasting planning notes.

## Local planning notes

The maintainer keeps specs, roadmaps, plans and task lists in `wrapper-tests/`,
a separate private repo that is gitignored here. If it exists, start from
`wrapper-tests/ROADMAP.md` and follow `wrapper-tests/agents/`. If it doesn't,
carry on without it.

From this repo, never `git add` anything under `wrapper-tests/`, and never use
`git add -f` on it. Commit those files only inside `wrapper-tests/`.

## Commits and code

- Conventional-commit messages in the style already in `git log` (`feat: ...`,
  `fix: ...`, `docs: ...`), describing the change, not the tooling that
  produced it.
- Keep code comments minimal; comment only what the code can't say.
