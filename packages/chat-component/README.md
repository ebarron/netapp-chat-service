# @edjbarron/netapp-chat-component

React chat UI for the [netapp-chat-service](https://github.com/ebarron/netapp-chat-service)
agentic backend (LLM + MCP tool routing). Drop in a production-ready assistant —
from a simple slide-over panel to a full "AI-forward" workspace with a live canvas
beside the chat.

> **New here?** The [**User Guide**](./UserGuide.md) is the complete how-to:
> installation, integration patterns, every prop, and worked examples. This README
> is a capabilities overview and quick start.

## Why this package

- **Batteries-included assistant UI.** Streaming responses, read-only/read-write
  mode toggle, MCP capability controls, tool-status cards, and action-approval
  gating — all wired to the backend contract out of the box.
- **A canvas, not just a chat.** Render engine-driven charts, dashboards, and
  object views beside the conversation, or portal your **own** pages into a canvas
  tab while the assistant stays aware of what's on screen.
- **An "AI-forward" workspace shell.** `ChatAppShell` composes a host header, a
  docked assistant, a tabbed canvas, and navigation (overlay or docked) — the host
  plugs in via slots; routing stays host-side.
- **Composable layout.** Host-controlled and drag-resizable assistant/canvas
  split, left/right assistant placement, a collapsible assistant with a reveal
  rail, and docked-or-overlay navigation.
- **Mobile-ready (opt-in).** `mobileLayout` turns `ChatAppShell` into a
  single-column phone shell below a breakpoint — host pages stay mounted, nav
  collapses to a burger overlay, and a "Page · Assistant" bottom-tab bar
  switches regions.
- **Navigation by prompt.** "Open alerting" can drive the same navigation a click
  would, via a host-registered tool and a single SSE event.
- **Additive and safe.** Every advanced capability is opt-in; with none of the
  new props used the component behaves exactly as the prior release.

## Capabilities at a glance

| Area | Highlights |
|------|-----------|
| Assistant | Streaming chat, mode toggle, capability/budget controls, host-driven prompt injection, busy-state notifications |
| Canvas | Engine charts/dashboards, host-page portals, per-tab context summaries exposed to the assistant |
| Workspace shell | `ChatAppShell` with header/nav/destination slots and a managed reserved-nav-tab lifecycle |
| Layout | Configurable + draggable split, assistant placement (left/right), collapsible assistant, docked/overlay nav, opt-in single-column mobile mode |
| Transport | `createChatAPI` with shared auth headers/credentials across every request incl. SSE |

## Quick start

```bash
npm install @edjbarron/netapp-chat-component
# peer deps:
npm install react react-dom @mantine/core @mantine/charts @mantine/hooks @tabler/icons-react
```

```tsx
import { MantineProvider } from '@mantine/core';
import { ChatAPIProvider, ChatPanel, createChatAPI } from '@edjbarron/netapp-chat-component';

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

Supports React 18 and 19, Mantine 8.x and 9.x. For everything beyond this — docked
mode, the workspace shell, canvas portals, layout modes, and auth — see the
[**User Guide**](./UserGuide.md).

## Documentation

- [**User Guide**](./UserGuide.md) — installation, integration, and full prop reference
- [Agentic-forward UI seams (C1–C6)](https://github.com/ebarron/netapp-chat-service/blob/main/docs/agentic-forward-seams.md)
- [Agentic-forward layout modes (C7–C8)](https://github.com/ebarron/netapp-chat-service/blob/main/docs/agentic-forward-layout-modes.md)
- [Mobile layout](https://github.com/ebarron/netapp-chat-service/blob/main/docs/mobile-layout.md)
- [CHANGELOG](./CHANGELOG.md)

## Exports

- Components: `ChatPanel`, `CanvasPanel`, `ChatAppShell`, `ModeToggle`, `CapabilityControls`, `ActionConfirmation`, `ToolStatusCard`
- Charts: `ChartBlock`, `DashboardBlock`, `ObjectDetailBlock`, `AutoJsonBlock`
- API: `createChatAPI`, `ChatAPIProvider`, `useChatAPI`
- Hook: `useChatPanel`
- Types: `ChatMessage`, `Capability`, `PendingApproval`, `ChatMode`, `CanvasTab`, `CanvasEventInfo`, `CanvasTabSummary`, `HostCanvasTabInput`, `ChatPanelHandle`, `PanelData`, `DashboardData`, `ChartData`, `PanelWidth`, `ObjectDetailData`, `ChatAppShellProps`

## Backend

This component talks to the `netapp-chat-service` Go backend. See the
[main repo](https://github.com/ebarron/netapp-chat-service) for the API contract,
configuration, and deployment.

## License

MIT
