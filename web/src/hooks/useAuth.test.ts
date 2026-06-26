import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useAuth } from './useAuth';
import { ApiClient } from '../api';

describe('useAuth', () => {
  let client: ApiClient;

  beforeEach(() => {
    localStorage.clear();
    client = new ApiClient();
  });

  it('initializes with token from localStorage', () => {
    localStorage.setItem('adminToken', 'existing-token');
    const { result } = renderHook(() => useAuth(client));
    expect(result.current.isAuthenticated).toBe(true);
  });

  it('initializes without token', () => {
    const { result } = renderHook(() => useAuth(client));
    expect(result.current.isAuthenticated).toBe(false);
  });

  it('login saves token and sets authenticated', async () => {
    vi.spyOn(client, 'login').mockResolvedValue({ token: 'new-token' });
    const { result } = renderHook(() => useAuth(client));

    await act(async () => {
      await result.current.login('password');
    });

    expect(result.current.isAuthenticated).toBe(true);
    expect(localStorage.getItem('adminToken')).toBe('new-token');
  });

  it('logout clears token', () => {
    localStorage.setItem('adminToken', 'token');
    const { result } = renderHook(() => useAuth(client));

    act(() => {
      result.current.logout();
    });

    expect(result.current.isAuthenticated).toBe(false);
    expect(localStorage.getItem('adminToken')).toBeNull();
  });

  it('setup saves token and sets authenticated', async () => {
    vi.spyOn(client, 'setup').mockResolvedValue({ token: 'setup-token' });
    const { result } = renderHook(() => useAuth(client));

    await act(async () => {
      await result.current.setup('new-password');
    });

    expect(result.current.isAuthenticated).toBe(true);
    expect(localStorage.getItem('adminToken')).toBe('setup-token');
  });

  it('updates auth state when the API client reports unauthorized', () => {
    localStorage.setItem('adminToken', 'token');
    let unauthorizedHandler: (() => void) | null = null;
    vi.spyOn(client, 'setUnauthorizedHandler').mockImplementation((handler) => {
      unauthorizedHandler = handler;
    });
    const { result } = renderHook(() => useAuth(client));

    act(() => {
      unauthorizedHandler?.();
    });

    expect(result.current.isAuthenticated).toBe(false);
  });
});
