// Tiny DOM helpers: h() element builder and a Disposables bag.

export type Child = Node | string | number | null | undefined | false | Child[];

export type Props = {
  class?: string;
  style?: string | Partial<Record<keyof CSSStyleDeclaration, string>>;
  dataset?: Record<string, string>;
  [key: string]: unknown;
};

export function h<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  props?: Props | null,
  ...children: Child[]
): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  if (props) applyProps(el, props);
  append(el, children);
  return el;
}

export function applyProps(el: HTMLElement, props: Props): void {
  for (const [key, value] of Object.entries(props)) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class') {
      el.className = String(value);
    } else if (key === 'style') {
      if (typeof value === 'string') el.style.cssText = value;
      else Object.assign(el.style, value);
    } else if (key === 'dataset') {
      Object.assign(el.dataset, value as Record<string, string>);
    } else if (key.startsWith('on') && typeof value === 'function') {
      el.addEventListener(key.slice(2).toLowerCase(), value as EventListener);
    } else if (key in el && typeof value !== 'string') {
      (el as unknown as Record<string, unknown>)[key] = value;
    } else {
      el.setAttribute(key, value === true ? '' : String(value));
    }
  }
}

export function append(parent: Node, children: Child[]): void {
  for (const c of children) {
    if (c === null || c === undefined || c === false) continue;
    if (Array.isArray(c)) append(parent, c);
    else if (c instanceof Node) parent.appendChild(c);
    else parent.appendChild(document.createTextNode(String(c)));
  }
}

/** Collects teardown functions and runs them once, in reverse order. */
export class Disposables {
  private fns: (() => void)[] = [];

  add(fn: () => void): void {
    this.fns.push(fn);
  }

  dispose(): void {
    const fns = this.fns;
    this.fns = [];
    for (let i = fns.length - 1; i >= 0; i--) fns[i]();
  }
}
