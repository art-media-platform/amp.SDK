/**
 * @art-media-platform/web — TypeScript SDK for art.media.platform web apps.
 *
 * Wrap your app with <AmpProvider client={new AmpWebClient({...})}> and use
 * the hooks.  The adapter speaks the amp.exe `app.www` wire contract
 * (amp.SDK/amp/webapi); all reads, writes, uploads, and subscriptions go
 * through it.
 */

// Core types
export type {
  AmpAuth,
  AmpItemMeta,
  AmpMediaResult,
  AmpMember,
  AmpMutationResult,
  AmpQueryOpts,
  AmpQueryResult,
  AmpUploadResult,
  Address,
  BlobRef,
  EmailCredential,
  LoginCredentials,
  SubscriptionEvent,
  TagResolution,
  TxOp,
  TxOpKind,
  TxResult,
  UploadOpts,
  WalletChallenge,
  WithdrawNote,
  WithdrawOpts,
  WithdrawReason,
} from './types.js';

// Adapter interface + implementation
export type { AmpAdapter } from './adapter.js';
export { AmpWebClient } from './web-client.js';
export type { AmpWebClientOpts } from './web-client.js';

// Typed errors
export { AmpError, AmpErrorCode } from './errors.js';

// Provider
export { AmpProvider } from './provider.js';
export type { AmpProviderProps } from './provider.js';

// Hooks
export { useAmpAuth } from './hooks/useAmpAuth.js';
export { useAmpQuery } from './hooks/useAmpQuery.js';
export { useAmpMutation } from './hooks/useAmpMutation.js';
export { useAmpUpload } from './hooks/useAmpUpload.js';
export { useAmpMedia } from './hooks/useAmpMedia.js';
export { useAmpCrypto } from './hooks/useAmpCrypto.js';

// Sealed-box BYOK
export { CryptoKitID, base64ToBytes, bytesToBase64, createAmpCrypto, getKit, open, registerKit, seal } from './crypto/index.js';
export type { AmpCrypto, KeyPair, KitOps, PubKeyRef } from './crypto/index.js';

// Device-local EncryptKey storage (auto-managed on login; override to customize)
export {
  IndexedDBKeyStorage,
  MemoryKeyStorage,
  defaultEncryptKeyStorage,
  resolveDeviceEncryptKey,
} from './crypto/keystore.js';
export type { EncryptKeyStorage } from './crypto/keystore.js';
