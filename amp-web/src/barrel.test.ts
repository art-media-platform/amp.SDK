/**
 * Entry-point drift guard: the root barrel (src/index.ts, the '.' export) is
 * dependency-free — nothing reachable from it imports `react` (or a .tsx
 * module) — so a React-free bundler consumer or a Node harness imports
 * AmpWebClient and the codecs without React.  The React surface lives on
 * src/react.ts ('./react'), which this test asserts DOES reach react, so the
 * root assertion is one that can fail.
 */

import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const SRC = dirname(fileURLToPath(import.meta.url));
const SPECIFIER_RE = /(?:^|\n)\s*(?:import|export)\b[^'"\n]*?from\s+['"]([^'"]+)['"]|(?:^|\n)\s*import\s+['"]([^'"]+)['"]/g;

/** Walk the static import graph from one source file: visited files + bare (package) specifiers. */
function importGraph(entry: string): { files: Set<string>; packages: Set<string> } {
  const files = new Set<string>();
  const packages = new Set<string>();
  const queue = [resolve(SRC, entry)];
  while (queue.length) {
    const file = queue.pop()!;
    if (files.has(file)) continue;
    files.add(file);
    const text = readFileSync(file, 'utf8');
    for (const match of text.matchAll(SPECIFIER_RE)) {
      const spec = match[1] ?? match[2];
      if (!spec) continue;
      if (!spec.startsWith('.')) {
        packages.add(spec);
        continue;
      }
      const base = join(dirname(file), spec.replace(/\.js$/, ''));
      const target = ['.ts', '.tsx'].map(ext => base + ext).find(existsSync);
      if (!target) throw new Error(`${file}: unresolved import ${spec}`);
      queue.push(target);
    }
  }
  return { files, packages };
}

const isReact = (spec: string): boolean => spec === 'react' || spec.startsWith('react/') || spec.startsWith('react-dom');

describe('entry points', () => {
  it("the root barrel's import graph carries no react and no .tsx module", () => {
    const { files, packages } = importGraph('index.ts');
    const offenders = [...packages].filter(isReact);
    expect(offenders, `react reachable from src/index.ts via ${[...files].join(', ')}`).toEqual([]);
    expect([...files].filter(f => f.endsWith('.tsx'))).toEqual([]);
    // Guard sanity: the walk saw the client and the codecs.
    expect([...files].some(f => f.endsWith('/web-client.ts'))).toBe(true);
    expect([...packages].some(p => p.startsWith('@noble/'))).toBe(true);
  });

  it('the ./react entry reaches react (the surface the root gave up)', () => {
    const { files, packages } = importGraph('react.ts');
    expect([...packages].filter(isReact)).toEqual(['react']);
    expect([...files].some(f => f.endsWith('/provider.tsx'))).toBe(true);
    for (const hook of ['useAmpAuth', 'useAmpQuery', 'useAmpMutation', 'useAmpUpload', 'useAmpMedia', 'useAmpCrypto', 'useAmpResolve', 'useAmpBrand']) {
      expect([...files].some(f => f.endsWith(`/hooks/${hook}.ts`)), hook).toBe(true);
    }
  });

  it('package.json exports "." and "./react" (and ./admin) as built entries', () => {
    const pkg = JSON.parse(readFileSync(join(SRC, '..', 'package.json'), 'utf8')) as { exports: Record<string, { import?: string }> };
    expect(pkg.exports['.']?.import).toBe('./dist/index.js');
    expect(pkg.exports['./react']?.import).toBe('./dist/react.js');
    expect(pkg.exports['./admin']?.import).toBe('./dist/admin.js');
  });
});
