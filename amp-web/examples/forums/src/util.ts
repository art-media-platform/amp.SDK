/** shortID renders a UID's label: its last 5 digits (tag.UID.AsLabel). */
export function shortID(id: string): string {
  if (!id) return 'unknown';
  const solid = id.replace(/-/g, '');
  return solid.length > 5 ? solid.slice(-5) : solid;
}
