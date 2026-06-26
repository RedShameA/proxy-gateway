import { useState, useCallback } from 'react';
import { Button, Typography, Tag, Table, Toast, Select, Input, Card, Pagination, Spin } from '@douyinfe/semi-ui';
import { IconPlus } from '@douyinfe/semi-icons';
import { AdaptivePanel } from '../components/AdaptivePanel';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { useData } from '../hooks/useData';
import { subscriptionSourceLabel, subscriptionStateLabel, subscriptionStateColor, formatRelativeTime } from '../display';
import type { ApiClient } from '../api';
import type { SubscriptionSummary, SubscriptionDetail, EvaluationSchedulePolicy } from '../types';

const { Title, Text } = Typography;

type SubscriptionFormValues = {
  name: string;
  source_type: 'remote' | 'local';
  url: string;
  content: string;
  refresh_mode: EvaluationSchedulePolicy['mode'];
  custom_interval: number;
};

const initialFormValues: SubscriptionFormValues = {
  name: '',
  source_type: 'remote',
  url: '',
  content: '',
  refresh_mode: 'inherit',
  custom_interval: 3600,
};

export function SubscriptionsPage({ client }: { client: ApiClient }) {
  const isMobile = useMediaQuery('(max-width: 767px)');
  const [detailVisible, setDetailVisible] = useState(false);
  const [createVisible, setCreateVisible] = useState(false);
  const [selected, setSelected] = useState<SubscriptionDetail | null>(null);
  const [page, setPage] = useState(1);
  const [submitting, setSubmitting] = useState(false);
  const [formValues, setFormValues] = useState<SubscriptionFormValues>(initialFormValues);
  const pageSize = isMobile ? 5 : 10;

  const fetcher = useCallback(() => client.getSubscriptions(page, pageSize), [client, page, pageSize]);
  const { data, loading, refresh } = useData(fetcher);

  const subs = data?.items || [];
  const total = data?.total || 0;

  const openDetail = async (s: SubscriptionSummary) => {
    try {
      const detail = await client.getSubscription(s.id);
      setSelected(detail);
    } catch {
      setSelected({ ...s, url: null, content: null, skipped_entries: [], created_at: 0, updated_at: 0 } as SubscriptionDetail);
    }
    setDetailVisible(true);
  };

  const handleRefresh = async (id: string) => {
    try {
      await client.refreshSubscription(id);
      Toast.success('刷新成功');
      setDetailVisible(false);
      refresh();
    } catch (err) {
      Toast.error(`刷新失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await client.deleteSubscription(id);
      Toast.success('删除成功');
      setDetailVisible(false);
      refresh();
    } catch (err) {
      Toast.error(`删除失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const handleCreate = async () => {
    if (!formValues.name.trim()) {
      Toast.error('订阅名称不能为空');
      return;
    }
    if (formValues.source_type === 'remote' && !formValues.url.trim()) {
      Toast.error('远程订阅 URL 不能为空');
      return;
    }
    if (formValues.source_type === 'local' && !formValues.content.trim()) {
      Toast.error('本地订阅内容不能为空');
      return;
    }
    try {
      setSubmitting(true);
      const refreshMode = formValues.refresh_mode || 'inherit';
      await client.createSubscription({
        name: formValues.name,
        source_type: formValues.source_type,
        url: formValues.url,
        content: formValues.content,
        auto_refresh_enabled: refreshMode !== 'disabled',
        auto_refresh_interval_seconds: refreshMode === 'custom' ? formValues.custom_interval : 0,
      });
      Toast.success('订阅已添加');
      setCreateVisible(false);
      setFormValues(initialFormValues);
      refresh();
    } catch (err) {
      Toast.error(`创建失败: ${err instanceof Error ? err.message : '未知错误'}`);
    } finally {
      setSubmitting(false);
    }
  };

  const columns = [
    { title: '名称', dataIndex: 'name', width: 200, render: (v: string, r: SubscriptionSummary) => <a onClick={() => openDetail(r)}>{v}</a> },
    { title: '类型', dataIndex: 'source_type', width: 100, render: (v: string) => <Tag>{subscriptionSourceLabel(v)}</Tag> },
    { title: '状态', dataIndex: 'state', width: 80, render: (v: string) => <Tag color={subscriptionStateColor(v) as any}>{subscriptionStateLabel(v)}</Tag> },
    { title: '节点数', dataIndex: 'node_count', width: 80 },
    { title: '跳过', dataIndex: 'skipped_count', width: 60 },
    { title: '上次刷新', dataIndex: 'last_refresh_at', width: 140, render: (v: number | null) => formatRelativeTime(v) },
  ];

  const policyLabel = (p: EvaluationSchedulePolicy | undefined | null) => {
    if (!p) return '跟随全局';
    if (p.mode === 'disabled') return '已关闭';
    if (p.mode === 'custom' && p.interval_seconds) return `自定义 ${p.interval_seconds}s`;
    return '跟随全局';
  };

  if (loading) return <div className="page-loading"><Spin size="large" /></div>;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Title heading={4}>订阅管理</Title>
        <Button icon={<IconPlus />} type="primary" onClick={() => setCreateVisible(true)}>添加订阅</Button>
      </div>

      {!isMobile && <Table dataSource={subs} columns={columns} rowKey="id" size="default" pagination={false} />}

      {isMobile && (
        <div style={{ display: 'grid', gap: 8 }}>
          {subs.map(sub => (
            <Card key={sub.id} style={{ cursor: 'pointer', overflow: 'hidden' }}>
              <div onClick={() => openDetail(sub)}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                  <Text strong style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0, flex: 1 }}>{sub.name}</Text>
                  <Tag color={subscriptionStateColor(sub.state) as any} size="small" style={{ flexShrink: 0 }}>{subscriptionStateLabel(sub.state)}</Tag>
                </div>
                <Text size="small" type="secondary" style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{subscriptionSourceLabel(sub.source_type)} · 节点 {sub.node_count} · 跳过 {sub.skipped_count}</Text>
                <Text size="small" type="secondary" style={{ display: 'block', marginTop: 4, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>上次刷新: {formatRelativeTime(sub.last_refresh_at)}</Text>
              </div>
            </Card>
          ))}
        </div>
      )}

      {total > pageSize && (
        <Pagination total={total} pageSize={pageSize} currentPage={page} onChange={setPage} style={{ justifyContent: 'center', marginTop: 8 }} />
      )}

      <AdaptivePanel title="订阅详情" visible={detailVisible} onClose={() => setDetailVisible(false)}>
        {selected && (
          <div style={{ display: 'grid', gap: 16 }}>
            <div><Text type="secondary">名称</Text><div><Text strong>{selected.name}</Text></div></div>
            <div><Text type="secondary">类型</Text><div><Tag>{subscriptionSourceLabel(selected.source_type)}</Tag></div></div>
            <div><Text type="secondary">状态</Text><div><Tag color={subscriptionStateColor(selected.state) as any}>{subscriptionStateLabel(selected.state)}</Tag></div></div>
            {selected.url && <div><Text type="secondary">URL</Text><div><Text style={{ wordBreak: 'break-all' }}>{selected.url}</Text></div></div>}
            {selected.content && <div><Text type="secondary">本地内容</Text><div><Text style={{ wordBreak: 'break-all', fontFamily: 'monospace', fontSize: 12 }}>{selected.content}</Text></div></div>}
            <div><Text type="secondary">刷新策略</Text><div>{policyLabel(selected.refresh_policy)}</div></div>
            <div><Text type="secondary">节点数 / 跳过数</Text><div>{selected.node_count} / {selected.skipped_count}</div></div>
            {((selected as any).skipped_entry_summary || selected.skipped_entries || []).length > 0 && (
              <div>
                <Text type="secondary">跳过条目</Text>
                {((selected as any).skipped_entry_summary || selected.skipped_entries || []).map((e: any, i: number) => <div key={i} style={{ padding: '4px 0' }}><Tag size="small">{e.message}</Tag> x{e.count}</div>)}
              </div>
            )}
            <div style={{ display: 'flex', gap: 8 }}>
              <Button onClick={() => handleRefresh(selected.id)}>刷新</Button>
              <Button type="danger" onClick={() => handleDelete(selected.id)}>删除</Button>
            </div>
          </div>
        )}
      </AdaptivePanel>

      <AdaptivePanel title="添加订阅" visible={createVisible} onClose={() => setCreateVisible(false)}>
        <div style={{ display: 'grid', gap: 16 }}>
          <label>
            <Text type="secondary">名称</Text>
            <Input placeholder="订阅名称" value={formValues.name} onChange={(v) => setFormValues(prev => ({ ...prev, name: v }))} />
          </label>
          <label>
            <Text type="secondary">类型</Text>
            <Select value={formValues.source_type} onChange={(v) => setFormValues(prev => ({ ...prev, source_type: String(v) as SubscriptionFormValues['source_type'] }))} style={{ width: '100%' }}>
              <Select.Option value="remote">远程订阅</Select.Option>
              <Select.Option value="local">本地订阅</Select.Option>
            </Select>
          </label>
          {formValues.source_type === 'remote' && (
            <label>
              <Text type="secondary">URL</Text>
              <Input placeholder="https://..." value={formValues.url} onChange={(v) => setFormValues(prev => ({ ...prev, url: v }))} />
            </label>
          )}
          {formValues.source_type === 'local' && (
            <label>
              <Text type="secondary">本地内容</Text>
              <Input placeholder="本地订阅内容" value={formValues.content} onChange={(v) => setFormValues(prev => ({ ...prev, content: v }))} />
            </label>
          )}
          <label>
            <Text type="secondary">刷新策略</Text>
            <Select value={formValues.refresh_mode} onChange={(v) => setFormValues(prev => ({ ...prev, refresh_mode: String(v) as SubscriptionFormValues['refresh_mode'] }))} style={{ width: '100%' }}>
              <Select.Option value="inherit">跟随全局</Select.Option>
              <Select.Option value="custom">自定义</Select.Option>
              <Select.Option value="disabled">关闭</Select.Option>
            </Select>
          </label>
          <div style={{ marginTop: 16 }}>
            <Button type="primary" loading={submitting} onClick={handleCreate}>保存</Button>
          </div>
        </div>
      </AdaptivePanel>
    </div>
  );
}
