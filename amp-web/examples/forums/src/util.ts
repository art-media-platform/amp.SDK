/** shortID abbreviates a base32 UID for compact display. */
export function shortID(id: string): string {
  if (!id) return 'unknown';
  return id.length > 12 ? `${id.slice(0, 6)}…${id.slice(-4)}` : id;
}
