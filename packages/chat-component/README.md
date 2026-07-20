# @edjbarron/netapp-chat-component

React chat UI component for the [netapp-chat-service](https://github.com/ebarron/netapp-chat-service) agentic chat backend (LLM + MCP tool routing).

Provides a `ChatPanel`, optional `CanvasPanel`, capability/mode controls, and a chart/dashboard rendering kit.

## Install

```bash
npm install @edjbarron/netapp-chat-component
```

### Peer dependencies

You must install these in your host app:

```bash
npm install react react-dom \
  @mantine/core @mantine/charts @mantine/hooks \
  @tabler/icons-react
```

Supports React 18 and 19, Mantine 8.x.

## Usage

```tsx
import { MantineProvider } from '@mantine/core';
import {
  ChatAPIProvider,
  ChatPanel,
  createChatAPI,
} from '@edjbarron/netapp-chat-component';

import '@mantine/core/styles.css';
import '@mantine/charts/styles.css';
import '@edjbarron/netapp-chat-component/styles.css';

const api = createChatAPI('https://your-chat-service.example.com/api');

export function App() {
  return (
    <MantineProvider>
      <ChatAPIProvider value={api}>
        <ChatPanel />
      </ChatAPIProvider>
    </MantineProvider>
  );
}
```

### Initial chat mode

`<ChatPanel>` opens in **read-write mode** by default. To start in read-only mode (information-retrieval tools only), pass `defaultMode`:

```tsx
<ChatPanel defaultMode="read-only" />
```

The user can still toggle mode at runtime via the in-panel `ModeToggle`; `defaultMode` only sets the initial value. The backend filters tools by mode based on each MCP tool's `ToolAnnotations.ReadOnlyHint` — if your MCP servers don't yet emit annotations, leave `defaultMode` at its default so all their tools remain available.

### Driving the panel from the host

The host can open the panel and **auto-send a prompt** programmatically — useful
for "Explain this" / "Ask about this" buttons elsewhere in your app that deep-link
a question into the assistant. Three optional props support this:

| Prop | Type | Description |
|------|------|-------------|
| `pendingPrompt` | `string` | A prompt to auto-send once. When it changes to a non-empty value while the panel is `opened` and not streaming, it is sent as a user message exactly once. |
| `onPromptConsumed` | `() => void` | Called after `pendingPrompt` has been submitted. Clear your own state here so the same prompt isn't resent on a later re-render. |
| `onBusyChange` | `(busy: boolean) => void` | Notifies the host when the assistant's busy (streaming) state changes, so you can disable your trigger control while a turn is in flight. |
| `onCanvasEvent` | `(info: { action: 'open' \| 'close'; tabId: string; title: string; kind: string }) => void` | Fired when a canvas tab opens/updates or closes. Lets the host react to a specific canvas (e.g. refresh a page when a matching canvas changes). A canvas payload whose `content.close` is truthy closes the matching tab and reports `action: 'close'`. |

```tsx
function App() {
  const [opened, setOpened] = useState(false);
  const [pendingPrompt, setPendingPrompt] = useState('');
  const [busy, setBusy] = useState(false);

  const askAssistant = (prompt: string) => {
    setPendingPrompt(prompt);
    setOpened(true);
  };

  return (
    <>
      <button disabled={busy} onClick={() => askAssistant('Explain rule X')}>
        Explain this
      </button>

      <ChatPanel
        opened={opened}
        onClose={() => setOpened(false)}
        pendingPrompt={pendingPrompt}
        onPromptConsumed={() => setPendingPrompt('')}
        onBusyChange={setBusy}
      />
    </>
  );
}
```

A common pattern is to expose `askAssistant` and `busy` through a React context so
any page can request a prompt and grey out its trigger while the assistant is
busy. If the host opens the panel mid-turn, the prompt is held until the current
turn finishes, then sent.

### Agentic-forward UI seams (opt-in)

These additive, opt-in features let you build a persistent docked assistant beside
a canvas that also hosts your own pages, with everything on screen exposed to the
assistant. All default to today's behavior — see
[docs/agentic-forward-seams.md](https://github.com/ebarron/netapp-chat-service/blob/main/docs/agentic-forward-seams.md)
for the full reference.

| Prop | Type | Description |
|------|------|-------------|
| `variant` | `'drawer' \| 'docked'` | **C1.** `'drawer'` (default) is today's slide-over. `'docked'` renders a persistent full-height panel (assistant + canvas) that fills its parent, for a full-width-header shell. Always present (not gated by `opened`). |
| `onHostTabPortal` | `(tabId: string, el: HTMLElement \| null) => void` | **C2–C4.** Portal mount callback for host-content canvas tabs. `el` is the mount node when a host tab mounts, `null` when it unmounts. Render your page into `el` via `ReactDOM.createPortal`. |
| `onOpenNav` | `(destination: string) => void` | **C6.** Called when the engine emits an `open_nav` SSE event (from a host-registered `open_nav_view` tool). Absence is a safe no-op. |

**Host content in a canvas tab (C2–C4/C5).** Obtain the imperative handle via a
ref and drive host tabs. The component renders an empty mount node and exposes it
via `onHostTabPortal`; it never imports host pages.

```tsx
import { createPortal } from 'react-dom';
import type { ChatPanelHandle, CanvasTabSummary } from '@edjbarron/netapp-chat-component';

const chat = useRef<ChatPanelHandle>(null);
const [navMount, setNavMount] = useState<HTMLElement | null>(null);

<ChatPanel
  ref={chat}
  variant="docked"
  opened
  onClose={() => {}}
  onHostTabPortal={(tabId, el) => { if (tabId === 'nav') setNavMount(el); }}
/>

// Open the single reused, eviction-exempt nav tab and attach a context summary:
chat.current?.openHostCanvasTab({
  tabId: 'nav',
  title: 'Alerting',
  kind: 'nav-view',
  qualifier: '/alerting',
  evictable: false, // exempt from max-tab eviction, still user-closable
  summary: { status: 'warning', digest: '3 rules enabled, 1 disabled.' } satisfies CanvasTabSummary,
});

{navMount && createPortal(<AlertingPage />, navMount)}
```

`ChatPanelHandle` also exposes `updateHostCanvasTab`, `setCanvasTabSummary`,
`closeCanvasTab`, and `focusCanvasTab`.

**Canvas context provider (C5).** Attach a `CanvasTabSummary`
(`kind`/`name`/`qualifier`/`status`/`key_properties` + free-text `digest`) to any
tab; the component forwards it in the existing `canvas_tabs` field of
`/chat/message` so the assistant can answer questions about on-screen content.
Empty fields are omitted cleanly. **Exclude secrets** from summaries/digests.

The injected prompt is sent as a normal **user** message and is subject to the
same mode (read-only/read-write), capability filtering, and action-approval
gating as any typed prompt — the host cannot bypass these.

### Auth headers and credentials

`createChatAPI` accepts custom `headers` and a `credentials` mode that are applied to **every** request (including the streaming `POST /chat/message`):

```ts
const api = createChatAPI('/api', {
  headers: {
    Authorization: `Bearer ${token}`,
    'X-Tenant': 'acme',
  },
  credentials: 'same-origin', // defaults to 'include'
});
```

If you implement `ChatAPI` yourself instead of using `createChatAPI`, you must implement `stream(path, body, signal?): Promise<Response>` in addition to `get`/`post`/`delete`. The component uses `stream()` for the SSE message endpoint so your transport layer can apply auth uniformly.

## Backend

This component talks to the `netapp-chat-service` Go backend. See the [main repo](https://github.com/ebarron/netapp-chat-service) for the API contract, configuration, and deployment.

## Exports

- Components: `ChatPanel`, `CanvasPanel`, `ModeToggle`, `CapabilityControls`, `ActionConfirmation`, `ToolStatusCard`
- Charts: `ChartBlock`, `DashboardBlock`, `ObjectDetailBlock`, `AutoJsonBlock`
- API: `createChatAPI`, `ChatAPIProvider`, `useChatAPI`
- Hook: `useChatPanel`
- Types: `ChatMessage`, `Capability`, `PendingApproval`, `ChatMode`, `CanvasTab`, `CanvasEventInfo`, `CanvasTabSummary`, `HostCanvasTabInput`, `ChatPanelHandle`, `PanelData`, `DashboardData`, `ChartData`, `PanelWidth`, `ObjectDetailData`

## License

MIT
