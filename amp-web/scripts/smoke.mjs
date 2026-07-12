// End-to-end smoke test for @art-media-platform/web against a live `ampd`
// app.www portal.
//
// Drives the compiled AmpWebClient (../dist) through the full wire surface:
// wallet login → tx batch → list/read → upsert → subscribe → withdraw →
// tag resolve → upload → media resolve.
//
//   VAULT_URL=<portal> node scripts/smoke.mjs   (default http://127.0.0.1:5193)
//
// Needs a portal with a wallet-login dev backend (an AMP-internal fixture;
// SKILL §14.7).  The bundle's offline guard is `npm test` — the drift suite
// runs against the shipped webapi/testdata fixtures with no server.

import { secp256k1 } from '@noble/curves/secp256k1';
import { keccak_256 } from '@noble/hashes/sha3';
import { bytesToHex } from '@noble/hashes/utils';
import { AmpWebClient, base64ToBytes, bytesToBase64 } from '../dist/index.js';

const VAULT = process.env.VAULT_URL || 'http://127.0.0.1:5193';
const PLANET = process.env.SMOKE_PLANET || 'smoke-planet';

let pass = 0;
let fail = 0;
function check(name, cond, extra = '') {
  if (cond) { pass++; console.log(`  ✓ ${name}`); }
  else { fail++; console.error(`  ✗ ${name} ${extra}`); }
}

const dec = (bytes) => new TextDecoder().decode(bytes);

function byteEq(a, b) {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

// True if `needle` appears as a contiguous run in `haystack` — used to assert a
// sealed blob never embeds the plaintext it wraps.
function contains(haystack, needle) {
  if (needle.length === 0) return true;
  for (let i = 0; i + needle.length <= haystack.length; i++) {
    let hit = true;
    for (let j = 0; j < needle.length; j++) {
      if (haystack[i + j] !== needle[j]) { hit = false; break; }
    }
    if (hit) return true;
  }
  return false;
}

function ethAddress(pubUncompressed) {
  const hash = keccak_256(pubUncompressed.slice(1)); // drop the 0x04 prefix byte
  return '0x' + bytesToHex(hash.slice(-20));
}

// EIP-191 personal_sign over `message`, matching app.metamask.WrapPersonalSign.
function personalSign(message, priv) {
  const msgBytes = new TextEncoder().encode(message);
  const prefix = new TextEncoder().encode(`\x19Ethereum Signed Message:\n${msgBytes.length}`);
  const wrapped = new Uint8Array(prefix.length + msgBytes.length);
  wrapped.set(prefix, 0);
  wrapped.set(msgBytes, prefix.length);
  const sig = secp256k1.sign(keccak_256(wrapped), priv);
  const out = new Uint8Array(65);
  out.set(sig.toCompactRawBytes(), 0); // r || s
  out[64] = sig.recovery;              // v (0/1; server accepts 0/1 or 27/28)
  return '0x' + bytesToHex(out);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function main() {
  const priv = secp256k1.utils.randomPrivateKey();
  const address = ethAddress(secp256k1.getPublicKey(priv, false));
  const amp = new AmpWebClient({ vaultUrl: VAULT, planetTag: PLANET });

  // ── Identity ──
  const ch = await amp.getWalletChallenge(address);
  check('challenge has nonce + message', !!ch.Nonce && !!ch.Message);
  const member = await amp.login({
    Scheme: 'wallet',
    Address: address,
    Signature: personalSign(ch.Message, priv),
    Nonce: ch.Nonce,
  });
  check('login returns member.ID', !!member.ID);
  check('member.Address echoes wallet', (member.Address || '').toLowerCase() === address.toLowerCase(), `got ${member.Address}`);

  // ── Batched write (the Maplable debounced-save shape) ──
  const ops = Array.from({ length: 50 }, (_, i) => ({
    Kind: 'create', Channel: 'projects', Attr: 'labels', Value: { lat: i, lon: i * 2 },
  }));
  const results = await amp.tx(ops);
  check('tx returns 50 results', results.length === 50, `got ${results.length}`);

  // ── List + single read ──
  const list = await amp.query('projects', 'labels', { limit: 100 });
  check('list returns 50 items', list.data.length === 50, `got ${list.data.length}`);
  check('list hasMore=false', list.hasMore === false);
  check('item value spread onto row', list.data[0]?.lat !== undefined);
  check('item carries _ItemID meta', !!list.data[0]?._ItemID);

  const id = results[0].ItemID;
  await amp.upsert('projects', 'labels', id, { lat: 99, lon: 99 });
  const single = await amp.query('projects', 'labels', { itemID: id });
  check('single read sees upsert', single.data[0]?.lat === 99, JSON.stringify(single.data[0]));

  // ── Live subscription ──
  let updateEvent = null;
  const unsub = amp.subscribe('projects', 'labels', (e) => { if (e.type === 'update') updateEvent = e; });
  await sleep(400); // allow WS connect + subscribe round-trip
  await amp.create('projects', 'labels', { lat: 7, lon: 7 });
  await sleep(600);
  check('subscribe delivered an update frame', !!updateEvent, JSON.stringify(updateEvent));
  check('update frame carries FromID', !!updateEvent?.FromID);
  unsub();

  // ── Withdraw (distinct from delete) ──
  // WithdrawOpts is the SDK option bag — camelCase by design, never serialized.
  await amp.withdraw('projects', 'labels', id, { reason: 'Retracted', rationale: 'smoke' });
  const afterWd = await amp.query('projects', 'labels', { itemID: id });
  check('withdraw note surfaces on read', afterWd.data[0]?._Withdrawn?.Reason === 'Retracted', JSON.stringify(afterWd.data[0]?._Withdrawn));

  // ── Tag resolution (server canonization) ──
  const tr = await amp.resolveTag('amp.member.profile');
  check('resolveTag canonic round-trips', tr.Canonic === 'amp.member.profile' && !!tr.ID, JSON.stringify(tr));
  const trs = await amp.resolveTags(['projects.labels', 'users.profile']);
  check('resolveTags returns 2', trs.length === 2);

  // ── Blob upload + caller-carries-the-Tag resolve ──
  const file = new File([new Uint8Array([1, 2, 3, 4])], 'smoke.bin', { type: 'application/octet-stream' });
  const blob = await amp.upload(file, 'projects', { attr: 'media' });
  check('upload returns blob UID', !!blob.UID, JSON.stringify(blob));
  const resolved = await amp.resolveMedia({ UID: blob.UID, ContentType: blob.ContentType, I: blob.I, Units: blob.Units });
  check('resolveMedia fills URI', !!resolved.URI, JSON.stringify(resolved));

  // ── Sealed-box BYOK round-trip (device EncryptKey auto-installed at login) ──
  // No setEncryptKey: seal-to-self uses the per-member key the adapter resolved
  // on login.  The sealed bytes ride a channel item as base64 and must come
  // back byte-identical (the host stores them opaque), then reopen on the
  // same device.
  const secret = new TextEncoder().encode('CESIUM-ION-LIVE-' + Math.random().toString(36).slice(2));
  const sealed = await amp.seal(secret);
  check('seal produces opaque bytes (no plaintext substring)', !contains(sealed, secret));
  const sealedB64 = bytesToBase64(sealed);
  const [keyRes] = await amp.tx([
    { Kind: 'create', Channel: 'users', Attr: 'api_keys_overrides', Value: { cesium: sealedB64 } },
  ]);
  const back = await amp.query('users', 'api_keys_overrides', { itemID: keyRes.ItemID });
  const storedB64 = back.data[0]?.cesium;
  check('sealed bytes survive the wire round-trip unchanged', storedB64 === sealedB64);
  const opened = await amp.open(base64ToBytes(storedB64));
  check('open recovers the BYOK secret (same device)', byteEq(opened, secret), dec(opened));

  await amp.logout();

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
}

main().catch((err) => {
  console.error('smoke crashed:', err);
  process.exit(2);
});
