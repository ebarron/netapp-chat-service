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

const api = createChatAPI({ baseUrl: 'https://your-chat-service.example.com' });

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
