/**
 * Live wire check — BYOK over the real Go app.www handlers.
 *
 * Gated on SMOKE_LIVE=1 with a portal up (cmd/www-smoke on :5193); inert in the
 * normal unit run.  Proves the device EncryptKey auto-installs on a real login
 * and that a sealed BYOK secret survives a real tx → read round-trip opaque,
 * then reopens.  Complements scripts/smoke.mjs (the dist-built harness).
 *
 *   (cd amp.planet && SMOKE_ADDR=127.0.0.1:5193 go run ./cmd/www-smoke) &
 *   SMOKE_LIVE=1 npx vitest run src/live-wire.test.ts
 */

import { describe, expect, it } from 'vitest';
import { secp256k1 } from '@noble/curves/secp256k1';
import { keccak_256 } from '@noble/hashes/sha3';
import { bytesToHex } from '@noble/hashes/utils';

import { AmpVaultAdapter } from './vault-adapter';
import { base64ToBytes, bytesToBase64 } from './crypto/base64';

// Read env without @types/node — the SDK stays browser-typed.
const env = (globalThis as { process?: { env: Record<string, string | undefined> } }).process?.env ?? {};
const VAULT = env.VAULT_URL || 'http://127.0.0.1:5193';
const PLANET = env.SMOKE_PLANET || 'smoke-planet';
const run = env.SMOKE_LIVE ? describe : describe.skip;

function ethAddress(pubUncompressed: Uint8Array): string {
  return '0x' + bytesToHex(keccak_256(pubUncompressed.slice(1)).slice(-20));
}

// EIP-191 personal_sign, matching app.metamask.WrapPersonalSign.
function personalSign(message: string, priv: Uint8Array): string {
  const msg = new TextEncoder().encode(message);
  const prefix = new TextEncoder().encode(`\x19Ethereum Signed Message:\n${msg.length}`);
  const wrapped = new Uint8Array(prefix.length + msg.length);
  wrapped.set(prefix, 0);
  wrapped.set(msg, prefix.length);
  const sig = secp256k1.sign(keccak_256(wrapped), priv);
  const out = new Uint8Array(65);
  out.set(sig.toCompactRawBytes(), 0);
  out[64] = sig.recovery;
  return '0x' + bytesToHex(out);
}

run('live BYOK over app.www', () => {
  it('auto-installs the device key on login and round-trips a sealed secret', async () => {
    const priv = secp256k1.utils.randomPrivateKey();
    const address = ethAddress(secp256k1.getPublicKey(priv, false));
    const amp = new AmpVaultAdapter({ vaultUrl: VAULT, planetTag: PLANET });

    const ch = await amp.getWalletChallenge();
    await amp.login({ scheme: 'wallet', address, signature: personalSign(ch.message, priv), nonce: ch.nonce });

    // No setEncryptKey: seal-to-self uses the auto-installed device key.
    const secret = new TextEncoder().encode('CESIUM-ION-LIVE-' + Math.random().toString(36).slice(2));
    const sealedB64 = bytesToBase64(await amp.seal(secret));

    const [res] = await amp.tx([
      { kind: 'create', channel: 'users', attr: 'api_keys_overrides', value: { cesium: sealedB64 } },
    ]);
    const back = await amp.query<{ cesium: string }>('users', 'api_keys_overrides', { itemID: res.itemID });
    expect(back.data[0]?.cesium).toBe(sealedB64); // host stored the bytes opaque

    const opened = await amp.open(base64ToBytes(back.data[0]!.cesium));
    expect(new TextDecoder().decode(opened)).toBe(new TextDecoder().decode(secret));

    await amp.logout();
  });
});
