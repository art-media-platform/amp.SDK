/**
 * AOM-reference gate for the shipped TypeScript — the pack.sh 3d gate, run
 * under `npm test` so the pre-cut slate and every land-time test run catch a
 * cite before the tag (O4 §4.20 leg 5).
 *
 * The pack's de-link pass walks .md only, so an AOM doc named in a src/
 * comment would ship raw.  Shipped src/ may cite only docs the bundle
 * carries: the ONE list is aom-allowlist.txt, read here and by pack.sh.  A
 * refused cite is fixed at its source — the .sdl / .proto for src/generated/,
 * the .ts for the hand-written client — never by widening the list.
 *
 * Runs from both views this file ships in: the amp.SDK repo (amp-web/) and
 * the amp-web-SDK bundle (pack.sh stages aom-allowlist.txt beside src/).
 */

import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const HERE = dirname(fileURLToPath(import.meta.url));           // amp-web/src
const SRC_ROOT = HERE;
const ALLOWLIST_PATH = join(HERE, '..', 'aom-allowlist.txt');
const PACK_SH_PATH = join(HERE, '..', 'pack.sh');                // repo view only

// Must equal pack.sh's AOM_REF_RE byte-for-byte — the test below asserts it.
const AOM_REF_RE_SOURCE = String.raw`\b(DD|SD|AD|QO|ZO|O[0-9]|Q[0-9]?)-[a-z0-9]+(-[a-z0-9]+)*(\.md)?`;
const AOM_REF_RE = new RegExp(AOM_REF_RE_SOURCE, 'g');

const MODES = new Set(['whole', 'public']);

interface AllowlistEntry {
  line: number;
  doc: string;
  mode: string;
  extra: string[];
}

/** `<doc>.md <whole|public>` per entry; `#` comments and blank lines skipped. */
function readAllowlist(): AllowlistEntry[] {
  return readFileSync(ALLOWLIST_PATH, 'utf8')
    .split('\n')
    .map((text, idx) => ({ text, line: idx + 1 }))
    .filter(({ text }) => !/^\s*(#|$)/.test(text))
    .map(({ text, line }) => {
      const [doc, mode, ...extra] = text.trim().split(/\s+/);
      return { line, doc, mode: mode ?? '', extra };
    });
}

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) walk(path, out);
    else out.push(path);
  }
  return out;
}

describe('aom-allowlist.txt', () => {
  it('exists beside src/ and parses under the pack.sh grammar', () => {
    expect(existsSync(ALLOWLIST_PATH), `missing ${ALLOWLIST_PATH}`).toBe(true);
    const entries = readAllowlist();
    expect(entries.length).toBeGreaterThan(0);
    const malformed = entries
      .filter(e => !e.doc.endsWith('.md') || !MODES.has(e.mode) || e.extra.length > 0)
      .map(e => `aom-allowlist.txt:${e.line}: ${[e.doc, e.mode, ...e.extra].join(' ')}`);
    expect(malformed, 'entries must be `<doc>.md <whole|public>`').toEqual([]);
    expect(entries.some(e => e.mode === 'whole'), 'at least one whole-doc entry').toBe(true);
  });

  it.skipIf(!existsSync(PACK_SH_PATH))('shares its cite regex with pack.sh AOM_REF_RE', () => {
    const packSh = readFileSync(PACK_SH_PATH, 'utf8');
    const match = /^AOM_REF_RE='([^']*)'$/m.exec(packSh);
    expect(match, "pack.sh must define AOM_REF_RE='…' on one line").not.toBeNull();
    expect(match![1]).toBe(AOM_REF_RE_SOURCE);
  });
});

describe('shipped src/ cites only allowlisted AOM docs', () => {
  it('names no AOM doc outside aom-allowlist.txt', () => {
    const allowed = new Set(readAllowlist().map(e => e.doc.replace(/\.md$/, '')));
    const offenders: string[] = [];
    for (const file of walk(SRC_ROOT)) {
      const lines = readFileSync(file, 'utf8').split('\n');
      lines.forEach((text, idx) => {
        for (const hit of text.matchAll(AOM_REF_RE)) {
          const doc = hit[0].replace(/\.md$/, '');
          if (!allowed.has(doc)) {
            offenders.push(`src/${relative(SRC_ROOT, file)}:${idx + 1}: ${doc}`);
          }
        }
      });
    }
    expect(
      offenders,
      'fix the citation at source (.sdl/.proto for src/generated/, the .ts otherwise) and regen; ' +
        'never widen aom-allowlist.txt for an internal doc. Allowlisted: ' +
        [...allowed].sort().join(' '),
    ).toEqual([]);
  });
});
