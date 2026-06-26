import { useState, useCallback } from 'react';
import { Typography, Tag, Table, Select, Input, Button, Descriptions, Card, Pagination, Spin } from '@douyinfe/semi-ui';
import { AdaptivePanel } from '../components/AdaptivePanel';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { useData } from '../hooks/useData';
import { logResultLabel, logResultColor, failureStageLabel, formatTime, formatRelativeTime, formatBytes, pathSummaryText } from '../display';
import type { ApiClient } from '../api';
import type { RequestLogEntry } from '../types';

const { Title, Text } = Typography;

export function LogsPage({ client }: { client: ApiClient }) {
  const isMobile = useMediaQuery('(max-width: 767px)');
  const [detailVisible, setDetailVisible] = useState(false);
  const [selected, setSelected] = useState<RequestLogEntry | null>(null);
  const [filters, setFilters] = useState({ profileId: '', result: '', target: '' });
  const [page, setPage] = useState(1);
  const pageSize = isMobile ? 5 : 10;

  const fetcher = useCallback(() => client.getRequestLogs({
    page, page_size: pageSize,
    access_profile_id: filters.profileId,
    result: filters.result,
    target: filters.target,
  }), [client, page, pageSize, filters]);
  const { data, loading } = useData(fetcher);

  const logs = data?.items || [];
  const total = data?.total || 0;

  const { data: profiles } = useData(useCallback(() => client.getAccessProfiles(1, 100), [client]));

  const columns = [
    { title: '时间', dataIndex: 'occurred_at', width: 120, render: (v: number) => formatRelativeTime(v) },
    { title: '访问策略', width: 120, render: (_: any, r: RequestLogEntry) => <span>{r.access_profile.name}</span> },
    { title: '凭证', width: 100, render: (_: any, r: RequestLogEntry) => <span>{r.proxy_credential.remark}</span> },
    { title: '目标', dataIndex: 'target', width: 200, render: (v: string) => <Text ellipsis={{ showTooltip: true }}>{v}</Text> },
    { title: '节点', width: 150, render: (_: any, r: RequestLogEntry) => r.proxy_path ? <span>{r.proxy_path.path_type === 'single' ? r.proxy_path.node.name : `${r.proxy_path.front_node.name}→${r.proxy_path.exit_node.name}`}</span> : '-' },
    { title: '结果', dataIndex: 'result', width: 80, render: (v: string) => <Tag color={logResultColor(v) as any}>{logResultLabel(v)}</Tag> },
    { title: '耗时', width: 80, render: (_: any, r: RequestLogEntry) => r.result === 'running' ? '-' : `${r.duration_ms}ms` },
  ];

  if (loading) return <div className="page-loading"><Spin size="large" /></div>;

  return (
    <div>
      <Title heading={4} style={{ marginBottom: 16 }}>请求日志</Title>

      <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
        <Select placeholder="访问策略" value={filters.profileId} onChange={v => { setFilters(f => ({ ...f, profileId: v as string })); setPage(1); }} style={{ width: 160 }}>
          {(profiles?.items || []).map(p => <Select.Option key={p.id} value={p.id}>{p.name}</Select.Option>)}
        </Select>
        <Select placeholder="结果" value={filters.result} onChange={v => { setFilters(f => ({ ...f, result: v as string })); setPage(1); }} style={{ width: 100 }}>
          <Select.Option value="running">进行中</Select.Option>
          <Select.Option value="success">成功</Select.Option>
          <Select.Option value="failure">失败</Select.Option>
        </Select>
        <Input placeholder="搜索目标" value={filters.target} onChange={v => { setFilters(f => ({ ...f, target: v })); setPage(1); }} style={{ width: 200 }} />
        <Button onClick={() => { setFilters({ profileId: '', result: '', target: '' }); setPage(1); }}>重置</Button>
      </div>

      {!isMobile && (
        <Table dataSource={logs} columns={columns} rowKey="id" size="default"
          pagination={false}
          onRow={(r: RequestLogEntry | undefined) => ({ onClick: () => { if (r) { setSelected(r); setDetailVisible(true); } } })}
          style={{ cursor: 'pointer' }} />
      )}

      {isMobile && (
        <div style={{ display: 'grid', gap: 8 }}>
          {logs.map(log => (
            <Card key={log.id} style={{ cursor: 'pointer', overflow: 'hidden' }}>
              <div onClick={() => { setSelected(log); setDetailVisible(true); }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                  <Text strong style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0, flex: 1 }}>{log.target}</Text>
                  <Tag color={logResultColor(log.result) as any} size="small" style={{ flexShrink: 0 }}>{logResultLabel(log.result)}</Tag>
                </div>
                <Text size="small" type="secondary" style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{log.access_profile.name} · {log.proxy_credential.remark}</Text>
                <div style={{ display: 'flex', gap: 12, marginTop: 4, overflow: 'hidden' }}>
                  <Text size="small" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0, flex: 1 }}>{log.proxy_path ? (log.proxy_path.path_type === 'single' ? log.proxy_path.node.name : `${log.proxy_path.front_node.name}→${log.proxy_path.exit_node.name}`) : '-'}</Text>
                  <Text size="small" style={{ flexShrink: 0 }}>{log.result === 'running' ? '-' : `${log.duration_ms}ms`}</Text>
                  <Text size="small" type="secondary" style={{ flexShrink: 0 }}>{formatRelativeTime(log.occurred_at)}</Text>
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}

      {total > pageSize && (
        <Pagination total={total} pageSize={pageSize} currentPage={page} onChange={setPage} style={{ justifyContent: 'center', marginTop: 8 }} />
      )}

      <AdaptivePanel title="请求详情" visible={detailVisible} onClose={() => setDetailVisible(false)}>
        {selected && (
          <div style={{ display: 'grid', gap: 16 }}>
            <Descriptions row size="small" data={[
              { key: '时间', value: formatTime(selected.occurred_at) },
              { key: '访问策略', value: selected.access_profile.name },
              { key: '策略标识', value: selected.access_profile.profile_identifier },
              { key: '凭证', value: selected.proxy_credential.remark },
              { key: '目标', value: selected.target },
              { key: '结果', value: <Tag color={logResultColor(selected.result) as any}>{logResultLabel(selected.result)}</Tag> },
              { key: '失败阶段', value: selected.failure_stage ? failureStageLabel(selected.failure_stage) : '-' },
              { key: '错误信息', value: selected.error || '-' },
              { key: '耗时', value: selected.result === 'running' ? '-' : `${selected.duration_ms}ms` },
              { key: 'HTTP 状态', value: selected.http_status ? String(selected.http_status) : '-' },
              { key: '入站流量', value: formatBytes(selected.ingress_bytes) },
              { key: '出站流量', value: formatBytes(selected.egress_bytes) },
            ]} />
            {selected.proxy_path && (
              <div>
                <Text type="secondary">代理路径</Text>
                <div>{pathSummaryText(selected.proxy_path)}</div>
              </div>
            )}
          </div>
        )}
      </AdaptivePanel>
    </div>
  );
}
