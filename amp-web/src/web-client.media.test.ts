/**
 * mediaUrl(): the no-I/O fallback must build the SAME path the server's
 * PublishAsset registers (app.www www.webService.go) — the blob UID's base32
 * plus the MIME subtype as extension, derived from Tag.ContentType() (lower-
 * cased ContentTypeRaw; empty ⇒ text/plain).  A path without the extension
 * 404s: nothing is registered under it.
 */

import { describe, expect, it } from 'vitest';

import { MemoryKeyStorage } from './crypto/keystore.js';
import { MemorySessionStore } from './session-store.js';
import { AmpWebClient } from './web-client.js';

const VAULT = 'http://127.0.0.1:5193';
const UID = '06e-fvw28sb600-36dtm6dtm6-dtm';

function client(): AmpWebClient {
  return new AmpWebClient({
    vaultUrl: VAULT,
    planetTag: 'home',
    encryptKeyStorage: new MemoryKeyStorage(),
    sessionStore: new MemorySessionStore(),
  });
}

describe('mediaUrl', () => {
  it('appends the MIME subtype the server publishes under', () => {
    const url = client().mediaUrl({ UID, ContentTypeRaw: 'video/mp4' });
    expect(url).toBe(`${VAULT}/www/${UID}.mp4`);
  });

  it('path equals the URI /media/resolve answers for the same Tag', () => {
    // The server's published path for this Tag (www.webService.go PublishAsset):
    // AssetID = blob UID base32, "." + subtype of ContentType().
    const resolved = `/www/${UID}.mp4`;
    const url = new URL(client().mediaUrl({ UID, ContentTypeRaw: 'video/mp4' }));
    expect(url.pathname).toBe(resolved);
  });

  it('mirrors Tag.ContentType(): lower-cased, empty ⇒ text/plain', () => {
    const cl = client();
    expect(cl.mediaUrl({ UID, ContentTypeRaw: 'Image/PNG' })).toBe(`${VAULT}/www/${UID}.png`);
    expect(cl.mediaUrl({ UID })).toBe(`${VAULT}/www/${UID}.plain`);
    expect(cl.mediaUrl({ UID, ContentTypeRaw: '' })).toBe(`${VAULT}/www/${UID}.plain`);
  });

  it('adds no extension when the ID already carries one or the type has no subtype', () => {
    const cl = client();
    expect(cl.mediaUrl({ UID: 'clip.mp4', ContentTypeRaw: 'video/mp4' })).toBe(`${VAULT}/www/clip.mp4`);
    expect(cl.mediaUrl({ UID, ContentTypeRaw: 'mp4' })).toBe(`${VAULT}/www/${UID}`);
    expect(cl.mediaUrl({ UID, ContentTypeRaw: '/mp4' })).toBe(`${VAULT}/www/${UID}`);
  });
});
