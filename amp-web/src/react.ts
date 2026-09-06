/**
 * @art-media-platform/web/react — the React surface: <AmpProvider> and the
 * hooks.  The root entry (`@art-media-platform/web`) is dependency-free so a
 * React-free bundler consumer or a Node harness imports AmpWebClient and the
 * codecs without React; React apps import this subpath alongside it.
 */

export { AmpProvider, useAmpClient } from './provider.js';
export type { AmpProviderProps } from './provider.js';

export { useAmpAuth } from './hooks/useAmpAuth.js';
export { useAmpQuery } from './hooks/useAmpQuery.js';
export { useAmpMutation } from './hooks/useAmpMutation.js';
export { useAmpUpload } from './hooks/useAmpUpload.js';
export { useAmpMedia } from './hooks/useAmpMedia.js';
export { useAmpCrypto } from './hooks/useAmpCrypto.js';
export { useAmpResolve } from './hooks/useAmpResolve.js';
export { useAmpBrand } from './hooks/useAmpBrand.js';
