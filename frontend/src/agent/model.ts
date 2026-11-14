// Transcript model of one agent session: a reducer over ACP session/update
// payloads plus the entries Goi adds itself (user prompts, permission
// requests, turn ends). Pure; the view re-renders from it.

export type ToolStatus = 'pending' | 'in_progress' | 'completed' | 'failed';

export interface ContentBlock {
  type: string;
  text?: string;
  uri?: string;
  name?: string;
  mimeType?: string;
  resource?: { uri?: string; text?: string; mimeType?: string };
}

export type ToolContent =
  | { type: 'content'; content: ContentBlock }
  | { type: 'diff'; path: string; oldText?: string | null; newText: string }
  | { type: 'terminal'; terminalId: string };

export interface ToolLocation {
  path: string;
  line?: number | null;
}

export interface ToolEntry {
  kind: 'tool';
  id: string;
  title: string;
  toolKind: string;
  status: ToolStatus;
  content: ToolContent[];
  locations: ToolLocation[];
  rawInput?: unknown;
}

export interface PermissionOption {
  optionId: string;
  name: string;
  kind: string;
}

export interface PermissionEntry {
  kind: 'permission';
  requestId: string;
  title: string;
  toolKind: string;
  content: ToolContent[];
  locations: ToolLocation[];
  options: PermissionOption[];
  /** The chosen optionId, or "cancelled"; undefined while pending. */
  answer?: string;
}

export interface UserEntry {
  kind: 'user';
  text: string;
  /** Labels of the context attached to the prompt (file names…). */
  context: string[];
}

export interface AgentEntry {
  kind: 'agent';
  text: string;
  messageId?: string;
}

export interface ThoughtEntry {
  kind: 'thought';
  text: string;
}

export interface TurnEntry {
  kind: 'turn';
  stopReason: string;
  error: string;
}

export interface NoticeEntry {
  kind: 'notice';
  level: 'info' | 'warn' | 'error';
  text: string;
}

export type Entry = UserEntry | AgentEntry | ThoughtEntry | ToolEntry | PermissionEntry | TurnEntry | NoticeEntry;

export interface PlanItem {
  content: string;
  priority: string;
  status: string;
}

export interface AgentCommand {
  name: string;
  description: string;
  hint?: string;
}

export interface Mode {
  id: string;
  name: string;
  description?: string;
}

export interface SessionState {
  entries: Entry[];
  plan: PlanItem[];
  modes: Mode[];
  currentMode: string;
  commands: AgentCommand[];
  /** A prompt turn is running. */
  busy: boolean;
  title: string;
  usage: { used: number; size: number } | null;
  /** Set when the session can't continue (agent stopped). */
  ended: string;
  /** Bumped on every change, for cheap re-render checks. */
  version: number;
}

export function emptySession(modes: Mode[] = [], currentMode = ''): SessionState {
  return { entries: [], plan: [], modes, currentMode, commands: [], busy: false, title: '', usage: null, ended: '', version: 0 };
}

type Obj = Record<string, unknown>;

const str = (v: unknown): string => (typeof v === 'string' ? v : '');
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);

/** A content block's display text (Markdown). */
export function blockText(b: unknown): string {
  if (!b || typeof b !== 'object') return '';
  const c = b as ContentBlock;
  switch (c.type) {
    case 'text':
      return c.text ?? '';
    case 'resource_link':
      return `[${c.name || c.uri || 'link'}](${c.uri ?? ''})`;
    case 'resource':
      return c.resource?.text ? '```\n' + c.resource.text + '\n```' : `[${c.resource?.uri ?? 'resource'}]`;
    case 'image':
      return '[image]';
    case 'audio':
      return '[audio]';
  }
  return '';
}

function toolContent(v: unknown): ToolContent[] {
  const out: ToolContent[] = [];
  for (const raw of arr(v)) {
    const c = raw as Obj;
    if (c.type === 'diff' && typeof c.path === 'string') {
      out.push({ type: 'diff', path: c.path, oldText: (c.oldText as string | null | undefined) ?? null, newText: str(c.newText) });
    } else if (c.type === 'terminal' && typeof c.terminalId === 'string') {
      out.push({ type: 'terminal', terminalId: c.terminalId });
    } else if (c.type === 'content' && c.content && typeof c.content === 'object') {
      out.push({ type: 'content', content: c.content as ContentBlock });
    }
  }
  return out;
}

function locations(v: unknown): ToolLocation[] {
  return arr(v)
    .map((l) => l as Obj)
    .filter((l) => typeof l.path === 'string')
    .map((l) => ({ path: l.path as string, line: typeof l.line === 'number' ? l.line : null }));
}

const STATUSES: ToolStatus[] = ['pending', 'in_progress', 'completed', 'failed'];

function patchTool(t: ToolEntry, u: Obj): void {
  if (typeof u.title === 'string' && u.title) t.title = u.title;
  if (typeof u.kind === 'string') t.toolKind = u.kind;
  if (STATUSES.includes(u.status as ToolStatus)) t.status = u.status as ToolStatus;
  if (Array.isArray(u.content)) t.content = toolContent(u.content);
  if (Array.isArray(u.locations)) t.locations = locations(u.locations);
  if ('rawInput' in u) t.rawInput = u.rawInput;
}

function findTool(s: SessionState, id: string): ToolEntry | undefined {
  for (let i = s.entries.length - 1; i >= 0; i--) {
    const e = s.entries[i];
    if (e.kind === 'tool' && e.id === id) return e;
  }
  return undefined;
}

/** Appends streamed text to the last entry of kind, or starts a new one. */
function appendChunk(s: SessionState, kind: 'agent' | 'thought', text: string, messageId?: string): void {
  const last = s.entries[s.entries.length - 1];
  if (last && last.kind === kind && (kind === 'thought' || !messageId || !(last as AgentEntry).messageId || (last as AgentEntry).messageId === messageId)) {
    last.text += text;
    if (kind === 'agent' && messageId) (last as AgentEntry).messageId = messageId;
    return;
  }
  s.entries.push(kind === 'agent' ? { kind, text, messageId } : { kind, text });
}

/** Applies one session/update payload. Returns false for ignored kinds. */
export function applyUpdate(s: SessionState, u: Obj): boolean {
  switch (u.sessionUpdate) {
    case 'agent_message_chunk':
      appendChunk(s, 'agent', blockText(u.content), typeof u.messageId === 'string' ? u.messageId : undefined);
      break;
    case 'agent_thought_chunk':
      appendChunk(s, 'thought', blockText(u.content));
      break;
    case 'tool_call': {
      const id = str(u.toolCallId);
      let t = findTool(s, id);
      if (!t) {
        t = { kind: 'tool', id, title: str(u.title) || id, toolKind: str(u.kind) || 'other', status: 'pending', content: [], locations: [] };
        s.entries.push(t);
      }
      patchTool(t, u);
      break;
    }
    case 'tool_call_update': {
      const id = str(u.toolCallId);
      let t = findTool(s, id);
      if (!t) {
        t = { kind: 'tool', id, title: str(u.title) || id, toolKind: str(u.kind) || 'other', status: 'pending', content: [], locations: [] };
        s.entries.push(t);
      }
      patchTool(t, u);
      break;
    }
    case 'plan':
      s.plan = arr(u.entries).map((e) => {
        const o = e as Obj;
        return { content: str(o.content), priority: str(o.priority), status: str(o.status) };
      });
      break;
    case 'available_commands_update':
      s.commands = arr(u.availableCommands).map((c) => {
        const o = c as Obj;
        const input = o.input as Obj | null | undefined;
        return { name: str(o.name), description: str(o.description), hint: input ? str(input.hint) : undefined };
      });
      break;
    case 'current_mode_update':
      s.currentMode = str(u.currentModeId);
      break;
    case 'session_info_update':
      if (typeof u.title === 'string') s.title = u.title;
      break;
    case 'usage_update':
      if (typeof u.used === 'number' && typeof u.size === 'number') s.usage = { used: u.used, size: u.size };
      break;
    case 'notice': {
      const sev = str(u.severity);
      const text = [str(u.title), str(u.description)].filter(Boolean).join(': ');
      s.entries.push({ kind: 'notice', level: sev === 'error' ? 'error' : sev === 'warning' ? 'warn' : 'info', text });
      break;
    }
    default:
      return false;
  }
  s.version++;
  return true;
}

export function addUser(s: SessionState, text: string, context: string[] = []): void {
  s.entries.push({ kind: 'user', text, context });
  s.busy = true;
  s.version++;
}

export interface PermissionParams {
  toolCall?: Obj;
  options?: PermissionOption[];
}

export function addPermission(s: SessionState, requestId: string, p: PermissionParams): PermissionEntry {
  const tc = p.toolCall ?? {};
  const e: PermissionEntry = {
    kind: 'permission',
    requestId,
    title: str(tc.title) || 'The agent wants to run a tool',
    toolKind: str(tc.kind) || 'other',
    content: toolContent(tc.content),
    locations: locations(tc.locations),
    options: (p.options ?? []).map((o) => ({ optionId: o.optionId, name: o.name, kind: o.kind })),
  };
  s.entries.push(e);
  s.version++;
  return e;
}

export function answerPermission(s: SessionState, requestId: string, answer: string): PermissionEntry | undefined {
  const e = s.entries.find((x): x is PermissionEntry => x.kind === 'permission' && x.requestId === requestId);
  if (e && !e.answer) {
    e.answer = answer;
    s.version++;
  }
  return e;
}

/** Pending permission requests (unanswered). */
export function pendingPermissions(s: SessionState): PermissionEntry[] {
  return s.entries.filter((x): x is PermissionEntry => x.kind === 'permission' && !x.answer);
}

/** Ends the running turn; unanswered permission requests become cancelled. */
export function endTurn(s: SessionState, stopReason: string, error = ''): void {
  s.busy = false;
  for (const p of pendingPermissions(s)) p.answer = 'cancelled';
  for (const e of s.entries) if (e.kind === 'tool' && (e.status === 'pending' || e.status === 'in_progress') && stopReason === 'cancelled') e.status = 'failed';
  s.entries.push({ kind: 'turn', stopReason, error });
  s.version++;
}

export function endSession(s: SessionState, message: string): void {
  if (s.busy) endTurn(s, 'cancelled', message);
  s.ended = message || 'The agent stopped';
  s.version++;
}

export function addNotice(s: SessionState, level: NoticeEntry['level'], text: string): void {
  s.entries.push({ kind: 'notice', level, text });
  s.version++;
}

/** Commands matching a "/prefix" typed at the start of the prompt. */
export function matchCommands(cmds: readonly AgentCommand[], input: string): AgentCommand[] {
  const m = /^\/(\S*)$/.exec(input);
  if (!m) return [];
  const q = m[1].toLowerCase();
  return cmds.filter((c) => c.name.toLowerCase().startsWith(q));
}

/** Human label of a stop reason. */
export function stopLabel(reason: string): string {
  switch (reason) {
    case 'end_turn':
      return '';
    case 'cancelled':
      return 'Stopped';
    case 'max_tokens':
      return 'Stopped: the response hit the token limit';
    case 'max_turn_requests':
      return 'Stopped: too many steps in one turn';
    case 'refusal':
      return 'The agent refused to continue';
  }
  return reason ? `Stopped (${reason})` : '';
}
