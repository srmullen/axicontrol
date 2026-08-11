# Per-upload print page as the single job-creation entry point

The uploads list and the generic `/jobs` page each grew their own way to start work on a file: uploads had an inline whole-file preview, `/jobs` had a "pick any uploaded file from a dropdown" new-job form. This produced two disconnected UIs for the same action, and left Layers-mode's manual pass-advance step showing only "Pass 2/4" with no indication of which physical layer that meant.

We're consolidating: the uploads page becomes the app's home page (list, drag-and-drop single-file upload, delete only — no inline preview). Clicking an upload navigates to a new page scoped to that file, which is the sole place a Job can be created; `/jobs`'s new-job form is removed, leaving `/jobs` as pure cross-file history. The print page shows the upload's current in-progress Job (if any) inline, including per-layer labels and previews during Layers-mode pauses, and shows the AxiDraw's global busy state (even when the busy Job belongs to a different upload) rather than only rejecting a submission after the fact.

We're also adding a third Plot Mode, **Single Layer** — one Pass against exactly one chosen Layer, distinct from the existing **Layers** mode which sequences through every discovered Layer with manual pauses between passes.

## Considered options

- Keep `/jobs`'s generic form as a second way to start a Job, alongside the new per-upload flow — rejected as redundant; two paths to create the same thing invites drift and confusion.
- Render per-layer previews client-side by toggling visibility on an inlined SVG — rejected; ADR-0010 requires uploaded SVGs be rendered only via `<img>` so the browser's script-blocking applies, which inlining would bypass. Per-layer preview instead needs a new backend endpoint that renders an isolated-layer SVG, served the same `<img>`-only way.
