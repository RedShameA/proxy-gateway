import { useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button, Typography, Tag, Card, Collapse, Descriptions, Toast, Modal, Pagination, Spin, Input } from '@douyinfe/semi-ui';
import { IconArrowLeft, IconPlus } from '@douyinfe/semi-icons';
import { AdaptivePanel } from '../components/AdaptivePanel';
import { AccessProfileForm } from '../components/AccessProfileForm';
import { CopyableUrl } from '../components/CopyableUrl';
import { MaintenanceRunDrawer, MaintenanceRunRow } from '../components/MaintenanceRunView';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { useData } from '../hooks/useData';
import {
  profileTypeLabel, profileStateLabel, profileStateColor, pathSummaryText, latencyText,
  latencyKindLabel, formatAbsoluteTime, formatTime,
  chainEvaluationModeLabel,
  egressCountryModeLabel, sourceModeLabel, switchReasonLabel, profileIssueSummary,
} from '../display';
import type { ApiClient } from '../api';
import type { AccessProfileDetail, AccessProfileWriteRequest, MaintenanceRunSummary, ProxyCredentialSummary } from '../types';

const { Title, Text } = Typography;
const proxyCredentialPasswordPattern = /^[A-Za-z0-9_-]+$/;
const defaultCredentialRemark = '凭证';

function generateDefaultPassword(): string {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let result = '';
  for (let i = 0; i < 8; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}

export function AccessProfileDetailPage({ client }: { client: ApiClient }) {
  const isMobile = useMediaQuery('(max-width: 767px)');
  const { id } = useParams<{ id: string }>();
  const nav = useNavigate();
  const [editVisible, setEditVisible] = useState(false);
  const [newCredVisible, setNewCredVisible] = useState(false);
  const [newPassword, setNewPassword] = useState(generateDefaultPassword());
  const [newRemark, setNewRemark] = useState(defaultCredentialRemark);
  const [credPage, setCredPage] = useState(1);
  const [submitting, setSubmitting] = useState(false);
  const [selectedRun, setSelectedRun] = useState<MaintenanceRunSummary | null>(null);

  const fetcher = useCallback(() => client.getAccessProfile(id!), [client, id]);
  const { data: profile, loading, refresh } = useData(fetcher);
  const { data: countries } = useData(useCallback(() => client.getEgressCountries(), [client]));
  const { data: nodes } = useData(useCallback(() => client.getNodes(1, 100), [client]));
  const { data: subscriptions } = useData(useCallback(() => client.getSubscriptions(1, 100), [client]));

  if (loading) return <div className="page-loading"><Spin size="large" /></div>;
  if (!profile) {
    return <div><Title heading={4}>策略不存在</Title><Button onClick={() => nav('/access-profiles')}>返回列表</Button></div>;
  }

  const hasValidBest = profile.best_observed_valid && profile.best_observed_path &&
    JSON.stringify(profile.best_observed_path) !== JSON.stringify(profile.current_path);

  const credPageSize = isMobile ? 3 : 10;
  const credPaged = profile.proxy_credentials.slice((credPage - 1) * credPageSize, credPage * credPageSize);
  const evaluationDetails = profile.last_evaluation_details || {};
  const detailString = (key: string) => typeof evaluationDetails[key] === 'string' ? evaluationDetails[key] as string : '';
  const detailNumber = (key: string) => typeof evaluationDetails[key] === 'number' ? evaluationDetails[key] as number : null;
  const nodeLabel = (nodeID: string) => {
    if (!nodeID) return '';
    const node = (nodes?.items || []).find(n => n.id === nodeID);
    return node ? node.name : nodeID;
  };
  const detailPath = (nodeKey: string, exitKey?: string) => {
    const nodeID = detailString(nodeKey);
    const exitID = exitKey ? detailString(exitKey) : '';
    if (!nodeID && !exitID) return '-';
    if (exitKey) return `${nodeLabel(nodeID) || '-'} -> ${nodeLabel(exitID) || '-'}`;
    return nodeLabel(nodeID) || '-';
  };
  const countText = (value: number | null | undefined) => value === null || value === undefined ? '-' : String(value);
  const candidateCount = detailNumber('candidate_count') ??
    (profile.type === 'chain' ? profile.candidate_stats.path_combinations : profile.candidate_stats.total);
  const switchReason = switchReasonLabel(detailString('switch_reason') || profile.switch_reason) || profile.last_error || '-';
  const statusReason = profileIssueSummary(profile) || switchReason;
  const evaluationSummary = [
    { key: '最近评估时间', value: formatTime(profile.last_evaluated_at) },
    { key: '评估模式', value: profile.type === 'chain' && profile.chain_evaluation_mode ? chainEvaluationModeLabel(profile.chain_evaluation_mode) : profileTypeLabel(profile.type) },
    { key: '节点粘滞', value: profile.type === 'fastest' || profile.type === 'chain' ? (profile.node_sticky_enabled ? '开启' : '关闭') : '不适用' },
    { key: 'Test URL', value: profile.test_url || '-' },
    { key: '候选数', value: countText(candidateCount) },
    { key: '失败数', value: countText(detailNumber('failure_count')) },
    { key: '最佳路径', value: profile.type === 'chain' ? detailPath('best_node_id', 'best_exit_node_id') : detailPath('best_node_id') },
    { key: '选中路径', value: profile.type === 'chain' ? detailPath('selected_node_id', 'selected_exit_node_id') : detailPath('selected_node_id') },
    { key: '状态原因', value: statusReason },
    { key: '切换原因', value: switchReason },
    { key: '原始错误', value: profile.last_error || '-' },
  ];
  const candidateSummary = [
    { key: '总候选', value: countText(profile.candidate_stats.total) },
    { key: '可用候选', value: countText(profile.candidate_stats.usable) },
    { key: '未知出口国家', value: countText(profile.candidate_stats.unknown_egress_country) },
    ...(profile.type === 'chain' ? [
      { key: '前置候选', value: countText(profile.candidate_stats.front_candidates) },
      { key: '出口节点', value: countText(profile.candidate_stats.exit_nodes) },
      { key: '路径组合', value: countText(profile.candidate_stats.path_combinations) },
    ] : []),
  ];

  const handleToggleCred = async (cred: ProxyCredentialSummary) => {
    try {
      await client.updateProxyCredential(id!, cred.id, { enabled: !cred.enabled });
      Toast.success(cred.enabled ? '已禁用' : '已启用');
      refresh();
    } catch (err) {
      Toast.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const handleDeleteCred = async (cred: ProxyCredentialSummary) => {
    try {
      await client.deleteProxyCredential(id!, cred.id);
      Toast.success('已删除');
      refresh();
    } catch (err) {
      Toast.error(`删除失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const handleCreateCred = async () => {
    if (!newRemark.trim()) {
      Toast.error('代理凭证备注不能为空');
      return;
    }
    if (newPassword.length < 6 || newPassword.length > 32) {
      Toast.error('代理凭证密码长度需为 6-32 个字符');
      return;
    }
    if (!proxyCredentialPasswordPattern.test(newPassword)) {
      Toast.error('代理凭证密码只能包含字母、数字、连字符和下划线');
      return;
    }
    try {
      setSubmitting(true);
      await client.createProxyCredential(id!, { remark: newRemark, password: newPassword });
      Toast.success('凭证已创建');
      setNewCredVisible(false);
      setNewRemark(defaultCredentialRemark);
      setNewPassword(generateDefaultPassword());
      refresh();
    } catch (err) {
      Toast.error(`创建失败: ${err instanceof Error ? err.message : '未知错误'}`);
    } finally {
      setSubmitting(false);
    }
  };

  const handleSaveProfile = async (payload: AccessProfileWriteRequest) => {
    try {
      setSubmitting(true);
      await client.updateAccessProfile(id!, payload);
      Toast.success('保存成功');
      setEditVisible(false);
      refresh();
    } catch (err) {
      Toast.error(`保存失败: ${err instanceof Error ? err.message : '未知错误'}`);
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeleteProfile = async () => {
    try {
      await client.deleteAccessProfile(id!);
      Toast.success('已删除');
      nav('/access-profiles');
    } catch (err) {
      Toast.error(`删除失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const handleEvaluate = async () => {
    try {
      await client.evaluateAccessProfile(id!);
      Toast.success('已触发评估');
    } catch (err) {
      Toast.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const handleSwitchToBest = async () => {
    try {
      await client.switchToBestObserved(id!);
      Toast.success('已切换到当前观测最快路径');
      refresh();
    } catch (err) {
      Toast.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const renderCredentialCard = (cred: ProxyCredentialSummary) => (
    <Card key={cred.id} style={{ overflow: 'hidden' }} bodyStyle={{ padding: isMobile ? 12 : 16 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginBottom: 12, alignItems: 'flex-start' }}>
        <div style={{ minWidth: 0 }}>
          <Text strong ellipsis={{ showTooltip: true }}>{cred.remark}</Text>
          <Text size="small" type="secondary" style={{ display: 'block', marginTop: 2 }}>
            最后使用: {formatAbsoluteTime(cred.last_used_at)}
          </Text>
        </div>
        <Tag color={cred.enabled ? 'green' : 'grey'} size="small" style={{ flexShrink: 0 }}>{cred.enabled ? '启用' : '禁用'}</Tag>
      </div>
      <div style={{ display: 'grid', gap: 8, minWidth: 0 }}>
        <CopyableUrl url={cred.http_proxy_url} label="HTTP" />
        <CopyableUrl url={cred.https_proxy_url} label="HTTPS" />
        <CopyableUrl url={cred.socks5_proxy_url} label="SOCKS5" />
      </div>
      <div style={{ display: 'flex', gap: 8, marginTop: 12, justifyContent: 'flex-end', flexWrap: 'wrap' }}>
        <Button size="small" onClick={() => handleToggleCred(cred)}>{cred.enabled ? '禁用' : '启用'}</Button>
        <Button size="small" type="danger" onClick={() => { Modal.confirm({ title: '确认删除', content: '该凭证对应代理 URL 将立即失效，历史日志保留。', onOk: () => handleDeleteCred(cred) }); }}>删除</Button>
      </div>
    </Card>
  );

  return (
    <div className="page-scroll">
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 20 }}>
        <div>
          <Button icon={<IconArrowLeft />} type="tertiary" theme="borderless" onClick={() => nav(-1)} style={{ marginLeft: -8, marginBottom: 4 }}>返回策略列表</Button>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 4, flexWrap: 'wrap' }}>
            <Title heading={4} style={{ margin: 0 }}>{profile.name}</Title>
            <Tag>{profileTypeLabel(profile.type)}</Tag>
            <Tag color={profileStateColor(profile.state) as any}>{profileStateLabel(profile.state)}</Tag>
          </div>
          <Text type="secondary">策略标识: {profile.profile_identifier}</Text>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button onClick={() => setEditVisible(true)}>编辑</Button>
          <Button type="danger" onClick={() => { Modal.confirm({ title: '确认删除', content: '当前代理 URL 将立即失效，历史日志保留。', onOk: handleDeleteProfile }); }}>删除</Button>
        </div>
      </div>

      {/* Current path card */}
      <Card style={{ marginBottom: 16 }}>
        <Title heading={5} style={{ marginBottom: 12 }}>当前路径</Title>
        {profile.current_path ? (
          <Descriptions row size="small" data={[
            { key: '路径', value: pathSummaryText(profile.current_path) },
            { key: '延迟', value: latencyText(profile.current_path.latency_ms) },
            { key: '延迟类型', value: latencyKindLabel(profile.current_path.latency_kind) },
            { key: '评估时间', value: formatTime(profile.current_path.evaluated_at) },
          ]} />
        ) : (
          <Text type="secondary">无可用路径</Text>
        )}
      </Card>

      {/* Best observed path */}
      {hasValidBest && (
        <Card style={{ marginBottom: 16 }}>
          <Title heading={5} style={{ marginBottom: 12 }}>当前观测最快路径</Title>
          <Descriptions row size="small" data={[
            { key: '路径', value: pathSummaryText(profile.best_observed_path) },
            { key: '相对提升', value: profile.best_observed_relative_improvement ? `${(profile.best_observed_relative_improvement * 100).toFixed(1)}%` : '-' },
            { key: '绝对提升', value: profile.best_observed_absolute_improvement_ms ? `${profile.best_observed_absolute_improvement_ms}ms` : '-' },
            { key: '未切换原因', value: switchReasonLabel(profile.no_switch_reason) || '-' },
          ]} />
          <Button type="primary" style={{ marginTop: 12 }} onClick={handleSwitchToBest}>
            切换到当前观测最快路径
          </Button>
        </Card>
      )}

      {/* Proxy credentials */}
      <Card style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
          <Title heading={5} style={{ margin: 0 }}>连接凭证</Title>
          <Button icon={<IconPlus />} type="primary" size="small" onClick={() => setNewCredVisible(true)}>新建凭证</Button>
        </div>
        <div style={{
          display: 'grid',
          gap: 12,
          gridTemplateColumns: isMobile ? '1fr' : 'repeat(auto-fill, minmax(420px, 1fr))',
        }}>
          {credPaged.map(renderCredentialCard)}
        </div>
        {profile.proxy_credentials.length > credPageSize && (
          <Pagination total={profile.proxy_credentials.length} pageSize={credPageSize} currentPage={credPage} onChange={setCredPage} style={{ justifyContent: 'center', marginTop: 8 }} />
        )}
      </Card>

      {/* Evaluation details */}
      <Collapse style={{ marginBottom: 16 }}>
        <Collapse.Panel header="评估详情" itemKey="evaluation">
          <div style={{ display: 'grid', gap: 12 }}>
            <Descriptions row size="small" data={evaluationSummary} />
            <Descriptions row size="small" data={candidateSummary} />
            <Button onClick={handleEvaluate}>立即评估</Button>
            <div>
              <Text strong>最近事件</Text>
              {profile.recent_events.length > 0 ? (
                <div style={{ marginTop: 8 }}>
                  {profile.recent_events.map(run => (
                    <MaintenanceRunRow key={run.id} run={run} onClick={setSelectedRun} />
                  ))}
                </div>
              ) : (
                <Text type="secondary" size="small" style={{ display: 'block', marginTop: 8 }}>暂无评估事件</Text>
              )}
            </div>
          </div>
        </Collapse.Panel>
      </Collapse>

      {/* Candidate filter */}
      <Collapse style={{ marginBottom: 16 }}>
        <Collapse.Panel header="候选筛选" itemKey="filter">
          <Descriptions row size="small" data={[
            { key: '来源模式', value: sourceModeLabel(profile.candidate_filter.source_mode) },
            { key: '协议', value: profile.candidate_filter.protocols.join(', ') || '全部' },
            { key: '名称包含', value: profile.candidate_filter.name_include || '-' },
            { key: '名称排除', value: profile.candidate_filter.name_exclude || '-' },
            { key: '出口国家模式', value: egressCountryModeLabel(profile.candidate_filter.egress_country_mode) },
            { key: '出口国家', value: profile.candidate_filter.egress_countries.join(', ') || '-' },
          ]} />
        </Collapse.Panel>
      </Collapse>

      {/* New credential panel */}
      <AdaptivePanel title="新建凭证" visible={newCredVisible} onClose={() => setNewCredVisible(false)}>
        <div style={{ display: 'grid', gap: 16 }}>
          <div>
            <Text type="secondary">备注</Text>
            <Input value={newRemark} onChange={setNewRemark} placeholder="凭证名称" style={{ marginTop: 4 }} />
          </div>
          <div>
            <Text type="secondary">密码</Text>
            <Input value={newPassword} onChange={setNewPassword} style={{ marginTop: 4 }} />
          </div>
          <Button type="primary" loading={submitting} onClick={handleCreateCred}>创建</Button>
        </div>
      </AdaptivePanel>

      {/* Edit panel */}
      <AdaptivePanel title="编辑策略" visible={editVisible} onClose={() => setEditVisible(false)} width={640}>
        {editVisible && (
          <AccessProfileForm
            initial={profile}
            nodes={nodes?.items || []}
            countries={countries || []}
            subscriptions={subscriptions?.items || []}
            submitting={submitting}
            submitLabel="保存"
            onSubmit={handleSaveProfile}
          />
        )}
      </AdaptivePanel>
      <MaintenanceRunDrawer run={selectedRun} visible={Boolean(selectedRun)} onClose={() => setSelectedRun(null)} />
    </div>
  );
}
