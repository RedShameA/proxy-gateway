import { Routes, Route, Navigate } from 'react-router-dom';
import { lazy, Suspense, useState, useEffect, useMemo, type ReactNode } from 'react';
import { ApiClient } from './api';
import { useAuth } from './hooks/useAuth';

const AppShell = lazy(() => import('./layouts/AppShell').then(module => ({ default: module.AppShell })));
const LoginPage = lazy(() => import('./pages/LoginPage').then(module => ({ default: module.LoginPage })));
const OverviewPage = lazy(() => import('./pages/OverviewPage').then(module => ({ default: module.OverviewPage })));
const SubscriptionsPage = lazy(() => import('./pages/SubscriptionsPage').then(module => ({ default: module.SubscriptionsPage })));
const NodesPage = lazy(() => import('./pages/NodesPage').then(module => ({ default: module.NodesPage })));
const AccessProfilesPage = lazy(() => import('./pages/AccessProfilesPage').then(module => ({ default: module.AccessProfilesPage })));
const AccessProfileDetailPage = lazy(() => import('./pages/AccessProfileDetailPage').then(module => ({ default: module.AccessProfileDetailPage })));
const SettingsPage = lazy(() => import('./pages/SettingsPage').then(module => ({ default: module.SettingsPage })));
const LogsPage = lazy(() => import('./pages/LogsPage').then(module => ({ default: module.LogsPage })));

function LoadingScreen() {
  return <div className="page-loading">加载中...</div>;
}

function withPageFallback(element: ReactNode) {
  return <Suspense fallback={<LoadingScreen />}>{element}</Suspense>;
}

export function App() {
  const client = useMemo(() => new ApiClient(), []);
  const { isAuthenticated, login, setup, logout } = useAuth(client);
  const [requiresSetup, setRequiresSetup] = useState(false);
  const [version, setVersion] = useState('');
  const [checkingSetup, setCheckingSetup] = useState(true);

  useEffect(() => {
    client.getSetupStatus()
      .then(s => {
        setRequiresSetup(s.requires_setup);
        setVersion(s.build?.version || '');
      })
      .catch(() => {})
      .finally(() => setCheckingSetup(false));
  }, [isAuthenticated, client]);

  if (checkingSetup) {
    return <LoadingScreen />;
  }

  if (!isAuthenticated) {
    return withPageFallback(<LoginPage requiresSetup={requiresSetup} onLogin={login} onSetup={setup} />);
  }

  return (
    <Routes>
      <Route element={withPageFallback(<AppShell version={version} onLogout={logout} />)}>
        <Route path="/" element={withPageFallback(<OverviewPage client={client} />)} />
        <Route path="/subscriptions" element={withPageFallback(<SubscriptionsPage client={client} />)} />
        <Route path="/nodes" element={withPageFallback(<NodesPage client={client} />)} />
        <Route path="/access-profiles" element={withPageFallback(<AccessProfilesPage client={client} />)} />
        <Route path="/access-profiles/:id" element={withPageFallback(<AccessProfileDetailPage client={client} />)} />
        <Route path="/settings" element={withPageFallback(<SettingsPage client={client} />)} />
        <Route path="/logs" element={withPageFallback(<LogsPage client={client} />)} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
