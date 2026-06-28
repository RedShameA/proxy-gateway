import type { CSSProperties, ReactNode } from 'react';
import { SideSheet, Descriptions, Tag, Typography } from '@douyinfe/semi-ui';
import { formatRelativeTime, formatTime, runReasonLabel, runResultLabel, runStateLabel, runStatusColor, runTriggerSourceLabel, runTypeLabel, switchReasonLabel } from '../display';
import type { MaintenanceRunSummary } from '../types';

const { Text } = Typography;

const detailValueStyle: CSSProperties = {
  maxWidth: '100%',
  minWidth: 0,
  whiteSpace: 'pre-wrap',
  overflowWrap: 'anywhere',
  wordBreak: 'break-word',
};

const detailBlockStyle: CSSProperties = {
  ...detailValueStyle,
  margin: 0,
  maxHeight: 240,
  overflow: 'auto',
  border: '1px solid var(--semi-color-border)',
  borderRadius: 6,
  padding: 8,
  background: 'var(--semi-color-fill-0)',
  fontSize: 12,
  lineHeight: 1.6,
};

const detailKeyLabels: Record<string, string> = {
  added_count: '新增数',
  best_exit_node_id: '最佳出口节点',
  best_latency_ms: '最佳耗时',
  best_node_id: '最佳节点',
  candidate_count: '候选数',
  cancelled_count: '取消数',
  config_version: '配置版本',
  current_config_version: '当前配置版本',
  current_exit_node_id: '当前出口节点',
  current_latency_ms: '当前耗时',
  current_node_id: '当前节点',
  current_path_result: '当前路径结果',
  deleted_maintenance_runs: '删除维护历史',
  deleted_request_logs: '删除请求日志',
  exit_node_id: '出口节点',
  failure_count: '失败数',
  failure_reasons: '失败原因',
  force_switch: '强制切换',
  geoip: 'GeoIP 状态',
  ignored_count: '忽略数',
  ignored_entry_summary: '忽略条目',
  imported: '导入数',
  imported_count: '导入数',
  log_retention_enabled: '请求日志保留',
  log_retention_days: '请求日志保留天数',
  maintenance_history_retention_enabled: '维护历史保留',
  maintenance_history_retention_days: '维护历史保留天数',
  node_ids: '节点',
  partial: '部分结果',
  probe_url: '探测地址',
  profile_id: '策略',
  profile_state: '策略状态',
  reason: '原因',
  removed_count: '移除数',
  retained_count: '保留数',
  sample_failures: '失败样例',
  selected_exit_node_id: '选中出口节点',
  selected_node_id: '选中节点',
  skipped_count: '跳过数',
  skipped_entry_summary: '跳过条目',
  source: '来源',
  subscription_id: '订阅',
  success_count: '成功数',
  switch_decision: '切换决策',
  switch_reason: '切换原因',
  target_scope: '目标范围',
  test_url: '测试地址',
  updated_count: '更新数',
};

export function MaintenanceRunRow({ run, onClick }: { run: MaintenanceRunSummary; onClick: (run: MaintenanceRunSummary) => void }) {
  const progress = run.total_count > 0 ? `${run.finished_count}/${run.total_count}` : '';
  const statusText = run.state === 'finished' ? runResultLabel(run.result) : runStateLabel(run.state);
  const left = [runTypeLabel(run.run_type), runTriggerSourceLabel(run.trigger_source)].filter(Boolean).join(' · ');
  return (
    <button
      type="button"
      onClick={() => onClick(run)}
      style={{
        width: '100%',
        border: '1px solid var(--semi-color-border)',
        background: 'var(--semi-color-bg-1)',
        borderRadius: 8,
        padding: '8px 12px',
        marginBottom: 8,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 12,
        cursor: 'pointer',
        textAlign: 'left',
      }}
    >
      <Text strong ellipsis={{ showTooltip: true }} style={{ minWidth: 0, flex: 1 }}>{left}</Text>
      <span style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'flex-end', gap: 8, flexShrink: 0, minWidth: 0 }}>
        <Tag color={runStatusColor(run.state, run.result) as any} size="small">{statusText}</Tag>
        {progress && <Text size="small" type="secondary">{progress}</Text>}
        {run.reason_code && <Text size="small" type="secondary" ellipsis={{ showTooltip: true }} style={{ maxWidth: 160 }}>{runReasonLabel(run.reason_code)}</Text>}
        <Text size="small" type="secondary" style={{ whiteSpace: 'nowrap' }}>{formatRelativeTime(run.created_at)}</Text>
      </span>
    </button>
  );
}

export function MaintenanceRunDrawer({ run, visible, onClose }: { run: MaintenanceRunSummary | null; visible: boolean; onClose: () => void }) {
  const target = run ? maintenanceRunTargetDisplay(run) : { visible: false, value: '-' };
  const summaryRows = run ? [
    { key: '触发来源', value: runTriggerSourceLabel(run.trigger_source) },
    ...(target.visible ? [{ key: '目标', value: target.value }] : []),
    { key: '状态', value: run.state === 'finished' ? runResultLabel(run.result) : runStateLabel(run.state) },
    { key: '原因', value: run.reason_code ? runReasonLabel(run.reason_code) : '-' },
    { key: '进度', value: run.total_count > 0 ? `${run.finished_count}/${run.total_count}` : '-' },
    { key: '创建时间', value: formatTime(run.created_at) },
    { key: '开始时间', value: formatTime(run.started_at) },
    { key: '结束时间', value: formatTime(run.finished_at) },
  ] : [];
  const detailRows = run ? Object.entries(run.detail || {}).map(([key, value]) => ({
    key: detailKeyLabel(key),
    value: <DetailValue fieldKey={key} value={value} />,
  })) : [];
  return (
    <SideSheet title={run ? runTypeLabel(run.run_type) : '维护历史'} visible={visible} onCancel={onClose} width={520}>
      {run && (
        <div style={{ display: 'grid', gap: 16 }}>
          <Descriptions row size="small" data={summaryRows} />
          {run.last_error && <Text type="danger" style={detailValueStyle}>错误: {run.last_error}</Text>}
          <Descriptions row size="small" data={detailRows.length > 0 ? detailRows : [{ key: '详情', value: '-' }]} />
        </div>
      )}
    </SideSheet>
  );
}

export function maintenanceRunTargetDisplay(run: MaintenanceRunSummary): { visible: boolean; value: string } {
  if (!maintenanceRunShouldShowTarget(run)) {
    return { visible: false, value: '-' };
  }
  return { visible: true, value: run.target_label || run.target_id || '-' };
}

function maintenanceRunShouldShowTarget(run: MaintenanceRunSummary): boolean {
  if (run.run_type === 'subscription_refresh' || run.run_type === 'profile_evaluation' || run.run_type === 'profile_switch') {
    return true;
  }
  if (run.run_type === 'node_observation') {
    return run.detail?.target_scope === 'single_node';
  }
  return false;
}

function DetailValue({ fieldKey, value }: { fieldKey: string; value: unknown }) {
  if (value === null || value === undefined || value === '') {
    return <span style={detailValueStyle}>-</span>;
  }
  if (typeof value === 'boolean') {
    return <span style={detailValueStyle}>{value ? '是' : '否'}</span>;
  }
  if (typeof value === 'number') {
    const suffix = fieldKey.endsWith('_latency_ms') ? ' ms' : '';
    return <span style={detailValueStyle}>{value}{suffix}</span>;
  }
  if (typeof value === 'string') {
    return <span style={detailValueStyle}>{formatDetailString(fieldKey, value)}</span>;
  }
  return <pre style={detailBlockStyle}>{formatDetailValue(value)}</pre>;
}

function detailKeyLabel(key: string): string {
  return detailKeyLabels[key] || key;
}

function formatDetailString(fieldKey: string, value: string): ReactNode {
  if (fieldKey === 'switch_decision' || fieldKey === 'switch_reason' || fieldKey === 'reason') {
    return switchReasonLabel(value);
  }
  if (fieldKey === 'profile_state' || fieldKey === 'current_path_result') {
    return runProfileStateLabel(value);
  }
  if (fieldKey === 'target_scope') {
    return {
      all_nodes: '全部节点',
      due_nodes: '到期节点',
      single_node: '单个节点',
    }[value] || value;
  }
  return value;
}

function runProfileStateLabel(value: string): string {
  return {
    degraded: '降级',
    failed: '失败',
    invalid_config: '配置无效',
    no_candidate: '无候选',
    pending: '等待中',
    ready: '就绪',
    running: '运行中',
    waiting_observation: '等待观测',
  }[value] || value;
}

function formatDetailValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return '-';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}
