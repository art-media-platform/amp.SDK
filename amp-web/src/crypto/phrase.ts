/**
 * safe.Phrase codec — the canonical identity-seed export vocabulary
 * (SD-did-identity §12.1): a phrase written down in one AMP app restores in
 * any other.
 *
 * Byte-compatible with amp.SDK/stdlib/safe/phrase.go: each word carries 8
 * bits, the trailing PhraseChecksumSize words are the leading bytes of the
 * BLAKE2s-256 digest of the entropy, and purpose-specific keys derive via
 * HKDF-SHA256.  The wordlist is the generated Phrase.Words const
 * (safe.consts.sdl) — never a bespoke list.
 */

import { blake2s } from '@noble/hashes/blake2s';

import { Phrase } from '../generated/safe.consts.js';
import { deriveKey } from './aead.js';

/** Number of words in the canonical wordlist (8 bits/word → byte-aligned entropy). */
export const PhraseWordCount = 256;

/** Checksum bytes appended before encoding; each adds one word to the phrase. */
export const PhraseChecksumSize = 4;

// Wordlist init mirrors phrase.go's phraseLookup guards: exactly
// PhraseWordCount unique words or the module refuses to load.
const wordlist: readonly string[] = Phrase.Words.split(/\s+/).filter((word) => word.length > 0);
const wordIndex = new Map<string, number>();
if (wordlist.length !== PhraseWordCount) {
  throw new Error(`safe: Phrase.Words must contain exactly ${PhraseWordCount} words; got ${wordlist.length}`);
}
for (const [idx, word] of wordlist.entries()) {
  if (wordIndex.has(word)) {
    throw new Error(`safe: Phrase.Words contains duplicate: ${word}`);
  }
  wordIndex.set(word, idx);
}

/** Canonical word at `idx` (0..PhraseWordCount-1).  Throws if out of range. */
export function phraseWordAt(idx: number): string {
  const word = wordlist[idx];
  if (word === undefined) {
    throw new Error(`safe: phrase word index ${idx} out of range`);
  }
  return word;
}

/** Index of `word` in the canonical wordlist, or -1.  Case-sensitive. */
export function phraseWordIndex(word: string): number {
  return wordIndex.get(word) ?? -1;
}

function phraseDigest(entropy: Uint8Array): Uint8Array {
  return blake2s(entropy);
}

/**
 * Encode entropy as a phrase, appending the checksum.  The returned phrase
 * has entropy.length + PhraseChecksumSize words.
 */
export function encodePhrase(entropy: Uint8Array): string[] {
  const digest = phraseDigest(entropy);
  const out: string[] = [];
  for (const bite of entropy) {
    out.push(phraseWordAt(bite));
  }
  for (let pos = 0; pos < PhraseChecksumSize; pos++) {
    out.push(phraseWordAt(digest[pos]));
  }
  return out;
}

/** Decode a phrase back to its entropy after verifying the checksum.  Throws on failure. */
export function decodePhrase(words: readonly string[]): Uint8Array {
  if (words.length <= PhraseChecksumSize) {
    throw new Error(`safe: phrase too short (got ${words.length} words, need > ${PhraseChecksumSize})`);
  }
  const raw = new Uint8Array(words.length);
  try {
    for (const [pos, word] of words.entries()) {
      const idx = phraseWordIndex(word);
      if (idx < 0) {
        throw new Error(`safe: unknown phrase word ${JSON.stringify(word)}`);
      }
      raw[pos] = idx;
    }
    const cut = raw.length - PhraseChecksumSize;
    const entropy = raw.subarray(0, cut);
    const supplied = raw.subarray(cut);
    const expect = phraseDigest(entropy);
    let diff = 0;
    for (let pos = 0; pos < PhraseChecksumSize; pos++) {
      diff |= supplied[pos] ^ expect[pos];
    }
    if (diff !== 0) {
      throw new Error('safe: phrase checksum mismatch');
    }
    return new Uint8Array(entropy);
  } finally {
    raw.fill(0);
  }
}

/**
 * Split a whitespace-separated phrase string into words.  Case is normalized;
 * surrounding and internal whitespace runs are collapsed.
 */
export function parsePhrase(input: string): string[] {
  return input.toLowerCase().split(/\s+/).filter((word) => word.length > 0);
}

/** Join a phrase into a single space-separated string. */
export function phraseToString(words: readonly string[]): string {
  return words.join(' ');
}

/**
 * Derive a purpose-specific 32-byte key from the phrase's entropy
 * (HKDF-SHA256, no salt, purpose as info — safe.DeriveSubKey).  The phrase is
 * verified as a side-effect; a bad checksum throws without producing key
 * material.
 */
export function derivePhraseKey(words: readonly string[], purpose: string): Uint8Array {
  const entropy = decodePhrase(words);
  try {
    return deriveKey(entropy, null, new TextEncoder().encode(purpose));
  } finally {
    entropy.fill(0);
  }
}
