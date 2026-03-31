// API client
export { createChatAPI } from './ChatAPI';
export type { ChatAPI } from './ChatAPI';
export { ChatAPIProvider, useChatAPI } from './ChatAPIContext';

// Components
export { ChatPanel } from './ChatPanel';
export { CanvasPanel } from './CanvasPanel';
export { ModeToggle } from './ModeToggle';
export { CapabilityControls } from './CapabilityControls';
export { ActionConfirmation } from './ActionConfirmation';
export { ToolStatusCard } from './ToolStatusCard';

// Hook
export { useChatPanel } from './useChatPanel';

// Types
export type { ChatMessage, Capability, PendingApproval, ChatMode, CanvasTab } from './useChatPanel';

// Charts
export { ChartBlock, DashboardBlock, ObjectDetailBlock, AutoJsonBlock } from './charts';
export type { PanelData, DashboardData, ChartData, PanelWidth, ObjectDetailData } from './charts';
