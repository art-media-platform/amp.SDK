// Minimal ambient shims for the Node builtins used only by test files (DOGFOOD-gated
// *.live.test.ts + the fixture-reading drift.test.ts), so the browser-first SDK typechecks
// without pulling @types/node into its global type surface (which would let shipped src
// silently depend on node globals like Buffer/process — the live tests already cast
// globalThis for process.env to keep node out of the public types).
declare module 'node:fs' {
  export function existsSync(path: string): boolean;
  export function readFileSync(path: string, encoding: string): string;
  export function readdirSync(
    path: string,
    options: { recursive: boolean; encoding: 'utf8' },
  ): string[];
}
declare module 'node:path' {
  export function dirname(path: string): string;
  export function join(...paths: string[]): string;
  export function relative(from: string, to: string): string;
}
declare module 'node:url' {
  export function fileURLToPath(url: string | URL): string;
}
