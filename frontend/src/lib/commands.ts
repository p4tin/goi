// Command registry: every user-invokable action is a command with a stable id.
// Keymaps bind keys to ids; the palette lists them.

export interface Command {
  id: string;
  title: string;
  category?: string;
  run: (...args: unknown[]) => unknown;
  /** When present and false, the command is unavailable. */
  when?: () => boolean;
}

export class CommandRegistry {
  private cmds = new Map<string, Command>();
  private listeners = new Set<() => void>();

  /** Registers cmd and returns an unregister function. Duplicate ids throw. */
  register(cmd: Command): () => void {
    if (this.cmds.has(cmd.id)) throw new Error(`command already registered: ${cmd.id}`);
    this.cmds.set(cmd.id, cmd);
    this.changed();
    return () => {
      if (this.cmds.get(cmd.id) === cmd) {
        this.cmds.delete(cmd.id);
        this.changed();
      }
    };
  }

  get(id: string): Command | undefined {
    return this.cmds.get(id);
  }

  all(): Command[] {
    return [...this.cmds.values()];
  }

  isEnabled(id: string): boolean {
    const c = this.cmds.get(id);
    return !!c && (c.when ? c.when() : true);
  }

  /** Runs a command. Unknown ids throw; disabled commands resolve to undefined. */
  async run(id: string, ...args: unknown[]): Promise<unknown> {
    const c = this.cmds.get(id);
    if (!c) throw new Error(`unknown command: ${id}`);
    if (c.when && !c.when()) return undefined;
    return await c.run(...args);
  }

  onDidChange(fn: () => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  private changed(): void {
    for (const fn of this.listeners) fn();
  }
}

export const commands = new CommandRegistry();
