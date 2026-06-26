import { describe, it, expect } from 'vitest';
import { appBrand } from '../brand';

describe('test infrastructure', () => {
  it('runs tests', () => {
    expect(true).toBe(true);
  });

  it('shows the app version in the shell brand', () => {
    expect(appBrand('v0.1.0')).toEqual({ name: 'Proxy Gateway', version: 'v0.1.0' });
  });

  it('falls back to the app name when version is unavailable', () => {
    expect(appBrand()).toEqual({ name: 'Proxy Gateway', version: '' });
  });
});
