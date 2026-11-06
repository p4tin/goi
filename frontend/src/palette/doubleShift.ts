// Detects a double tap of Shift: press+release, press+release within
// timeoutMs, with no other key in between. Key repeat is ignored.

export interface KeyLike {
  type: 'keydown' | 'keyup';
  key: string;
  repeat?: boolean;
}

export interface DoubleShift {
  handle(e: KeyLike, now?: number): void;
}

export function createDoubleShift(onTrigger: () => void, timeoutMs = 300): DoubleShift {
  // taps: completed clean Shift taps; lastTapAt: time the last tap ended.
  let taps = 0;
  let lastTapAt = 0;
  let shiftDown = false;
  let dirty = false; // another key was pressed while Shift was down

  return {
    handle(e, now = Date.now()) {
      if (e.repeat) return;
      if (e.key !== 'Shift') {
        if (e.type === 'keydown') {
          taps = 0;
          dirty = shiftDown;
        }
        return;
      }
      if (e.type === 'keydown') {
        if (taps > 0 && now - lastTapAt > timeoutMs) taps = 0;
        shiftDown = true;
        dirty = false;
        return;
      }
      // Shift keyup
      if (!shiftDown) return;
      shiftDown = false;
      if (dirty) {
        taps = 0;
        return;
      }
      if (taps > 0 && now - lastTapAt > timeoutMs) taps = 0;
      taps++;
      lastTapAt = now;
      if (taps >= 2) {
        taps = 0;
        onTrigger();
      }
    },
  };
}
