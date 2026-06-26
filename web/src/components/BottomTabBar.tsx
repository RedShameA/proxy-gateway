import { IconHome, IconSetting, IconHistogram, IconDownload, IconServer, IconShield } from '@douyinfe/semi-icons';
import { useNavigate, useLocation } from 'react-router-dom';
import { Typography } from '@douyinfe/semi-ui';

const tabs = [
  { path: '/', icon: <IconHome />, label: '概览' },
  { path: '/subscriptions', icon: <IconDownload />, label: '订阅' },
  { path: '/nodes', icon: <IconServer />, label: '节点' },
  { path: '/access-profiles', icon: <IconShield />, label: '策略' },
  { path: '/settings', icon: <IconSetting />, label: '设置' },
  { path: '/logs', icon: <IconHistogram />, label: '日志' },
];

export function BottomTabBar() {
  const nav = useNavigate();
  const loc = useLocation();
  const active = loc.pathname.startsWith('/access-profiles') ? '/access-profiles' : loc.pathname;
  return (
    <div className="bottom-tab-bar">
      {tabs.map(t => (
        <div key={t.path} className={`bottom-tab-item ${active === t.path ? 'active' : ''}`} onClick={() => nav(t.path)}>
          {t.icon}
          <Typography.Text size="small" type={active === t.path ? 'primary' : 'secondary'}>{t.label}</Typography.Text>
        </div>
      ))}
    </div>
  );
}
