// Git Log window (R8): implemented in M4 T8.

import { gitInfo } from './state';

export async function showHistory(path: string): Promise<void> {
  gitInfo('History', path);
}
