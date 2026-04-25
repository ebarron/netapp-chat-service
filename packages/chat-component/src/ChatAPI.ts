/**
 * ChatAPI is the interface between the chat component and the host product's
 * HTTP layer. Host products implement this to bridge their auth/routing.
 *
 * The default implementation (createChatAPI) uses plain fetch().
 */
export interface ChatAPI {
  /** Base URL for chat endpoints, e.g. "/api/2.0" or "https://host/api/chat" */
  baseURL: string;

  /** GET request — returns parsed JSON. */
  get(path: string): Promise<any>;

  /** POST request — returns parsed JSON. */
  post(path: string, data?: any): Promise<any>;

  /** DELETE request — returns parsed JSON. */
  delete(path: string, data?: any): Promise<any>;

  /**
   * Streaming POST — returns the raw Response so the caller can read
   * `response.body` as an SSE stream. Uses the same headers/credentials
   * configured on this ChatAPI. The caller is responsible for status
   * checking; this method does NOT throw on non-2xx.
   */
  stream(path: string, body: any, signal?: AbortSignal): Promise<Response>;
}

/**
 * Creates a default ChatAPI using fetch(). Suitable for standalone use
 * or when the host product doesn't need custom HTTP handling.
 */
export function createChatAPI(baseURL: string, options?: {
  /** Extra headers to include (e.g. Authorization). */
  headers?: Record<string, string>;
  /** Fetch credentials mode. Defaults to 'include'. */
  credentials?: RequestCredentials;
}): ChatAPI {
  const { headers = {}, credentials = 'include' } = options ?? {};

  async function request(method: string, path: string, data?: any): Promise<any> {
    const url = `${baseURL}${path}`;
    const init: RequestInit = {
      method,
      credentials,
      headers: {
        'Content-Type': 'application/json',
        ...headers,
      },
    };
    if (data !== undefined) {
      init.body = JSON.stringify(data);
    }
    const resp = await fetch(url, init);
    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(text || resp.statusText);
    }
    return resp.json();
  }

  return {
    baseURL,
    get: (path) => request('GET', path),
    post: (path, data) => request('POST', path, data),
    delete: (path, data) => request('DELETE', path, data),
    stream: (path, body, signal) =>
      fetch(`${baseURL}${path}`, {
        method: 'POST',
        credentials,
        headers: {
          'Content-Type': 'application/json',
          ...headers,
        },
        body: JSON.stringify(body),
        signal,
      }),
  };
}
