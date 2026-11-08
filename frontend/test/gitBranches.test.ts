// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { branchEntries, validBranchName } from '../src/git/branches';
import type { GitBranch } from '../src/lib/api';

const b = (name: string, extra: Partial<GitBranch> = {}): GitBranch => ({ name, remote: false, current: false, upstream: '', hash: '', subject: '', date: '', ...extra });

describe('branches', () => {
  it('orders local (current first) then remote', () => {
    const { local, remote } = branchEntries([b('zeta'), b('origin/main', { remote: true }), b('main', { current: true }), b('alpha')]);
    expect(local.map((x) => x.name)).toEqual(['main', 'alpha', 'zeta']);
    expect(remote.map((x) => x.name)).toEqual(['origin/main']);
  });
  it('validates branch names', () => {
    expect(validBranchName('feature/login')).toBeNull();
    expect(validBranchName('')).not.toBeNull();
    expect(validBranchName('has space')).not.toBeNull();
    expect(validBranchName('a..b')).not.toBeNull();
    expect(validBranchName('-x')).not.toBeNull();
    expect(validBranchName('x.lock')).not.toBeNull();
    expect(validBranchName('x/')).not.toBeNull();
  });
});
