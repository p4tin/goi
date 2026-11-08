import { describe, expect, it } from 'vitest';
import { versionLabel } from '../src/app/about';

describe('about', () => {
  it('shows the build version for releases', () => {
    expect(versionLabel('0.1', '0.1.0')).toBe('0.1.0');
    expect(versionLabel('0.1', 'dev')).toBe('0.1 (dev)');
  });
});
