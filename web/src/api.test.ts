import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest';
import { ApiClient } from './api';

const mockFetch = vi.fn() as Mock;
globalThis.fetch = mockFetch as unknown as typeof fetch;

function jsonResponse(data: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(data)),
  };
}

describe('ApiClient', () => {
  let client: ApiClient;

  beforeEach(() => {
    mockFetch.mockReset();
    localStorage.clear();
    client = new ApiClient();
  });

  describe('auth token', () => {
    it('sends Authorization header when token is set', async () => {
      client.setToken('test-token');
      mockFetch.mockResolvedValueOnce(jsonResponse({}));

      await client.get('/api/test');

      const [, init] = mockFetch.mock.calls[0];
      expect(init.headers.get('Authorization')).toBe('Bearer test-token');
    });

    it('does not send Authorization header when token is empty', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({}));

      await client.get('/api/test');

      const [, init] = mockFetch.mock.calls[0];
      expect(init.headers.get('Authorization')).toBeNull();
    });

    it('persists token to localStorage', () => {
      client.setToken('my-token');
      expect(localStorage.getItem('adminToken')).toBe('my-token');
    });

    it('clears token from localStorage', () => {
      client.setToken('my-token');
      client.clearToken();
      expect(localStorage.getItem('adminToken')).toBeNull();
    });
  });

  describe('401 handling', () => {
    it('clears token, notifies auth state, and throws on 401', async () => {
      const onUnauthorized = vi.fn();
      client.setToken('expired');
      client.setUnauthorizedHandler(onUnauthorized);
      mockFetch.mockResolvedValueOnce(jsonResponse({ error: 'unauthorized' }, 401));

      await expect(client.get('/api/test')).rejects.toThrow('unauthorized');
      expect(localStorage.getItem('adminToken')).toBeNull();
      expect(onUnauthorized).toHaveBeenCalledOnce();
    });
  });

  describe('error handling', () => {
    it('throws with backend error message', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ error: 'not found' }, 404));

      await expect(client.get('/api/test')).rejects.toThrow('not found');
    });

    it('throws with HTTP status when no error message', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({}, 500));

      await expect(client.get('/api/test')).rejects.toThrow('HTTP 500');
    });
  });

  describe('HTTP methods', () => {
    it('sends GET request', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ ok: true }));

      const result = await client.get<{ ok: boolean }>('/api/test');

      expect(result).toEqual({ ok: true });
      expect(mockFetch.mock.calls[0][1].method).toBe('GET');
    });

    it('sends POST request with JSON body', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ id: '1' }));

      await client.post('/api/test', { name: 'foo' });

      const [, init] = mockFetch.mock.calls[0];
      expect(init.method).toBe('POST');
      expect(init.body).toBe(JSON.stringify({ name: 'foo' }));
      expect(init.headers.get('Content-Type')).toBe('application/json');
    });

    it('sends PATCH request with JSON body', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ id: '1' }));

      await client.patch('/api/test', { name: 'bar' });

      const [, init] = mockFetch.mock.calls[0];
      expect(init.method).toBe('PATCH');
    });

    it('sends DELETE request', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({}));

      await client.delete('/api/test');

      expect(mockFetch.mock.calls[0][1].method).toBe('DELETE');
    });
  });

  describe('API methods', () => {
    it('calls GET /api/overview', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ resource_counts: {} }));
      await client.getOverview();
      expect(mockFetch.mock.calls[0][0]).toBe('/api/overview');
    });

    it('calls POST /api/admin/login', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ token: 'abc' }));
      await client.login('pass');
      expect(mockFetch.mock.calls[0][0]).toBe('/api/admin/login');
    });

    it('calls GET /api/subscriptions with pagination', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ items: [], total: 0 }));
      await client.getSubscriptions(2, 10);
      expect(mockFetch.mock.calls[0][0]).toBe('/api/subscriptions?page=2&page_size=10');
    });

    it('calls GET /api/nodes with pagination', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ items: [], total: 0 }));
      await client.getNodes(1, 10);
      expect(mockFetch.mock.calls[0][0]).toBe('/api/nodes?page=1&page_size=10');
    });

    it('calls POST /api/nodes for structured node creation', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ id: 'node-1' }));
      await client.createNode({ name: 'manual-http', type: 'http', server: '127.0.0.1', server_port: 8080 });
      expect(mockFetch.mock.calls[0][0]).toBe('/api/nodes');
      expect(mockFetch.mock.calls[0][1].body).toBe(JSON.stringify({ name: 'manual-http', type: 'http', server: '127.0.0.1', server_port: 8080 }));
    });

    it('calls POST /api/nodes for paste import', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ imported_nodes: 1 }));
      await client.importNodes('http://127.0.0.1:8080#manual');
      expect(mockFetch.mock.calls[0][0]).toBe('/api/nodes');
      expect(mockFetch.mock.calls[0][1].body).toBe(JSON.stringify({ import_text: 'http://127.0.0.1:8080#manual' }));
    });

    it('calls DELETE /api/nodes/:id', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ deleted: true }));
      await client.deleteNode('node-1');
      expect(mockFetch.mock.calls[0][0]).toBe('/api/nodes/node-1');
      expect(mockFetch.mock.calls[0][1].method).toBe('DELETE');
    });

    it('calls PATCH /api/nodes/:id for manual node edits', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ updated: true, id: 'node-1', split: false }));
      await client.updateNode('node-1', {
        name: 'manual-updated',
        type: 'socks5',
        server: '127.0.0.2',
        server_port: 1080,
        username: 'user',
        password: 'pass',
      });
      expect(mockFetch.mock.calls[0][0]).toBe('/api/nodes/node-1');
      expect(mockFetch.mock.calls[0][1].method).toBe('PATCH');
      expect(mockFetch.mock.calls[0][1].body).toBe(JSON.stringify({
        name: 'manual-updated',
        type: 'socks5',
        server: '127.0.0.2',
        server_port: 1080,
        username: 'user',
        password: 'pass',
      }));
    });

    it('calls PATCH /api/nodes/:id for URI or JSON node edits', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ updated: true, id: 'node-1', split: false }));
      await client.updateNode('node-1', {
        import_text: 'socks5://user:pass@127.0.0.1:1080#manual-socks',
      });
      expect(mockFetch.mock.calls[0][0]).toBe('/api/nodes/node-1');
      expect(mockFetch.mock.calls[0][1].method).toBe('PATCH');
      expect(mockFetch.mock.calls[0][1].body).toBe(JSON.stringify({
        import_text: 'socks5://user:pass@127.0.0.1:1080#manual-socks',
      }));
    });

    it('calls GET /api/access-profiles with pagination', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ items: [], total: 0 }));
      await client.getAccessProfiles(1, 9);
      expect(mockFetch.mock.calls[0][0]).toBe('/api/access-profiles?page=1&page_size=9');
    });

    it('calls GET /api/access-profiles/:id', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ id: 'p1' }));
      await client.getAccessProfile('p1');
      expect(mockFetch.mock.calls[0][0]).toBe('/api/access-profiles/p1');
    });

    it('calls POST /api/access-profiles', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ id: 'new' }));
      await client.createAccessProfile({ name: 'test', type: 'fastest', profile_identifier: 'test' });
      expect(mockFetch.mock.calls[0][0]).toBe('/api/access-profiles');
    });

    it('calls GET /api/request-logs with filters', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ items: [], total: 0 }));
      await client.getRequestLogs({ page: 1, page_size: 10, access_profile_id: 'p1' });
      expect(mockFetch.mock.calls[0][0]).toBe('/api/request-logs?page=1&page_size=10&access_profile_id=p1');
    });

    it('calls GET /api/system/settings', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({}));
      await client.getSettings();
      expect(mockFetch.mock.calls[0][0]).toBe('/api/system/settings');
    });

    it('calls GET /api/dictionaries/egress-countries', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse([]));
      await client.getEgressCountries();
      expect(mockFetch.mock.calls[0][0]).toBe('/api/dictionaries/egress-countries');
    });
  });
});
