// Runs before every test file (vite.config.ts test.setupFiles).
//
// Test files that import lib/api load @wailsio/runtime, whose drag module
// polls every 50 ms after it loads and clears that interval on its first
// tick. A file that finishes sooner tears down `window` first, and the tick
// then throws "window is not defined" as an unhandled error, which fails
// the run (seen on CI, timing-dependent). Waiting past one poll lets it
// clear itself.
import { afterAll } from 'vitest';

afterAll(() => new Promise<void>((resolve) => setTimeout(resolve, 60)));
