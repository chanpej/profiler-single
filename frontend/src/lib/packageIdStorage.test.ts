import { afterEach, describe, expect, it } from 'vitest';
import { loadPackageId, savePackageId } from './packageIdStorage';

describe('packageIdStorage', () => {
  afterEach(() => {
    localStorage.clear();
  });

  it('returns empty when unset', () => {
    expect(loadPackageId()).toBe('');
  });

  it('persists and restores package id', () => {
    savePackageId('com.company.game');
    expect(loadPackageId()).toBe('com.company.game');
  });
});
