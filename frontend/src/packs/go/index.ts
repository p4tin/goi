// Goi's frontend pack: the Go-specific features and the product identity.

import type { FrontendPack } from '../../pack';
import icon from './icon.svg';

const pack: FrontendPack = {
  product: {
    name: __PRODUCT_NAME__,
    tagline: 'A cross-platform Go IDE',
    icon,
    site: 'https://github.com/p4tin/goi',
  },
  features: [],
};

export default pack;
