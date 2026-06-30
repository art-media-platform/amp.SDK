/**
 * forums-attrs.ts — the single source of the forums wire contract on the client.
 *
 * Channel/Attr identifiers are CANONIC strings (the ampd bridge folds them to
 * tag.UIDs); node/item identifiers (topic, post, member) are base32 tag.UIDs read
 * from query results (`_ItemID`) or `useAmpAuth().member.ID`.  Verb-op Value keys
 * are PascalCase, matching the Go/proto field names — the Go side is the spec
 * (amp.planet/amp/apps/app.forums/forums.proto).
 */

// Board root + sub-channels (canonics — folded server-side).
export const BOARD = 'app.forums';
export const PROFILES = 'app.forums.profiles';

// Attr canonics (one per forums.proto message).
export const ATTR_FORUM = 'app.forums.Forum';
export const ATTR_TOPIC = 'app.forums.Topic';
export const ATTR_POST = 'app.forums.Post';
export const ATTR_PROFILE = 'app.forums.Profile';
export const ATTR_READSTATE = 'app.forums.ReadState';
export const ATTR_SUBSCRIPTION = 'app.forums.Subscription';

// Verb URLs (the single write path; one batch = one verb).
export const VERB = {
  topic: 'amp://~/forums/topic',
  post: 'amp://~/forums/post',
  moderate: 'amp://~/forums/moderate',
  subscribe: 'amp://~/forums/subscribe',
  profile: 'amp://~/forums/profile',
  read: 'amp://~/forums/read',
  view: 'amp://~/forums/view',
} as const;

// forums.proto enums (wire integers).
export const PostStatus = { Live: 0, Edited: 1, Removed: 2 } as const;
export const NotifyFrequency = { Immediate: 0, Daily: 1, Weekly: 2, Never: 3 } as const;

// A content document is an amp.Tags map {ContentType: Text} — the pure-text JSON form the Go side
// decodes into amp.Tags (SD-content-substrate.md).  Post bodies + profile signatures ride a
// text/html (display) leaf + an optional text/markdown (re-edit) leaf.
export type TagsDoc = Record<string, string>;

// ── Wire item shapes (PascalCase, matching the proto messages) ──────────

export interface Forum {
  Labels?: { Title?: string; Caption?: string };
  ParentForum?: string;
  SortOrder?: number;
  Locked?: boolean;
}

export interface Topic {
  Title: string;
  Author?: string; // amp.Tag — base32 UID under .UID(); see authorUID()
  Forum?: string;
  Pinned?: boolean;
  Locked?: boolean;
  ReplyCount?: number;
  ViewCount?: number;
}

export interface Post {
  Body?: TagsDoc;          // text/html (display) + optional text/markdown (re-edit) leaves
  Author?: string;
  Status?: number;         // PostStatus; Edited ⇒ show the "edited" badge
}

export interface Profile {
  DisplayName?: string;
  Signature?: TagsDoc;     // text/html signature document
  JoinedAt?: number;
  PostCount?: number;
}

export interface ReadState {
  Topic?: string;
  LastReadEdit?: string;
  UnreadCount?: number;
}

/**
 * authorUID normalizes an amp.Tag author field to a base32 member UID.  The wire
 * serializes amp.Tag as an object carrying UID halves; the bridge surfaces it as
 * a string when it round-trips a plain UID, so accept either shape.
 */
export function authorUID(author: unknown): string {
  if (typeof author === 'string') return author;
  if (author && typeof author === 'object') {
    const tag = author as Record<string, unknown>;
    if (typeof tag.UID === 'string') return tag.UID;
    if (typeof tag.ID === 'string') return tag.ID;
  }
  return '';
}

/**
 * postBody builds a Post.Body / Profile.Signature document — the pure-text amp.Tags map
 * {ContentType: Text} the custodian and the embedded host both decode into amp.Tags (mirrors
 * the Go postBody; SD-content-substrate.md).  text/markdown is added only when a source is given.
 */
export function postBody(html: string, source?: string): TagsDoc {
  const doc: TagsDoc = { 'text/html': html };
  if (source) {
    doc['text/markdown'] = source;
  }
  return doc;
}

/** tagText reads a leaf's Text from a Tags map by IANA ContentType (mirrors Go TextByContentType). */
export function tagText(doc: TagsDoc | undefined, contentType: string): string {
  return doc?.[contentType] ?? '';
}
