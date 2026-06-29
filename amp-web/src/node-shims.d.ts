// Minimal ambient shims for the Node builtins used only by DOGFOOD-gated *.live.test.ts files,
// so the browser-first SDK typechecks without pulling @types/node into its global type surface
// (which would let shipped src silently depend on node globals like Buffer/process — the live
// tests already cast globalThis for process.env to keep node out of the public types).
declare module 'node:fs' {
  export function readFileSync(path: string, encoding: string): string;
}
