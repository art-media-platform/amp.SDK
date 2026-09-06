/**
 * tag.UID base32 text → the [hi, lo] 64-bit pair (the shape the generated
 * consts carry).  Byte-compatible with Go's tag.UID_ParseBase32: 26 geohash
 * digits, either case, '-' and whitespace ignored; each digit shifts the
 * 128-bit value left by 5.  A wire field that carries a UID as two fixed64
 * halves (forums.Post BaseEdit_0/_1) is built from this pair.
 */

import type { UID } from './generated/amp.std.consts.js';

const BASE32_ALPHABET = '0123456789bcdefghjkmnpqrstuvwxyz';
const UID_BASE32_DIGITS = 26;
const MASK64 = (1n << 64n) - 1n;

export function parseUID(text: string): UID {
  let hi = 0n;
  let lo = 0n;
  let digits = 0;
  for (const ch of text) {
    if (ch === '-' || /\s/.test(ch)) continue;
    const digit = BASE32_ALPHABET.indexOf(ch.toLowerCase());
    if (digit < 0 || digits >= UID_BASE32_DIGITS) {
      throw new Error(`malformed base32 UID: ${JSON.stringify(text)}`);
    }
    hi = ((hi << 5n) | (lo >> 59n)) & MASK64;
    lo = ((lo << 5n) | BigInt(digit)) & MASK64;
    digits++;
  }
  if (digits === 0) {
    throw new Error(`malformed base32 UID: ${JSON.stringify(text)}`);
  }
  return [hi, lo];
}
