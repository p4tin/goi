// 3-way merge editor (R10): implemented in M4 T9.

import { openDiff } from './diffView';

export async function openMerge(path: string): Promise<void> {
  await openDiff({ path, mode: 'head' });
}
