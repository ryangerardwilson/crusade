# AGENTS.md

<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always follow OpenSpec before coding when the request:
- Mentions planning or proposals (proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or
  material performance/security work
- Sounds ambiguous and you need the authoritative spec first

**TL;DR**
- Search: `npm exec -- openspec list --specs` and `npm exec -- openspec list`
- Behavior change: create a change → write proposal/deltas/tasks →
  `npm exec -- openspec validate <id> --strict --no-interactive` → implement →
  archive
- Skip a change for: bug fixes that restore intended behavior, typos,
  formatting, non-breaking config, tests for existing behavior
- Specs are truth. Changes are proposals. Keep them in sync.
- Project context and artifact rules live in `openspec/config.yaml`.

Keep this managed block so `openspec update` can refresh the instructions.
<!-- OPENSPEC:END -->

## Spec-driven development

This repository uses OpenSpec for product and engineering intent.

| Layer | Owns | Location |
| --- | --- | --- |
| Accepted truth | Normative behavior and contracts | `openspec/specs/<capability>/spec.md` |
| Proposed truth | One reviewable behavioral change | `openspec/changes/<change-name>/` |
| Implementation | How accepted behavior works | project source |
| Product prose | Current operator contract | `README.md`, `PROJECTSCOPE.md` |

When implementation and an accepted spec disagree, determine whether the spec
is wrong or the implementation is non-conforming. Never silently promote
current code or stale prose to product truth. Until a capability spec exists,
`README.md` and `PROJECTSCOPE.md` are the operator contract.

## Start here

For every non-trivial task:

1. Read `openspec/config.yaml`.
2. Run `npm exec -- openspec list --specs` and
   `npm exec -- openspec list` when repository state is not already known.
3. Load only the capabilities relevant to the task.
4. Inspect `git status --short` and preserve unrelated changes.
5. Classify the work:
   - New or changed behavior, architecture, or operations: create or continue
     one named OpenSpec change before implementation.
   - Conformance bug where an accepted requirement already says the right
     thing: cite it, fix implementation, and add a regression; do not invent a
     spec delta.
   - Typo, formatting, dead code, or internal refactor with no observable
     contract change: edit directly and verify proportionately.

## OpenSpec lifecycle

For changed intent:

1. Create a verb-led kebab-case change:
   `npm exec -- openspec new change <change-name>`.
2. Write `proposal.md` with the problem, outcome, scope, non-goals, and
   affected capabilities.
3. Add complete deltas under
   `openspec/changes/<change-name>/specs/<capability>/spec.md`.
4. Put boundaries, failure behavior, and trade-offs in `design.md`; put
   ordered, independently verifiable work in `tasks.md`.
5. Run
   `npm exec -- openspec validate <change-name> --strict --no-interactive`
   before implementation.
6. Implement tasks in order and mark a task complete only with evidence.
7. Archive after verification and keep accepted specs in sync.

Do not edit canonical specs directly to smuggle in new behavior.

## Project identity

Repository: `crusade` at `github.com/ryangerardwilson/crusade`.

crusade is one Go binary. The command is `crusade`. Modes are `do`, `align`,
`box`, and `nlp`. Config stays at `~/.config/xyz/config.json`. The database is
`~/.sqlx/data/crusade.sqlite`, opened through `sqlite3`. Practice sessions stay
in the sqlx database `nlp`.

Do not add this module to the home `go.work`. Test with `GOWORK=off go test ./...`.
Work and commit inside this Git root.

Project-local OpenSpec specs and this `AGENTS.md` outrank outside docs for
facts about this repository.

<!-- rgw-ast:begin -->
# c3po ast boundary

Run `c3po ast status --json` before the first edit in a repository. When it reports
`"enforced": true`, the `c3po ast hook` PreToolUse hook enforces the rules below. Every
change is checked against the file's hash, so concurrent agents and stale context
cannot silently overwrite a file.

- **Trace** code before editing it: `c3po ast callers|callees|impact <symbol>`, then
  `map` and `show`. `search` is for a literal you already know. `c3po ast help callers`
  explains confidence levels and partial results.
- **Read** with `c3po ast read <file> --lines A-B`, or a host Read with `limit` of at
  most `max_read_lines` (200 by default).
- **Edit** with `c3po ast patch <file> --expect-hash <sha> --old <text> --new <text>`.
  `c3po ast hash <file>` prints the hash; each patch prints the next one. For several
  files at once: `c3po ast apply --from-file <manifest.json|git diff>`.
- **Create** with a host Write to a new path, or `c3po ast create <file> --expect-absent --stdin`.
- **Move** with `c3po ast move <src> <dst> --expect-hash <sha>`; **delete** with
  `c3po ast delete <file> --expect-hash <sha>`.
- **Generators** listed in `generators.allow` (for example `openspec new|archive`) run as
  `c3po ast exec -- <command>`.
- A denied tool call's reason names the exact command to run instead.

Unenforced repositories use normal tools. Policy is global
(`~/.config/c3po/ast/config.toml`). A nested AGENTS.md does not need to repeat this block.
<!-- rgw-ast:end -->
