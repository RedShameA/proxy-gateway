import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { MaintenanceRunDrawer, MaintenanceRunRow, maintenanceRunTargetDisplay } from './MaintenanceRunView';
import { renderWithRouter } from '../test/utils';
import type { MaintenanceRunSummary } from '../types';

function maintenanceRun(overrides: Partial<MaintenanceRunSummary> = {}): MaintenanceRunSummary {
  return {
    id: 'run_1',
    run_type: 'profile_evaluation',
    trigger_source: 'manual',
    target_id: 'profile_1',
    target_label: '测试策略',
    state: 'finished',
    result: 'success',
    reason_code: 'selected_node_removed',
    total_count: 1,
    finished_count: 1,
    detail: {},
    last_error: '',
    created_at: 1700000000000,
    started_at: 1700000000000,
    finished_at: 1700000000100,
    updated_at: 1700000000100,
    ...overrides,
  };
}

describe('maintenanceRunTargetDisplay', () => {
  it('shows target for semantic target run types with fallback', () => {
    expect(maintenanceRunTargetDisplay(maintenanceRun({ run_type: 'subscription_refresh', target_label: '订阅 A', target_id: 'sub_1' }))).toEqual({ visible: true, value: '订阅 A' });
    expect(maintenanceRunTargetDisplay(maintenanceRun({ run_type: 'profile_switch', target_label: '', target_id: 'profile_1' }))).toEqual({ visible: true, value: 'profile_1' });
    expect(maintenanceRunTargetDisplay(maintenanceRun({ run_type: 'profile_evaluation', target_label: '', target_id: '' }))).toEqual({ visible: true, value: '-' });
    expect(maintenanceRunTargetDisplay(maintenanceRun({ run_type: 'node_observation', detail: { target_scope: 'single_node' }, target_label: '节点 A', target_id: 'node_1' }))).toEqual({ visible: true, value: '节点 A' });
  });

  it('hides target for aggregate and non-object maintenance runs', () => {
    for (const run of [
      maintenanceRun({ run_type: 'node_observation', detail: { target_scope: 'all_nodes' }, target_label: '旧聚合目标' }),
      maintenanceRun({ run_type: 'node_observation', detail: {}, target_label: '旧聚合目标' }),
      maintenanceRun({ run_type: 'node_observation', detail: { target_scope: 'unknown' }, target_label: '旧聚合目标' }),
      maintenanceRun({ run_type: 'startup_cleanup', target_label: 'Startup cleanup' }),
      maintenanceRun({ run_type: 'log_cleanup', target_label: 'Retention cleanup' }),
      maintenanceRun({ run_type: 'geoip_update', target_id: 'country.mmdb', target_label: 'GeoIP Database' }),
    ]) {
      expect(maintenanceRunTargetDisplay(run)).toEqual({ visible: false, value: '-' });
    }
  });
});

describe('MaintenanceRunRow', () => {
  it('does not render target label on the card', () => {
    renderWithRouter(<MaintenanceRunRow run={maintenanceRun({ target_label: '卡片目标不该出现' })} onClick={() => {}} />);

    expect(screen.getByText('策略评估 · 手动')).toBeInTheDocument();
    expect(screen.queryByText('卡片目标不该出现')).not.toBeInTheDocument();
  });
});

describe('MaintenanceRunDrawer', () => {
  it('translates maintenance detail reason and state codes', () => {
    const run = maintenanceRun({
      detail: {
        current_path_result: 'ready',
        reason: 'selected_node_removed',
        switch_reason: 'selected_node_removed',
      },
    });

    renderWithRouter(<MaintenanceRunDrawer run={run} visible onClose={() => {}} />);

    expect(screen.getByText('目标')).toBeInTheDocument();
    expect(screen.getByText('测试策略')).toBeInTheDocument();
    expect(screen.getAllByText('原因').length).toBeGreaterThan(0);
    expect(screen.getAllByText('选中节点已移除').length).toBeGreaterThan(0);
    expect(screen.getByText('当前路径结果')).toBeInTheDocument();
    expect(screen.getByText('就绪')).toBeInTheDocument();
    expect(screen.queryByText('selected_node_removed')).not.toBeInTheDocument();
    expect(screen.queryByText('ready')).not.toBeInTheDocument();
  });

  it('shows dash for semantic target rows when target is missing', () => {
    renderWithRouter(<MaintenanceRunDrawer run={maintenanceRun({ target_id: '', target_label: '' })} visible onClose={() => {}} />);

    expect(screen.getByText('目标')).toBeInTheDocument();
    expect(screen.getAllByText('-').length).toBeGreaterThan(0);
  });

  it('hides target row for runs without semantic targets', () => {
    renderWithRouter(<MaintenanceRunDrawer run={maintenanceRun({ run_type: 'log_cleanup', target_label: 'Retention cleanup' })} visible onClose={() => {}} />);

    expect(screen.queryByText('目标')).not.toBeInTheDocument();
    expect(screen.queryByText('Retention cleanup')).not.toBeInTheDocument();
  });
});
