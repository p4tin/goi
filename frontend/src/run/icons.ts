// Small inline SVG icons for run/debug toolbars. Static markup only; colors
// come from currentColor so CSS theme tokens apply.

const svg = (body: string): string =>
  `<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`;

export const icons = {
  run: svg('<path d="M4.5 3v10l8-5z" fill="currentColor" stroke="none"/>'),
  debug: svg('<ellipse cx="8" cy="9" rx="3.5" ry="4.5"/><path d="M8 4.5V13.5M4.5 9H2M14 9h-2.5M5 5.5 3.5 4M11 5.5 12.5 4M5 12.5 3.5 14M11 12.5 12.5 14M6.5 4.5a1.5 1.5 0 0 1 3 0"/>'),
  stop: svg('<rect x="4" y="4" width="8" height="8" rx="1" fill="currentColor" stroke="none"/>'),
  rerun: svg('<path d="M12.5 8a4.5 4.5 0 1 1-1.3-3.2"/><path d="M12.5 2.5v3h-3"/>'),
  rerunFailed: svg('<path d="M12.5 8a4.5 4.5 0 1 1-1.3-3.2"/><path d="M12.5 2.5v3h-3"/><circle cx="8" cy="8" r="1.6" fill="currentColor" stroke="none"/>'),
  wrap: svg('<path d="M2.5 4h11M2.5 8h9a2 2 0 0 1 0 4H8m0 0 1.5-1.5M8 12l1.5 1.5M2.5 12h3"/>'),
  scrollEnd: svg('<path d="M8 2.5v9M4.5 8 8 11.5 11.5 8M3.5 13.5h9"/>'),
  clear: svg('<path d="M3 4.5h10M6 4.5V3h4v1.5M4.5 4.5l.7 8.5h5.6l.7-8.5"/>'),
  search: svg('<circle cx="7" cy="7" r="4"/><path d="m10 10 3.5 3.5"/>'),
  close: svg('<path d="m4.5 4.5 7 7M11.5 4.5l-7 7"/>'),
  chevron: svg('<path d="m5 6.5 3 3 3-3"/>'),
  resume: svg('<path d="M4 3v10" /><path d="M7 3v10l6.5-5z" fill="currentColor" stroke="none"/>'),
  pause: svg('<path d="M5.5 3.5v9M10.5 3.5v9" stroke-width="2"/>'),
  stepOver: svg('<path d="M2.5 9a5 5 0 0 1 10-1.5"/><path d="M13 4.5v3.3H9.7"/><circle cx="8" cy="12.5" r="1.3" fill="currentColor" stroke="none"/>'),
  stepInto: svg('<path d="M8 2v7M5 6.5 8 9.5 11 6.5"/><circle cx="8" cy="12.8" r="1.3" fill="currentColor" stroke="none"/>'),
  forceStepInto: svg('<path d="M8 2v7M5 6.5 8 9.5 11 6.5M4 2h8"/><circle cx="8" cy="12.8" r="1.3" fill="currentColor" stroke="none"/>'),
  stepOut: svg('<path d="M8 10V3M5 6 8 3l3 3"/><circle cx="8" cy="12.8" r="1.3" fill="currentColor" stroke="none"/>'),
  runToCursor: svg('<path d="M2.5 8h7M7 5l3 3-3 3"/><path d="M12.5 4v8"/>'),
  breakpoints: svg('<circle cx="6" cy="8" r="3" fill="currentColor" stroke="none"/><circle cx="11" cy="8" r="3"/>'),
  mute: svg('<circle cx="8" cy="8" r="4"/><path d="m3 13 10-10"/>'),
  plus: svg('<path d="M8 3v10M3 8h10"/>'),
  minus: svg('<path d="M3 8h10"/>'),
  copy: svg('<rect x="5" y="5" width="8" height="8" rx="1"/><path d="M3 10.5V3h7.5"/>'),
  up: svg('<path d="m4.5 9.5 3.5-3.5 3.5 3.5"/>'),
  down: svg('<path d="m4.5 6.5 3.5 3.5 3.5-3.5"/>'),
  save: svg('<path d="M3 3h8l2 2v8H3z"/><path d="M5.5 3v3h4V3M5.5 13v-3.5h5V13"/>'),
  filter: svg('<path d="M2.5 3.5h11l-4.3 5v4l-2.4 1v-5z"/>'),
  passed: svg('<circle cx="8" cy="8" r="5.5"/><path d="m5.5 8 1.8 1.8L10.8 6"/>'),
  failed: svg('<circle cx="8" cy="8" r="5.5"/><path d="m6 6 4 4M10 6l-4 4"/>'),
  skipped: svg('<circle cx="8" cy="8" r="5.5"/><path d="M5.5 8h5"/>'),
  running: svg('<circle cx="8" cy="8" r="5.5" stroke-dasharray="3 2.2"/>'),
  expandAll: svg('<path d="m4.5 4.5 3.5 3.5 3.5-3.5M4.5 8.5l3.5 3.5 3.5-3.5"/>'),
  collapseAll: svg('<path d="m4.5 7.5 3.5-3.5 3.5 3.5M4.5 11.5l3.5-3.5 3.5 3.5"/>'),
  eye: svg('<path d="M1.5 8s2.5-4.5 6.5-4.5S14.5 8 14.5 8 12 12.5 8 12.5 1.5 8 1.5 8z"/><circle cx="8" cy="8" r="2"/>'),
  settings: svg('<circle cx="8" cy="8" r="2.2"/><path d="M8 1.8v1.7M8 12.5v1.7M1.8 8h1.7M12.5 8h1.7M3.6 3.6l1.2 1.2M11.2 11.2l1.2 1.2M3.6 12.4l1.2-1.2M11.2 4.8l1.2-1.2"/>'),
};

export type IconName = keyof typeof icons;

/** An icon button. */
export function iconButton(name: IconName, title: string, onClick: () => void, cls = ''): HTMLButtonElement {
  const b = document.createElement('button');
  b.type = 'button';
  b.className = `tb-btn ${cls}`.trim();
  b.title = title;
  b.setAttribute('aria-label', title);
  b.innerHTML = icons[name];
  b.addEventListener('click', (e) => {
    e.stopPropagation();
    onClick();
  });
  return b;
}

/** An icon element (span). */
export function icon(name: IconName, cls = ''): HTMLSpanElement {
  const s = document.createElement('span');
  s.className = `ico ${cls}`.trim();
  s.innerHTML = icons[name];
  return s;
}
