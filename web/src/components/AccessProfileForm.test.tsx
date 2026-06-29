import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AccessProfileForm } from './AccessProfileForm';
import { renderWithRouter } from '../test/utils';
import type { AccessProfileDetail, AccessProfileWriteRequest, NodeSummary } from '../types';

const unknownCountry = {
  value: '',
  iso_code: null,
  name_zh: '未知',
  is_unknown: true,
};

function node(id: string, name: string): NodeSummary {
  return {
    id,
    name,
    type: 'http',
    protocol: 'http',
    server: '127.0.0.1',
    server_port: 8080,
    username: '',
    password: '',
    enabled: true,
    state: 'enabled',
    sources: [],
    egress_ip: null,
    egress_country: unknownCountry,
    observation_latency_ms: null,
    last_observed_at: null,
    last_error: '',
  };
}

function baseProfile(overrides: Partial<AccessProfileDetail> = {}): AccessProfileDetail {
  return {
    id: 'profile_1',
    name: '测试策略',
    type: 'fastest',
    state: 'ready',
    profile_identifier: 'existing-profile',
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

describe('AccessProfileForm', () => {
  it('generates a default profile identifier for new profiles', async () => {
    const onSubmit = vi.fn<(payload: AccessProfileWriteRequest) => Promise<void>>().mockResolvedValue(undefined);
    renderWithRouter(
      <AccessProfileForm
        nodes={[]}
        countries={[]}
        subscriptions={[]}
        submitLabel="创建"
        onSubmit={onSubmit}
      />,
    );

    const identifier = screen.getByPlaceholderText('策略标识') as HTMLInputElement;
    expect(identifier.value).toMatch(/^profile-[a-z0-9]{10}$/);

    fireEvent.change(screen.getByPlaceholderText('策略名称'), { target: { value: '测试策略' } });
    fireEvent.click(screen.getByRole('button', { name: '创建' }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
    const payload = onSubmit.mock.calls[0][0];
    expect(payload.profile_identifier).toBe(identifier.value);
  });

  it('keeps the existing profile identifier when editing', () => {
    renderWithRouter(
      <AccessProfileForm
        initial={baseProfile({ profile_identifier: 'kept-profile-id' })}
        nodes={[]}
        countries={[]}
        subscriptions={[]}
        submitLabel="保存"
        onSubmit={vi.fn()}
      />,
    );

    const identifier = screen.getByPlaceholderText('策略标识') as HTMLInputElement;
    expect(identifier.value).toBe('kept-profile-id');
  });

  it('submits end-to-end chain profiles with multiple exit nodes without changing mode', async () => {
    const onSubmit = vi.fn<(payload: AccessProfileWriteRequest) => Promise<void>>().mockResolvedValue(undefined);
    renderWithRouter(
      <AccessProfileForm
        initial={baseProfile({
          type: 'chain',
          exit_node_ids: ['exit-a', 'exit-b'],
          chain_evaluation_mode: 'end_to_end',
        })}
        nodes={[node('exit-a', '出口 A'), node('exit-b', '出口 B')]}
        countries={[]}
        subscriptions={[]}
        submitLabel="保存"
        onSubmit={onSubmit}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '保存' }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
    const payload = onSubmit.mock.calls[0][0];
    expect(payload.chain_evaluation_mode).toBe('end_to_end');
    expect(payload.exit_node_ids).toEqual(['exit-a', 'exit-b']);
  });

  it('submits chain-link profiles with a single exit node', async () => {
    const onSubmit = vi.fn<(payload: AccessProfileWriteRequest) => Promise<void>>().mockResolvedValue(undefined);
    renderWithRouter(
      <AccessProfileForm
        initial={baseProfile({
          type: 'chain',
          exit_node_ids: ['exit-a'],
          chain_evaluation_mode: 'chain_link',
        })}
        nodes={[node('exit-a', '出口 A')]}
        countries={[]}
        subscriptions={[]}
        submitLabel="保存"
        onSubmit={onSubmit}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '保存' }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
    const payload = onSubmit.mock.calls[0][0];
    expect(payload.chain_evaluation_mode).toBe('chain_link');
    expect(payload.exit_node_ids).toEqual(['exit-a']);
  });

  it('does not silently convert invalid chain-link multi-exit state to end-to-end', async () => {
    const onSubmit = vi.fn<(payload: AccessProfileWriteRequest) => Promise<void>>().mockResolvedValue(undefined);
    renderWithRouter(
      <AccessProfileForm
        initial={baseProfile({
          type: 'chain',
          exit_node_ids: ['exit-a', 'exit-b'],
          chain_evaluation_mode: 'chain_link',
        })}
        nodes={[node('exit-a', '出口 A'), node('exit-b', '出口 B')]}
        countries={[]}
        subscriptions={[]}
        submitLabel="保存"
        onSubmit={onSubmit}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '保存' }));

    await waitFor(() => expect(onSubmit).not.toHaveBeenCalled());
  });
});
