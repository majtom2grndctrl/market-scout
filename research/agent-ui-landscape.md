# Agent UI Landscape

> **Note — epic reframed; conclusions stand.** The "Conversational Analysis Surface" is now the Compositional Analysis Surface (`composition-grammar.md`). The calls below survive the reframe and strengthen under it: build the contract don't buy it, and d3 as math with app-owned marks.
>
> **Purpose:** Decision support for the Compositional Analysis Surface epic. Which layer of the agent-to-UI stack to buy, and which to build.
> **Sources:** Vendor docs and Chromium feature status, read 2026-08-15.
> **Status:** Current on transport and charts. Product framing reframed by `composition-grammar.md`.

---

## Three Layers

Model access, orchestration, and agent-to-UI transport are separate concerns. The repo owns the first two and none of the third.

| Layer | Concern | Repo today |
|---|---|---|
| Model access | Provider routing, one key, token and tool-call deltas | `@langchain/openrouter` |
| Orchestration | Tool loop, memory, multi-step | `@langchain/core` |
| Agent-to-UI transport | Stream to correct React tree | Nothing |

OpenRouter has no opinion about React and does not intend to. "What would a UI toolkit add over the provider SDK" is entirely a layer-three question.

## What Layer Three Costs To Build

- **Partial JSON.** Tool arguments arrive as fragments. Rendering a component before its arguments finish needs an incremental parser producing a valid partial object at every token boundary.
- **Message parts.** An assistant turn is an ordered list — text, reasoning, tool call, tool result — not a string. Interleaving prose and components in order, each result bound to its call, is a state machine.
- **Tool locality.** Some tools reach Postgres; some read UI state in the browser. The split is a routing concern.
- **Persistence.** Reload rebuilds the transcript, including live components and their state.
- **Stream control.** Abort mid-response, resume after refresh, pause for approval.

None of it is hard. Together it is weeks, and none of it shows up in a case study.

## Candidates

| | AI SDK | CopilotKit |
|---|---|---|
| Scope | Transport and message parts | Agentic app framework |
| Generative UI | You map tool parts to components | Shipped, with conventions |
| State sharing | Not covered | Readable state, frontend actions |
| Backend | Any provider, OpenRouter included | AG-UI into LangGraph |
| What it decides for you | Little | The agent-to-UI contract |

AG-UI is the open event protocol underneath CopilotKit, adopted past it — Microsoft Agent Framework, AWS Bedrock AgentCore, Pydantic AI. Worth reading as an enumeration of the events this kind of system needs, whether or not either library ships.

## Direction

Take AI SDK. Own the contract.

The registry, the state projection, and the recovery affordances are the designed artifact of this epic (`project.md` §Why it exists). CopilotKit decides all three. Building on it lands the design work as configuration of someone else's decisions, and a reviewer who knows the library reads it that way. AI SDK stops at transport and leaves the contract open.

Keep OpenRouter. One key across model families makes component-selection accuracy measurable per model for pocket change, which is what `component-selection-evals` rests on.

## Charts

**Rejected — chart component libraries** (Recharts, nivo, Chart.js, Tremor, shadcn's Recharts block). Fast to a first chart, then a wall. Every bespoke mark fights a props API, and theming becomes a second system running beside the design tokens.

**Rejected — spec-driven grammars** (Observable Plot, Vega-Lite). Terse, and a model writes them well. Styling escape hatch is narrow. Useful for exploring what the data says; not a product surface.

**Taken — d3 modules as math, marks rendered by the app.** Charts become JSX carrying Tailwind classes, so they inherit tokens, dark mode, and type scale with nothing to theme. Agents edit markup rather than an opaque config surface, and verify by rendering.

**Escape hatch — canvas** (uPlot, WebGL) for a density surface SVG cannot carry: a full company-by-day matrix, not a trend line. No such screen is planned.

## HTML-in-Canvas

Not applicable. `ctx.drawElement()` sits behind `chrome://flags/#canvas-draw-element` in Chromium 147+ — no origin trial, no Firefox or WebKit implementation announced, spec still a WICG living explainer.

It solves DOM loss inside an existing canvas scene. This app has no such scene: SVG keeps selectable text, focusable marks, and ARIA for free, so there is nothing to buy back. Revisit only alongside the canvas escape hatch above.

## Sources

- AG-UI protocol — https://docs.ag-ui.com/introduction
- AG-UI in Microsoft Agent Framework — https://learn.microsoft.com/en-us/agent-framework/integrations/ag-ui/
- AG-UI on AWS Bedrock AgentCore — https://aws.amazon.com/blogs/machine-learning/build-generative-ui-for-ai-agents-on-amazon-bedrock-agentcore-with-the-ag-ui-protocol/
- HTML-in-Canvas browser support — https://html-in-canvas.dev/docs/browser-support/
- HTML-in-Canvas developer testing announcement — https://groups.google.com/a/chromium.org/g/blink-dev/c/LYJyOdLbOfY
