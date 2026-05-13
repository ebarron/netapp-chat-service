# RFC 0001 — Decoupling action-form rendering from `@edjbarron/netapp-chat-component`

- **Status**: Draft
- **Author**: ebarron
- **Date**: 2026-05-13
- **Affects**: `@edjbarron/netapp-chat-component` (npm package), `netapp-chat-service` (action-form JSON schema)
- **Tracking**: TBD

## Background

The `<ChatPanel>` component renders structured tool-call UI (action-forms,
callouts, charts) emitted by the backend agent inside fenced
`canvas-dashboard` blocks. Today the rendering of these blocks is fully
owned by the chat-component package — in particular `ActionFormBlock.tsx`
hard-codes:

- the responsive grid (`base: 1, xs: 2, sm: 3`),
- the form max-width (`maw={560}`),
- the submit / secondary button layout,
- the per-field-type widget choice (`TextInput` / `Select` / `Switch`),
- the way checkbox values are coerced to strings,
- the divider, spacing, and overall stack.

Bespoke consumers (currently only NAbox) cannot influence any of this
without forking the component or shipping a new chat-component release
for every layout tweak. Recent history bears this out — three back-to-back
patch releases (0.1.10 → 0.1.11 → 0.1.12 → 0.1.13) were all pure
visual/layout changes driven by NAbox.

This is the wrong shape. The chat-component should own *streaming, parsing,
state, and a sensible default UI*. Consumers should own *visual decisions
specific to their domain*. There are two complementary decoupling moves
that, together, eliminate the round-trip-through-npm pattern.

## Goals

1. Allow consumers to influence action-form layout without a chat-component
   release.
2. Allow consumers to fully replace the rendering of any block type when
   their needs diverge from the default.
3. Maintain 100 % backward compatibility with existing 0.1.x consumers.
4. Avoid coupling the agent (server) to consumer-specific UI vocabulary.

## Non-goals

- Replacing Mantine as the underlying UI kit.
- Removing or deprecating the built-in `ActionFormBlock`.
- Introducing a server-pushed CSS or theming system.

## Two-part proposal

### Part 1 — `netapp-chat-service` change: extend the action-form JSON schema with optional layout hints

Action-form blocks are emitted by the LLM (guided by interest docs) as
JSON. Today the schema is roughly:

```ts
type ActionFormData = {
  fields: ActionFormField[];
  submit: { label: string; tool: string; params?: Record<string, unknown> };
  secondary?: { label: string; action: 'message'; message: string };
};

type ActionFormField = {
  key: string;
  label: string;
  type: 'text' | 'select' | 'checkbox';
  // ... per-type extras
};
```

Add three optional, additive fields. None of them are required to render;
omitting them yields the current default behavior.

```ts
type ActionFormData = {
  fields: ActionFormField[];
  submit: { /* unchanged */ };
  secondary?: { /* unchanged */ };

  // NEW — all optional, all backward-compatible
  layout?: {
    /** column count per breakpoint; default { base:1, xs:2, sm:3 } capped at fields.length */
    columns?: { base?: number; xs?: number; sm?: number; md?: number };
    /** form max-width in px; default 560; pass null to disable */
    maxWidth?: number | null;
    /** spacing between fields; default 'xs' */
    spacing?: 'xs' | 'sm' | 'md';
  };
  groups?: Array<{
    /** optional section title rendered above the group */
    title?: string;
    /** field keys included in this group, in render order */
    fieldKeys: string[];
    /** layout override for this group only */
    columns?: { base?: number; xs?: number; sm?: number; md?: number };
  }>;
};

type ActionFormField = {
  key: string;
  label: string;
  type: 'text' | 'select' | 'checkbox';
  // ... per-type extras

  // NEW — all optional
  /** explicit column span (1..N) within the row; default 1 */
  span?: number;
  /** UI hint — e.g. 'switch' (default for checkbox), 'chip', 'segmented' */
  variant?: string;
  /** group field with another field on the same logical line; advisory */
  rowKey?: string;
};
```

#### Why server-side?

The interest doc author already knows the form is "wide" or "narrow" or
"has logical sub-sections" (e.g. *Identity* / *Performance* / *Monitoring*).
Pushing those decisions into the JSON spec means each consumer's
domain-specific knowledge stays in the consumer's repo (NAbox's interest
.md files), not in the shared component package.

#### Schema versioning

These fields are purely additive. Older chat-component versions will
ignore them silently (they don't exist in the current type). No protocol
version bump required.

### Part 2 — `@edjbarron/netapp-chat-component` change: render-slot props for full override

Add a new optional prop `renderers` to `<ChatPanel>` and `useChatPanel`:

```ts
type BlockRenderers = {
  actionForm?: React.ComponentType<ActionFormBlockProps>;
  callout?: React.ComponentType<CalloutBlockProps>;
  area?: React.ComponentType<AreaChartBlockProps>;
  // ... one slot per block type
};

interface ChatPanelProps {
  // ... existing props
  renderers?: Partial<BlockRenderers>;
}
```

When a consumer passes `renderers={{ actionForm: MyActionForm }}`, the
component:

1. Continues to handle parsing, validation, capability gating, and
   submit-message wiring.
2. Calls `MyActionForm` with the same `data` / `onAction` / `readOnly`
   props the built-in component receives.

To make slot overrides cheap, we also export the helpers a custom
renderer would otherwise re-implement:

```ts
export { useActionFormState } from './charts/useActionFormState';
// returns { values, setField, requiredMissing, handleSubmit }
```

This keeps the consumer's bespoke `MyActionForm` to ~30 lines of JSX
plus `useActionFormState(data)`.

## Backward compatibility

| Change | Impact on existing 0.1.x consumers |
|---|---|
| Optional `layout`, `groups`, `span`, `variant`, `rowKey` schema fields | None — current `ActionFormBlock` ignores unknown fields; defaults preserved when fields are absent. |
| New `renderers` prop on `<ChatPanel>` | None — undefined `renderers` ⇒ built-in components used (current behavior). |
| Exported `useActionFormState` hook | Pure addition. |

No symbol is removed, no prop type narrowed, no required prop added. Both
parts are SemVer **minor** changes.

## Versioning & rollout

- chat-component bump: **0.2.0** (minor, additive).
- chat-service bump: none required for protocol; only the TypeScript
  type in `@edjbarron/netapp-chat-component` needs to widen.
- Existing NAbox install on 0.1.13 keeps working unchanged after we
  publish 0.2.0; NAbox can opt in at its own pace.

## Migration plan

Two independent, non-blocking steps. They can ship in either order, or
the consumer can stop after step 1 if their needs are met.

### Step 1 — `netapp-chat-service` (this repo)

1. Extend `ActionFormData` and `ActionFormField` types in
   `packages/chat-component/src/charts/chartTypes.ts` with the optional
   fields above.
2. Update `ActionFormBlock.tsx` to honor `layout.columns`,
   `layout.maxWidth`, `layout.spacing`, `groups`, and `field.span` when
   present; preserve all current defaults when absent.
3. Document the schema additions in
   `packages/chat-component/README.md` and the canvas-dashboard spec
   in `docs/chatbot-canvas-design.md`.
4. Add unit tests for `ActionFormBlock` with and without `layout` /
   `groups` to ensure default behavior is preserved.
5. Bump chat-component to **0.2.0**, tag `chat-component-v0.2.0`, and
   publish via the existing CI workflow.

After step 1, NAbox (and any other consumer) can change form layout by
editing interest .md files only — no chat-component release needed for
NAbox-driven layout tweaks.

### Step 2 — `@edjbarron/netapp-chat-component` slot API

1. Refactor `<ChatPanel>` so each block type is rendered through an
   internal `renderers` object that defaults to the built-ins.
2. Add the `renderers?: Partial<BlockRenderers>` prop and forward it.
3. Extract the per-block stateful logic into hooks (start with
   `useActionFormState`); export them from the package's public surface.
4. Add a unit test that overrides `actionForm` with a stub component
   and asserts the stub is rendered with the expected props.
5. Document the slot API in README with a worked example.
6. Tag and publish (same release as step 1 if both land together;
   otherwise minor bump again).

After step 2, NAbox can replace the entire action-form UI with its own
Mantine code that uses NAbox's design tokens, while still relying on
the chat-component for streaming and capability handling.

## Open questions

1. Should `groups` allow nested groups for tabbed forms? (Tentatively no
   — defer until a real need appears.)
2. Should `field.variant` be free-form strings or a typed enum? Free-form
   is more extensible but forfeits compile-time safety. Suggest:
   typed enum per field type, with `variant?: string & {}` escape hatch.
3. Should renderers receive the full `<ChatPanel>` context (theme, mode,
   capabilities) via React context instead of props? Probably yes for
   step 2 — simpler override surface.

## Alternatives considered

- **Theme-only prop bag**: covers the easy cases (column count, max-width)
  but every new knob needs a release. Rejected — doesn't move enough
  decisions out of the package.
- **Headless / zero-JSX**: maximally decoupled but forces every consumer
  to ship a full rendering layer. Overkill for one consumer; revisit if
  a third party adopts the component.
- **Server-driven CSS**: lets the backend dictate look-and-feel. Bad
  separation of concerns — the agent should not know about pixels.
