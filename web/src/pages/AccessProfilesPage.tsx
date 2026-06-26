import { useState, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Button, Typography, Row, Col, Toast, Pagination, Spin } from '@douyinfe/semi-ui';
import { IconPlus } from '@douyinfe/semi-icons';
import { AdaptivePanel } from '../components/AdaptivePanel';
import { AccessProfileForm } from '../components/AccessProfileForm';
import { AccessProfileCard } from '../components/AccessProfileCard';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { useData } from '../hooks/useData';
import type { ApiClient } from '../api';
import type { AccessProfileWriteRequest } from '../types';

const { Title } = Typography;

export function AccessProfilesPage({ client }: { client: ApiClient }) {
  const isMobile = useMediaQuery('(max-width: 767px)');
  const [createVisible, setCreateVisible] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const page = Number(searchParams.get('page')) || 1;
  const setPage = (p: number) => setSearchParams(p > 1 ? { page: String(p) } : {});
  const nav = useNavigate();
  const pageSize = isMobile ? 5 : 9;

  const fetcher = useCallback(() => client.getAccessProfiles(page, pageSize), [client, page, pageSize]);
  const { data, loading, refresh } = useData(fetcher);

  const profiles = data?.items || [];
  const total = data?.total || 0;

  const { data: countries } = useData(useCallback(() => client.getEgressCountries(), [client]));
  const { data: nodes } = useData(useCallback(() => client.getNodes(1, 100), [client]));
  const { data: subscriptions } = useData(useCallback(() => client.getSubscriptions(1, 100), [client]));

  const handleCreate = async (payload: AccessProfileWriteRequest) => {
    try {
      setSubmitting(true);
      await client.createAccessProfile(payload);
      Toast.success('策略已创建');
      setCreateVisible(false);
      refresh();
    } catch (err) {
      Toast.error(`创建失败: ${err instanceof Error ? err.message : '未知错误'}`);
    } finally {
      setSubmitting(false);
    }
  };

  if (loading) return <div className="page-loading"><Spin size="large" /></div>;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Title heading={4}>访问策略</Title>
        <Button icon={<IconPlus />} type="primary" onClick={() => setCreateVisible(true)}>创建策略</Button>
      </div>

      <Row gutter={12}>
        {profiles.map(p => (
          <Col key={p.id} xs={24} sm={12} md={8} style={{ marginBottom: 12 }}>
            <AccessProfileCard profile={p} onClick={() => nav(`/access-profiles/${p.id}`)} />
          </Col>
        ))}
      </Row>

      {total > pageSize && (
        <Pagination total={total} pageSize={pageSize} currentPage={page} onChange={setPage} style={{ justifyContent: 'center', marginTop: 8 }} />
      )}

      <AdaptivePanel title="创建访问策略" visible={createVisible} onClose={() => setCreateVisible(false)} width={640}>
        {createVisible && (
          <AccessProfileForm
            nodes={nodes?.items || []}
            countries={countries || []}
            subscriptions={subscriptions?.items || []}
            submitting={submitting}
            submitLabel="创建"
            onSubmit={handleCreate}
          />
        )}
      </AdaptivePanel>
    </div>
  );
}
