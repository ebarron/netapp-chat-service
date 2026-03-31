import { render as testingLibraryRender, renderHook as testingLibraryRenderHook } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ChatAPIProvider } from '../src/ChatAPIContext';
import type { ChatAPI } from '../src/ChatAPI';
import { vi } from 'vitest';

/** Creates a mock ChatAPI for tests. */
export function createMockChatAPI(overrides?: Partial<ChatAPI>): ChatAPI {
  return {
    baseURL: '/api/2.0',
    get: vi.fn().mockResolvedValue({ configured: true, capabilities: [] }),
    post: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
    ...overrides,
  };
}

const defaultMockAPI = createMockChatAPI();

function createWrapper(api: ChatAPI) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <MantineProvider>
        <ChatAPIProvider value={api}>
          {children}
        </ChatAPIProvider>
      </MantineProvider>
    );
  };
}

export function render(ui: React.ReactNode, options?: { api?: ChatAPI }) {
  const api = options?.api ?? defaultMockAPI;
  return testingLibraryRender(<>{ui}</>, {
    wrapper: createWrapper(api),
  });
}

export function renderHook<T>(hook: () => T, options?: { api?: ChatAPI }) {
  const api = options?.api ?? defaultMockAPI;
  return testingLibraryRenderHook(hook, {
    wrapper: createWrapper(api),
  });
}
