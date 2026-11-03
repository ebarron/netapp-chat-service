import { render as testingLibraryRender, renderHook as testingLibraryRenderHook } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { StrictMode } from 'react';
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
    stream: vi.fn().mockResolvedValue(new Response('', { status: 200 })),
    ...overrides,
  };
}

const defaultMockAPI = createMockChatAPI();

function createWrapper(api: ChatAPI, strict = false) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    const content = (
      <MantineProvider>
        <ChatAPIProvider value={api}>
          {children}
        </ChatAPIProvider>
      </MantineProvider>
    );
    return strict ? <StrictMode>{content}</StrictMode> : content;
  };
}

export function render(ui: React.ReactNode, options?: { api?: ChatAPI }) {
  const api = options?.api ?? defaultMockAPI;
  return testingLibraryRender(<>{ui}</>, {
    wrapper: createWrapper(api),
  });
}

export function renderHook<T>(hook: () => T, options?: { api?: ChatAPI; strict?: boolean }) {
  const api = options?.api ?? defaultMockAPI;
  return testingLibraryRenderHook(hook, {
    wrapper: createWrapper(api, options?.strict),
  });
}
