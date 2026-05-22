/**
 * @art-media-platform/web — TypeScript SDK for art.media.platform web apps.
 *
 * Wrap your app with <AmpProvider adapter={new AmpVaultAdapter({...})}> and use
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
  BlobRef,
  CitationRef,
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
} from './types';

// Adapter interface + implementation
export type { AmpAdapter } from './adapter';
export { AmpVaultAdapter } from './vault-adapter';
export type { AmpVaultAdapterOpts } from './vault-adapter';

// Provider
export { AmpProvider } from './provider';
export type { AmpProviderProps } from './provider';

// Hooks
export { useAmpAuth } from './hooks/useAmpAuth';
export { useAmpQuery } from './hooks/useAmpQuery';
export { useAmpMutation } from './hooks/useAmpMutation';
export { useAmpUpload } from './hooks/useAmpUpload';
export { useAmpMedia } from './hooks/useAmpMedia';
export { useAmpCrypto } from './hooks/useAmpCrypto';

// Sealed-box BYOK
export { CryptoKitID, base64ToBytes, bytesToBase64, createAmpCrypto, getKit, open, registerKit, seal } from './crypto';
export type { AmpCrypto, KeyPair, KitOps, PubKeyRef } from './crypto';

// Device-local EncryptKey storage (auto-managed on login; override to customize)
export {
  IndexedDBKeyStorage,
  MemoryKeyStorage,
  defaultEncryptKeyStorage,
  resolveDeviceEncryptKey,
} from './crypto/keystore';
export type { EncryptKeyStorage } from './crypto/keystore';
