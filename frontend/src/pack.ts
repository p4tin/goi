// The frontend pack of the app this bundle was built for. vite.config.ts
// resolves 'virtual:pack' to src/packs/<pack>/index.ts from the build mode
// (`--mode goi` → packs/go). Core code reads the product and the pack's
// features from here and never imports a pack directly.

import type { Workbench } from './app/workbench';
import pack from 'virtual:pack';

/** The product shown in About, Welcome and window titles. */
export interface Product {
  /** Display name ("Goi"), from apps/<app>/build/config.yml. */
  name: string;
  /** One-line description ("A cross-platform Go IDE"). */
  tagline: string;
  /** URL of the app icon (an imported asset). */
  icon: string;
  /** Project page opened from About. */
  site: string;
}

/** A pack feature and where it goes in the feature order: right after the
 * core feature (or earlier pack feature) with id `after`. Order matters for
 * dock defaults (the first panel registered in a dock is its default tab). */
export interface PackFeature {
  id: string;
  after: string;
  install: (wb: Workbench) => void;
}

/** What a frontend pack exports (src/packs/<pack>/index.ts). */
export interface FrontendPack {
  product: Product;
  features: PackFeature[];
}

export const product: Product = pack.product;
export const packFeatures: PackFeature[] = pack.features;
