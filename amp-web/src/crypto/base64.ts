/**
 * Base64 for sealed bytes on the JSON wire.
 *
 * Channel item values are JSON, and a raw Uint8Array does NOT survive
 * JSON.stringify — it becomes an index-keyed object ({"0":1,"1":2,…}) that
 * cannot be reopened.  Encode sealed bytes to a base64 string before `upsert`
 * and decode on read.  Dependency-free (btoa/atob, present in browsers and
 * Node 16+) and stack-safe for large inputs.
 */

const CHUNK = 0x8000;

/** Encode bytes as a base64 string suitable for a JSON channel item value. */
export function bytesToBase64(bytes: Uint8Array): string {
  let binary = '';
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK));
  }
  return btoa(binary);
}

/** Decode a base64 string (as read back from a channel item) into bytes. */
export function base64ToBytes(b64: string): Uint8Array {
  const binary = atob(b64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    out[i] = binary.charCodeAt(i);
  }
  return out;
}
