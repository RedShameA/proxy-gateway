import { useState, useCallback } from 'react';
import { Button, Typography, Tag, Table, Select, Input, InputNumber, TextArea, Toast, Descriptions, Card, Pagination, Spin, Modal, Dropdown } from '@douyinfe/semi-ui';
import { IconDelete, IconEdit, IconPlus, IconRefresh } from '@douyinfe/semi-icons';
import { AdaptivePanel } from '../components/AdaptivePanel';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { useData } from '../hooks/useData';
import { nodeStateLabel, nodeStateColor, formatRelativeTime, latencyText, countryDisplay, formatTime } from '../display';
import type { ApiClient } from '../api';
import type { NodeDetail, NodeSummary } from '../types';

const { Title, Text } = Typography;

type ImportMode = 'paste' | 'form';

const nodeImportPlaceholder = 'http://user:pass@1.2.3.4:8080#节点名\nsocks5://user:pass@1.2.3.4:1080#节点名\n{"type":"http","tag":"节点名","server":"1.2.3.4","server_port":8080}';

type ManualNodeFormState = {
  name: string;
  type: 'http' | 'socks5';
  server: string;
  server_port: number;
  username: string;
  password: string;
};

const emptyManualNodeForm: ManualNodeFormState = {
  name: '',
  type: 'http',
  server: '',
  server_port: 0,
  username: '',
  password: '',
};

function hasManualSource(node: NodeSummary) {
  return (node.sources || []).some(source => source.source_type === 'manual');
}

function supportsStructuredManualNodeForm(node: NodeSummary) {
  const protocol = node.protocol || node.type;
  return protocol === 'http' || protocol === 'socks5';
}

function nodeAddressText(node: NodeSummary) {
  return `${node.server}:${node.server_port}`;
}

function nodeSourceText(node: NodeSummary) {
  return (node.sources || []).map(source => source.source_name).join(', ') || '—';
}

function stringSelectValue(value: unknown) {
  return typeof value === 'string' ? value : '';
}

function NodeSourceCell({ node }: { node: NodeSummary }) {
  const deletable = hasManualSource(node);
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
      <Text ellipsis={{ showTooltip: true }} style={{ minWidth: 0, flex: 1 }}>
        {nodeSourceText(node)}
      </Text>
      <Tag size="small" color={(deletable ? 'red' : 'grey') as any} style={{ flexShrink: 0 }}>
        {deletable ? '可删' : '订阅'}
      </Tag>
    </div>
  );
}

function NodeHealthCell({ node }: { node: NodeSummary }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
      <Tag color={nodeStateColor(node.state) as any} size="small" style={{ flexShrink: 0 }}>
        {nodeStateLabel(node.state)}
      </Tag>
      <Text size="small" type="secondary" ellipsis={{ showTooltip: true }} style={{ minWidth: 0 }}>
        {latencyText(node.observation_latency_ms)}
      </Text>
    </div>
  );
}

function NodeActionsMenu({ onEdit, onDelete }: { onEdit: () => void; onDelete: () => void }) {
  const [visible, setVisible] = useState(false);
  const runAndClose = (action: () => void) => {
    setVisible(false);
    action();
  };

  return (
    <Dropdown
      trigger="click"
      position="bottomRight"
      visible={visible}
      onVisibleChange={setVisible}
      render={(
        <Dropdown.Menu>
          <Dropdown.Item icon={<IconEdit />} onClick={() => runAndClose(onEdit)}>
            编辑
          </Dropdown.Item>
          <Dropdown.Item type="danger" icon={<IconDelete />} onClick={() => runAndClose(onDelete)}>
            删除
          </Dropdown.Item>
        </Dropdown.Menu>
      )}
    >
      <Button size="small" onClick={e => e.stopPropagation()}>更多</Button>
    </Dropdown>
  );
}

function manualNodeFormFromNode(node: NodeSummary): ManualNodeFormState {
  return {
    name: node.name,
    type: node.protocol === 'socks5' || node.type === 'socks5' ? 'socks5' : 'http',
    server: node.server,
    server_port: node.server_port,
    username: node.username || '',
    password: node.password || '',
  };
}

function manualNodeFormToURI(form: ManualNodeFormState) {
  const name = encodeURIComponent(form.name.trim());
  const server = form.server.trim();
  const port = Number(form.server_port) || 0;
  const username = form.username ? encodeURIComponent(form.username) : '';
  const password = form.password ? encodeURIComponent(form.password) : '';
  const auth = username || password ? `${username}:${password}@` : '';
  return `${form.type}://${auth}${server}:${port}${name ? `#${name}` : ''}`;
}

function formatImportText(text: string) {
  const trimmed = text.trim();
  if (!trimmed) return '';
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return trimmed;
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  } catch {
    return trimmed;
  }
}

function importTextFromNodeDetail(node: NodeDetail, form: ManualNodeFormState, supportsForm: boolean) {
  return formatImportText(node.raw_json || (supportsForm ? manualNodeFormToURI(form) : ''));
}

function validateManualNodeForm(form: ManualNodeFormState) {
  const name = form.name.trim();
  const server = form.server.trim();
  const port = Number(form.server_port);
  if (!name) {
    return { ok: false as const, message: '节点名称不能为空' };
  }
  if (!server || !Number.isInteger(port) || port <= 0 || port > 65535) {
    return { ok: false as const, message: '节点服务器不能为空，端口需为 1-65535' };
  }
  return {
    ok: true as const,
    value: {
      name,
      type: form.type,
      server,
      server_port: port,
      username: form.username,
      password: form.password,
    },
  };
}

function ManualNodeFields({ form, onChange }: { form: ManualNodeFormState; onChange: (form: ManualNodeFormState) => void }) {
  return (
    <div style={{ display: 'grid', gap: 12 }}>
      <div>
        <Text type="secondary">名称</Text>
        <Input value={form.name} onChange={v => onChange({ ...form, name: v })} placeholder="节点名称" style={{ marginTop: 4 }} />
      </div>
      <div>
        <Text type="secondary">协议</Text>
        <Select value={form.type} onChange={v => onChange({ ...form, type: v as 'http' | 'socks5' })} style={{ width: '100%', marginTop: 4 }}>
          <Select.Option value="http">HTTP</Select.Option>
          <Select.Option value="socks5">SOCKS5</Select.Option>
        </Select>
      </div>
      <div>
        <Text type="secondary">服务器</Text>
        <Input value={form.server} onChange={v => onChange({ ...form, server: v })} placeholder="1.2.3.4" style={{ marginTop: 4 }} />
      </div>
      <div>
        <Text type="secondary">端口</Text>
        <InputNumber value={form.server_port || undefined} min={1} max={65535} onChange={v => onChange({ ...form, server_port: Number(v) || 0 })} style={{ width: '100%', marginTop: 4 }} />
      </div>
      <div>
        <Text type="secondary">用户名</Text>
        <Input value={form.username} onChange={v => onChange({ ...form, username: v })} placeholder="可选" style={{ marginTop: 4 }} />
      </div>
      <div>
        <Text type="secondary">密码</Text>
        <Input value={form.password} onChange={v => onChange({ ...form, password: v })} placeholder="可选" style={{ marginTop: 4 }} />
      </div>
    </div>
  );
}

export function NodesPage({ client }: { client: ApiClient }) {
  const isMobile = useMediaQuery('(max-width: 767px)');
  const [detailVisible, setDetailVisible] = useState(false);
  const [importVisible, setImportVisible] = useState(false);
  const [editVisible, setEditVisible] = useState(false);
  const [selected, setSelected] = useState<NodeSummary | null>(null);
  const [editingNode, setEditingNode] = useState<NodeSummary | null>(null);
  const [filters, setFilters] = useState({ country: '', state: '', protocol: '', name: '' });
  const [page, setPage] = useState(1);
  const [importMode, setImportMode] = useState<ImportMode>('paste');
  const [importText, setImportText] = useState('');
  const [manualNodeForm, setManualNodeForm] = useState(emptyManualNodeForm);
  const [editNodeForm, setEditNodeForm] = useState(emptyManualNodeForm);
  const [editMode, setEditMode] = useState<ImportMode>('form');
  const [editImportText, setEditImportText] = useState('');
  const [importing, setImporting] = useState(false);
  const [loadingEdit, setLoadingEdit] = useState(false);
  const [savingEdit, setSavingEdit] = useState(false);
  const pageSize = isMobile ? 5 : 10;

  const fetcher = useCallback(() => client.getNodes(page, pageSize, filters), [client, page, pageSize, filters]);
  const { data, loading, refresh } = useData(fetcher);

  const nodes = data?.items || [];
  const total = data?.total || 0;

  const { data: countries } = useData(useCallback(() => client.getEgressCountries(), [client]));

  const handleToggleNode = async (node: NodeSummary) => {
    try {
      await client.updateNode(node.id, { enabled: !node.enabled });
      Toast.success(`已${node.enabled ? '禁用' : '启用'} ${node.name}`);
      refresh();
    } catch (err) {
      Toast.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const handleObserveAll = async () => {
    try {
      await client.runNodeObservations();
      Toast.success('已触发全部节点观测');
    } catch (err) {
      Toast.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const handleImport = async () => {
    if (importMode === 'form') {
      await handleStructuredImport();
      return;
    }
    if (!importText.trim()) {
      Toast.error('请输入代理链接');
      return;
    }
    try {
      setImporting(true);
      const result = await client.importNodes(importText);
      Toast.success(`导入完成：成功 ${result.imported_nodes ?? 0}，跳过 ${result.skipped_entries ?? 0}`);
      setImportVisible(false);
      setImportText('');
      refresh();
    } catch (err) {
      Toast.error(`导入失败: ${err instanceof Error ? err.message : '未知错误'}`);
    } finally {
      setImporting(false);
    }
  };

  const handleStructuredImport = async () => {
    const validation = validateManualNodeForm(manualNodeForm);
    if (!validation.ok) {
      Toast.error(validation.message);
      return;
    }
    try {
      setImporting(true);
      await client.createNode(validation.value);
      Toast.success('节点已导入');
      setImportVisible(false);
      setManualNodeForm(emptyManualNodeForm);
      refresh();
    } catch (err) {
      Toast.error(`导入失败: ${err instanceof Error ? err.message : '未知错误'}`);
    } finally {
      setImporting(false);
    }
  };

  const openEditNode = async (node: NodeSummary) => {
    if (!hasManualSource(node)) {
      Toast.error('订阅节点不能直接编辑');
      return;
    }
    if (loadingEdit) return;
    try {
      setLoadingEdit(true);
      const detail = await client.getNode(node.id);
      const editableNode = { ...node, ...detail };
      const form = manualNodeFormFromNode(editableNode);
      const supportsForm = supportsStructuredManualNodeForm(editableNode);
      setEditingNode(editableNode);
      setEditNodeForm(form);
      setEditMode(supportsForm ? 'form' : 'paste');
      setEditImportText(importTextFromNodeDetail(detail, form, supportsForm));
      setEditVisible(true);
    } catch (err) {
      Toast.error(`读取节点失败: ${err instanceof Error ? err.message : '未知错误'}`);
    } finally {
      setLoadingEdit(false);
    }
  };

  const handleSaveEdit = async () => {
    if (!editingNode) return;
    if (editMode === 'paste') {
      if (!editImportText.trim()) {
        Toast.error('请输入 URI 或 JSON');
        return;
      }
      try {
        setSavingEdit(true);
        const result = await client.updateNode(editingNode.id, { import_text: editImportText });
        Toast.success(result.split ? '已创建新的手动节点副本，订阅节点保持不变' : '节点已保存');
        setEditVisible(false);
        setDetailVisible(false);
        setEditingNode(null);
        setSelected(null);
        setEditImportText('');
        refresh();
      } catch (err) {
        Toast.error(`保存失败: ${err instanceof Error ? err.message : '未知错误'}`);
      } finally {
        setSavingEdit(false);
      }
      return;
    }
    const validation = validateManualNodeForm(editNodeForm);
    if (!validation.ok) {
      Toast.error(validation.message);
      return;
    }
    try {
      setSavingEdit(true);
      const result = await client.updateNode(editingNode.id, validation.value);
      Toast.success(result.split ? '已创建新的手动节点副本，订阅节点保持不变' : '节点已保存');
      setEditVisible(false);
      setDetailVisible(false);
      setEditingNode(null);
      setSelected(null);
      refresh();
    } catch (err) {
      Toast.error(`保存失败: ${err instanceof Error ? err.message : '未知错误'}`);
    } finally {
      setSavingEdit(false);
    }
  };

  const handleDeleteNode = async (node: NodeSummary) => {
    try {
      await client.deleteNode(node.id);
      Toast.success('已删除手动节点');
      setDetailVisible(false);
      setSelected(null);
      refresh();
    } catch (err) {
      Toast.error(`删除失败: ${err instanceof Error ? err.message : '未知错误'}`);
    }
  };

  const confirmDeleteNode = (node: NodeSummary) => {
    Modal.confirm({
      title: '确认删除',
      content: '将移除该节点的手动来源；如果它同时来自订阅，订阅来源会保留。',
      onOk: () => handleDeleteNode(node),
    });
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      width: 160,
      render: (v: string) => (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 144 }}>
          {v}
        </Text>
      ),
    },
    { title: '协议', dataIndex: 'protocol', width: 92, render: (v: string) => <Text ellipsis={{ showTooltip: true }}>{v}</Text> },
    {
      title: '地址',
      width: 220,
      render: (_: any, r: NodeSummary) => <Text ellipsis={{ showTooltip: true }}>{nodeAddressText(r)}</Text>,
    },
    { title: '出口', width: 86, render: (_: any, r: NodeSummary) => <Text ellipsis={{ showTooltip: true }}>{countryDisplay(r.egress_country as any)}</Text> },
    { title: '健康', width: 128, render: (_: any, r: NodeSummary) => <NodeHealthCell node={r} /> },
    { title: '来源', width: 132, render: (_: any, r: NodeSummary) => <NodeSourceCell node={r} /> },
    { title: '观测', dataIndex: 'last_observed_at', width: 100, render: (v: number | null) => <Text size="small" type="secondary">{formatRelativeTime(v)}</Text> },
    { title: '操作', width: 104, render: (_: any, r: NodeSummary) => (
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <Button size="small" onClick={(e) => { e.stopPropagation(); handleToggleNode(r); }}>
          {r.enabled ? '禁用' : '启用'}
        </Button>
        {hasManualSource(r) && (
          <NodeActionsMenu
            onEdit={() => openEditNode(r)}
            onDelete={() => confirmDeleteNode(r)}
          />
        )}
      </div>
    )},
  ];

  if (loading) return <div className="page-loading"><Spin size="large" /></div>;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Title heading={4}>节点池</Title>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button icon={<IconRefresh />} onClick={handleObserveAll}>观察全部</Button>
          <Button icon={<IconPlus />} type="primary" onClick={() => setImportVisible(true)}>手动导入</Button>
        </div>
      </div>

      <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
        <Input placeholder="搜索名称" value={filters.name} onChange={v => { setFilters(f => ({ ...f, name: v })); setPage(1); }} style={{ width: 160 }} />
        <Select showClear placeholder="出口国家" value={filters.country} onChange={v => { setFilters(f => ({ ...f, country: stringSelectValue(v) })); setPage(1); }} style={{ width: 140 }}>
          {(countries || []).map(c => <Select.Option key={c.value} value={c.value}>{c.name_zh}</Select.Option>)}
        </Select>
        <Select showClear placeholder="状态" value={filters.state} onChange={v => { setFilters(f => ({ ...f, state: stringSelectValue(v) })); setPage(1); }} style={{ width: 120 }}>
          {['usable', 'unusable', 'pending_observation', 'disabled'].map(s => <Select.Option key={s} value={s}>{nodeStateLabel(s as any)}</Select.Option>)}
        </Select>
        <Select showClear placeholder="协议" value={filters.protocol} onChange={v => { setFilters(f => ({ ...f, protocol: stringSelectValue(v) })); setPage(1); }} style={{ width: 140 }}>
          {['http', 'socks5', 'shadowsocks', 'vmess'].map(protocol => <Select.Option key={protocol} value={protocol}>{protocol}</Select.Option>)}
        </Select>
      </div>

      {!isMobile && (
        <Table
          dataSource={nodes}
          columns={columns}
          rowKey="id"
          size="default"
          pagination={false}
          onRow={(r: NodeSummary | undefined) => ({ onClick: () => { if (r) { setSelected(r); setDetailVisible(true); } } })}
          style={{ cursor: 'pointer' }}
        />
      )}

      {isMobile && (
        <div style={{ display: 'grid', gap: 8 }}>
          {nodes.map(node => (
            <Card key={node.id} style={{ cursor: 'pointer', overflow: 'hidden' }}>
              <div onClick={() => { setSelected(node); setDetailVisible(true); }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                  <Text strong style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0, flex: 1 }}>{node.name}</Text>
                  <Tag color={nodeStateColor(node.state) as any} size="small" style={{ flexShrink: 0 }}>{nodeStateLabel(node.state)}</Tag>
                </div>
                <Text size="small" type="secondary" style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{node.protocol} · {node.server}:{node.server_port}</Text>
                <div style={{ display: 'flex', gap: 12, marginTop: 4 }}>
                  <Text size="small">{countryDisplay(node.egress_country as any)}</Text>
                  <Text size="small">{latencyText(node.observation_latency_ms)}</Text>
                </div>
                <Text size="small" type="secondary" style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  来源: {(node.sources || []).map(s => s.source_name).join(', ') || '—'} · {formatRelativeTime(node.last_observed_at)}
                </Text>
              </div>
              <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                <Button size="small" onClick={(e) => { e.stopPropagation(); handleToggleNode(node); }}>
                  {node.enabled ? '禁用' : '启用'}
                </Button>
                {hasManualSource(node) && (
                  <>
                    <Button size="small" icon={<IconEdit />} onClick={(e) => { e.stopPropagation(); openEditNode(node); }}>
                      编辑
                    </Button>
                    <Button size="small" type="danger" icon={<IconDelete />} onClick={(e) => { e.stopPropagation(); confirmDeleteNode(node); }}>
                      删除
                    </Button>
                  </>
                )}
              </div>
            </Card>
          ))}
        </div>
      )}

      {total > pageSize && (
        <Pagination total={total} pageSize={pageSize} currentPage={page} onChange={setPage} style={{ justifyContent: 'center', marginTop: 8 }} />
      )}

      <AdaptivePanel title="节点详情" visible={detailVisible} onClose={() => setDetailVisible(false)}>
        {selected && (
          <div style={{ display: 'grid', gap: 16 }}>
            <Descriptions row size="small" data={[
              { key: '名称', value: selected.name },
              { key: '协议', value: selected.protocol },
              { key: '地址', value: `${selected.server}:${selected.server_port}` },
              { key: '出口 IP', value: selected.egress_ip || '-' },
              { key: '出口国家', value: countryDisplay(selected.egress_country as any) },
              { key: '状态', value: <Tag color={nodeStateColor(selected.state) as any}>{nodeStateLabel(selected.state)}</Tag> },
              { key: '探测耗时', value: latencyText(selected.observation_latency_ms) },
              { key: '最后观测', value: formatTime(selected.last_observed_at) },
              { key: '来源', value: (selected.sources || []).map(s => `${s.source_name} (${s.source_type === 'manual' ? '手动' : '订阅'})`).join(', ') },
              { key: '错误', value: selected.last_error || '-' },
            ]} />
            <Button icon={<IconRefresh />} onClick={async () => {
              try {
                await client.runNodeObservations([selected.id]);
                Toast.success('该节点观测完成');
                refresh();
              } catch (err) {
                Toast.error(`操作失败: ${err instanceof Error ? err.message : '未知错误'}`);
              }
            }}>立即观测</Button>
            {hasManualSource(selected) && (
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                <Button icon={<IconEdit />} onClick={() => openEditNode(selected)}>编辑手动节点</Button>
                <Button type="danger" icon={<IconDelete />} onClick={() => confirmDeleteNode(selected)}>删除手动节点</Button>
              </div>
            )}
          </div>
        )}
      </AdaptivePanel>

      <AdaptivePanel title="手动导入节点" visible={importVisible} onClose={() => setImportVisible(false)}>
        <div style={{ display: 'grid', gap: 16 }}>
          <div>
            <Text type="secondary">导入方式</Text>
            <Select value={importMode} onChange={v => setImportMode(v as ImportMode)} style={{ width: '100%', marginTop: 8 }}>
              <Select.Option value="paste">粘贴 URI / JSON</Select.Option>
              <Select.Option value="form">HTTP / SOCKS5 表单</Select.Option>
            </Select>
          </div>
          {importMode === 'paste' && (
            <div style={{ display: 'grid', gap: 8 }}>
              <Text type="secondary">粘贴代理链接或 sing-box outbound JSON</Text>
              <TextArea rows={6} placeholder={nodeImportPlaceholder} value={importText} onChange={setImportText} />
              <Text size="small" type="secondary">URI 使用 #名称；sing-box JSON 使用 tag；vmess 使用 ps。</Text>
            </div>
          )}
          {importMode === 'form' && (
            <ManualNodeFields form={manualNodeForm} onChange={setManualNodeForm} />
          )}
          <Button type="primary" loading={importing} onClick={handleImport}>导入</Button>
        </div>
      </AdaptivePanel>

      <AdaptivePanel title="编辑手动节点" visible={editVisible} onClose={() => setEditVisible(false)}>
        {editingNode && (
          <div style={{ display: 'grid', gap: 16 }}>
            {supportsStructuredManualNodeForm(editingNode) && (
              <div>
                <Text type="secondary">编辑方式</Text>
                <Select
                  value={editMode}
                  onChange={v => {
                    const mode = v as ImportMode;
                    setEditMode(mode);
                    if (mode === 'paste' && !editImportText.trim()) {
                      setEditImportText(manualNodeFormToURI(editNodeForm));
                    }
                  }}
                  style={{ width: '100%', marginTop: 8 }}
                >
                  <Select.Option value="form">HTTP / SOCKS5 表单</Select.Option>
                  <Select.Option value="paste">粘贴 URI / JSON</Select.Option>
                </Select>
              </div>
            )}
            {editMode === 'form' && supportsStructuredManualNodeForm(editingNode) && (
              <ManualNodeFields form={editNodeForm} onChange={setEditNodeForm} />
            )}
            {editMode === 'paste' && (
              <div style={{ display: 'grid', gap: 8 }}>
                <Text type="secondary">粘贴代理链接或 sing-box outbound JSON</Text>
                <TextArea rows={6} placeholder={nodeImportPlaceholder} value={editImportText} onChange={setEditImportText} />
                <Text size="small" type="secondary">URI 使用 #名称；sing-box JSON 使用 tag；vmess 使用 ps。</Text>
              </div>
            )}
            <Button type="primary" loading={savingEdit} onClick={handleSaveEdit}>保存</Button>
          </div>
        )}
      </AdaptivePanel>
    </div>
  );
}
