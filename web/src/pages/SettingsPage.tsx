import { useState, useCallback, useEffect } from 'react';
import { Button, Typography, Tag, Card, Switch, InputNumber, Input, Toast, Descriptions, Spin } from '@douyinfe/semi-ui';
import { AdaptivePanel } from '../components/AdaptivePanel';
import { useData } from '../hooks/useData';
import { formatRelativeTime } from '../display';
import type { ApiClient } from '../api';

const { Title, Text } = Typography;

type ScheduleDraftItem = {
  key: string;
  label: string;
  enabled: boolean;
  interval_seconds: number | string | null;
};

type RetentionDraft = {
  requestLogEnabled: boolean;
  requestLogDays: number | string;
  maintenanceHistoryEnabled: boolean;
  maintenanceHistoryDays: number | string;
};

export function SettingsPage({ client }: { client: ApiClient }) {
  const [toleranceVisible, setToleranceVisible] = useState(false);
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [endpoint, setEndpoint] = useState('');
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [toleranceValues, setToleranceValues] = useState({ relative: 20, absolute: 100 });
  const [scheduleValues, setScheduleValues] = useState<ScheduleDraftItem[]>([]);
  const [scanValues, setScanValues] = useState({
    nodeObservationConcurrency: 8,
    profileEvaluationConcurrency: 2,
    globalConcurrency: 32,
    connectTimeoutSeconds: 10,
    probeTimeoutSeconds: 10,
  });
  const [retentionValues, setRetentionValues] = useState<RetentionDraft>({
    requestLogEnabled: true,
    requestLogDays: 10,
    maintenanceHistoryEnabled: true,
    maintenanceHistoryDays: 7,
  });

  const fetcher = useCallback(() => client.getSettings(), [client]);
  const { data: settings, loading, error, refresh } = useData(fetcher);

  useEffect(() => {
    if (!settings) return;
    setEndpoint(settings.public_proxy_endpoint || '');
    setToleranceValues({
      relative: Math.round((settings.switching_tolerance.relative_improvement_threshold || 0) * 100),
      absolute: settings.switching_tolerance.absolute_latency_improvement_ms,
    });
    setScheduleValues(settings.maintenance_schedules.map(item => ({ ...item })));
    setScanValues({
      nodeObservationConcurrency: settings.maintenance.node_observation_concurrency,
      profileEvaluationConcurrency: settings.maintenance.profile_evaluation_concurrency,
      globalConcurrency: settings.evaluation.global_concurrency,
      connectTimeoutSeconds: settings.evaluation.connect_timeout_seconds,
      probeTimeoutSeconds: settings.evaluation.probe_timeout_seconds,
    });
    setRetentionValues({
      requestLogEnabled: settings.log_retention_enabled,
      requestLogDays: settings.log_retention_days,
      maintenanceHistoryEnabled: settings.maintenance_history_retention_enabled,
      maintenanceHistoryDays: settings.maintenance_history_retention_days,
    });
  }, [settings]);

  if (loading) return <div className="page-loading"><Spin size="large" /></div>;
  if (error) return <div style={{ padding: 20 }}><Title heading={4}>加载失败</Title><Text type="danger">{error}</Text><Button onClick={refresh} style={{ marginTop: 12 }}>重试</Button></div>;
  if (!settings) return null;

  const updateScheduleValue = (key: string, patch: Partial<ScheduleDraftItem>) => {
    setScheduleValues(prev => prev.map(item => item.key === key ? { ...item, ...patch } : item));
  };

  const saveScheduleSettings = async () => {
    const next = [];
    for (const item of scheduleValues) {
      let interval: number | null = null;
      if (item.interval_seconds !== null) {
        const parsed = Math.trunc(Number(item.interval_seconds));
        if (item.enabled && (!Number.isFinite(parsed) || parsed < 60)) {
          Toast.error('调度间隔至少 60 秒');
          return;
        }
        interval = Number.isFinite(parsed) && parsed > 0 ? parsed : null;
      }
      next.push({ ...item, interval_seconds: interval });
    }
    try {
      await client.updateSettings({ maintenance_schedules: next });
      setScheduleValues(next);
      Toast.success('已保存');
      refresh();
    } catch (err) {
      Toast.error(`保存失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const saveScanSettings = async () => {
    const next = {
      nodeObservationConcurrency: Math.max(1, Math.trunc(Number(scanValues.nodeObservationConcurrency) || settings.maintenance.node_observation_concurrency || 1)),
      profileEvaluationConcurrency: Math.max(1, Math.trunc(Number(scanValues.profileEvaluationConcurrency) || settings.maintenance.profile_evaluation_concurrency || 1)),
      globalConcurrency: Math.max(1, Math.trunc(Number(scanValues.globalConcurrency) || settings.evaluation.global_concurrency || 1)),
      connectTimeoutSeconds: Math.max(1, Math.trunc(Number(scanValues.connectTimeoutSeconds) || settings.evaluation.connect_timeout_seconds || 10)),
      probeTimeoutSeconds: Math.max(1, Math.trunc(Number(scanValues.probeTimeoutSeconds) || settings.evaluation.probe_timeout_seconds || 10)),
    };
    try {
      await client.updateSettings({
        maintenance: {
          ...settings.maintenance,
          node_observation_concurrency: next.nodeObservationConcurrency,
          profile_evaluation_concurrency: next.profileEvaluationConcurrency,
        },
        evaluation: {
          ...settings.evaluation,
          global_concurrency: next.globalConcurrency,
          connect_timeout_seconds: next.connectTimeoutSeconds,
          probe_timeout_seconds: next.probeTimeoutSeconds,
        },
      });
      setScanValues(next);
      Toast.success('已保存');
      refresh();
    } catch (err) {
      Toast.error(`保存失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const saveRetentionSettings = async () => {
    const requestLogDays = Math.trunc(Number(retentionValues.requestLogDays));
    const maintenanceHistoryDays = Math.trunc(Number(retentionValues.maintenanceHistoryDays));
    if (retentionValues.requestLogEnabled && (!Number.isFinite(requestLogDays) || requestLogDays <= 0)) {
      Toast.error('请求日志保留天数必须大于 0');
      return;
    }
    if (retentionValues.maintenanceHistoryEnabled && (!Number.isFinite(maintenanceHistoryDays) || maintenanceHistoryDays <= 0)) {
      Toast.error('维护历史保留天数必须大于 0');
      return;
    }
    const next = {
      requestLogEnabled: retentionValues.requestLogEnabled,
      requestLogDays: Number.isFinite(requestLogDays) && requestLogDays > 0 ? requestLogDays : settings.log_retention_days,
      maintenanceHistoryEnabled: retentionValues.maintenanceHistoryEnabled,
      maintenanceHistoryDays: Number.isFinite(maintenanceHistoryDays) && maintenanceHistoryDays > 0 ? maintenanceHistoryDays : settings.maintenance_history_retention_days,
    };
    try {
      await client.updateSettings({
        log_retention_enabled: next.requestLogEnabled,
        log_retention_days: next.requestLogDays,
        maintenance_history_retention_enabled: next.maintenanceHistoryEnabled,
        maintenance_history_retention_days: next.maintenanceHistoryDays,
      });
      setRetentionValues(next);
      Toast.success('已保存');
      refresh();
    } catch (err) {
      Toast.error(`保存失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  return (
    <div className="page-scroll">
      <Title heading={4} style={{ marginBottom: 20 }}>设置</Title>

      {/* Maintenance schedules */}
      <Card style={{ marginBottom: 16 }}>
        <Title heading={5} style={{ marginBottom: 12 }}>后台维护调度</Title>
        {scheduleValues.map(item => (
          <div key={item.key} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '10px 0', borderBottom: '1px solid var(--semi-color-border)' }}>
            <Text style={{ minWidth: 100, flex: 1 }}>{item.label}</Text>
            <Switch checked={item.enabled} onChange={(v) => updateScheduleValue(item.key, { enabled: v })} />
            {item.enabled && item.interval_seconds !== null && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                <InputNumber
                  value={item.interval_seconds}
                  min={60}
                  style={{ width: 100 }}
                  size="small"
                  onChange={v => updateScheduleValue(item.key, { interval_seconds: v })}
                />
                <Text size="small">秒</Text>
              </div>
            )}
            {!item.enabled && <Text type="secondary" size="small">已关闭</Text>}
          </div>
        ))}
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 12 }}>
          <Button type="primary" onClick={saveScheduleSettings}>保存</Button>
        </div>
      </Card>

      <Card style={{ marginBottom: 16 }}>
        <Title heading={5} style={{ marginBottom: 12 }}>扫描与评估</Title>
        {[
          {
            label: '节点观测并发',
            value: scanValues.nodeObservationConcurrency,
            onChange: (value: number | string) => setScanValues(prev => ({ ...prev, nodeObservationConcurrency: Number(value) || 1 })),
          },
          {
            label: '策略任务并发',
            value: scanValues.profileEvaluationConcurrency,
            onChange: (value: number | string) => setScanValues(prev => ({ ...prev, profileEvaluationConcurrency: Number(value) || 1 })),
          },
          {
            label: '候选探测并发',
            value: scanValues.globalConcurrency,
            onChange: (value: number | string) => setScanValues(prev => ({ ...prev, globalConcurrency: Number(value) || 1 })),
          },
          {
            label: '连接超时 (秒)',
            value: scanValues.connectTimeoutSeconds,
            onChange: (value: number | string) => setScanValues(prev => ({ ...prev, connectTimeoutSeconds: Number(value) || 1 })),
          },
          {
            label: '整体探测超时 (秒)',
            value: scanValues.probeTimeoutSeconds,
            onChange: (value: number | string) => setScanValues(prev => ({ ...prev, probeTimeoutSeconds: Number(value) || 1 })),
          },
        ].map(item => (
          <div key={item.label} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '10px 0', borderBottom: '1px solid var(--semi-color-border)' }}>
            <Text type="secondary" style={{ minWidth: 130, flex: 1, whiteSpace: 'nowrap' }}>{item.label}</Text>
            <InputNumber value={item.value} min={1} style={{ width: 100 }} size="small" onChange={item.onChange} />
          </div>
        ))}
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 12 }}>
          <Button type="primary" onClick={saveScanSettings}>保存</Button>
        </div>
      </Card>

      {/* Retention cleanup */}
      <Card style={{ marginBottom: 16 }}>
        <Title heading={5} style={{ marginBottom: 12 }}>保留清理</Title>
        {[
          {
            label: '请求日志清理',
            enabled: retentionValues.requestLogEnabled,
            days: retentionValues.requestLogDays,
            setEnabled: (enabled: boolean) => setRetentionValues(prev => ({ ...prev, requestLogEnabled: enabled })),
            setDays: (value: number | string) => setRetentionValues(prev => ({ ...prev, requestLogDays: value })),
          },
          {
            label: '维护历史清理',
            enabled: retentionValues.maintenanceHistoryEnabled,
            days: retentionValues.maintenanceHistoryDays,
            setEnabled: (enabled: boolean) => setRetentionValues(prev => ({ ...prev, maintenanceHistoryEnabled: enabled })),
            setDays: (value: number | string) => setRetentionValues(prev => ({ ...prev, maintenanceHistoryDays: value })),
          },
        ].map(item => (
          <div key={item.label} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '10px 0', borderBottom: '1px solid var(--semi-color-border)' }}>
            <Text style={{ minWidth: 120, flex: 1 }}>{item.label}</Text>
            <Switch checked={item.enabled} onChange={item.setEnabled} />
            <Text style={{ width: 48 }}>{item.enabled ? '已启用' : '已关闭'}</Text>
            {item.enabled && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                <InputNumber value={item.days} min={1} style={{ width: 80 }} size="small"
                  onChange={item.setDays} />
                <Text size="small">天</Text>
              </div>
            )}
          </div>
        ))}
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 12 }}>
          <Button type="primary" onClick={saveRetentionSettings}>保存</Button>
        </div>
      </Card>

      {/* Switching tolerance */}
      <Card style={{ marginBottom: 16 }}>
        <Title heading={5} style={{ marginBottom: 12 }}>切换容忍度默认值</Title>
        <Descriptions row size="small" data={[
          { key: '相对提升', value: `${(settings.switching_tolerance.relative_improvement_threshold * 100).toFixed(0)}%` },
          { key: '绝对延迟提升', value: `${settings.switching_tolerance.absolute_latency_improvement_ms} ms` },
        ]} />
        <Button style={{ marginTop: 12 }} onClick={() => setToleranceVisible(true)}>编辑</Button>
      </Card>

      {/* GeoIP */}
      <Card style={{ marginBottom: 16 }}>
        <Title heading={5} style={{ marginBottom: 12 }}>GeoIP 数据库</Title>
        <Descriptions row size="small" data={[
          { key: '来源', value: settings.geoip.source },
          { key: '状态', value: settings.geoip.loaded ? <Tag color="green">已加载</Tag> : <Tag color="red">未加载</Tag> },
          { key: '上次更新', value: formatRelativeTime(settings.geoip.updated_at) },
          { key: '下次更新', value: formatRelativeTime(settings.geoip.next_update_at) },
        ]} />
        <Button style={{ marginTop: 12 }} onClick={async () => {
          try {
            await client.post('/api/geoip');
            Toast.success('已触发 GeoIP 更新');
            refresh();
          } catch (err) { Toast.error(`更新失败: ${err instanceof Error ? err.message : '未知错误'}`); }
        }}>立即更新</Button>
      </Card>

      {/* Proxy access address */}
      <Card style={{ marginBottom: 16 }}>
        <Title heading={5} style={{ marginBottom: 12 }}>代理访问地址</Title>
        <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>用于生成完整代理 URL 的外部可达地址</Text>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <Input defaultValue={settings.public_proxy_endpoint} placeholder="host:port" style={{ width: 300 }}
            onChange={setEndpoint} />
          <Button type="primary" onClick={async () => {
            try {
              await client.updateSettings({ public_proxy_endpoint: endpoint.trim() });
              Toast.success('已保存');
              refresh();
            } catch (err) { Toast.error(`保存失败: ${err instanceof Error ? err.message : '未知错误'}`); }
          }}>保存</Button>
        </div>
      </Card>

      {/* Admin password */}
      <Card>
        <Title heading={5} style={{ marginBottom: 12 }}>管理员</Title>
        <Button onClick={() => setPasswordVisible(true)}>修改管理员密码</Button>
      </Card>

      {/* Tolerance edit panel */}
      <AdaptivePanel title="编辑切换容忍度" visible={toleranceVisible} onClose={() => setToleranceVisible(false)}>
        <div style={{ display: 'grid', gap: 16 }}>
          <Text type="secondary" size="small">任一项设为 0 时不使用该门槛；两项都为 0 时，候选路径只要更快就允许切换。</Text>
          <div>
            <Text type="secondary">相对提升 (%)</Text>
            <InputNumber value={toleranceValues.relative}
              min={0} max={100} style={{ width: '100%', marginTop: 4 }}
              onChange={v => setToleranceValues(prev => ({ ...prev, relative: Number(v) || 0 }))} />
          </div>
          <div>
            <Text type="secondary">绝对延迟提升 (ms)</Text>
            <InputNumber value={toleranceValues.absolute}
              min={0} style={{ width: '100%', marginTop: 4 }}
              onChange={v => setToleranceValues(prev => ({ ...prev, absolute: Number(v) || 0 }))} />
          </div>
          <Button type="primary" onClick={async () => {
            try {
              await client.patch('/api/system/settings', {
                switching_tolerance: {
                  relative_improvement_threshold: toleranceValues.relative / 100,
                  absolute_latency_improvement_ms: toleranceValues.absolute,
                }
              });
              Toast.success('已保存');
              setToleranceVisible(false);
              refresh();
            } catch (err) { Toast.error(`保存失败: ${err instanceof Error ? err.message : '未知错误'}`); }
          }}>保存</Button>
        </div>
      </AdaptivePanel>

      {/* Password change panel */}
      <AdaptivePanel title="修改管理员密码" visible={passwordVisible} onClose={() => setPasswordVisible(false)}>
        <div style={{ display: 'grid', gap: 16 }}>
          <div>
            <Text type="secondary">当前密码</Text>
            <Input type="password" value={currentPassword} onChange={setCurrentPassword} style={{ marginTop: 4 }} />
          </div>
          <div>
            <Text type="secondary">新密码 (至少 12 个字符)</Text>
            <Input type="password" value={newPassword} onChange={setNewPassword} style={{ marginTop: 4 }} />
          </div>
          <div>
            <Text type="secondary">确认新密码</Text>
            <Input type="password" value={confirmPassword} onChange={setConfirmPassword} style={{ marginTop: 4 }} />
          </div>
          <Button type="primary" onClick={async () => {
            if (newPassword.length < 12) {
              Toast.error('新密码至少 12 个字符');
              return;
            }
            if (newPassword !== confirmPassword) {
              Toast.error('两次密码不一致');
              return;
            }
            try {
              await client.changePassword(currentPassword, newPassword);
              Toast.success('密码已修改，请重新登录');
              setPasswordVisible(false);
              client.clearToken();
              window.location.reload();
            } catch (err) {
              Toast.error(`修改失败: ${err instanceof Error ? err.message : '未知错误'}`);
            }
          }}>确认修改</Button>
        </div>
      </AdaptivePanel>
    </div>
  );
}
