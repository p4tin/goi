// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { ChangeLog } from '../src/agent/changes';
import { compact, diffStats, lineDiff } from '../src/agent/diff';
import { fileLocation, renderMarkdown } from '../src/agent/markdown';
import {
  addPermission,
  addUser,
  answerPermission,
  applyUpdate,
  emptySession,
  endSession,
  endTurn,
  matchCommands,
  pendingPermissions,
  stopLabel,
} from '../src/agent/model';
import type { AgentEntry, ToolEntry } from '../src/agent/model';

const chunk = (kind: string, text: string, extra: object = {}) => ({ sessionUpdate: kind, content: { type: 'text', text }, ...extra });

describe('transcript reducer', () => {
  it('merges streamed chunks and separates thoughts from messages', () => {
    const s = emptySession();
    addUser(s, 'hi', ['a.go']);
    applyUpdate(s, chunk('agent_thought_chunk', 'thinking '));
    applyUpdate(s, chunk('agent_thought_chunk', 'more'));
    applyUpdate(s, chunk('agent_message_chunk', 'Hel'));
    applyUpdate(s, chunk('agent_message_chunk', 'lo'));
    expect(s.entries.map((e) => e.kind)).toEqual(['user', 'thought', 'agent']);
    expect((s.entries[1] as { text: string }).text).toBe('thinking more');
    expect((s.entries[2] as AgentEntry).text).toBe('Hello');
    expect(s.busy).toBe(true);
    // A different messageId starts a new message.
    applyUpdate(s, chunk('agent_message_chunk', 'A', { messageId: 'm1' }));
    applyUpdate(s, chunk('agent_message_chunk', 'B', { messageId: 'm2' }));
    expect(s.entries.filter((e) => e.kind === 'agent').map((e) => (e as AgentEntry).text)).toEqual(['HelloA', 'B']);
  });

  it('creates and patches tool calls by id', () => {
    const s = emptySession();
    applyUpdate(s, { sessionUpdate: 'tool_call', toolCallId: 't1', title: 'Edit a.go', kind: 'edit', status: 'pending', locations: [{ path: '/p/a.go', line: 3 }] });
    applyUpdate(s, chunk('agent_message_chunk', 'between'));
    applyUpdate(s, {
      sessionUpdate: 'tool_call_update',
      toolCallId: 't1',
      status: 'completed',
      content: [{ type: 'diff', path: '/p/a.go', oldText: 'a', newText: 'b' }, { type: 'terminal', terminalId: 'x-t1' }, { type: 'bogus' }],
    });
    const t = s.entries[0] as ToolEntry;
    expect(s.entries.length).toBe(2);
    expect(t).toMatchObject({ title: 'Edit a.go', toolKind: 'edit', status: 'completed', locations: [{ path: '/p/a.go', line: 3 }] });
    expect(t.content).toEqual([{ type: 'diff', path: '/p/a.go', oldText: 'a', newText: 'b' }, { type: 'terminal', terminalId: 'x-t1' }]);
    // An update for an unknown call creates it.
    applyUpdate(s, { sessionUpdate: 'tool_call_update', toolCallId: 't9', status: 'in_progress' });
    expect((s.entries[2] as ToolEntry).status).toBe('in_progress');
  });

  it('tracks plan, commands, mode, usage and ignores unknown updates', () => {
    const s = emptySession([{ id: 'ask', name: 'Ask' }], 'ask');
    const v = s.version;
    expect(applyUpdate(s, { sessionUpdate: 'plan', entries: [{ content: 'Do it', priority: 'high', status: 'pending' }] })).toBe(true);
    applyUpdate(s, { sessionUpdate: 'available_commands_update', availableCommands: [{ name: 'review', description: 'Review', input: { hint: 'what' } }, { name: 'init', description: 'Init' }] });
    applyUpdate(s, { sessionUpdate: 'current_mode_update', currentModeId: 'code' });
    applyUpdate(s, { sessionUpdate: 'usage_update', used: 10, size: 100 });
    expect(applyUpdate(s, { sessionUpdate: 'compaction_update' })).toBe(false);
    expect(s.plan).toEqual([{ content: 'Do it', priority: 'high', status: 'pending' }]);
    expect(s.commands[0]).toEqual({ name: 'review', description: 'Review', hint: 'what' });
    expect(s.currentMode).toBe('code');
    expect(s.usage).toEqual({ used: 10, size: 100 });
    expect(s.version).toBe(v + 4);
    expect(matchCommands(s.commands, '/re').map((c) => c.name)).toEqual(['review']);
    expect(matchCommands(s.commands, '/').length).toBe(2);
    expect(matchCommands(s.commands, 'review')).toEqual([]);
  });

  it('handles permissions and turn ends', () => {
    const s = emptySession();
    addUser(s, 'go');
    addPermission(s, 'r1', { toolCall: { title: 'Write a.go', kind: 'edit', content: [{ type: 'diff', path: 'a', newText: 'x' }] }, options: [{ optionId: 'allow', name: 'Allow', kind: 'allow_once' }] });
    addPermission(s, 'r2', { options: [] });
    expect(pendingPermissions(s).map((p) => p.requestId)).toEqual(['r1', 'r2']);
    expect(answerPermission(s, 'r1', 'allow')?.answer).toBe('allow');
    expect(answerPermission(s, 'r1', 'reject')?.answer).toBe('allow'); // first answer sticks
    applyUpdate(s, { sessionUpdate: 'tool_call', toolCallId: 't', title: 'Run', status: 'in_progress' });
    endTurn(s, 'cancelled');
    expect(s.busy).toBe(false);
    expect(pendingPermissions(s)).toEqual([]);
    expect(s.entries.find((e) => e.kind === 'permission' && e.requestId === 'r2')).toMatchObject({ answer: 'cancelled' });
    expect((s.entries.find((e) => e.kind === 'tool') as ToolEntry).status).toBe('failed');
    expect(s.entries[s.entries.length - 1]).toEqual({ kind: 'turn', stopReason: 'cancelled', error: '' });
    endSession(s, 'agent exited');
    expect(s.ended).toBe('agent exited');
    expect(stopLabel('end_turn')).toBe('');
    expect(stopLabel('cancelled')).toBe('Stopped');
  });
});

describe('markdown', () => {
  it('renders blocks and inline formatting without HTML injection', () => {
    const el = renderMarkdown('# Title\n\nSome **bold**, *it*, `code` and <b>raw</b>.\n\n- one\n- [x] done\n\n1. first\n\n> quoted\n\n---\n\n| a | b |\n|---|---|\n| 1 | 2 |');
    expect(el.querySelector('h3')?.textContent).toBe('Title');
    expect(el.querySelector('strong')?.textContent).toBe('bold');
    expect(el.querySelector('em')?.textContent).toBe('it');
    expect(el.querySelector('p code')?.textContent).toBe('code');
    expect(el.querySelector('b')).toBeNull();
    expect(el.textContent).toContain('<b>raw</b>');
    expect(el.querySelectorAll('ul li').length).toBe(2);
    expect(el.querySelector('.md-check.done')).not.toBeNull();
    expect(el.querySelector('ol li')?.textContent).toBe('first');
    expect(el.querySelector('blockquote')?.textContent).toBe('quoted');
    expect(el.querySelector('hr')).not.toBeNull();
    expect([...el.querySelectorAll('td')].map((t) => t.textContent)).toEqual(['1', '2']);
  });

  it('renders code blocks with actions, also while the fence is open', () => {
    const copied: string[] = [];
    const inserted: string[] = [];
    const el = renderMarkdown('```go\nfunc main() {}\n```\nafter\n```\nstreaming', { copy: (c) => copied.push(c), insert: (c) => inserted.push(c) });
    const blocks = el.querySelectorAll('.md-code');
    expect(blocks.length).toBe(2);
    expect(blocks[0].querySelector('.md-code-lang')?.textContent).toBe('go');
    expect(blocks[1].querySelector('code')?.textContent).toBe('streaming');
    (blocks[0].querySelectorAll('button')[0] as HTMLButtonElement).click();
    (blocks[0].querySelectorAll('button')[1] as HTMLButtonElement).click();
    expect(copied).toEqual(['func main() {}']);
    expect(inserted).toEqual(['func main() {}']);
  });

  it('makes links and file locations clickable', () => {
    const links: string[] = [];
    const files: string[] = [];
    const el = renderMarkdown('See [docs](https://go.dev) or https://example.com/x. Look at `internal/a.go:12` and `fmt.Println`.', {
      openLink: (u) => links.push(u),
      openFile: (p, l) => files.push(`${p}@${l}`),
    });
    const anchors = [...el.querySelectorAll('a')];
    anchors.forEach((a) => a.click());
    expect(links).toEqual(['https://go.dev', 'https://example.com/x']);
    expect(files).toEqual(['internal/a.go@12']);
    expect(fileLocation('fmt.Println')).toBeNull();
    expect(fileLocation('go.mod')).toEqual({ path: 'go.mod', line: undefined });
    expect(fileLocation('/abs/x.ts:3:9')).toEqual({ path: '/abs/x.ts', line: 3 });
  });
});

describe('line diff', () => {
  it('diffs lines and compacts unchanged runs', () => {
    const d = lineDiff('a\nb\nc\nd\ne\nf\ng', 'a\nb\nC\nd\ne\nf\ng\nh');
    expect(d.filter((l) => l.op !== ' ')).toEqual([
      { op: '-', text: 'c' },
      { op: '+', text: 'C' },
      { op: '+', text: 'h' },
    ]);
    expect(diffStats(d)).toEqual({ added: 2, removed: 1 });
    const c = compact(lineDiff('1\n2\n3\n4\n5\n6\n7\n8\n9', '1\n2\n3\n4\nX\n6\n7\n8\n9'), 1);
    expect(c.map((r) => r.op)).toEqual(['…', ' ', '-', '+', ' ', '…']);
    expect(lineDiff('', 'new')).toEqual([{ op: '+', text: 'new' }]);
    expect(lineDiff('same', 'same')).toEqual([{ op: ' ', text: 'same' }]);
  });
});

describe('change log', () => {
  it('keeps the content before the first write per session', () => {
    const log = new ChangeLog();
    log.record('s1', '/p/a.go', 'v0');
    log.record('s1', '/p/a.go', 'v1');
    log.record('s1', '/p/new.go', null);
    log.record('s2', '/p/a.go', 'other');
    expect(log.list('s1')).toEqual([
      { path: '/p/a.go', before: 'v0', writes: 2 },
      { path: '/p/new.go', before: null, writes: 1 },
    ]);
    log.forget('s1', '/p/a.go');
    expect(log.list('s1').map((c) => c.path)).toEqual(['/p/new.go']);
    log.forget('s1');
    expect(log.list('s1')).toEqual([]);
    expect(log.get('s2', '/p/a.go')?.before).toBe('other');
    expect(log.version()).toBe(6);
  });
});
