import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { MaintenanceRunDrawer } from './MaintenanceRunView';
import { renderWithRouter } from '../test/utils';
import type { MaintenanceRunSummary } from '../types';

describe('MaintenanceRunDrawer', () => {
  it('translates maintenance detail reason and state codes', () => {
    const run: MaintenanceRunSummary = {
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
      detail: {
        current_path_result: 'ready',
        reason: 'selected_node_removed',
        switch_reason: 'selected_node_removed',
      },
      last_error: '',
      created_at: 1700000000000,
      started_at: 1700000000000,
      finished_at: 1700000000100,
      updated_at: 1700000000100,
    };

    renderWithRouter(<MaintenanceRunDrawer run={run} visible onClose={() => {}} />);

    expect(screen.getAllByText('原因').length).toBeGreaterThan(0);
    expect(screen.getAllByText('选中节点已移除').length).toBeGreaterThan(0);
    expect(screen.getByText('当前路径结果')).toBeInTheDocument();
    expect(screen.getByText('就绪')).toBeInTheDocument();
    expect(screen.queryByText('selected_node_removed')).not.toBeInTheDocument();
    expect(screen.queryByText('ready')).not.toBeInTheDocument();
  });
});
