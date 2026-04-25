import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createChatAPI } from './ChatAPI';

/**
 * Tests for createChatAPI — in particular that stream() honors the
 * headers/credentials/signal configured at construction. The original
 * bug was that useChatPanel called fetch() directly for the SSE POST,
 * silently dropping auth headers; the fix routes streaming through
 * ChatAPI.stream(). These tests guard against that regressing inside
 * the default implementation itself.
 */
describe('createChatAPI', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue(
      new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } }),
    );
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe('stream()', () => {
    it('sends configured Authorization header on the streaming POST', async () => {
      const api = createChatAPI('/api/2.0', {
        headers: { Authorization: 'Bearer abc123' },
      });

      await api.stream('/chat/message', { message: 'hi' });

      expect(fetchMock).toHaveBeenCalledTimes(1);
      const [url, init] = fetchMock.mock.calls[0];
      expect(url).toBe('/api/2.0/chat/message');
      expect(init.method).toBe('POST');
      expect(init.headers).toMatchObject({
        'Content-Type': 'application/json',
        Authorization: 'Bearer abc123',
      });
      expect(init.body).toBe(JSON.stringify({ message: 'hi' }));
    });

    it('honors configured credentials mode', async () => {
      const api = createChatAPI('/api/2.0', { credentials: 'omit' });

      await api.stream('/chat/message', {});

      const [, init] = fetchMock.mock.calls[0];
      expect(init.credentials).toBe('omit');
    });

    it('defaults credentials to "include" when no option is provided', async () => {
      const api = createChatAPI('/api/2.0');

      await api.stream('/chat/message', {});

      const [, init] = fetchMock.mock.calls[0];
      expect(init.credentials).toBe('include');
    });

    it('propagates the AbortSignal to fetch', async () => {
      const api = createChatAPI('/api/2.0');
      const controller = new AbortController();

      await api.stream('/chat/message', {}, controller.signal);

      const [, init] = fetchMock.mock.calls[0];
      expect(init.signal).toBe(controller.signal);
    });

    it('does NOT throw on non-2xx responses (caller handles status)', async () => {
      fetchMock.mockResolvedValueOnce(new Response('nope', { status: 401 }));
      const api = createChatAPI('/api/2.0');

      const resp = await api.stream('/chat/message', {});
      expect(resp.status).toBe(401);
    });
  });

  describe('get()/post()/delete() (regression)', () => {
    it('post() also includes configured Authorization header', async () => {
      const api = createChatAPI('/api/2.0', {
        headers: { Authorization: 'Bearer xyz' },
      });

      await api.post('/sessions', { mode: 'read-only' });

      const [, init] = fetchMock.mock.calls[0];
      expect(init.headers).toMatchObject({ Authorization: 'Bearer xyz' });
      expect(init.credentials).toBe('include');
    });
  });
});
