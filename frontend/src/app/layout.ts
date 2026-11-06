// Per-project layout persistence. Features register named parts; the store
// serializes them together and restores them when a project opens.

export interface LayoutPart<T> {
  save(): T;
  restore(value: T): void;
}

export interface LayoutBackend {
  save(layout: Record<string, unknown>): Promise<void>;
  load(): Promise<Record<string, unknown> | null>;
}

export class LayoutStore {
  private parts = new Map<string, LayoutPart<unknown>>();
  private loaded: Record<string, unknown> = {};
  private timer: ReturnType<typeof setTimeout> | null = null;

  constructor(
    private backend: LayoutBackend,
    private delayMs = 1000,
  ) {}

  /** Registers a part; if a saved value exists it is restored immediately. */
  register<T>(key: string, part: LayoutPart<T>): () => void {
    this.parts.set(key, part as LayoutPart<unknown>);
    if (key in this.loaded) {
      try {
        part.restore(this.loaded[key] as T);
      } catch (err) {
        console.warn('layout restore', key, err);
      }
    }
    return () => this.parts.delete(key);
  }

  /** Loads saved layout and restores every registered part. */
  async load(): Promise<void> {
    this.loaded = (await this.backend.load()) ?? {};
    for (const [key, part] of this.parts) {
      if (key in this.loaded) {
        try {
          part.restore(this.loaded[key]);
        } catch (err) {
          console.warn('layout restore', key, err);
        }
      }
    }
  }

  snapshot(): Record<string, unknown> {
    const out: Record<string, unknown> = { ...this.loaded };
    for (const [key, part] of this.parts) out[key] = part.save();
    return out;
  }

  /** Debounced save. */
  scheduleSave(): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      this.timer = null;
      void this.flush();
    }, this.delayMs);
  }

  async flush(): Promise<void> {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    await this.backend.save(this.snapshot());
  }
}
