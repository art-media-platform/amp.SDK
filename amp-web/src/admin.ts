/**
 * AmpAdminClient — the operator tier (`/api/v1/admin/*`) for SERVER-SIDE
 * tooling only: Node scripts, migration jobs, payment webhooks.
 *
 * NEVER import this module from browser code.  The operator Bearer is
 * higher-privilege than a member session and must not enter XSS-exposed
 * script (SKILL-amp-web-SDK.md §12 rule 9; SECURITY-amp-web-SDK.md).  The
 * guards that keep it out of the browser surface:
 *   - excluded from the package root export — import '@art-media-platform/web/admin';
 *   - the constructor refuses to run where a browser `window`+`document` exist;
 *   - webapi/testdata/operator-go-only.json pins which verbs may appear here,
 *     and drift.test.ts confines admin endpoint strings to this one module.
 */

import { ampErrorFromResponse } from './errors.js';
import type { EmailCredential } from './types.js';

export interface AmpAdminClientOpts {
  vaultUrl: string;      // operated node URL — e.g. https://prod.plan.tools
  adminToken: string;    // operator Bearer (allowlist-gated host-side); lives in server env only
}

/** Options for seeding an email-scheme credential (admin issue). */
export interface IssueEmailCredentialOpts {
  email: string;
  password: string;
}

/** Body of a successful admin issue (webapi.EmailIssueResponse). */
export interface EmailIssueResult {
  MemberID: string;      // derived member tag.UID, base32
  Email: string;
}

export class AmpAdminClient {
  private vaultUrl: string;
  private adminToken: string;

  constructor(opts: AmpAdminClientOpts) {
    if (typeof window !== 'undefined' && typeof document !== 'undefined') {
      throw new Error(
        'AmpAdminClient is server-side tooling only — the operator Bearer must never enter browser JS (SKILL-amp-web-SDK.md §12)',
      );
    }
    this.vaultUrl = opts.vaultUrl.replace(/\/$/, '');
    this.adminToken = opts.adminToken;
  }

  private async adminFetch<T>(path: string, body: EmailCredential): Promise<T> {
    const resp = await fetch(`${this.vaultUrl}/api/v1${path}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${this.adminToken}`,
      },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      throw await ampErrorFromResponse(resp);
    }
    return resp.json();
  }

  /**
   * Seed (or replace) an email-scheme credential without exposing open signup
   * — POST /api/v1/admin/credentials/email/issue.  Returns the derived
   * MemberID so the caller can persist the identity mapping without
   * re-hashing.  The allowlist gate rides host-side; a non-admin Bearer is
   * AmpError 403.
   */
  issueEmailCredential(opts: IssueEmailCredentialOpts): Promise<EmailIssueResult> {
    return this.adminFetch<EmailIssueResult>('/admin/credentials/email/issue', {
      Email: opts.email,
      Password: opts.password,
    });
  }
}
