# Frontend stack: server-rendered Go templates + htmx, no separate JS toolchain

The frontend is server-rendered: stdlib `html/template` pages served directly by the Go backend, with htmx handling interactivity (form submissions, partial page updates) and its SSE extension consuming the live Job/Pass status stream already decided in [ADR-0006](./0006-webhook-and-sse-notifications.md). Styling is plain hand-written CSS. There is no separate frontend toolchain — no Node, no npm, no bundler, no build step for the UI — the single Go binary serves everything, mirroring the single-backend-service decision in [ADR-0007](./0007-single-backend-service.md).

This is a deliberate departure from the more common SPA-plus-JSON-API shape (React/Svelte calling a backend API). It fits because this is a single-user application with a modest set of forms and status views, not a product with a large or evolving UI surface — the cost a SPA would normally buy back (rich client-side state, a component ecosystem) isn't a cost this app is paying in the first place, while the toolchain it would add (a second language runtime, a build/watch step, a second set of dependencies to keep current) is a real, ongoing cost this app has no matching benefit for.

Templating uses stdlib `html/template` rather than `templ` (compile-time typed Go templates) — runtime-parsed templates are one less dependency and no codegen step, and `html/template`'s context-aware auto-escaping is directly relevant given the XSS concerns already surfaced in [ADR-0010](./0010-svg-sanitization-and-img-preview.md).

## Considered Options

- React or Svelte SPA — rejected: would add a second language/toolchain (Node, bundler, npm deps) for a UI scope (forms, status views, file library) that doesn't need SPA-grade interactivity.
- `templ` for typed templates — rejected: compile-time template safety is a real benefit, but not worth the added codegen step and dependency for this UI's size.
- Tailwind CSS or a classless framework (Pico.css) for styling — rejected in favor of plain CSS: the page count is small enough that hand-written styles stay manageable without a utility framework or build step.
