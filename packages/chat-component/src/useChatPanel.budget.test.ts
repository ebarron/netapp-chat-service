import { renderHook, act, createMockChatAPI } from '../test-utils';
import { vi, describe, it, expect } from 'vitest';
import { useChatPanel } from './useChatPanel';

describe('useChatPanel tool budget', () => {
  it('blocks setMode("read-write") when read-write budget exceeds max', async () => {
    const api = createMockChatAPI({
      get: vi.fn().mockImplementation(async (path: string) => {
        if (path === '/chat/capabilities') {
          return {
            capabilities: [],
            tool_budgets: {
              read_only: { used: 30, max: 128 },
              read_write: { used: 200, max: 128 },
            },
          };
        }
        return { configured: true };
      }),
    });

    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.fetchCapabilities();
    });

    expect(result.current.mode).toBe('read-only');

    act(() => {
      result.current.setMode('read-write');
    });

    // Mode must NOT have switched and an error must be surfaced.
    expect(result.current.mode).toBe('read-only');
    expect(result.current.capabilityError).toMatch(/200 tools/);
  });

  it('allows setMode("read-write") when within budget', async () => {
    const api = createMockChatAPI({
      get: vi.fn().mockImplementation(async (path: string) => {
        if (path === '/chat/capabilities') {
          return {
            capabilities: [],
            tool_budgets: {
              read_only: { used: 10, max: 128 },
              read_write: { used: 50, max: 128 },
            },
          };
        }
        return { configured: true };
      }),
    });

    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.fetchCapabilities();
    });

    act(() => {
      result.current.setMode('read-write');
    });

    expect(result.current.mode).toBe('read-write');
    expect(result.current.capabilityError).toBeNull();
  });

  it('updateCapability surfaces server budget rejection', async () => {
    const api = createMockChatAPI({
      get: vi.fn().mockResolvedValue({
        capabilities: [
          {
            id: 'harvest',
            name: 'Harvest',
            description: '',
            state: 'off',
            available: true,
            tools_count: 200,
            read_only_tools_count: 200,
          },
        ],
        tool_budgets: {
          read_only: { used: 0, max: 128 },
          read_write: { used: 0, max: 128 },
        },
      }),
      post: vi.fn().mockRejectedValue(
        new Error(JSON.stringify({ message: 'Enabling these capabilities would use 200 tools (max 128).' }))
      ),
    });

    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.fetchCapabilities();
    });

    let returned: boolean = true;
    await act(async () => {
      returned = await result.current.updateCapability('harvest', 'allow');
    });

    expect(returned).toBe(false);
    expect(result.current.capabilityError).toMatch(/would use 200 tools/);
  });
});
