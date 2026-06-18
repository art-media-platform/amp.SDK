/**
 * Tunr onboarding dogfood — mirrors Charlie/Maplable's path (SKILL §0 + §4.7)
 * against a live node, as the "Tunr" app: wallet (SIWE) login -> acceptInvite ->
 * hello-world create/query.  Gated on DOGFOOD=1; inert in the normal unit run.
 *
 *   DOGFOOD=1 VAULT_URL=https://prod.plan.tools \
 *     TUNR_INVITE_FILE=/tmp/tunr.invite TUNR_PASS=… \
 *     npx vitest run src/tunr-onboard.live.test.ts
 */
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { secp256k1 } from '@noble/curves/secp256k1';
import { keccak_256 } from '@noble/hashes/sha3';
import { bytesToHex } from '@noble/hashes/utils';

import { AmpWebClient } from './web-client.js';

const env = (globalThis as { process?: { env: Record<string, string | undefined> } }).process?.env ?? {};
const VAULT = env.VAULT_URL || 'https://prod.plan.tools';
const INVITE = env.TUNR_INVITE_FILE ? readFileSync(env.TUNR_INVITE_FILE, 'utf8') : (env.TUNR_INVITE || '');
const PASS = env.TUNR_PASS || '';
const run = env.DOGFOOD ? describe : describe.skip;

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

run('Tunr onboarding against a live node (mirror Charlie)', () => {
  it('login -> acceptInvite -> hello-world round-trip', async () => {
    const priv = secp256k1.utils.randomPrivateKey();
    const address = ethAddress(secp256k1.getPublicKey(priv, false));
    const tunr = new AmpWebClient({ vaultUrl: VAULT, planetTag: '' });

    // SKILL §0/§2 — wallet (SIWE) login; auto-provisions the Tunr home planet.
    const ch = await tunr.getWalletChallenge(address);
    const member = await tunr.login({ Scheme: 'wallet', Address: address, Signature: personalSign(ch.Message, priv), Nonce: ch.Nonce });
    console.log('[Tunr] logged in: member', member.ID, 'home planet', member.PlanetID);

    // SKILL §4.7 — join Federation Alpha with the sealed invite.
    const joined = await tunr.acceptInvite({ inviteText: INVITE, passphrase: PASS });
    console.log('[Tunr] joined federation planet', joined.PlanetID, 'as member', joined.MemberID);
    expect(joined.PlanetID).toBeTruthy();
    expect(joined.MemberID).toBeTruthy();

    // SKILL §0 — hello-world Tx on the home planet, read straight back.
    const itemID = await tunr.create('hello', 'world', { msg: 'hi from Tunr' });
    const back = await tunr.query<{ msg: string }>('hello', 'world', { itemID });
    console.log('[Tunr] hello-world round-trip ->', JSON.stringify(back.data[0]?.msg ?? back.data[0]));
    expect(back.data[0]?.msg).toBe('hi from Tunr');

    await tunr.logout();
  }, 30000);
});
