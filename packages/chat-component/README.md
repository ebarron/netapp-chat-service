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
