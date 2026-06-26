import { useState, useCallback, useEffect } from 'react';
import type { ApiClient } from '../api';

export function useAuth(client: ApiClient) {
  const [isAuthenticated, setIsAuthenticated] = useState(() => !!localStorage.getItem('adminToken'));

  useEffect(() => {
    const handleUnauthorized = () => setIsAuthenticated(false);
    client.setUnauthorizedHandler(handleUnauthorized);
    return () => client.setUnauthorizedHandler(null);
  }, [client]);

  const login = useCallback(async (password: string) => {
    const resp = await client.login(password);
    client.setToken(resp.token);
    setIsAuthenticated(true);
  }, [client]);

  const setup = useCallback(async (password: string) => {
    const resp = await client.setup(password);
    client.setToken(resp.token);
    setIsAuthenticated(true);
  }, [client]);

  const logout = useCallback(() => {
    client.clearToken();
    setIsAuthenticated(false);
  }, [client]);

  return { isAuthenticated, login, setup, logout };
}
