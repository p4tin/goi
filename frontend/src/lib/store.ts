// Minimal reactive primitives: signal, computed, effect, batch.
// Reads inside an effect are tracked; writes re-run dependent effects.

interface Observer {
  run(): void;
  deps: Set<Set<Observer>>;
}

export interface ReadSignal<T> {
  (): T;
  /** Read without tracking. */
  peek(): T;
}

export interface Signal<T> extends ReadSignal<T> {
  set(value: T): void;
  update(fn: (value: T) => T): void;
}

let current: Observer | null = null;
let batchDepth = 0;
const pending = new Set<Observer>();

function flush(): void {
  while (pending.size > 0) {
    const list = [...pending];
    pending.clear();
    for (const o of list) o.run();
  }
}

function notify(subs: Set<Observer>): void {
  for (const o of subs) pending.add(o);
  if (batchDepth === 0) flush();
}

function untrack(o: Observer): void {
  for (const s of o.deps) s.delete(o);
  o.deps.clear();
}

export function signal<T>(initial: T, equals: (a: T, b: T) => boolean = Object.is): Signal<T> {
  let value = initial;
  const subs = new Set<Observer>();
  const read = (() => {
    if (current) {
      subs.add(current);
      current.deps.add(subs);
    }
    return value;
  }) as Signal<T>;
  read.peek = () => value;
  read.set = (next: T) => {
    if (equals(value, next)) return;
    value = next;
    notify(subs);
  };
  read.update = (fn) => read.set(fn(value));
  return read;
}

/** Runs fn now and whenever a signal it read changes. Returns a disposer. */
export function effect(fn: () => void | (() => void)): () => void {
  let cleanup: void | (() => void);
  let disposed = false;
  const o: Observer = {
    deps: new Set(),
    run() {
      if (disposed) return;
      untrack(o);
      if (typeof cleanup === 'function') cleanup();
      const prev = current;
      current = o;
      try {
        cleanup = fn();
      } finally {
        current = prev;
      }
    },
  };
  o.run();
  return () => {
    disposed = true;
    untrack(o);
    if (typeof cleanup === 'function') cleanup();
  };
}

/** A derived, read-only signal. */
export function computed<T>(fn: () => T): ReadSignal<T> {
  const s = signal<T>(undefined as T);
  effect(() => s.set(fn()));
  const read = (() => s()) as ReadSignal<T>;
  read.peek = s.peek;
  return read;
}

/** Defers effect re-runs until fn returns. */
export function batch(fn: () => void): void {
  batchDepth++;
  try {
    fn();
  } finally {
    batchDepth--;
    if (batchDepth === 0) flush();
  }
}
