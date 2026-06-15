# Host-Driven Prompt Injection

> **Status:** Draft
> **Audience:** Engineers working on `@edjbarron/netapp-chat-component` and any
> host product that embeds the chat panel.
> **Last Updated:** June 14, 2026
> **Related:** [reusable-chat-interface.md](reusable-chat-interface.md),
> [chatbot-design-spec.md](chatbot-design-spec.md)

---

## 1. Problem Statement

A host product that embeds `<ChatPanel>` may want to **launch the assistant from
its own UI with a pre-formed prompt** — for example, a button on some object
page that opens the assistant and immediately asks a question about whatever the
user is looking at. This is a generic capability: "deep-link a question into the
assistant."

Today this is impossible. `ChatPanel` owns its chat state internally via
`useChatPanel()`. The only inputs a host can supply are static
`suggestedPrompts` / `bookmarkPrompts` that the **user** must click. There is:

- **No way to push a prompt** into the panel programmatically.
- **No way to read the panel's busy state** (`streaming`) from the host, so the
  host cannot disable its trigger control while a turn is in flight.

This spec adds a small, generic, reusable mechanism to the chat component so any
host can drive a one-shot prompt and observe busy state. It is intentionally
**product-agnostic** — the component change knows nothing about any particular
host feature.

## 2. Goals / Non-Goals

### Goals
1. A host can open the panel and **auto-send one prompt** programmatically.
2. The host can observe **busy state** so its trigger control can be disabled
   while the assistant is streaming.
3. The mechanism is **generic** — usable by any embedding product and any page.
4. **No regressions**: embeds that pass neither new prop behave exactly as today.

### Non-Goals
- Multi-prompt queuing beyond "replace the pending prompt." If the panel is busy
  the host is expected to disable its trigger, so a second prompt cannot be
  submitted until the first completes.
- Driving an *external* `useChatPanel()` instance. The fix keeps the panel as the
  single owner of chat state.
- Changing the SSE/transport contract. This is a frontend-component change only.

## 3. Current Behaviour (verified)

- `src/ChatPanel.tsx` calls `useChatPanel({ defaultMode })` internally and
  destructures `sendMessage`, `streaming`, etc. Public props today:
  `opened, onClose, title, subtitle, suggestedPrompts, bookmarkPrompts,
  fullPage, defaultMode`.
- `src/useChatPanel.ts` holds `streaming` state and `sendMessage(text)` no-ops
  when `!text.trim() || streaming`.
- `handleSuggestedPrompt(prompt)` already calls `sendMessage(prompt)` — the
  one-shot send path exists internally; it just isn't reachable from the host.
- Current published version: **0.1.14** (see `CHANGELOG.md`). This change ships in
  the next minor release.

## 4. Design

Three new **optional** `ChatPanel` props, controlled-prop style (matches the rest
of the component API; no imperative ref needed).

```ts
interface ChatPanelProps {
  // …existing…

  /**
   * A prompt to auto-send once. When this value changes to a non-empty
   * string while the panel is `opened` and not streaming, the panel sends
   * it as a user message exactly once, then calls `onPromptConsumed`.
   * The host should clear its own state in `onPromptConsumed` so the same
   * prompt isn't resent on a later re-render.
   */
  pendingPrompt?: string;

  /** Called after `pendingPrompt` has been submitted. */
  onPromptConsumed?: () => void;

  /**
   * Notifies the host when the assistant's busy state changes, so the host
   * can disable a trigger control while a turn is streaming.
   */
  onBusyChange?: (busy: boolean) => void;
}
```

### 4.1 Auto-send effect (inside `ChatPanel`)

```tsx
const lastSentRef = useRef<string | null>(null);

useEffect(() => {
  if (!opened) return;
  const p = pendingPrompt?.trim();
  if (!p || streaming) return;           // wait until panel open & idle
  if (lastSentRef.current === p) return; // guard against double-send
  lastSentRef.current = p;
  sendMessage(p);
  onPromptConsumed?.();
}, [opened, pendingPrompt, streaming, sendMessage, onPromptConsumed]);
```

Gating on `!streaming` means that if the host opens the panel mid-turn, the
prompt is held until the current turn finishes, then sent. In normal use the host
disables its trigger while busy (via `onBusyChange`), so this is a safety net.

Note on the dedup ref: it stores the **last sent string**. A host that wants to
re-send identical text clears `pendingPrompt` to `''` between sends (it does so
in `onPromptConsumed`); combined with setting a fresh non-empty value, this lets
intentional re-sends through while still blocking accidental double-sends on
unrelated re-renders. (See test 5 in §6.)

### 4.2 Busy-state notification

```tsx
useEffect(() => {
  onBusyChange?.(streaming);
}, [streaming, onBusyChange]);
```

`streaming` already exists in `useChatPanel`'s return; we surface it to the host.

### 4.3 Why not an imperative `ref`/controller?

A `useImperativeHandle` controller (`ref.current.send(text)`, `ref.current.busy`)
is also viable but:
- Controlled props compose better with host React state and are easier to test.
- The host's natural model is "I have a prompt string in state; render the panel
  with it" — a prop, not a method call.
- Busy state as a callback avoids the host polling a ref.

If a future need arises (e.g. cancel-from-host), add a ref then. Not now.

## 5. Host Integration Pattern (reference, non-normative)

This is how a host *would* consume the feature; it is not part of the component
contract. A typical host owns `opened` via some disclosure hook and adds two
pieces of state:

```tsx
const [pendingPrompt, setPendingPrompt] = useState('');
const [assistantBusy, setAssistantBusy] = useState(false);

const askAssistant = useCallback((prompt: string) => {
  setPendingPrompt(prompt);
  openPanel();
}, [openPanel]);

<ChatPanel
  opened={opened}
  onClose={closePanel}
  pendingPrompt={pendingPrompt}
  onPromptConsumed={() => setPendingPrompt('')}
  onBusyChange={setAssistantBusy}
  /* …existing props… */
/>
```

The host typically exposes `askAssistant` + `busy` through its own React context
so any page can request a prompt and disable its trigger while busy. The host
constructs the prompt string from whatever domain data it has; the component
neither knows nor cares about its content.

## 6. Testing

All component tests use Vitest + React Testing Library (the package's existing
harness in `test-utils/` and `vitest.setup.ts`). Add a new describe block to
`src/ChatPanel.test.tsx`. `sendMessage` is exercised through the mocked
`ChatAPI`; assert on the messages the panel renders and on the host callbacks.

Required cases:

1. **Auto-sends once when opened + idle.** Render with `opened` and a
   `pendingPrompt`; assert the prompt appears as a user message and the mocked
   API send was called exactly once.
2. **Does not resend on unrelated re-render.** Re-render the same element (props
   unchanged); assert the send count stays at 1 (the `lastSentRef` guard).
3. **`onPromptConsumed` fires after send.** Assert the callback is invoked once,
   after the message is dispatched.
4. **Defers while streaming.** With the API mock holding the stream open
   (`streaming === true`), render a `pendingPrompt`; assert nothing is sent.
   Resolve the stream; assert the prompt is then sent.
5. **Re-send of identical text after consume.** Set `pendingPrompt='x'`, let it
   consume (host clears to `''`), set `pendingPrompt='x'` again; assert it sends
   a second time (host-intended re-send is honored).
6. **`onBusyChange` reflects streaming.** Assert it is called with `true` on
   stream start and `false` on stream end.
7. **No-op when props omitted.** Render without any of the three props; assert no
   crash, no calls, identical behaviour to the pre-change baseline (regression
   guard).

Coverage gate: the new branch (auto-send effect + busy effect) must be covered;
keep the package's existing coverage threshold green. Run `npm test` in
`packages/chat-component/`.

## 7. Documentation (README)

Update `packages/chat-component/README.md` as part of this change:

- **Props table**: add `pendingPrompt`, `onPromptConsumed`, `onBusyChange` with
  types, defaults (all optional/undefined), and one-line descriptions.
- **New "Driving the panel from the host" section**: a short, framework-neutral
  example mirroring §5 (the `askAssistant` pattern), explicitly noting the
  greyed-out-while-busy UX via `onBusyChange`.
- **Note** that the prompt is sent as a normal user message and is subject to the
  same mode (read-only/read-write) and approval gating as any typed prompt.
- Update **`CHANGELOG.md`** with an `### Added` entry under the new version,
  describing the three props.
- Bump `package.json` `version` (0.1.14 → next minor), run `npm run build` to
  refresh `dist/`, and regenerate the publishable tarball.

## 8. Backwards Compatibility & Versioning

- All three props are optional. Embeds that pass none are unaffected
  (`pendingPrompt` undefined → effect early-returns; `onBusyChange` undefined →
  no call).
- No transport/SSE change; no breaking change to existing exports.
- **Minor** semver bump (additive, non-breaking). Consuming products bump their
  dependency when they want to use it.

## 9. Security / Safety

- The injected prompt is sent as a normal **user** message and is subject to the
  same mode gating (read-only vs read-write), capability filtering, and
  action-approval flow as any typed prompt.
- A host trigger cannot bypass `streaming` (send is a no-op while busy) nor the
  approval flow for write actions. The component performs no privileged action on
  the host's behalf beyond submitting text the user's session is already allowed
  to submit.
