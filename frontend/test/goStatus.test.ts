import { describe, expect, it } from 'vitest';
import { formatSdkLabel, formatTarget, sdkSourceLabel } from '../src/views/goStatus';

describe('formatSdkLabel', () => {
  it('shows the version without the go prefix', () => {
    expect(formatSdkLabel({ ok: true, problem: '', sdk: { version: 'go1.26.0', goroot: '/usr/local/go' } })).toEqual({
      text: 'Go 1.26.0',
      title: '/usr/local/go',
      warn: false,
    });
  });

  it('handles a missing version', () => {
    expect(formatSdkLabel({ ok: true, problem: '', sdk: { version: '' } }).text).toBe('Go');
  });

  it('warns when no SDK', () => {
    const l = formatSdkLabel({ ok: false, problem: 'no Go SDK found' });
    expect(l).toEqual({ text: '⚠ No Go SDK', title: 'no Go SDK found', warn: true });
    expect(formatSdkLabel({ ok: false, problem: '' }).title).toBe('No Go SDK configured');
  });
});

describe('formatTarget', () => {
  it('formats host target', () => {
    expect(formatTarget({ goos: 'darwin', goarch: 'arm64', goversion: 'go1.26.0', cross: false })).toEqual({
      text: 'darwin/arm64',
      title: 'GOOS/GOARCH: darwin/arm64 · go1.26.0',
      warn: false,
    });
  });

  it('marks cross targets', () => {
    const l = formatTarget({ goos: 'linux', goarch: 'amd64', goversion: '', cross: true });
    expect(l.text).toBe('linux/amd64 (cross)');
    expect(l.warn).toBe(true);
    expect(l.title).toBe('GOOS/GOARCH: linux/amd64');
  });
});

describe('sdkSourceLabel', () => {
  it('maps known sources', () => {
    expect(sdkSourceLabel('path')).toBe('PATH');
    expect(sdkSourceLabel('toolchain')).toBe('Toolchain');
    expect(sdkSourceLabel('manual')).toBe('Manual');
    expect(sdkSourceLabel('sdk')).toBe('~/sdk');
    expect(sdkSourceLabel('standard')).toBe('System');
  });

  it('falls back for unknown sources', () => {
    expect(sdkSourceLabel('weird')).toBe('weird');
    expect(sdkSourceLabel('')).toBe('Unknown');
  });
});
