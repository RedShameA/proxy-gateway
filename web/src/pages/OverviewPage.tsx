import { useCallback, useState } from 'react';
import { Card, Row, Col, Tag, Typography, Spin, Space, Descriptions } from '@douyinfe/semi-ui';
import { useNavigate } from 'react-router-dom';
import { profileStateLabel, profileStateColor, formatRelativeTime, failureStageLabel } from '../display';
import { AccessProfileCard } from '../components/AccessProfileCard';
import { MaintenanceRunDrawer, MaintenanceRunRow } from '../components/MaintenanceRunView';
import { useData } from '../hooks/useData';
import { useMediaQuery } from '../hooks/useMediaQuery';
import type { ApiClient } from '../api';
import type { MaintenanceRunSummary, ProfileState, OverviewResponse, RequestLogEntry } from '../types';

const { Title, Text } = Typography;

export function OverviewPage({ client }: { client: ApiClient }) {
  const nav = useNavigate();
  const isMobile = useMediaQuery('(max-width: 767px)');
  const [selectedRun, setSelectedRun] = useState<MaintenanceRunSummary | null>(null);
  const fetcher = useCallback(() => client.getOverview(), [client]);
  const { data, loading, error } = useData<OverviewResponse>(fetcher);

  if (loading) return <div className="page-loading"><Spin size="large" /></div>;
  if (error) return <div>加载失败: {error}</div>;
  if (!data) return null;

  const sc = data.profile_state_counts;
  const stateOrder: ProfileState[] = ['ready', 'degraded', 'running', 'waiting_observation', 'pending', 'failed', 'no_candidate', 'invalid_config'];
  const activeStates = stateOrder.filter(s => sc[s] > 0);
  const pairedCardBodyHeight = 300;
  const pairedCardStyle = isMobile ? undefined : { height: '100%' };
  const profileCardBodyStyle = isMobile
    ? undefined
    : { height: pairedCardBodyHeight, overflowY: 'auto' as const };
  const profileListStyle = { display: 'grid', gap: 8 };
  const listEmptyStyle = { padding: '16px' };

  return (
    <div className="page-scroll">
      <Title heading={4} style={{ marginBottom: 20 }}>系统概览</Title>

      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        {[
          { label: '节点', value: data.resource_counts.nodes, sub: `${data.resource_counts.usable_nodes} 可用` },
          { label: '订阅', value: data.resource_counts.subscriptions },
          { label: '访问策略', value: data.resource_counts.access_profiles, sub: `${data.resource_counts.proxy_credentials} 凭证` },
          { label: '24h 请求', value: data.resource_counts.requests_24h, sub: `${data.resource_counts.failed_requests_24h} 失败`, danger: data.resource_counts.failed_requests_24h > 0 },
        ].map((m, i) => (
          <Col key={i} xs={24} sm={12} md={6}>
            <Card style={{ minHeight: 92 }}>
              <Text type="secondary" size="small">{m.label}</Text>
              <Title heading={3} style={{ margin: '6px 0 0' }}>{m.value}</Title>
              <Text size="small" type={m.danger ? 'danger' : 'secondary'} style={{ visibility: m.sub ? 'visible' : 'hidden' }}>{m.sub || '-'}</Text>
            </Card>
          </Col>
        ))}
      </Row>

      <Card title="访问策略状态分布" style={{ marginBottom: 16 }}>
        {activeStates.length === 0 ? (
          <Text type="secondary">暂无状态数据</Text>
        ) : (
          <Space wrap spacing={[8, 8]}>
            {activeStates.map(s => (
            <Tag key={s} color={profileStateColor(s) as any} size="large">
              {profileStateLabel(s)}: {sc[s]}
            </Tag>
          ))}
          </Space>
        )}
      </Card>

      <Row gutter={[12, 12]} className="overview-paired-row" style={{ marginBottom: 16 }}>
        <Col xs={24} md={12} className="overview-paired-col">
          <Card title="访问策略" className="overview-paired-card" style={pairedCardStyle} bodyStyle={profileCardBodyStyle}>
            {data.access_profiles.length === 0 ? (
              <Text type="secondary">暂无访问策略</Text>
            ) : (
              <div style={profileListStyle}>
                {data.access_profiles.map(profile => (
                  <AccessProfileCard
                    key={profile.id}
                    profile={profile}
                    onClick={() => nav(`/access-profiles/${profile.id}`)}
                  />
                ))}
              </div>
            )}
          </Card>
        </Col>

        <Col xs={24} md={12} className="overview-paired-col">
          <Card title="最近失败" className="overview-paired-card" style={pairedCardStyle} bodyStyle={profileCardBodyStyle}>
            {data.recent_failures.length === 0 ? (
              <div style={listEmptyStyle}><Text type="secondary">暂无失败记录</Text></div>
            ) : (
              <div style={profileListStyle}>
                {data.recent_failures.map(failure => (
                  <RecentFailureCard key={failure.id} failure={failure} />
                ))}
              </div>
            )}
          </Card>
        </Col>
      </Row>

      <Card title="维护历史" style={{ marginBottom: 16 }}>
        {data.maintenance_runs.length === 0 && <Text type="secondary">暂无维护历史</Text>}
        {data.maintenance_runs.map(run => (
          <MaintenanceRunRow key={run.id} run={run} onClick={setSelectedRun} />
        ))}
      </Card>

      <Card title="GeoIP 状态">
        <Descriptions row size="small" data={[
          { key: '来源', value: data.geoip_status.source || '-' },
          { key: '更新', value: formatRelativeTime(data.geoip_status.updated_at) },
          { key: '下次更新', value: formatRelativeTime(data.geoip_status.next_update_at) },
          ...(data.geoip_status.last_error ? [{ key: '错误', value: <Text type="danger">{data.geoip_status.last_error}</Text> }] : []),
        ]} />
      </Card>
      <MaintenanceRunDrawer run={selectedRun} visible={Boolean(selectedRun)} onClose={() => setSelectedRun(null)} />
    </div>
  );
}

function RecentFailureCard({ failure }: { failure: RequestLogEntry }) {
  const errorText = failure.error || '无错误信息';
  const credentialName = failure.proxy_credential.remark || '-';

  return (
    <Card className="overview-failure-card" style={{ overflow: 'hidden' }}>
      <div className="overview-failure-content">
        <div className="overview-failure-header">
          <Text strong ellipsis={{ showTooltip: true }} className="overview-failure-target">
            {failure.target}
          </Text>
          <Text size="small" type="secondary" className="overview-failure-time">
            {formatRelativeTime(failure.occurred_at)}
          </Text>
        </div>
        <div className="overview-failure-error-row">
          <Tag color="red" size="small" className="overview-failure-stage">
            {failureStageLabel(failure.failure_stage)}
          </Tag>
          <Text size="small" type="danger" ellipsis={{ showTooltip: true }} className="overview-failure-error">
            {errorText}
          </Text>
        </div>
        <Text size="small" type="secondary" ellipsis={{ showTooltip: true }} className="overview-failure-meta">
          策略：{failure.access_profile.name} · 凭证：{credentialName}
        </Text>
      </div>
    </Card>
  );
}
