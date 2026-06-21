/**
 * @art-media-platform/web — TypeScript SDK for art.media.platform web apps.
 *
 * Wrap your app with <AmpProvider client={new AmpWebClient({...})}> and use
 * the hooks.  The adapter speaks the `ampd` `app.www` wire contract
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
  InviteAcceptOpts,
  InviteAcceptResult,
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
export { AmpProvider, useAmpClient } from './provider.js';
export type { AmpProviderProps } from './provider.js';

// Hooks
export { useAmpAuth } from './hooks/useAmpAuth.js';
export { useAmpQuery } from './hooks/useAmpQuery.js';
export { useAmpMutation } from './hooks/useAmpMutation.js';
export { useAmpUpload } from './hooks/useAmpUpload.js';
export { useAmpMedia } from './hooks/useAmpMedia.js';
export { useAmpCrypto } from './hooks/useAmpCrypto.js';

// Sealed-box BYOK — seal/open via the session-bound client (client.seal / .open)
// or the useAmpCrypto() hook.  base64 helpers ride sealed bytes through JSON.
export { CryptoKitID, base64ToBytes, bytesToBase64 } from './crypto/index.js';
export type { AmpCrypto, KeyPair, KitOps, PubKeyRef } from './crypto/index.js';

// Device-local EncryptKey storage (auto-managed on login; override to customize)
export {
  IndexedDBKeyStorage,
  MemoryKeyStorage,
  defaultEncryptKeyStorage,
  resolveDeviceEncryptKey,
} from './crypto/keystore.js';
export type { EncryptKeyStorage } from './crypto/keystore.js';

// Card / WebRect bridge (window.amp) — types for card authors; the host (Unity
// WebView / browser shim) injects the implementation.  Importing the package
// also augments the global `Window` with an optional `amp` field.
export type {
  AmpBridge,
  BridgeMember,
  FormPayload,
  ListOpts,
  SubmitResult,
  TxReceipt,
} from './card-bridge.js';
import './card-bridge.js';
