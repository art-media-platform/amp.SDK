// End-to-end smoke test for @amp/web against a live amp.exe app.www portal.
//
// Drives the compiled AmpVaultAdapter (../dist) through the full wire surface:
// wallet login → tx batch → list/read → upsert → subscribe → withdraw →
// tag resolve → upload → media resolve.  Run against `cmd/www-smoke`.
//
//   node scripts/smoke.mjs        (expects a portal on http://127.0.0.1:5193)

import { secp256k1 } from '@noble/curves/secp256k1';
import { keccak_256 } from '@noble/hashes/sha3';
import { bytesToHex } from '@noble/hashes/utils';
import { AmpVaultAdapter } from '../dist/index.js';

const VAULT = process.env.VAULT_URL || 'http://127.0.0.1:5193';
const PLANET = process.env.SMOKE_PLANET || 'smoke-planet';

let pass = 0;
let fail = 0;
function check(name, cond, extra = '') {
  if (cond) { pass++; console.log(`  ✓ ${name}`); }
  else { fail++; console.error(`  ✗ ${name} ${extra}`); }
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
  const amp = new AmpVaultAdapter({ vaultUrl: VAULT, planetTag: PLANET });

  // ── Identity ──
  const ch = await amp.getWalletChallenge();
  check('challenge has nonce + message', !!ch.nonce && !!ch.message);
  const member = await amp.login({
    scheme: 'wallet',
    address,
    signature: personalSign(ch.message, priv),
    nonce: ch.nonce,
  });
  check('login returns member.id', !!member.id);
  check('member.address echoes wallet', (member.address || '').toLowerCase() === address.toLowerCase(), `got ${member.address}`);

  // ── Batched write (the Maplable debounced-save shape) ──
  const ops = Array.from({ length: 50 }, (_, i) => ({
    kind: 'create', channel: 'projects', attr: 'labels', value: { lat: i, lon: i * 2 },
  }));
  const results = await amp.tx(ops);
  check('tx returns 50 results', results.length === 50, `got ${results.length}`);

  // ── List + single read ──
  const list = await amp.query('projects', 'labels', { limit: 100 });
  check('list returns 50 items', list.data.length === 50, `got ${list.data.length}`);
  check('list hasMore=false', list.hasMore === false);
  check('item value spread onto row', list.data[0]?.lat !== undefined);
  check('item carries _itemID meta', !!list.data[0]?._itemID);

  const id = results[0].itemID;
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
  check('update frame carries fromID', !!updateEvent?.fromID);
  unsub();

  // ── Withdraw (distinct from delete) ──
  await amp.withdraw('projects', 'labels', id, { reason: 'Retracted', rationale: 'smoke' });
  const afterWd = await amp.query('projects', 'labels', { itemID: id });
  check('withdraw note surfaces on read', afterWd.data[0]?._withdrawn?.reason === 'Retracted', JSON.stringify(afterWd.data[0]?._withdrawn));

  // ── Tag resolution (server canonicalization) ──
  const tr = await amp.resolveTag('amp.member.profile');
  check('resolveTag canonic round-trips', tr.canonic === 'amp.member.profile' && !!tr.id, JSON.stringify(tr));
  const trs = await amp.resolveTags(['projects.labels', 'users.profile']);
  check('resolveTags returns 2', trs.length === 2);

  // ── Blob upload + caller-carries-the-Tag resolve ──
  const file = new File([new Uint8Array([1, 2, 3, 4])], 'smoke.bin', { type: 'application/octet-stream' });
  const blob = await amp.upload(file, 'projects', { attr: 'media' });
  check('upload returns blob id', !!blob.id, JSON.stringify(blob));
  const resolved = await amp.resolveMedia({ id: blob.id, contentType: blob.contentType, byteSize: blob.byteSize });
  check('resolveMedia fills streamURL', !!resolved.streamURL, JSON.stringify(resolved));

  await amp.logout();

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
}

main().catch((err) => {
  console.error('smoke crashed:', err);
  process.exit(2);
});
