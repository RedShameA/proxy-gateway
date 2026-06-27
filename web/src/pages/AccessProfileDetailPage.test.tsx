import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { AccessProfileDetailPage } from './AccessProfileDetailPage';
import { render } from '@testing-library/react';
import type { ApiClient } from '../api';
import type { AccessProfileDetail, ProxyCredentialSummary } from '../types';

function buttonByText(text: string): HTMLButtonElement {
  const button = screen.getByText(text).closest('button');
  if (!button) {
    throw new Error(`button not found: ${text}`);
  }
  return button;
}

function profileDetail(overrides: Partial<AccessProfileDetail> = {}): AccessProfileDetail {
  return {
    id: 'profile_1',
    name: '测试策略',
    type: 'fastest',
    state: 'ready',
    profile_identifier: 'test-profile',
    current_path: null,
    proxy_credentials_count: 0,
    enabled_proxy_credentials_count: 0,
    node_sticky_enabled: false,
    last_evaluated_at: null,
    last_error: '',
    switch_reason: '',
    fixed_node_id: null,
    exit_node_ids: [],
    chain_evaluation_mode: 'end_to_end',
    test_url: '',
    candidate_filter: {
      source_mode: 'all',
      source_ids: [],
      protocols: [],
      name_include: '',
      name_exclude: '',
      egress_country_mode: 'include',
      egress_countries: [],
    },
    switching_tolerance: {
      relative_improvement_threshold: 0.2,
      absolute_latency_improvement_ms: 100,
    },
    evaluation_schedule: {
      mode: 'inherit',
      interval_seconds: null,
    },
    last_evaluation_details: {},
    best_observed_path: null,
    best_observed_valid: false,
    best_observed_relative_improvement: null,
    best_observed_absolute_improvement_ms: null,
    no_switch_reason: '',
    candidate_stats: {
      total: 0,
      usable: 0,
      unknown_egress_country: 0,
      front_candidates: 0,
      exit_nodes: 0,
      path_combinations: 0,
    },
    proxy_credentials: [],
    latest_switch_reason: '',
    latest_switch_at: null,
    latest_switch_trigger: null,
    recent_events: [],
    created_at: 1700000000000,
    updated_at: 1700000000000,
    ...overrides,
  };
}

function credential(): ProxyCredentialSummary {
  return {
    id: 'cred_1',
    access_profile_id: 'profile_1',
    remark: '凭证',
    password: 'abc12345',
    http_proxy_url: 'http://test-profile:abc12345@127.0.0.1:8080',
    https_proxy_url: 'https://test-profile:abc12345@127.0.0.1:8080',
    socks5_proxy_url: 'socks5://test-profile:abc12345@127.0.0.1:8080',
    enabled: true,
    created_at: 1700000000000,
    last_used_at: null,
  };
}

function mockClient(profile: AccessProfileDetail) {
  return {
    getAccessProfile: vi.fn().mockResolvedValue(profile),
    getEgressCountries: vi.fn().mockResolvedValue([]),
    getNodes: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    getSubscriptions: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    createProxyCredential: vi.fn().mockResolvedValue(credential()),
  } as unknown as ApiClient;
}

beforeEach(() => {
  window.history.pushState({}, '', '/access-profiles/profile_1');
});

describe('AccessProfileDetailPage', () => {
  it('defaults new credential remark to credential and resets it after creation', async () => {
    const client = mockClient(profileDetail());
    render(
      <MemoryRouter initialEntries={['/access-profiles/profile_1']}>
        <Routes>
          <Route path="/access-profiles/:id" element={<AccessProfileDetailPage client={client} />} />
        </Routes>
      </MemoryRouter>,
    );

    await screen.findByText('新建凭证');
    fireEvent.click(buttonByText('新建凭证'));

    const remark = screen.getByPlaceholderText('凭证名称') as HTMLInputElement;
    expect(remark.value).toBe('凭证');

    fireEvent.change(remark, { target: { value: '自定义凭证' } });
    fireEvent.click(screen.getByRole('button', { name: '创建' }));

    await waitFor(() => expect(client.createProxyCredential).toHaveBeenCalledOnce());
    expect(client.createProxyCredential).toHaveBeenCalledWith(
      'profile_1',
      expect.objectContaining({ remark: '自定义凭证' }),
    );

    fireEvent.click(buttonByText('新建凭证'));
    expect(screen.getByPlaceholderText('凭证名称')).toHaveValue('凭证');
  });
});
