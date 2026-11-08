/**
 * Typed errors for @art-media-platform/web.
 *
 * Every non-2xx /api/v1/* response carries the webapi.ErrorResponse envelope
 * ({ Code, Message }).  AmpError surfaces that `Code` as a typed field so a
 * consumer can dispatch on it — e.g. treat AmpErrorCode.Unsupported as a
 * no-op until the substrate wires the scheme on, with no wire-shape change.
 */

/**
 * Stable error codes mirroring webapi.ErrorResponse codes (amp.SDK/amp/webapi).
 * These are the HTTP-boundary client vocabulary; the HTTP status rides the
 * response and is surfaced as AmpError.status.
 */
export const AmpErrorCode = {
  BadRequest:      'BadRequest',
  AuthRequired:    'AuthRequired',
  AuthFailed:      'AuthFailed',
  Forbidden:       'Forbidden',
  NotFound:        'NotFound',
  Conflict:        'Conflict',
  Unsupported:     'Unsupported',
  TxRejected:      'TxRejected',
  PayloadTooLarge: 'PayloadTooLarge',
  CapReached:      'CapReached',       // HTTP 507: a storage cap refused it
  RateLimited:     'RateLimited',      // HTTP 429 + Retry-After: retry after it
  NotReady:        'NotReady',         // HTTP 503 + Retry-After: the node cannot answer yet; retry after it
  Internal:        'Internal',
  Unimplemented:   'Unimplemented',
} as const;

export type AmpErrorCode = (typeof AmpErrorCode)[keyof typeof AmpErrorCode];

/**
 * AmpError is thrown for any non-2xx amp host response.  `code` is the wire
 * ErrorResponse code (one of AmpErrorCode when the host emits the envelope, or
 * empty for a non-JSON body); `status` is the HTTP status.
 */
export class AmpError extends Error {
  readonly code: string;
  readonly status: number;
  /** Seconds from the Retry-After header, when the host sent one. */
  readonly retryAfter?: number;
  /** The request the failed call journaled before failing (invite accept), when the host sent one. */
  readonly txID?: string;

  constructor(status: number, code: string, message: string, retryAfter?: number, txID?: string) {
    super(message);
    this.name = 'AmpError';
    this.code = code;
    this.status = status;
    this.retryAfter = retryAfter;
    this.txID = txID;
  }

  /**
   * retryable: the same request succeeds later — RateLimited (429) or
   * NotReady (503), each with Retry-After.  Every other code is a verdict on
   * the request itself.
   */
  get retryable(): boolean {
    return this.code === AmpErrorCode.RateLimited || this.code === AmpErrorCode.NotReady;
  }
}

/**
 * ampErrorFromResponse decodes a non-2xx Response into an AmpError, reading the
 * { Code, Message, TxID? } envelope when the body is JSON and falling back to
 * the raw text / status line otherwise; Retry-After rides along in seconds.
 */
export async function ampErrorFromResponse(resp: Response): Promise<AmpError> {
  const text = await resp.text().catch(() => '');
  let code = '';
  let message = text;
  let txID: string | undefined;
  if (text) {
    try {
      const body = JSON.parse(text) as { Code?: string; Message?: string; TxID?: string };
      code = body.Code ?? '';
      message = body.Message ?? text;
      txID = body.TxID || undefined;
    } catch {
      // Non-JSON body — keep the raw text as the message.
    }
  }
  const header = resp.headers?.get?.('Retry-After');
  const seconds = header ? Number(header) : NaN;
  const retryAfter = Number.isFinite(seconds) && seconds >= 0 ? seconds : undefined;
  return new AmpError(resp.status, code, message || resp.statusText, retryAfter, txID);
}
