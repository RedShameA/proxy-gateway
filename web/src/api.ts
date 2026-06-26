import type {
  OverviewResponse, SystemSettingsResponse, AccessProfileSummary, AccessProfileDetail,
  AccessProfileWriteRequest, ProxyCredentialSummary, ProxyCredentialCreateRequest, ProxyCredentialPatchRequest,
  NodeSummary, NodeDetail, SubscriptionSummary, SubscriptionDetail, SubscriptionWriteRequest,
  RequestLogEntry, EgressCountryDisplay, SetupStatus, LoginResponse, NodeCreateRequest, NodeImportResponse,
  NodePatchRequest, NodeUpdateResponse, MaintenanceRunSummary,
} from './types';

export class ApiClient {
  token = localStorage.getItem('adminToken') || '';
  private unauthorizedHandler: (() => void) | null = null;

  setUnauthorizedHandler(handler: (() => void) | null) {
    this.unauthorizedHandler = handler;
  }

  setToken(token: string) {
    this.token = token;
    localStorage.setItem('adminToken', token);
  }

  clearToken() {
    this.token = '';
    localStorage.removeItem('adminToken');
  }

  async get<T>(path: string): Promise<T> {
    return this.request<T>(path, { method: 'GET' });
  }

  async post<T>(path: string, body?: unknown): Promise<T> {
    return this.requestWithBody<T>(path, 'POST', body);
  }

  async patch<T>(path: string, body?: unknown): Promise<T> {
    return this.requestWithBody<T>(path, 'PATCH', body);
  }

  async delete<T>(path: string): Promise<T> {
    return this.request<T>(path, { method: 'DELETE' });
  }

  private async requestWithBody<T>(path: string, method: string, body?: unknown): Promise<T> {
    const init: RequestInit = { method };
    if (body !== undefined) {
      init.headers = { 'Content-Type': 'application/json' };
      init.body = JSON.stringify(body);
    }
    return this.request<T>(path, init);
  }

  private async request<T>(path: string, init: RequestInit): Promise<T> {
    const headers = new Headers(init.headers);
    if (this.token) {
      headers.set('Authorization', `Bearer ${this.token}`);
    }
    const resp = await fetch(path, { ...init, headers });
    const text = await resp.text();
    const data = text ? JSON.parse(text) : {};
    if (!resp.ok) {
      if (resp.status === 401) {
        this.clearToken();
        this.unauthorizedHandler?.();
      }
      throw new Error(data.error || `HTTP ${resp.status}`);
    }
    return data as T;
  }

  // Auth
  async getSetupStatus(): Promise<SetupStatus> {
    return this.get('/api/system/setup-status');
  }

  async login(password: string): Promise<LoginResponse> {
    return this.post('/api/admin/login', { password });
  }

  async setup(password: string): Promise<LoginResponse> {
    return this.post('/api/admin/setup', { password });
  }

  // Overview
  async getOverview(): Promise<OverviewResponse> {
    return this.get('/api/overview');
  }

  async getMaintenanceRuns(filters?: Record<string, string>, page = 1, pageSize = 10): Promise<{ items: MaintenanceRunSummary[]; total: number }> {
    const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => { if (value) params.set(key, value); });
    }
    return this.get(`/api/maintenance/runs?${params}`);
  }

  async getMaintenanceRun(id: string): Promise<{ run: MaintenanceRunSummary }> {
    return this.get(`/api/maintenance/runs/${id}`);
  }

  // Dictionaries
  async getEgressCountries(): Promise<EgressCountryDisplay[]> {
    return this.get('/api/dictionaries/egress-countries');
  }

  // Subscriptions
  async getSubscriptions(page = 1, pageSize = 10): Promise<{ items: SubscriptionSummary[]; total: number }> {
    return this.get(`/api/subscriptions?page=${page}&page_size=${pageSize}`);
  }

  async getSubscription(id: string): Promise<SubscriptionDetail> {
    return this.get(`/api/subscriptions/${id}`);
  }

  async createSubscription(data: SubscriptionWriteRequest): Promise<SubscriptionDetail> {
    return this.post('/api/subscriptions', data);
  }

  async updateSubscription(id: string, data: Partial<SubscriptionWriteRequest>): Promise<SubscriptionDetail> {
    return this.patch(`/api/subscriptions/${id}`, data);
  }

  async deleteSubscription(id: string): Promise<void> {
    return this.delete(`/api/subscriptions/${id}`);
  }

  async refreshSubscription(id: string): Promise<SubscriptionDetail> {
    return this.post(`/api/subscriptions/${id}/refresh`);
  }

  // Nodes
  async getNodes(page = 1, pageSize = 10, filters?: Record<string, string>): Promise<{ items: NodeSummary[]; total: number }> {
    const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
    if (filters) {
      Object.entries(filters).forEach(([k, v]) => { if (v) params.set(k, v); });
    }
    return this.get(`/api/nodes?${params}`);
  }

  async getNode(id: string): Promise<NodeDetail> {
    return this.get(`/api/nodes/${id}`);
  }

  async createNode(data: NodeCreateRequest): Promise<{ id: string }> {
    return this.post('/api/nodes', data);
  }

  async importNodes(importText: string): Promise<NodeImportResponse> {
    return this.post('/api/nodes', { import_text: importText });
  }

  async updateNode(id: string, data: NodePatchRequest): Promise<NodeUpdateResponse> {
    return this.patch(`/api/nodes/${id}`, data);
  }

  async deleteNode(id: string): Promise<void> {
    return this.delete(`/api/nodes/${id}`);
  }

  async runNodeObservations(nodeIds?: string[]): Promise<void> {
    return this.post('/api/nodes/observations/run', nodeIds ? { node_ids: nodeIds } : {});
  }

  // Access Profiles
  async getAccessProfiles(page = 1, pageSize = 9): Promise<{ items: AccessProfileSummary[]; total: number }> {
    return this.get(`/api/access-profiles?page=${page}&page_size=${pageSize}`);
  }

  async getAccessProfile(id: string): Promise<AccessProfileDetail> {
    return this.get(`/api/access-profiles/${id}`);
  }

  async createAccessProfile(data: AccessProfileWriteRequest): Promise<AccessProfileSummary> {
    return this.post('/api/access-profiles', data);
  }

  async updateAccessProfile(id: string, data: Partial<AccessProfileWriteRequest>): Promise<AccessProfileSummary> {
    return this.patch(`/api/access-profiles/${id}`, data);
  }

  async deleteAccessProfile(id: string): Promise<void> {
    return this.delete(`/api/access-profiles/${id}`);
  }

  async evaluateAccessProfile(id: string): Promise<void> {
    return this.post(`/api/access-profiles/${id}/actions/evaluate`);
  }

  async switchToBestObserved(id: string): Promise<void> {
    return this.post(`/api/access-profiles/${id}/actions/switch-to-best-observed`);
  }

  // Proxy Credentials
  async getProxyCredentials(profileId: string, page = 1, pageSize = 10): Promise<{ items: ProxyCredentialSummary[]; total: number }> {
    return this.get(`/api/access-profiles/${profileId}/proxy-credentials?page=${page}&page_size=${pageSize}`);
  }

  async createProxyCredential(profileId: string, data: ProxyCredentialCreateRequest): Promise<ProxyCredentialSummary> {
    return this.post(`/api/access-profiles/${profileId}/proxy-credentials`, data);
  }

  async updateProxyCredential(profileId: string, credId: string, data: ProxyCredentialPatchRequest): Promise<ProxyCredentialSummary> {
    return this.patch(`/api/access-profiles/${profileId}/proxy-credentials/${credId}`, data);
  }

  async deleteProxyCredential(profileId: string, credId: string): Promise<void> {
    return this.delete(`/api/access-profiles/${profileId}/proxy-credentials/${credId}`);
  }

  // Request Logs
  async getRequestLogs(params: { page?: number; page_size?: number; access_profile_id?: string; proxy_credential_id?: string; result?: string; target?: string } = {}): Promise<{ items: RequestLogEntry[]; total: number }> {
    const search = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => { if (v !== undefined && v !== '') search.set(k, String(v)); });
    return this.get(`/api/request-logs?${search}`);
  }

  // Settings
  async getSettings(): Promise<SystemSettingsResponse> {
    return this.get('/api/system/settings');
  }

  async updateSettings(data: Partial<SystemSettingsResponse>): Promise<SystemSettingsResponse> {
    return this.patch('/api/system/settings', data);
  }

  async changePassword(currentPassword: string, newPassword: string): Promise<void> {
    return this.post('/api/admin/password', { current_password: currentPassword, new_password: newPassword });
  }
}
