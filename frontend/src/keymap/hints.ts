// Connects keymap display hints to the Search Everywhere palette.

import { setKeyHintProvider } from '../palette/palette';
import { keyHintFor } from './install';

export function installKeyHints(): void {
  setKeyHintProvider(keyHintFor);
}
