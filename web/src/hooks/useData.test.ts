import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useData } from './useData';

describe('useData', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('starts with loading=true', () => {
    const fetcher = vi.fn(() => new Promise<string>(() => {}));
    const { result } = renderHook(() => useData(fetcher));

    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it('sets data on success', async () => {
    const fetcher = vi.fn().mockResolvedValue({ name: 'test' });
    const { result } = renderHook(() => useData(fetcher));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.data).toEqual({ name: 'test' });
    expect(result.current.error).toBeNull();
  });

  it('sets error on failure', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('fail'));
    const { result } = renderHook(() => useData(fetcher));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.data).toBeNull();
    expect(result.current.error).toBe('fail');
  });

  it('refresh re-calls fetcher', async () => {
    let count = 0;
    const fetcher = vi.fn().mockImplementation(() => Promise.resolve(++count));
    const { result } = renderHook(() => useData(fetcher));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.data).toBe(1);

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.data).toBe(2);
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
});
