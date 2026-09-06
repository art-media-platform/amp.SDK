/**
 * parseUID against forge-generated goldens: the generated consts carry both
 * the [hi, lo] pair and (in the SDL comment) the base32 text Go renders for
 * it, so a decode that disagrees with Go's tag.UID_ParseBase32 fails here.
 */

import { describe, expect, it } from 'vitest';

import { Attr } from './generated/amp.std.consts.js';
import { parseUID } from './uid.js';

describe('parseUID', () => {
  it('decodes the std App attr golden (amp.std.consts.sdl)', () => {
    expect(parseUID('4zs-80kzpjzhjx-53bkpndft1-m95')).toEqual(Attr.App.id);
    expect(Attr.App.id).toEqual([0x9FC2012FD63F847An, 0x51AA55A31D90CD25n]);
  });

  it('decodes an app.www golden (app.www.consts.sdl Www) with dashes stripped, either case', () => {
    const want = [0xF0D008C9E58CF55Bn, 0x07C0C4E35DFC53C4n];
    expect(parseUID('7hu-04dmtddype-hgh64wefzs-ny4')).toEqual(want);
    expect(parseUID('7HU04DMTDDYPEHGH64WEFZSNY4')).toEqual(want);
    expect(parseUID(' 7hu 04dmtddype hgh64wefzs ny4 ')).toEqual(want);
  });

  it('refuses a non-alphabet digit, an over-long text and an empty text', () => {
    expect(() => parseUID('4zs-80kzpjzhjx-53bkpndft1-m9a')).toThrow(/malformed/);
    expect(() => parseUID('4zs-80kzpjzhjx-53bkpndft1-m955')).toThrow(/malformed/);
    expect(() => parseUID('---')).toThrow(/malformed/);
  });
});
