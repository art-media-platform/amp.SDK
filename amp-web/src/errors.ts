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

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'AmpError';
    this.code = code;
    this.status = status;
  }
}

/**
 * ampErrorFromResponse decodes a non-2xx Response into an AmpError, reading the
 * { Code, Message } envelope when the body is JSON and falling back to the raw
 * text / status line otherwise.
 */
export async function ampErrorFromResponse(resp: Response): Promise<AmpError> {
  const text = await resp.text().catch(() => '');
  let code = '';
  let message = text;
  if (text) {
    try {
      const body = JSON.parse(text) as { Code?: string; Message?: string };
      code = body.Code ?? '';
      message = body.Message ?? text;
    } catch {
      // Non-JSON body — keep the raw text as the message.
    }
  }
  return new AmpError(resp.status, code, message || resp.statusText);
}
