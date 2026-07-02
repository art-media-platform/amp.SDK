/**
 * Wire-freeze drift guard — TypeScript side.
 *
 * Asserts the SAME golden fixtures the Go side round-trips
 * (amp.SDK/amp/webapi/testdata/*.json, guarded by amp/webapi/drift_test.go).
 * The Go structs are the wire spec; the shape tables below hand-list each
 * shape's keys AS GO DEFINES THEM — that hand-listing IS the drift guard,
 * since TS interfaces don't exist at runtime.  When a TS declaration in
 * types.ts disagrees with a table below, the TS side must move (Go is the
 * server); each known disagreement is flagged with a DRIFT note.
 *
 * Per fixture entry: required keys (Go fields without omitempty/omitzero)
 * must be present, and every present key must be known (required ∪ optional).
 * Nested wire objects are checked recursively.
 */

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { CryptoKitID } from './crypto/types.js';
import type { AccessLevel, InvitePolicyEntry, WithdrawReason } from './types.js';

const testdataDir = join(
  dirname(fileURLToPath(import.meta.url)),
  '..', '..', 'amp', 'webapi', 'testdata',
);

function loadFixture(file: string): Record<string, unknown> {
  return JSON.parse(readFileSync(join(testdataDir, file), 'utf8'));
}

// ── Shape tables (the GO wire spec, hand-listed) ─────────────────────

interface ShapeSpec {
  required: readonly string[];
  optional?: readonly string[];
  /** wire-object sub-shapes by key; arrays are checked element-wise */
  nested?: Record<string, string>;
}

const SHAPES: Record<string, ShapeSpec> = {
  // DRIFT: types.ts LoginCredentials omits the optional PlanetTag that Go
  // webapi.LoginRequest accepts on every scheme.
  LoginRequest: {
    required: ['Scheme'],
    optional: ['Address', 'Signature', 'Nonce', 'Email', 'Password',
      'MemberToken', 'ChallengeResponse', 'DID', 'PlanetTag'],
  },
  LoginResponse: {
    required: ['SessionToken', 'ExpiresAt', 'Member'],
    nested: { Member: 'AmpMember' },
  },
  // DisplayName is optional on the wire (Go omitempty; types.ts matches).
  // Go is the spec — DisplayName is optional here; types.ts should follow.
  AmpMember: {
    required: ['ID', 'PlanetID'],
    optional: ['DisplayName', 'Email', 'Kind', 'Address'],
  },
  SessionResponse: {
    required: ['Member', 'ExpiresAt'],
    nested: { Member: 'AmpMember' },
  },
  EmailCredential: {
    required: [],
    optional: ['Email', 'Password', 'Token', 'NewPassword', 'PlanetTag'],
  },
  EmailIssueResponse: { required: ['MemberID', 'Email'] },
  ErrorResponse: { required: ['Code', 'Message'] },

  WithdrawNote: {
    required: ['Reason'],
    optional: ['Rationale', 'Subject', 'Delegation', 'WithdrawnAt', 'WithdrawnBy'],
  },
  TxOp: {
    required: ['Kind', 'Channel', 'Attr'],
    optional: ['ItemID', 'Value', 'Withdraw'],
    nested: { Withdraw: 'WithdrawNote' },
  },
  TxRequest: {
    required: ['Ops'],
    optional: ['PlanetTag', 'InvokeURL'],
    nested: { Ops: 'TxOp' },
  },
  TxOpResult: { required: ['ItemID', 'EditID'], optional: ['Error'] },
  TxResponse: {
    required: ['TxID', 'Results'],
    nested: { Results: 'TxOpResult' },
  },
  Item: {
    required: ['_ItemID', '_EditID', '_FromID', '_UpdatedAt', 'Value'],
    optional: ['_Withdrawn'],
    nested: { _Withdrawn: 'WithdrawNote' },
  },
  ListResponse: {
    required: ['Items', 'HasMore'],
    optional: ['Next'],
    nested: { Items: 'Item' },
  },

  SubscribeFrame: {
    required: ['Type'],
    optional: ['Channel', 'Attr', 'ItemID', 'EditID', 'FromID', 'Value',
      'UpdatedAt', 'Withdraw', 'Error'],
    nested: { Withdraw: 'WithdrawNote' },
  },

  // NOTE: no TS interface models EditEntry / EditChainResponse yet (the SDK
  // has no edit-chain API); these assert the Go shape so the fixture stays
  // honest for the client that adds it.
  EditEntry: {
    required: ['EditID', 'CommitTx', 'Author', 'CommittedAt', 'Op'],
    optional: ['Withdraw', 'Body'],
    nested: { Withdraw: 'WithdrawNote' },
  },
  EditChainResponse: {
    required: ['Edits'],
    optional: ['Original'],
    nested: { Original: 'Item', Edits: 'EditEntry' },
  },

  InviteIssueRequest: {
    required: ['Planet', 'Passphrase'],
    optional: ['MaxRedemptions', 'Access', 'ExpiresAt', 'VaultAddrs'],
  },
  InviteIssueResponse: { required: ['PlanetID', 'InviteID', 'InviteText'] },
  InviteAcceptRequest: { required: ['InviteText', 'Passphrase'] },
  InviteAcceptResponse: { required: ['PlanetID', 'MemberID'] },
  InviteRevokeRequest: {
    required: ['Planet'],
    optional: ['InviteID', 'InviteText', 'Rotate'],
  },
  InviteListResponse: {
    required: ['Policies'],
    nested: { Policies: 'InvitePolicyEntry' },
  },
  InvitePolicyEntry: {
    required: ['InviteID', 'MaxRedemptions', 'Status'],
    optional: ['GrantedAccess', 'ExpiresAt', 'Redemptions'],
    nested: { Redemptions: 'InviteRedemptionEntry' },
  },
  InviteRedemptionEntry: { required: ['Member', 'RedeemedAt', 'Rank', 'InRank'] },

  // Address is BASE64 — opaque transport bytes (Go []byte), NOT a base32 UID.
  // No TS interface models the resolve/federation family yet.
  VaultEndpoint: { required: ['Transport', 'Address'] },
  ResolveRequest: { required: ['FQDN'] },
  ResolveResponse: {
    required: ['FQDN', 'PlanetID', 'AnsweredBy', 'TrustState',
      'PinPrecedence', 'Ambiguous', 'Hops'],
    optional: ['VaultAddrs'],
    nested: { VaultAddrs: 'VaultEndpoint' },
  },
  SearchRequest: { required: ['Query'], optional: ['Limit'] },
  SearchResponse: {
    required: ['Matches'],
    nested: { Matches: 'SearchMatch' },
  },
  SearchMatch: {
    required: ['PlanetID', 'FQDN', 'AnsweredBy', 'Score', 'AppName', 'AppDesc'],
    optional: ['Platforms'],
  },
  FederationPeersResponse: {
    required: ['Peers'],
    nested: { Peers: 'FederationPeerEntry' },
  },
  FederationPeerEntry: {
    required: ['FederationID'],
    optional: ['VaultAddrs', 'Label'],
    nested: { VaultAddrs: 'VaultEndpoint' },
  },

  MediaResolveRequest: {
    required: ['Blob'],
    optional: ['PlanetTag'],
    nested: { Blob: 'Tag' },
  },
  // amp.Tag JSON (Go tagJSON): every field omitempty.
  // The wire key is ContentTypeRaw (types.ts BlobRef matches).
  Tag: {
    required: [],
    optional: ['UID', 'I', 'J', 'K', 'Units', 'ContentTypeRaw', 'URI', 'Text'],
  },
};

// Keys carrying app-defined payloads (json.RawMessage Go-side) — any inner
// keys are legal, so they are exempt from nested checks by construction
// (absent from `nested` above): TxOp.Value, Item.Value, SubscribeFrame.Value,
// EditEntry.Body.

const FIXTURE_FILES = [
  'login.json',
  'tx-item.json',
  'subscribe.json',
  'edits.json',
  'invite.json',
  'vault.json',
  'media.json',
] as const;

const shapesChecked = new Set<string>();

function checkShape(value: unknown, shapeName: string, path: string): void {
  const spec = SHAPES[shapeName];
  expect(spec, `${path}: unknown shape ${shapeName} — add it to SHAPES`).toBeDefined();
  shapesChecked.add(shapeName);

  expect(value, `${path}: expected a wire object`).toBeTypeOf('object');
  expect(value, `${path}: expected a wire object, got null`).not.toBeNull();
  const obj = value as Record<string, unknown>;

  const known = new Set([...spec.required, ...(spec.optional ?? [])]);
  for (const key of spec.required) {
    expect(
      Object.prototype.hasOwnProperty.call(obj, key),
      `${path}: required key ${shapeName}.${key} missing from fixture`,
    ).toBe(true);
  }
  for (const key of Object.keys(obj)) {
    expect(
      known.has(key),
      `${path}: fixture key ${shapeName}.${key} is not in the ${shapeName} shape table — wire drift or stale table`,
    ).toBe(true);
  }

  for (const [key, subShape] of Object.entries(spec.nested ?? {})) {
    const sub = obj[key];
    if (sub === undefined || sub === null) continue;
    if (Array.isArray(sub)) {
      sub.forEach((el, i) => checkShape(el, subShape, `${path}.${key}[${i}]`));
    } else {
      checkShape(sub, subShape, `${path}.${key}`);
    }
  }
}

// ── Structural fixture checks ────────────────────────────────────────

describe('wire fixtures match the Go shapes', () => {
  for (const file of FIXTURE_FILES) {
    it(file, () => {
      const entries = loadFixture(file);
      expect(Object.keys(entries).length).toBeGreaterThan(0);
      for (const [name, value] of Object.entries(entries)) {
        const shape = name.split('.')[0];
        checkShape(value, shape, `${file}#${name}`);
      }
    });
  }

  it('every shape table is exercised by a fixture', () => {
    for (const shape of Object.keys(SHAPES)) {
      expect(shapesChecked.has(shape), `shape ${shape} never checked — fixture entry deleted?`).toBe(true);
    }
  });
});

// ── Enum-name goldens ────────────────────────────────────────────────

// `satisfies` ties each hand-list to the TS union at typecheck time; the
// runtime toEqual ties it to the shared fixture the Go side verifies.
const ACCESS_LEVELS = [
  'ReadOnly', 'ReadWrite', 'Moderator', 'Admin',
] as const satisfies readonly AccessLevel[];

const WITHDRAW_REASONS = [
  'Consent', 'Inaccuracy', 'Outdated', 'Coerced',
  'Forgotten', 'Departed', 'InviteRecall', 'Retracted',
] as const satisfies readonly WithdrawReason[];

const INVITE_STATUSES = [
  'InviteActive', 'InviteRevoked',
] as const satisfies readonly InvitePolicyEntry['Status'][];

describe('enum-name goldens', () => {
  const golden = loadFixture('access.json') as {
    AccessLevels: string[];
    WithdrawReasons: string[];
    InviteStatuses: string[];
    TrustStates: string[];
  };

  it('AccessLevel union matches the golden', () => {
    expect(golden.AccessLevels).toEqual([...ACCESS_LEVELS]);
  });

  it('WithdrawReason union matches the golden', () => {
    expect(golden.WithdrawReasons).toEqual([...WITHDRAW_REASONS]);
  });

  it('InvitePolicyEntry.Status union matches the golden', () => {
    expect(golden.InviteStatuses).toEqual([...INVITE_STATUSES]);
  });

  // TrustStates golden is Go-verified only: no TS type models the resolve
  // family yet (see ResolveResponse note above).
});

// ── CryptoKitID golden ───────────────────────────────────────────────

describe('CryptoKitID table matches the golden', () => {
  const golden = loadFixture('cryptokits.json') as {
    Kits: { Name: string; TsKitID: number; GoUID: string }[];
  };

  it('every golden kit maps to the TS small-int', () => {
    const table = CryptoKitID as Record<string, number>;
    for (const kit of golden.Kits) {
      expect(table[kit.Name], `kit ${kit.Name} missing from CryptoKitID`).toBe(kit.TsKitID);
    }
  });

  it('every TS kit appears in the golden', () => {
    const goldenNames = new Set(golden.Kits.map(kit => kit.Name));
    for (const name of Object.keys(CryptoKitID)) {
      expect(goldenNames.has(name), `CryptoKitID.${name} missing from the golden`).toBe(true);
    }
  });

  it('Go kit UIDs are well-formed base32 (authoritative values asserted Go-side)', () => {
    for (const kit of golden.Kits) {
      if (kit.Name === 'Unspecified') {
        expect(kit.GoUID).toBe('');
        continue;
      }
      expect(kit.GoUID).toMatch(/^[0-9a-z]{26}$/);
    }
  });
});
