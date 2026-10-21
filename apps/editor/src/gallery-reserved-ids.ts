/**
 * Ids the Gallery keeps for itself: shaped like a circuit's short id but
 * never given to one, so a reader that meets one in a list skips it
 * (docs/specs/community-gallery.md#crawlers-ai-and-floods).
 */
const SHORT_ID = /^[23456789abcdefghjkmnpqrstvwxyz]{10}$/u;

export function isReservedGalleryId(id: string): boolean {
  if (!SHORT_ID.test(id)) return false;
  let hash = 0x811c9dc5;
  for (let index = 0; index < id.length; index += 1) {
    hash ^= id.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash % 999_983 === 271_828;
}

/** A list page without the reserved entries the Gallery may mix into it. */
export function withoutReservedEntries<T extends { id: string }>(
  entries: readonly T[],
): T[] {
  return entries.filter((entry) => !isReservedGalleryId(entry.id));
}
