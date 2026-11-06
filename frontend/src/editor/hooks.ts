// Minimal synchronous and asynchronous hook lists used by the editor to let
// features extend open/save/close without editing the manager.

export class Hook<T> {
  private fns = new Set<(v: T) => void>();

  on(fn: (v: T) => void): () => void {
    this.fns.add(fn);
    return () => this.fns.delete(fn);
  }

  emit(v: T): void {
    for (const fn of [...this.fns]) {
      try {
        fn(v);
      } catch (err) {
        console.error('hook', err);
      }
    }
  }
}

export class AsyncHook<T> {
  private fns: ((v: T) => Promise<void> | void)[] = [];

  on(fn: (v: T) => Promise<void> | void): () => void {
    this.fns.push(fn);
    return () => {
      this.fns = this.fns.filter((f) => f !== fn);
    };
  }

  /** Runs handlers in registration order; a throwing handler is logged and
   * does not stop the others. */
  async run(v: T): Promise<void> {
    for (const fn of [...this.fns]) {
      try {
        await fn(v);
      } catch (err) {
        console.error('hook', err);
      }
    }
  }
}
