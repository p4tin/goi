// The product this bundle was built for: name and tagline from
// apps/<app>/build/config.yml, icon from the app's frontend pack
// (src/packs/<pack>/icon.svg), project page from vite.config.ts. Unlike
// pack.ts it imports nothing from the pack, so any module (including ones
// the pack itself imports) can read it at load time.

import icon from 'virtual:pack-icon';

/** The product shown in About, Welcome and window titles. */
export interface Product {
  /** Display name ("Goi"). */
  name: string;
  /** One-line description ("A cross-platform Go IDE"). */
  tagline: string;
  /** URL of the app icon. */
  icon: string;
  /** Project page opened from About. */
  site: string;
}

export const product: Product = {
  name: __PRODUCT_NAME__,
  tagline: __PRODUCT_TAGLINE__,
  icon,
  site: __PRODUCT_SITE__,
};
