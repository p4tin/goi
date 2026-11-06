import { describe, expect, it } from 'vitest';
import { filterCommands } from '../src/views/settings/keymapEditor';
import { listLines, originOf, parseEnvLines } from '../src/views/settings/settingsView';

describe('settings view helpers', () => {
  it('originOf uses exact keys then the highest leaf layer', () => {
    const o = { 'save.format': 'project', 'go.env.GOPROXY': 'user', 'go.env.GOFLAGS': 'project' } as const;
    expect(originOf({ ...o }, 'save.format')).toBe('project');
    expect(originOf({ ...o }, 'go.env')).toBe('project');
    expect(originOf({ ...o }, 'editor.fontSize')).toBe('default');
  });

  it('parseEnvLines and listLines', () => {
    expect(parseEnvLines('GOPROXY=direct\n\n GOFLAGS = -mod=mod \n')).toEqual({ GOPROXY: 'direct', GOFLAGS: '-mod=mod' });
    expect(parseEnvLines('oops')).toMatch(/KEY=value/);
    expect(listLines(' a \n\nb\n')).toEqual(['a', 'b']);
  });

  it('filterCommands matches label and id, sorted', () => {
    const cmds = [
      { id: 'file.save', title: 'Save', category: 'File', run: () => {} },
      { id: 'editor.find', title: 'Find in File', category: 'Editor', run: () => {} },
      { id: 'file.saveAll', title: 'Save All', category: 'File', run: () => {} },
    ];
    expect(filterCommands(cmds, 'save').map((c) => c.id)).toEqual(['file.save', 'file.saveAll']);
    expect(filterCommands(cmds, 'editor.find').map((c) => c.id)).toEqual(['editor.find']);
    expect(filterCommands(cmds, '').map((c) => c.id)).toEqual(['editor.find', 'file.save', 'file.saveAll']);
  });
});
