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
- Types: `ChatMessage`, `Capability`, `PendingApproval`, `ChatMode`, `CanvasTab`, `PanelData`, `DashboardData`, `ChartData`, `PanelWidth`, `ObjectDetailData`

## License

MIT
