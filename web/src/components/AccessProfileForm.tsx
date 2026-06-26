import { useMemo, useState } from 'react';
import { Button, Collapse, Input, InputNumber, Select, Switch, Toast, Tooltip, Typography } from '@douyinfe/semi-ui';
import { IconHelpCircleStroked } from '@douyinfe/semi-icons';
import type {
  AccessProfileDetail,
  AccessProfileType,
  AccessProfileWriteRequest,
  ChainEvaluationMode,
  EgressCountryDisplay,
  NodeSummary,
  SubscriptionSummary,
} from '../types';

const { Text } = Typography;

type FormState = {
  name: string;
  profile_identifier: string;
  type: AccessProfileType;
  fixed_node_id: string;
  exit_node_ids: string[];
  chain_evaluation_mode: ChainEvaluationMode;
  test_url: string;
  source_mode: 'all' | 'manual' | 'subscription' | 'selected_sources';
  source_ids: string[];
  protocols: string[];
  name_include: string;
  name_exclude: string;
  egress_country_mode: 'include' | 'exclude';
  egress_countries: string[];
  relative_percent: number;
  absolute_latency_improvement_ms: number;
  evaluation_mode: 'inherit' | 'custom' | 'disabled';
  evaluation_interval_seconds: number;
  node_sticky_enabled: boolean;
};

type Props = {
  initial?: AccessProfileDetail | null;
  nodes: NodeSummary[];
  countries: EgressCountryDisplay[];
  subscriptions: SubscriptionSummary[];
  submitting?: boolean;
  submitLabel: string;
  onSubmit: (payload: AccessProfileWriteRequest) => Promise<void>;
};

const typeOptions: { value: AccessProfileType; label: string }[] = [
  { value: 'fastest', label: '自动优选' },
  { value: 'fixed_node', label: '固定节点' },
  { value: 'random', label: '随机' },
  { value: 'chain', label: '链式' },
];

const protocolFallbacks = ['http', 'socks5', 'shadowsocks', 'vmess', 'trojan', 'direct'];
const profileIdentifierPattern = /^[A-Za-z0-9_-]+$/;

function identifierFrom(name: string, explicit: string) {
  const source = explicit.trim() || name.trim();
  const normalized = source
    .toLowerCase()
    .replace(/[^a-z0-9-_]/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 32);
  if (normalized.length >= 3) return normalized;
  return `profile-${Date.now().toString(36).slice(-6)}`;
}

function initialState(profile?: AccessProfileDetail | null): FormState {
  if (!profile) {
    return {
      name: '',
      profile_identifier: '',
      type: 'fastest',
      fixed_node_id: '',
      exit_node_ids: [],
      chain_evaluation_mode: 'end_to_end',
      test_url: '',
      source_mode: 'all',
      source_ids: [],
      protocols: [],
      name_include: '',
      name_exclude: '',
      egress_country_mode: 'include',
      egress_countries: [],
      relative_percent: 20,
      absolute_latency_improvement_ms: 100,
      evaluation_mode: 'inherit',
      evaluation_interval_seconds: 300,
      node_sticky_enabled: false,
    };
  }
  const schedule = profile.evaluation_schedule;
  const mode = schedule.mode === 'custom' && !schedule.interval_seconds ? 'inherit' : schedule.mode;
  return {
    name: profile.name,
    profile_identifier: profile.profile_identifier || '',
    type: profile.type,
    fixed_node_id: profile.fixed_node_id || '',
    exit_node_ids: profile.exit_node_ids || [],
    chain_evaluation_mode: profile.chain_evaluation_mode || 'end_to_end',
    test_url: profile.test_url || '',
    source_mode: profile.candidate_filter.source_mode,
    source_ids: profile.candidate_filter.source_ids || [],
    protocols: profile.candidate_filter.protocols || [],
    name_include: profile.candidate_filter.name_include || '',
    name_exclude: profile.candidate_filter.name_exclude || '',
    egress_country_mode: profile.candidate_filter.egress_country_mode || 'include',
    egress_countries: profile.candidate_filter.egress_countries || [],
    relative_percent: Math.round((profile.switching_tolerance.relative_improvement_threshold ?? 0.2) * 100),
    absolute_latency_improvement_ms: profile.switching_tolerance.absolute_latency_improvement_ms ?? 100,
    evaluation_mode: mode,
    evaluation_interval_seconds: schedule.interval_seconds || 300,
    node_sticky_enabled: profile.type === 'fastest' || profile.type === 'chain' ? profile.node_sticky_enabled : false,
  };
}

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map(item => String(item)).filter(Boolean);
}

function LabelWithHelp({ label, help }: { label: string; help: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
      <Text type="secondary">{label}</Text>
      <Tooltip content={help}>
        <IconHelpCircleStroked size="small" style={{ color: 'var(--semi-color-text-2)', cursor: 'help' }} />
      </Tooltip>
    </div>
  );
}

export function AccessProfileForm({ initial, nodes, countries, subscriptions, submitting, submitLabel, onSubmit }: Props) {
  const [values, setValues] = useState<FormState>(() => initialState(initial));

  const protocolOptions = useMemo(() => {
    const seen = new Set<string>();
    return [...protocolFallbacks, ...nodes.map(node => node.protocol)]
      .map(protocol => protocol.trim())
      .filter(protocol => {
        if (!protocol || seen.has(protocol)) return false;
        seen.add(protocol);
        return true;
      });
  }, [nodes]);

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setValues(prev => ({ ...prev, [key]: value }));
  };

  const setType = (type: AccessProfileType) => {
    setValues(prev => ({
      ...prev,
      type,
      fixed_node_id: type === 'fixed_node' ? prev.fixed_node_id : '',
      exit_node_ids: type === 'chain' ? prev.exit_node_ids : [],
      chain_evaluation_mode: type === 'chain' ? prev.chain_evaluation_mode : 'end_to_end',
      node_sticky_enabled: type === 'fastest' || type === 'chain' ? prev.node_sticky_enabled : false,
    }));
  };

  const setExitNodeIDs = (ids: string[]) => {
    setValues(prev => ({
      ...prev,
      exit_node_ids: ids,
      chain_evaluation_mode: ids.length > 1 ? 'end_to_end' : prev.chain_evaluation_mode,
    }));
  };

  const buildPayload = (): AccessProfileWriteRequest | null => {
    const name = values.name.trim();
    if (!name) {
      Toast.error('策略名称不能为空');
      return null;
    }
    const explicitIdentifier = values.profile_identifier.trim();
    if (explicitIdentifier && (explicitIdentifier.length < 3 || explicitIdentifier.length > 32)) {
      Toast.error('策略标识长度需为 3-32 个字符');
      return null;
    }
    if (explicitIdentifier && !profileIdentifierPattern.test(explicitIdentifier)) {
      Toast.error('策略标识只能包含字母、数字、连字符和下划线');
      return null;
    }
    if (values.type === 'fixed_node' && !values.fixed_node_id) {
      Toast.error('固定节点不能为空');
      return null;
    }
    if (values.type === 'chain' && values.exit_node_ids.length === 0) {
      Toast.error('出口节点不能为空');
      return null;
    }
    const chainMode = values.exit_node_ids.length > 1 ? 'end_to_end' : values.chain_evaluation_mode;
    if (values.type === 'chain' && chainMode === 'chain_link' && values.exit_node_ids.length !== 1) {
      Toast.error('最快前置只支持单出口节点');
      return null;
    }
    const testURL = values.test_url.trim();
    if (testURL) {
      const effectiveURL = testURL.includes('://') ? testURL : `https://${testURL}`;
      try {
        const parsed = new URL(effectiveURL);
        if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
          Toast.error('Test URL 仅支持 http:// 或 https://');
          return null;
        }
        if (!parsed.host.trim()) {
          Toast.error('Test URL 必须包含主机名');
          return null;
        }
      } catch {
        Toast.error('Test URL 必须包含主机名');
        return null;
      }
    }
    return {
      name,
      profile_identifier: identifierFrom(name, values.profile_identifier),
      type: values.type,
      fixed_node_id: values.type === 'fixed_node' ? values.fixed_node_id : null,
      exit_node_ids: values.type === 'chain' ? values.exit_node_ids : [],
      chain_evaluation_mode: values.type === 'chain' ? chainMode : null,
      test_url: testURL,
      candidate_filter: {
        source_mode: values.source_mode,
        source_ids: values.source_mode === 'selected_sources' ? values.source_ids : [],
        protocols: values.protocols,
        name_include: values.name_include.trim(),
        name_exclude: values.name_exclude.trim(),
        egress_country_mode: values.egress_country_mode,
        egress_countries: values.egress_countries,
      },
      switching_tolerance: {
        relative_improvement_threshold: Math.max(0, values.relative_percent) / 100,
        absolute_latency_improvement_ms: Math.max(0, values.absolute_latency_improvement_ms),
      },
      evaluation_schedule: {
        mode: values.evaluation_mode,
        interval_seconds: values.evaluation_mode === 'custom' ? Math.max(0, values.evaluation_interval_seconds) : null,
      },
      node_sticky_enabled: values.type === 'fastest' || values.type === 'chain' ? values.node_sticky_enabled : false,
    };
  };

  const submit = async () => {
    const payload = buildPayload();
    if (!payload) return;
    await onSubmit(payload);
  };

  return (
    <div style={{ display: 'grid', gap: 14 }}>
      <div>
        <Text type="secondary">名称</Text>
        <Input value={values.name} onChange={v => set('name', v)} placeholder="策略名称" style={{ marginTop: 4 }} />
      </div>
      <div>
        <Text type="secondary">策略标识</Text>
        <Input value={values.profile_identifier} onChange={v => set('profile_identifier', v)} placeholder="url-safe-identifier" style={{ marginTop: 4 }} />
      </div>
      <div>
        <Text type="secondary">类型</Text>
        <Select value={values.type} onChange={v => setType(v as AccessProfileType)} style={{ width: '100%', marginTop: 4 }}>
          {typeOptions.map(option => <Select.Option key={option.value} value={option.value}>{option.label}</Select.Option>)}
        </Select>
      </div>

      {values.type === 'fixed_node' && (
        <div>
          <Text type="secondary">固定节点</Text>
          <Select value={values.fixed_node_id || undefined} onChange={v => set('fixed_node_id', String(v || ''))} placeholder="选择节点" style={{ width: '100%', marginTop: 4 }}>
            {nodes.map(node => <Select.Option key={node.id} value={node.id}>{node.name}</Select.Option>)}
          </Select>
        </div>
      )}

      {values.type === 'chain' && (
        <>
          <div>
            <Text type="secondary">出口节点</Text>
            <Select multiple value={values.exit_node_ids} onChange={v => setExitNodeIDs(normalizeStringArray(v))} placeholder="选择出口节点" style={{ width: '100%', marginTop: 4 }}>
              {nodes.map(node => <Select.Option key={node.id} value={node.id}>{node.name}</Select.Option>)}
            </Select>
          </div>
          <div>
            <Text type="secondary">链式模式</Text>
            <Select value={values.chain_evaluation_mode} onChange={v => set('chain_evaluation_mode', v as ChainEvaluationMode)} style={{ width: '100%', marginTop: 4 }}>
              <Select.Option value="end_to_end">整链最快</Select.Option>
              <Select.Option value="chain_link" disabled={values.exit_node_ids.length > 1}>最快前置</Select.Option>
            </Select>
            {values.exit_node_ids.length > 1 && <Text size="small" type="secondary">多出口节点仅支持整链最快</Text>}
          </div>
        </>
      )}

      {(values.type === 'fastest' || values.type === 'fixed_node' || values.type === 'chain') && (
        <div>
          <Text type="secondary">Test URL</Text>
          <Input value={values.test_url} onChange={v => set('test_url', v)} placeholder="留空使用系统默认" style={{ marginTop: 4 }} />
        </div>
      )}

      <Collapse>
        <Collapse.Panel header={values.type === 'chain' ? '前置节点候选筛选' : '候选筛选'} itemKey="filter">
          <div style={{ display: 'grid', gap: 12 }}>
            <div>
              <Text type="secondary">来源模式</Text>
              <Select value={values.source_mode} onChange={v => set('source_mode', v as FormState['source_mode'])} style={{ width: '100%', marginTop: 4 }}>
                <Select.Option value="all">全部节点</Select.Option>
                <Select.Option value="manual">仅手动导入</Select.Option>
                <Select.Option value="subscription">仅订阅节点</Select.Option>
                <Select.Option value="selected_sources">指定订阅</Select.Option>
              </Select>
            </div>
            {values.source_mode === 'selected_sources' && (
              <div>
                <Text type="secondary">指定订阅</Text>
                <Select multiple value={values.source_ids} onChange={v => set('source_ids', normalizeStringArray(v))} style={{ width: '100%', marginTop: 4 }}>
                  {subscriptions.map(sub => <Select.Option key={sub.id} value={sub.id}>{sub.name}</Select.Option>)}
                </Select>
              </div>
            )}
            <div>
              <Text type="secondary">协议</Text>
              <Select multiple value={values.protocols} onChange={v => set('protocols', normalizeStringArray(v))} placeholder="全部协议" style={{ width: '100%', marginTop: 4 }}>
                {protocolOptions.map(protocol => <Select.Option key={protocol} value={protocol}>{protocol}</Select.Option>)}
              </Select>
            </div>
            <div>
              <Text type="secondary">名称包含</Text>
              <Input value={values.name_include} onChange={v => set('name_include', v)} placeholder="可选，正则表达式" style={{ marginTop: 4 }} />
            </div>
            <div>
              <Text type="secondary">名称排除</Text>
              <Input value={values.name_exclude} onChange={v => set('name_exclude', v)} placeholder="可选，正则表达式" style={{ marginTop: 4 }} />
            </div>
            <div>
              <Text type="secondary">{values.type === 'chain' ? '前置节点出口国家模式' : '出口国家模式'}</Text>
              <Select value={values.egress_country_mode} onChange={v => set('egress_country_mode', v as FormState['egress_country_mode'])} style={{ width: '100%', marginTop: 4 }}>
                <Select.Option value="include">包含所选国家</Select.Option>
                <Select.Option value="exclude">排除所选国家</Select.Option>
              </Select>
            </div>
            <div>
              <Text type="secondary">{values.type === 'chain' ? '前置节点出口国家' : '出口国家'}</Text>
              <Select multiple value={values.egress_countries} onChange={v => set('egress_countries', normalizeStringArray(v))} placeholder="不限制" style={{ width: '100%', marginTop: 4 }}>
                {countries.map(country => <Select.Option key={country.value} value={country.value}>{country.name_zh}</Select.Option>)}
              </Select>
            </div>
          </div>
        </Collapse.Panel>
        <Collapse.Panel header="切换与评估" itemKey="evaluation">
          <div style={{ display: 'grid', gap: 12 }}>
            {(values.type === 'fastest' || values.type === 'chain') && (
              <div>
                <LabelWithHelp label="节点粘滞" help="订阅刷新移除当前节点时，暂时保留旧路径继续服务；评估并切换到可用新路径后自动清理。" />
                <div style={{ marginTop: 4 }}>
                  <Switch checked={values.node_sticky_enabled} onChange={v => set('node_sticky_enabled', Boolean(v))} />
                </div>
              </div>
            )}
            <Text type="secondary" size="small">任一项设为 0 时不使用该门槛；两项都为 0 时，候选路径只要更快就允许切换。</Text>
            <div>
              <LabelWithHelp label="相对提升 (%)" help="候选路径相对当前路径的延迟提升达到该比例时，允许切换；设为 0 时不使用相对提升门槛。" />
              <InputNumber value={values.relative_percent} min={0} max={100} onChange={v => set('relative_percent', Number(v) || 0)} style={{ width: '100%', marginTop: 4 }} />
            </div>
            <div>
              <LabelWithHelp label="绝对延迟提升 (ms)" help="候选路径相对当前路径的延迟降低达到该毫秒数时，允许切换；设为 0 时不使用绝对提升门槛。" />
              <InputNumber value={values.absolute_latency_improvement_ms} min={0} onChange={v => set('absolute_latency_improvement_ms', Number(v) || 0)} style={{ width: '100%', marginTop: 4 }} />
            </div>
            <div>
              <Text type="secondary">评估计划</Text>
              <Select value={values.evaluation_mode} onChange={v => set('evaluation_mode', v as FormState['evaluation_mode'])} style={{ width: '100%', marginTop: 4 }}>
                <Select.Option value="inherit">跟随全局</Select.Option>
                <Select.Option value="custom">自定义</Select.Option>
                <Select.Option value="disabled">关闭</Select.Option>
              </Select>
            </div>
            {values.evaluation_mode === 'custom' && (
              <div>
                <Text type="secondary">自定义周期 (秒)</Text>
                <InputNumber value={values.evaluation_interval_seconds} min={0} onChange={v => set('evaluation_interval_seconds', Number(v) || 0)} style={{ width: '100%', marginTop: 4 }} />
              </div>
            )}
          </div>
        </Collapse.Panel>
      </Collapse>

      <Button type="primary" loading={submitting} onClick={submit}>{submitLabel}</Button>
    </div>
  );
}
