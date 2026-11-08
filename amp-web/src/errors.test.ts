import { describe, expect, it } from 'vitest';
import { ampErrorFromResponse } from './errors.js';

// Guards the PascalCase error envelope (webapi.ErrorResponse: { Code, Message }).
// The server emits PascalCase keys via encoding/json; reading lowercase here
// would leave AmpError.code empty and silently kill error-code dispatch.
describe('ampErrorFromResponse', () => {
  function jsonResponse(status: number, body: unknown): Response {
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  it('reads the PascalCase { Code, Message } envelope', async () => {
    const err = await ampErrorFromResponse(jsonResponse(501, { Code: 'Unsupported', Message: 'scheme not wired' }));
    expect(err.code).toBe('Unsupported');
    expect(err.message).toBe('scheme not wired');
    expect(err.status).toBe(501);
  });

  it('falls back to status text on a non-JSON body', async () => {
    const err = await ampErrorFromResponse(new Response('gateway down', { status: 502 }));
    expect(err.code).toBe('');
    expect(err.message).toBe('gateway down');
    expect(err.status).toBe(502);
  });

  it('reads Retry-After and TxID, and names the two retryable codes', async () => {
    const busy = new Response(JSON.stringify({ Code: 'NotReady', Message: 'anonymous session mint still running' }), {
      status: 503,
      headers: { 'Content-Type': 'application/json', 'Retry-After': '3' },
    });
    const err = await ampErrorFromResponse(busy);
    expect(err.code).toBe('NotReady');
    expect(err.retryAfter).toBe(3);
    expect(err.retryable).toBe(true);

    const limited = await ampErrorFromResponse(jsonResponse(429, { Code: 'RateLimited', Message: 'slow down' }));
    expect(limited.retryable).toBe(true);
    expect(limited.retryAfter).toBeUndefined();

    const journaled = await ampErrorFromResponse(jsonResponse(409, {
      Code: 'TxRejected', Message: 'refused', TxID: '06efv-w28sb-m00a94-ta94t-a94ta',
    }));
    expect(journaled.retryable).toBe(false);
    expect(journaled.txID).toBe('06efv-w28sb-m00a94-ta94t-a94ta');
  });
});
