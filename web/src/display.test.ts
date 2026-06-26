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
    expect(runReasonLabel('expired_after_restart')).toBe('重启后过期取消');
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
