/// <reference types="vite/client" />

/** The app's display name, from apps/<app>/build/config.yml (vite.config.ts). */
declare const __PRODUCT_NAME__: string;
/** The app id the frontend was built for ("goi", "pyi"). */
declare const __APP_ID__: string;
/** info.description of the app's build/config.yml. */
declare const __PRODUCT_TAGLINE__: string;
/** The app's project page (vite.config.ts). */
declare const __PRODUCT_SITE__: string;

declare module 'virtual:pack-icon' {
  const url: string;
  export default url;
}

declare module 'virtual:pack' {
  import type { FrontendPack } from './pack';
  const pack: FrontendPack;
  export default pack;
}
