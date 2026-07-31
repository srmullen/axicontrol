# Issue tracker: Taipir (local task board)

Issues and work items for this repo live as taipir tasks — markdown files under `tasks/` (per `taipir.toml`), indexed into a gitignored SQLite database at `.taipir/index.db`. The database is derived; the markdown files are the source of truth.

## Conventions

- One task per file: `tasks/<task-name>.md`, where `<task-name>` matches the `name:` frontmatter field
- YAML frontmatter carries structured fields: `name`, `description`, `lane`, `status`, `tags`, `priority`, `assigned-to`, `created-at`, `created-by`, `depends-on`, `related`, `parent`
- Workflow position is the `lane:` field, moving through the lanes configured in `taipir.toml`: `backlog → todo → in-progress → review → done → cancelled`
- Triage state is recorded as `tags:` entries (comma-separated), independent of lane — see `triage-labels.md` for the role → tag mapping
- Body is free-form markdown: requirements/description first, then an optional `## Child Tasks` table linking decomposed subtasks, then an `## Execution Report` (or `## Comments`) section appended at the bottom for results and conversation history

## When a skill says "publish to the issue tracker"

Prefer the CLI so the index stays in sync:

```
taipir create --name <slug> --description "..." --lane backlog --tags <role-tag,...>
```

Editing the markdown file directly under `tasks/` also works — run `taipir sync` (or `taipir build` to reindex everything) afterward.

## When a skill says "fetch the relevant ticket"

Run `taipir show <task-name>`, or read `tasks/<task-name>.md` directly. The user will normally give the task name (the `name:` frontmatter field, which matches the filename stem).

## Wayfinding operations

Used by `/wayfinder`. Taipir has no dedicated map/child file convention, so this adapts the built-in one onto taipir's native relationship fields — adjust if it doesn't fit in practice:

- **Map**: the parent task's own file — created with `taipir create --name <effort> ...`, holding the Notes / Decisions-so-far / Fog body.
- **Child ticket**: `taipir create --name <effort>-NN-<slug> --parent <effort> --depends-on <NN,NN>`, with the question in the body.
- **Blocking**: encoded via `--depends-on` (comma-separated task names). A ticket is unblocked when every task it depends on has `lane: done`.
- **Frontier**: `taipir list --parent <effort> --lane backlog` (or the first configured lane), filtered to tasks with no `assigned-to` set; first by number wins.
- **Claim**: set `assigned-to` and `lane: in-progress`, then `taipir sync`.
- **Resolve**: append the answer under `## Execution Report`, set `lane: done`, `taipir sync`, then append a context pointer (gist + link) to the parent map's Decisions-so-far section.
