import { afterEach, describe, it, expect, vi } from 'vitest';
import {
  profileTypeLabel, profileStateLabel, profileStateColor,
  nodeStateLabel, nodeStateColor,
  runStateLabel, runStatusColor, runTypeLabel, runTriggerSourceLabel, runResultLabel, runReasonLabel,
  logResultLabel, logResultColor,
  failureStageLabel, switchTriggerLabel, switchReasonLabel, profileIssueSummary,
  eventTypeLabel, eventTypeColor, eventTriggerLabel,
  chainEvaluationModeLabel, latencyKindLabel,
  formatAbsoluteTime, formatTime, formatRelativeTime, latencyText, formatBytes,
  subscriptionSourceLabel, subscriptionStateLabel, subscriptionStateColor,
  countryDisplay, pathSummaryText, egressCountryModeLabel, sourceModeLabel,
} from './display';

describe('profileTypeLabel', () => {
  it('returns Chinese labels for all types', () => {
    expect(profileTypeLabel('fixed_node')).toBe('固定节点');
    expect(profileTypeLabel('fastest')).toBe('自动优选');
    expect(profileTypeLabel('random')).toBe('随机');
    expect(profileTypeLabel('chain')).toBe('链式');
  });
});

describe('profileStateLabel', () => {
  it('returns Chinese labels for all states', () => {
    expect(profileStateLabel('pending')).toBe('等待中');
    expect(profileStateLabel('running')).toBe('运行中');
    expect(profileStateLabel('waiting_observation')).toBe('等待观测');
    expect(profileStateLabel('ready')).toBe('就绪');
    expect(profileStateLabel('degraded')).toBe('降级');
    expect(profileStateLabel('no_candidate')).toBe('无候选');
    expect(profileStateLabel('failed')).toBe('失败');
    expect(profileStateLabel('invalid_config')).toBe('配置无效');
  });
});

describe('profileStateColor', () => {
  it('returns correct colors', () => {
    expect(profileStateColor('ready')).toBe('green');
    expect(profileStateColor('failed')).toBe('red');
    expect(profileStateColor('no_candidate')).toBe('red');
    expect(profileStateColor('invalid_config')).toBe('red');
    expect(profileStateColor('degraded')).toBe('orange');
    expect(profileStateColor('running')).toBe('blue');
    expect(profileStateColor('waiting_observation')).toBe('blue');
    expect(profileStateColor('pending')).toBe('grey');
  });
});

describe('nodeStateLabel', () => {
  it('returns Chinese labels', () => {
    expect(nodeStateLabel('enabled')).toBe('已启用');
    expect(nodeStateLabel('disabled')).toBe('已禁用');
    expect(nodeStateLabel('usable')).toBe('可用');
    expect(nodeStateLabel('unusable')).toBe('不可用');
    expect(nodeStateLabel('pending_observation')).toBe('等待观测');
  });
});

describe('nodeStateColor', () => {
  it('returns correct colors', () => {
    expect(nodeStateColor('usable')).toBe('green');
    expect(nodeStateColor('unusable')).toBe('red');
    expect(nodeStateColor('disabled')).toBe('grey');
    expect(nodeStateColor('pending_observation')).toBe('blue');
  });
});

describe('runStateLabel', () => {
  it('returns Chinese labels', () => {
    expect(runStateLabel('queued')).toBe('排队中');
    expect(runStateLabel('running')).toBe('运行中');
    expect(runStateLabel('finished')).toBe('已结束');
  });
});

describe('runStatusColor', () => {
  it('returns correct colors', () => {
    expect(runStatusColor('finished', 'success')).toBe('green');
    expect(runStatusColor('finished', 'failure')).toBe('red');
    expect(runStatusColor('finished', 'warning')).toBe('orange');
    expect(runStatusColor('running', '')).toBe('blue');
    expect(runStatusColor('queued', '')).toBe('grey');
  });
});

describe('runTypeLabel', () => {
  it('returns Chinese labels', () => {
    expect(runTypeLabel('subscription_refresh')).toBe('订阅刷新');
    expect(runTypeLabel('node_observation')).toBe('节点观测');
    expect(runTypeLabel('profile_evaluation')).toBe('策略评估');
    expect(runTypeLabel('profile_switch')).toBe('策略切换');
    expect(runTypeLabel('geoip_update')).toBe('GeoIP 更新');
    expect(runTypeLabel('log_cleanup')).toBe('保留清理');
    expect(runTypeLabel('startup_cleanup')).toBe('启动清理');
  });
});

describe('run labels', () => {
  it('returns result, trigger and reason labels', () => {
    expect(runResultLabel('success')).toBe('成功');
    expect(runTriggerSourceLabel('access_profile_change')).toBe('策略配置变更');
    expect(runTriggerSourceLabel('current_node_observed')).toBe('原节点移除后重评');
    expect(runTriggerSourceLabel('current_node_removed')).toBe('当前节点已移除');
    expect(runReasonLabel('expired_after_restart')).toBe('重启后过期取消');
    expect(runReasonLabel('no_importable_nodes')).toBe('无可导入节点');
    expect(runReasonLabel('unknown_run_type')).toBe('未知维护任务类型');
    expect(runReasonLabel('current_node_removed')).toBe('当前节点已移除');
    expect(runReasonLabel('selected_node_removed')).toBe('选中节点已移除');
    expect(runReasonLabel('access_profile_change')).toBe('策略配置变更');
    expect(runReasonLabel('missing_fixed_node')).toBe('固定节点不可用或不存在');
    expect(runReasonLabel('candidate_filter_error')).toBe('候选过滤失败');
    expect(runReasonLabel('invalid_chain_config')).toBe('链式策略配置无效');
    expect(runReasonLabel('missing_exit_node')).toBe('出口节点不可用或不存在');
  });
});

describe('logResultLabel', () => {
  it('returns Chinese labels', () => {
    expect(logResultLabel('running')).toBe('进行中');
    expect(logResultLabel('success')).toBe('成功');
    expect(logResultLabel('failure')).toBe('失败');
  });
});

describe('logResultColor', () => {
  it('returns correct colors', () => {
    expect(logResultColor('running')).toBe('blue');
    expect(logResultColor('success')).toBe('green');
    expect(logResultColor('failure')).toBe('red');
  });
});

describe('failureStageLabel', () => {
  it('returns Chinese labels', () => {
    expect(failureStageLabel('authentication')).toBe('认证失败');
    expect(failureStageLabel('profile_selection')).toBe('策略选择');
    expect(failureStageLabel('path_selection')).toBe('路径选择');
    expect(failureStageLabel('dial')).toBe('连接失败');
    expect(failureStageLabel('proxy_handshake')).toBe('代理握手');
    expect(failureStageLabel('upstream')).toBe('上游不可达');
    expect(failureStageLabel('')).toBe('未分类');
  });
});

describe('switchTriggerLabel', () => {
  it('returns Chinese labels', () => {
    expect(switchTriggerLabel('tolerance')).toBe('容忍度切换');
    expect(switchTriggerLabel('failure_recovery')).toBe('故障恢复');
    expect(switchTriggerLabel('admin_manual')).toBe('管理员手动');
  });
});

describe('switchReasonLabel', () => {
  it('returns Chinese labels', () => {
    expect(switchReasonLabel('current_path_reused_after_failure')).toBe('当前路径探测失败，暂时保留旧路径');
    expect(switchReasonLabel('candidate_not_clearly_better')).toBe('候选路径提升未达切换阈值');
    expect(switchReasonLabel('current_path_failed_switch')).toBe('当前路径探测失败，已切换到可用路径');
    expect(switchReasonLabel('access_profile_change')).toBe('策略配置变更');
    expect(switchReasonLabel('current_node_removed')).toBe('当前节点已移除');
    expect(switchReasonLabel('selected_node_removed')).toBe('选中节点已移除');
    expect(switchReasonLabel('missing_fixed_node')).toBe('固定节点不可用或不存在');
    expect(switchReasonLabel('manual_switch_requested')).toBe('手动切换请求');
  });
});

describe('profileIssueSummary', () => {
  it('returns a friendly degraded summary', () => {
    expect(profileIssueSummary({
      state: 'degraded',
      switch_reason: 'current_path_reused_after_failure',
      last_error: 'EOF',
    })).toBe('当前路径探测失败，暂时保留旧路径');
  });

  it('returns raw failure text for failed states', () => {
    expect(profileIssueSummary({
      state: 'failed',
      switch_reason: 'all_candidates_failed',
      last_error: 'connection reset by peer',
    })).toBe('connection reset by peer');
  });

  it('returns empty text for healthy states', () => {
    expect(profileIssueSummary({
      state: 'ready',
      switch_reason: 'candidate_not_clearly_better',
      last_error: '',
    })).toBe('');
  });
});

describe('eventTypeLabel', () => {
  it('returns Chinese labels', () => {
    expect(eventTypeLabel('evaluation_started')).toBe('评估开始');
    expect(eventTypeLabel('evaluation_completed')).toBe('评估完成');
    expect(eventTypeLabel('evaluation_skipped')).toBe('评估跳过');
    expect(eventTypeLabel('path_switched')).toBe('路径切换');
    expect(eventTypeLabel('evaluation_failed')).toBe('评估失败');
  });
});

describe('eventTypeColor', () => {
  it('returns colors by event type', () => {
    expect(eventTypeColor('evaluation_started')).toBe('blue');
    expect(eventTypeColor('evaluation_completed')).toBe('green');
    expect(eventTypeColor('evaluation_skipped')).toBe('orange');
    expect(eventTypeColor('evaluation_failed')).toBe('red');
  });
});

describe('eventTriggerLabel', () => {
  it('returns Chinese labels', () => {
    expect(eventTriggerLabel('scheduled')).toBe('定时');
    expect(eventTriggerLabel('manual')).toBe('手动');
    expect(eventTriggerLabel('config_change')).toBe('配置变更');
    expect(eventTriggerLabel('access_profile_change')).toBe('策略配置变更');
    expect(eventTriggerLabel('node_observation')).toBe('节点观测');
    expect(eventTriggerLabel('pending_rerun')).toBe('待重跑');
  });
});

describe('chainEvaluationModeLabel', () => {
  it('returns Chinese labels', () => {
    expect(chainEvaluationModeLabel('chain_link')).toBe('最快前置');
    expect(chainEvaluationModeLabel('end_to_end')).toBe('整链最快');
  });
});

describe('latencyKindLabel', () => {
  it('returns Chinese labels', () => {
    expect(latencyKindLabel('end_to_end')).toBe('访问耗时');
    expect(latencyKindLabel('chain_link')).toBe('链路耗时');
    expect(latencyKindLabel(null)).toBe('探测耗时');
  });
});

describe('formatAbsoluteTime', () => {
  it('returns formatted time string with seconds', () => {
    const result = formatAbsoluteTime(1700000000000);
    expect(result).toContain('2023');
    expect(result).toMatch(/\d{1,2}:\d{2}:\d{2}/);
  });

  it('returns - for null', () => {
    expect(formatAbsoluteTime(null)).toBe('-');
  });
});

describe('formatTime', () => {
  it('is a compatibility alias for absolute time', () => {
    expect(formatTime(1700000000000)).toBe(formatAbsoluteTime(1700000000000));
  });

  it('returns - for null', () => {
    expect(formatTime(null)).toBe('-');
  });
});

describe('formatRelativeTime', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns - for null', () => {
    expect(formatRelativeTime(null)).toBe('-');
  });

  it('formats past timestamps as relative time', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-01T00:00:00Z'));

    expect(formatRelativeTime(Date.parse('2023-12-31T21:00:00Z'))).toBe('3 小时前');
  });

  it('formats future timestamps', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-01T00:00:00Z'));

    expect(formatRelativeTime(Date.parse('2024-01-01T03:00:00Z'))).toBe('3 小时后');
  });
});

describe('latencyText', () => {
  it('returns formatted latency', () => {
    expect(latencyText(150)).toBe('150 ms');
  });

  it('returns - for null or zero', () => {
    expect(latencyText(null)).toBe('-');
    expect(latencyText(0)).toBe('-');
  });
});

describe('formatBytes', () => {
  it('formats bytes correctly', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1024)).toBe('1.0 KB');
    expect(formatBytes(1048576)).toBe('1.0 MB');
  });
});

describe('subscriptionSourceLabel', () => {
  it('returns Chinese labels', () => {
    expect(subscriptionSourceLabel('remote')).toBe('远程订阅');
    expect(subscriptionSourceLabel('local')).toBe('本地订阅');
  });
});

describe('subscriptionStateLabel', () => {
  it('returns Chinese labels', () => {
    expect(subscriptionStateLabel('active')).toBe('正常');
    expect(subscriptionStateLabel('error')).toBe('错误');
    expect(subscriptionStateLabel('disabled')).toBe('已停用');
  });
});

describe('subscriptionStateColor', () => {
  it('returns correct colors', () => {
    expect(subscriptionStateColor('active')).toBe('green');
    expect(subscriptionStateColor('error')).toBe('red');
    expect(subscriptionStateColor('disabled')).toBe('grey');
  });
});

describe('countryDisplay', () => {
  it('displays country name with code', () => {
    expect(countryDisplay({ value: 'US', iso_code: 'US', name_zh: '美国', is_unknown: false })).toBe('美国 (US)');
  });

  it('displays unknown for unknown country', () => {
    expect(countryDisplay({ value: '__unknown__', iso_code: null, name_zh: '未知', is_unknown: true })).toBe('未知');
  });
});

describe('pathSummaryText', () => {
  it('returns - for null', () => {
    expect(pathSummaryText(null)).toBe('-');
  });

  it('formats single and chain path summaries', () => {
    const country = { value: 'JP', iso_code: 'JP', name_zh: '日本', is_unknown: false };

    expect(pathSummaryText({
      path_type: 'single',
      node: { id: 'node_1', name: 'Tokyo', protocol: 'vmess', server: 'example.com', server_port: 443, egress_ip: null, egress_country: country, observation_latency_ms: null, last_observed_at: null },
      latency_ms: 120,
      latency_kind: 'end_to_end',
      evaluated_at: null,
    })).toBe('Tokyo (vmess) — 日本 (JP) — 120ms');

    expect(pathSummaryText({
      path_type: 'chain',
      front_node: { id: 'front_1', name: 'Front', protocol: 'http', server: 'front.example.com', server_port: 8080, egress_ip: null, egress_country: country, observation_latency_ms: null, last_observed_at: null },
      exit_node: { id: 'exit_1', name: 'Exit', protocol: 'socks5', server: 'exit.example.com', server_port: 1080, egress_ip: null, egress_country: country, observation_latency_ms: null, last_observed_at: null },
      final_egress_country: country,
      chain_evaluation_mode: 'chain_link',
      latency_ms: 80,
      latency_kind: 'chain_link',
      evaluated_at: null,
    })).toBe('Front → Exit — 日本 (JP) — 80ms');
  });
});

describe('egressCountryModeLabel', () => {
  it('returns Chinese labels', () => {
    expect(egressCountryModeLabel('include')).toBe('包含所选国家');
    expect(egressCountryModeLabel('exclude')).toBe('排除所选国家');
  });
});

describe('sourceModeLabel', () => {
  it('returns Chinese labels', () => {
    expect(sourceModeLabel('all')).toBe('全部节点');
    expect(sourceModeLabel('manual')).toBe('仅手动导入');
    expect(sourceModeLabel('subscription')).toBe('仅订阅');
    expect(sourceModeLabel('selected_sources')).toBe('指定来源');
  });
});

describe('stable API enum display coverage', () => {
  it('covers maintenance run enums without leaking raw codes', () => {
    const runTypes = ['subscription_refresh', 'node_observation', 'profile_evaluation', 'profile_switch', 'geoip_update', 'log_cleanup', 'startup_cleanup'];
    const runTriggers = ['scheduled', 'manual', 'startup', 'access_profile_change', 'node_observation', 'subscription_refresh', 'current_node_removed', 'current_node_observed', 'country_profile_unknown_country', 'pending_rerun', 'manual_node_import'];
    const runStates = ['queued', 'running', 'finished'];
    const runResults = ['success', 'warning', 'failure', 'skipped', 'cancelled'];
    const runReasons = [
      'completed', 'partial_failure', 'all_failed', 'no_targets', 'previous_run_still_running',
      'waiting_for_observation', 'replaced_by_manual_run', 'expired_after_restart',
      'min_interval_not_reached', 'superseded_by_config_version', 'profile_load_failed',
      'profile_type_not_evaluable', 'current_path_degraded', 'evaluation_failed',
      'unknown_run_type', 'subscription_not_found', 'fetch_failed', 'parse_failed',
      'import_failed', 'no_importable_nodes', 'geoip_service_unavailable', 'geoip_update_failed',
      'request_log_cleanup_failed', 'maintenance_history_cleanup_failed',
    ];

    for (const code of runTypes) expect(runTypeLabel(code)).not.toBe(code);
    for (const code of runTriggers) expect(runTriggerSourceLabel(code)).not.toBe(code);
    for (const code of runStates) expect(runStateLabel(code)).not.toBe(code);
    for (const code of runResults) expect(runResultLabel(code)).not.toBe(code);
    for (const code of runReasons) expect(runReasonLabel(code)).not.toBe(code);
  });

  it('covers profile path and switch enums without leaking raw codes', () => {
    const switchReasons = [
      'initial_selection', 'current_path_still_best', 'candidate_not_clearly_better',
      'candidate_clearly_better', 'current_path_failed_switch',
      'current_path_reused_after_failure', 'access_profile_change', 'current_node_removed',
      'selected_node_removed', 'missing_fixed_node', 'force_switch', 'manual_switch_requested',
      'all_candidates_failed', 'no_candidate', 'candidate_filter_error',
      'invalid_chain_config', 'missing_exit_node',
    ];

    for (const code of switchReasons) {
      expect(switchReasonLabel(code)).not.toBe(code);
      expect(runReasonLabel(code)).not.toBe('其他原因');
    }
    expect(chainEvaluationModeLabel('chain_link')).not.toBe('chain_link');
    expect(chainEvaluationModeLabel('end_to_end')).not.toBe('end_to_end');
    expect(latencyKindLabel('chain_link')).not.toBe('chain_link');
    expect(latencyKindLabel('end_to_end')).not.toBe('end_to_end');
  });

  it('covers node subscription request log and filter enums', () => {
    for (const code of ['enabled', 'disabled', 'usable', 'unusable', 'pending_observation']) {
      expect(nodeStateLabel(code as never)).not.toBe(code);
    }
    for (const code of ['remote', 'local']) expect(subscriptionSourceLabel(code)).not.toBe(code);
    for (const code of ['active', 'error', 'disabled']) expect(subscriptionStateLabel(code)).not.toBe(code);
    for (const code of ['running', 'success', 'failure']) expect(logResultLabel(code)).not.toBe(code);
    for (const code of ['authentication', 'profile_selection', 'path_selection', 'dial', 'proxy_handshake', 'upstream']) {
      expect(failureStageLabel(code)).not.toBe(code);
    }
    for (const code of ['include', 'exclude']) expect(egressCountryModeLabel(code)).not.toBe(code);
    for (const code of ['all', 'manual', 'subscription', 'selected_sources']) expect(sourceModeLabel(code)).not.toBe(code);
  });
});
