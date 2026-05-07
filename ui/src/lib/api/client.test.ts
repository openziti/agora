import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { clearLocalUser, getLocalUserEmail, setLocalUserEmail } from '../cookies';

import { apiRequest, setUnauthorizedHandler } from './client';

const accountTokenHeader = ['X', 'TOKEN'].join('-');

let fetchMock: ReturnType<typeof vi.fn>;

class MemoryStorage implements Storage {
  private readonly values = new Map<string, string>();

  get length(): number {
    return this.values.size;
  }

  clear(): void {
    this.values.clear();
  }

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  key(index: number): string | null {
    return Array.from(this.values.keys())[index] ?? null;
  }

  removeItem(key: string): void {
    this.values.delete(key);
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
}

describe('apiRequest cookie auth contract', () => {
  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal('document', { cookie: 'agora-csrf=csrf-token; other=value' });
    vi.stubGlobal('localStorage', new MemoryStorage());
  });

  afterEach(() => {
    setUnauthorizedHandler(undefined);
    vi.unstubAllGlobals();
  });

  it('sends same-origin credentials and no account-token header on safe requests', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ ok: true }));

    await apiRequest('/dashboard/summary');

    const init = lastRequestInit();
    const headers = init.headers as Headers;

    expect(init.credentials).toBe('same-origin');
    expect(headers.has('X-CSRF-Token')).toBe(false);
    expect(headers.has(accountTokenHeader)).toBe(false);
  });

  it.each(['POST', 'PUT', 'PATCH', 'DELETE'] as const)(
    'sends the csrf cookie value on %s requests',
    async (method) => {
      fetchMock.mockResolvedValue(jsonResponse({ ok: true }));

      await apiRequest('/sessions', { method, body: { ok: true } });

      const init = lastRequestInit();
      const headers = init.headers as Headers;

      expect(init.credentials).toBe('same-origin');
      expect(headers.get('X-CSRF-Token')).toBe('csrf-token');
      expect(headers.has(accountTokenHeader)).toBe(false);
    },
  );

  it('stores only the display email in local storage', () => {
    setLocalUserEmail('demo@agora.local');

    expect(localStorage.length).toBe(1);
    expect(localStorage.key(0)).toBe('agora.user');
    expect(getLocalUserEmail()).toBe('demo@agora.local');

    clearLocalUser();

    expect(localStorage.length).toBe(0);
  });

  it('notifies the registered unauthorized handler on 401 responses', async () => {
    const unauthorizedHandler = vi.fn();
    setUnauthorizedHandler(unauthorizedHandler);
    fetchMock.mockResolvedValue(errorResponse(401));

    await expect(apiRequest('/sessions')).rejects.toMatchObject({ status: 401 });

    expect(unauthorizedHandler).toHaveBeenCalledWith(expect.objectContaining({ status: 401 }));
  });

  it('allows auth probes to suppress the registered unauthorized handler', async () => {
    const unauthorizedHandler = vi.fn();
    setUnauthorizedHandler(unauthorizedHandler);
    fetchMock.mockResolvedValue(errorResponse(401));

    await expect(apiRequest('/account/whoami', { suppressUnauthorizedHandler: true })).rejects.toMatchObject({
      status: 401,
    });

    expect(unauthorizedHandler).not.toHaveBeenCalled();
  });
});

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

function errorResponse(status: number): Response {
  return new Response(JSON.stringify({ code: 'unauthorized', message: 'unauthorized' }), {
    status,
    statusText: 'Unauthorized',
    headers: { 'Content-Type': 'application/json' },
  });
}

function lastRequestInit(): RequestInit {
  const init = fetchMock.mock.calls.at(-1)?.[1];
  if (!init) {
    throw new Error('expected fetch to be called with request options');
  }
  return init as RequestInit;
}
