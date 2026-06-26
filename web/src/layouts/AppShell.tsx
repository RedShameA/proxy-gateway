import { useState } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Layout, Nav, Button } from '@douyinfe/semi-ui';
import { IconHome, IconSetting, IconHistogram, IconDownload, IconExit, IconServer, IconShield, IconPulse } from '@douyinfe/semi-icons';
import { BottomTabBar } from '../components/BottomTabBar';
import { appBrand } from '../brand';

const { Sider, Content } = Layout;

const navItems = [
  { itemKey: '/', text: '概览', icon: <IconHome /> },
  { itemKey: '/subscriptions', text: '订阅管理', icon: <IconDownload /> },
  { itemKey: '/nodes', text: '节点池', icon: <IconServer /> },
  { itemKey: '/access-profiles', text: '访问策略', icon: <IconShield /> },
  { itemKey: '/settings', text: '设置', icon: <IconSetting /> },
  { itemKey: '/logs', text: '请求日志', icon: <IconHistogram /> },
];

function AppBrand({ version }: { version?: string }) {
  const brand = appBrand(version);
  return (
    <span className="app-brand">
      <span className="app-brand-name">{brand.name}</span>
      {brand.version && (
        <span className="app-version-badge" aria-label={`Version ${brand.version}`} title={`Version ${brand.version}`}>
          <span className="app-version-dot" aria-hidden="true" />
          <span>{brand.version}</span>
        </span>
      )}
    </span>
  );
}

export function AppShell({ version, onLogout }: { version?: string; onLogout: () => void }) {
  const nav = useNavigate();
  const loc = useLocation();
  const [collapsed, setCollapsed] = useState(false);

  const selectedKey = loc.pathname.startsWith('/access-profiles') ? '/access-profiles' : loc.pathname;

  return (
    <Layout className="app-layout">
      <Sider className="app-sider">
        <Nav
          selectedKeys={[selectedKey]}
          items={navItems}
          onSelect={({ itemKey }) => { if (itemKey) nav(itemKey as string); }}
          isCollapsed={collapsed}
          onCollapseChange={setCollapsed}
          collapseButton
          style={{ height: '100%', border: 'none' }}
          header={{
            text: collapsed ? 'PG' : <AppBrand version={version} />,
            logo: (
              <span className="app-brand-logo">
                <IconPulse />
              </span>
            ),
          }}
          footer={
            <Button icon={<IconExit />} onClick={onLogout} style={{ margin: 8 }} block>
              {!collapsed && '退出登录'}
            </Button>
          }
        />
      </Sider>
      <Content className="app-content">
        <Outlet />
      </Content>
      <BottomTabBar />
    </Layout>
  );
}
