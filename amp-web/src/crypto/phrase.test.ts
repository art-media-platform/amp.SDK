/**
 * Go ↔ TypeScript safe.Phrase interop.
 *
 * The golden vectors were generated ONCE from stdlib/safe (Go) and are checked
 * in as literals both there (phrase_test.go) and here — byte-identical
 * round-trips on BOTH sides are the wordlist's freeze contract: any byte
 * motion in the generated Phrase.Words const trips both suites.
 */

import { describe, expect, it } from 'vitest';
import { hexToBytes } from '@noble/hashes/utils';

import {
  PhraseChecksumSize,
  PhraseWordCount,
  decodePhrase,
  derivePhraseKey,
  encodePhrase,
  parsePhrase,
  phraseToString,
  phraseWordAt,
  phraseWordIndex,
} from './phrase.js';

const GOLDEN_PURPOSE = 'golden.purpose.v1';

const GOLDEN = [
  {
    seed: '000102030405060708090a0b0c0d0e0f',
    words:
      'able acid acre agent album alert alien alloy amber anchor angel ankle apple arena armor artist ' +
      'storm nickel canyon rose',
    key: '1a9b422a2e49008ffb5f7703b8bc0e1eda0af571549213cf23d8c3ff4d5f6a4e',
  },
  {
    seed: '9f8e7d6c5b4a39281706f5e4d3c2b1a0ff00eeddccbbaa998877665544332211',
    words:
      'harbor focus echo dart cloud camel brass black bacon alien torch silver quiet ocean ladder harvest ' +
      'zebra able stone rust petal meadow horn golden field drift crate chime bulb bone bear atom ' +
      'angel green blade laser',
    key: 'c33407ee95831f951c4fed138bd5b1bd89ada41b4487d4a67dfa7bd819ab0670',
  },
];

// GOLDEN[0]'s phrase with word[0] swapped for wordlist[1] — a verified
// checksum mismatch, so rejection is deterministic.
const GOLDEN_REJECT =
  'acid acid acre agent album alert alien alloy amber anchor angel ankle apple arena armor artist ' +
  'storm nickel canyon rose';

describe('Go↔TS safe.Phrase golden vectors', () => {
  it.each(GOLDEN)('encodes seed $seed to the golden words', ({ seed, words }) => {
    expect(phraseToString(encodePhrase(hexToBytes(seed)))).toBe(words);
  });

  it.each(GOLDEN)('decodes the golden words back to seed $seed', ({ seed, words }) => {
    expect(decodePhrase(parsePhrase(words))).toEqual(hexToBytes(seed));
  });

  it.each(GOLDEN)('derives the golden purpose key for seed $seed', ({ words, key }) => {
    expect(derivePhraseKey(parsePhrase(words), GOLDEN_PURPOSE)).toEqual(hexToBytes(key));
  });

  it('rejects the golden word-swap phrase', () => {
    expect(() => decodePhrase(parsePhrase(GOLDEN_REJECT))).toThrow(/checksum mismatch/);
  });
});

describe('safe.Phrase codec', () => {
  it('round-trips random entropy at several sizes', () => {
    for (const size of [1, 16, 32, 64]) {
      const entropy = crypto.getRandomValues(new Uint8Array(size));
      const phrase = encodePhrase(entropy);
      expect(phrase).toHaveLength(size + PhraseChecksumSize);
      expect(decodePhrase(phrase)).toEqual(entropy);
    }
  });

  it('normalizes case and whitespace via parsePhrase', () => {
    const entropy = crypto.getRandomValues(new Uint8Array(16));
    const mangled = `  ${phraseToString(encodePhrase(entropy)).toUpperCase()}   \t\n `;
    expect(decodePhrase(parsePhrase(mangled))).toEqual(entropy);
  });

  it('rejects an unknown word', () => {
    const phrase = encodePhrase(new Uint8Array(16));
    phrase[0] = 'xyzzy';
    expect(() => decodePhrase(phrase)).toThrow(/unknown phrase word/);
  });

  it('rejects too-short phrases', () => {
    expect(() => decodePhrase([])).toThrow(/too short/);
    expect(() => decodePhrase(['able'])).toThrow(/too short/);
  });

  it('derives stable, purpose-separated keys', () => {
    const phrase = encodePhrase(crypto.getRandomValues(new Uint8Array(16)));
    const key1 = derivePhraseKey(phrase, 'epoch');
    expect(derivePhraseKey(phrase, 'epoch')).toEqual(key1);
    expect(derivePhraseKey(phrase, 'founder-sig')).not.toEqual(key1);
  });

  it('holds the wordlist invariants', () => {
    const seen = new Set<string>();
    for (let idx = 0; idx < PhraseWordCount; idx++) {
      const word = phraseWordAt(idx);
      expect(word.length).toBeGreaterThan(0);
      expect(seen.has(word)).toBe(false);
      seen.add(word);
      expect(phraseWordIndex(word)).toBe(idx);
    }
    expect(phraseWordIndex('NotInList')).toBe(-1);
  });

  // Mirrors stdlib/safe TestPhrase_WordlistPrefixUnique4: the properties
  // safe.consts.sdl claims — unique on the first FOUR characters (three-char
  // prefixes are NOT unique), 4–7 characters long, alphabetized.
  it('holds the wordlist prefix, length, and order claims', () => {
    const prefixes = new Map<string, string>();
    let prev = '';
    for (let idx = 0; idx < PhraseWordCount; idx++) {
      const word = phraseWordAt(idx);
      expect(word.length, `word ${word}: length outside 4-7`).toBeGreaterThanOrEqual(4);
      expect(word.length, `word ${word}: length outside 4-7`).toBeLessThanOrEqual(7);
      expect(word > prev, `word ${word}: not alphabetized after ${prev}`).toBe(true);
      prev = word;
      const prefix = word.slice(0, 4);
      expect(prefixes.has(prefix), `4-char prefix collision: ${word} vs ${prefixes.get(prefix)}`).toBe(false);
      prefixes.set(prefix, word);
    }
  });
});
