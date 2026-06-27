import type { AccessProfileSummary, AccessProfileType, ProfileState, NodeState, ChainEvaluationMode, LatencyKind, UnixMillis, EgressCountryDisplay, ProxyPathSummary } from './types';

export function profileTypeLabel(t: AccessProfileType): string {
  return { fixed_node: '固定节点', fastest: '自动优选', random: '随机', chain: '链式' }[t] || t;
}
export function profileStateLabel(s: ProfileState): string {
  return { pending: '等待中', running: '运行中', waiting_observation: '等待观测', ready: '就绪', degraded: '降级', no_candidate: '无候选', failed: '失败', invalid_config: '配置无效' }[s] || s;
}
export function profileStateColor(s: ProfileState): string {
  if (s === 'ready') return 'green';
  if (['failed', 'no_candidate', 'invalid_config'].includes(s)) return 'red';
  if (s === 'degraded') return 'orange';
  if (s === 'running' || s === 'waiting_observation') return 'blue';
  return 'grey';
}
export function nodeStateLabel(s: NodeState): string {
  return { enabled: '已启用', disabled: '已禁用', usable: '可用', unusable: '不可用', pending_observation: '等待观测' }[s] || s;
}
export function nodeStateColor(s: NodeState): string {
  if (s === 'usable') return 'green';
  if (s === 'unusable') return 'red';
  if (s === 'disabled') return 'grey';
  if (s === 'pending_observation') return 'blue';
  return 'green';
}
export function runTypeLabel(t: string): string {
  return {
    subscription_refresh: '订阅刷新',
    node_observation: '节点观测',
    profile_evaluation: '策略评估',
    profile_switch: '策略切换',
    geoip_update: 'GeoIP 更新',
    log_cleanup: '保留清理',
    startup_cleanup: '启动清理',
  }[t] || t;
}
export function runTriggerSourceLabel(t: string): string {
  return {
    scheduled: '定时',
    manual: '手动',
    startup: '启动',
    config_change: '配置变更',
    access_profile_change: '策略配置变更',
    node_observation: '节点观测',
    subscription_refresh: '订阅刷新',
    current_node_removed: '当前节点已移除',
    current_node_observed: '原节点移除后重评',
    country_profile_unknown_country: '未知出口国家观测',
    pending_rerun: '待重跑',
    manual_node_import: '手动导入节点',
  }[t] || t;
}
export function runStateLabel(s: string): string {
  return { queued: '排队中', running: '运行中', finished: '已结束' }[s] || s;
}
export function runResultLabel(r: string): string {
  return { success: '成功', warning: '警告', failure: '失败', skipped: '跳过', cancelled: '取消', '': '未完成' }[r] || r;
}
export function runStatusColor(state: string, result: string): string {
  if (state === 'queued') return 'grey';
  if (state === 'running') return 'blue';
  return { success: 'green', warning: 'orange', failure: 'red', skipped: 'grey', cancelled: 'grey' }[result] || 'grey';
}
export function runReasonLabel(reason: string): string {
  return {
    completed: '已完成',
    partial_failure: '部分失败',
    all_failed: '全部失败',
    no_targets: '无目标',
    previous_run_still_running: '已有运行未结束',
    waiting_for_observation: '等待节点观测',
    replaced_by_manual_run: '被手动运行替换',
    expired_after_restart: '重启后过期取消',
    min_interval_not_reached: '未达到最小间隔',
    superseded_by_config_version: '配置版本已更新',
    profile_load_failed: '策略读取失败',
    profile_type_not_evaluable: '策略类型不可评估',
    current_path_degraded: '当前路径降级',
    evaluation_failed: '评估失败',
    unknown_run_type: '未知维护任务类型',
    subscription_not_found: '订阅不存在',
    fetch_failed: '拉取失败',
    parse_failed: '解析失败',
    import_failed: '导入失败',
    no_importable_nodes: '无可导入节点',
    geoip_service_unavailable: 'GeoIP 服务不可用',
    geoip_update_failed: 'GeoIP 更新失败',
    request_log_cleanup_failed: '请求日志清理失败',
    maintenance_history_cleanup_failed: '维护历史清理失败',
    manual_switch_requested: '手动切换请求',
    current_path_still_best: '当前路径仍最优',
    candidate_not_clearly_better: '候选提升未达阈值',
    candidate_clearly_better: '候选路径更优',
    current_path_failed_switch: '当前路径失败并切换',
    current_path_reused_after_failure: '当前路径失败但暂时保留',
    access_profile_change: '策略配置变更',
    current_node_removed: '当前节点已移除',
    selected_node_removed: '选中节点已移除',
    missing_fixed_node: '固定节点不可用或不存在',
    force_switch: '强制切换',
    initial_selection: '初始选择',
    candidate_filter_error: '候选过滤失败',
    invalid_chain_config: '链式策略配置无效',
    missing_exit_node: '出口节点不可用或不存在',
    no_candidate: '无候选',
    all_candidates_failed: '全部候选失败',
  }[reason] || '其他原因';
}
export function logResultLabel(r: string): string {
  return { running: '进行中', success: '成功', failure: '失败' }[r] || r;
}
export function logResultColor(r: string): string {
  return { running: 'blue', success: 'green', failure: 'red' }[r] || 'grey';
}
export function failureStageLabel(s: string): string {
  return { authentication: '认证失败', profile_selection: '策略选择', path_selection: '路径选择', dial: '连接失败', proxy_handshake: '代理握手', upstream: '上游不可达', '': '未分类' }[s] || s;
}
export function switchTriggerLabel(t: string): string {
  return { tolerance: '容忍度切换', failure_recovery: '故障恢复', admin_manual: '管理员手动' }[t] || t;
}
export function switchReasonLabel(reason: string): string {
  return {
    initial_selection: '初始选择路径',
    best_candidate_so_far: '正在评估，已发现当前最佳候选',
    current_path_still_best: '当前路径仍是最优',
    candidate_not_clearly_better: '候选路径提升未达切换阈值',
    candidate_clearly_better: '候选路径明显更优',
    current_path_reused_after_failure: '当前路径探测失败，暂时保留旧路径',
    current_path_failed_switch: '当前路径探测失败，已切换到可用路径',
    access_profile_change: '策略配置变更',
    current_node_removed: '当前节点已移除',
    selected_node_removed: '选中节点已移除',
    missing_fixed_node: '固定节点不可用或不存在',
    force_switch: '手动强制切换',
    manual_switch_requested: '手动切换请求',
    all_candidates_failed: '所有候选探测失败',
    no_candidate: '没有可用候选',
    candidate_filter_error: '候选过滤失败',
    invalid_chain_config: '链式策略配置无效',
    missing_exit_node: '出口节点不可用或不存在',
  }[reason] || reason;
}
export function profileIssueSummary(profile: Pick<AccessProfileSummary, 'state' | 'last_error' | 'switch_reason'>): string {
  if (profile.state === 'degraded') {
    return switchReasonLabel(profile.switch_reason) || '当前路径异常，暂时保留旧路径';
  }
  if (profile.state === 'failed' || profile.state === 'no_candidate' || profile.state === 'invalid_config') {
    return profile.last_error || switchReasonLabel(profile.switch_reason) || profileStateLabel(profile.state);
  }
  return '';
}
export function eventTypeLabel(t: string): string {
  return { evaluation_started: '评估开始', evaluation_completed: '评估完成', evaluation_skipped: '评估跳过', path_switched: '路径切换', evaluation_failed: '评估失败' }[t] || t;
}
export function eventTypeColor(t: string): string {
  return { evaluation_started: 'blue', evaluation_completed: 'green', evaluation_skipped: 'orange', path_switched: 'violet', evaluation_failed: 'red' }[t] || 'grey';
}
export function eventTriggerLabel(t: string): string {
  return runTriggerSourceLabel(t);
}
export function chainEvaluationModeLabel(m: ChainEvaluationMode): string {
  return m === 'chain_link' ? '最快前置' : '整链最快';
}
export function latencyKindLabel(k: LatencyKind | null): string {
  if (k === 'end_to_end') return '访问耗时';
  if (k === 'chain_link') return '链路耗时';
  return '探测耗时';
}

const absoluteTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
});

export function formatAbsoluteTime(ts: UnixMillis | null): string {
  if (!ts) return '-';
  return absoluteTimeFormatter.format(new Date(ts));
}
export function formatTime(ts: UnixMillis | null): string {
  return formatAbsoluteTime(ts);
}
export function formatRelativeTime(ts: UnixMillis | null): string {
  if (!ts) return '-';
  const diff = Math.floor((Date.now() - ts) / 1000);
  if (diff < 0) {
    const future = Math.abs(diff);
    if (future < 60) return '即将';
    if (future < 3600) return `${Math.ceil(future / 60)} 分钟后`;
    if (future < 86400) return `${Math.ceil(future / 3600)} 小时后`;
    if (future < 172800) return '明天';
    return `${Math.ceil(future / 86400)} 天后`;
  }
  if (diff < 60) return '刚刚';
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
  if (diff < 172800) return '昨天';
  return `${Math.floor(diff / 86400)} 天前`;
}
export function latencyText(ms: number | null): string {
  if (!ms || ms <= 0) return '-';
  return `${ms} ms`;
}
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}
export function subscriptionSourceLabel(t: string): string { return t === 'remote' ? '远程订阅' : '本地订阅'; }
export function subscriptionStateLabel(s: string): string {
  return { active: '正常', error: '错误', disabled: '已停用' }[s] || s;
}
export function subscriptionStateColor(s: string): string {
  return { active: 'green', error: 'red', disabled: 'grey' }[s] || 'grey';
}
export function countryDisplay(c: EgressCountryDisplay | string | null | undefined): string {
  if (!c) return '-';
  if (typeof c === 'string') return c || '-';
  return c.is_unknown ? '未知' : `${c.name_zh} (${c.iso_code})`;
}
export function pathSummaryText(p: ProxyPathSummary | null): string {
  if (!p) return '-';
  if (p.path_type === 'single') {
    const n = p.node;
    const lat = p.latency_ms ? ` — ${p.latency_ms}ms` : '';
    return `${n.name} (${n.protocol}) — ${countryDisplay(n.egress_country)}${lat}`;
  }
  const lat = p.latency_ms ? ` — ${p.latency_ms}ms` : '';
  return `${p.front_node.name} → ${p.exit_node.name} — ${countryDisplay(p.final_egress_country)}${lat}`;
}
export function egressCountryModeLabel(m: string): string {
  return m === 'include' ? '包含所选国家' : '排除所选国家';
}
export function sourceModeLabel(m: string): string {
  return { all: '全部节点', manual: '仅手动导入', subscription: '仅订阅', selected_sources: '指定来源' }[m] || m;
}
