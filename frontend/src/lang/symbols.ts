// Workspace symbols for Search Everywhere (R10).

import { setSymbolProvider } from '../palette/palette';
import type { SymbolEntry } from '../palette/palette';
import { lang } from '../lib/api';
import type { WorkspaceSymbol } from '../lib/api';
import { client } from './context';
import { relPath } from './diagnostics';
import { jumpTo } from './navigation';
import { kindBadge } from './structure';

export const SYMBOL_LIMIT = 50;

export function symbolEntry(s: WorkspaceSymbol, run: () => void): SymbolEntry {
  const where = `${relPath(s.location.path)}:${s.location.range.start.line + 1}`;
  return { label: s.name, detail: s.containerName ? `${s.containerName} · ${where}` : where, kind: kindBadge(s.kind), run };
}

export function installSymbols(): void {
  setSymbolProvider(async (q) => {
    if (!client.live()) {
      const st = client.status.peek().state;
      throw new Error(st === 'missing' ? 'Symbol search needs gopls (not installed)' : 'gopls is not ready yet');
    }
    const syms = await lang.workspaceSymbols(q);
    return syms.slice(0, SYMBOL_LIMIT).map((s) => symbolEntry(s, () => void jumpTo(s.location)));
  });
}
