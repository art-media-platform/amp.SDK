# `tag` — Universal Addressing

The AMP tag system is **phonetic, AI-friendly, search-friendly, and privacy-friendly**. It offers powerful and flexible linking similar to how #hashtags and [wikis](https://www.wikipedia.org/) add value, with a precise canonic form so the same idea always hashes to the same identifier no matter who typed it. We see this system as an excellent candidate to become an [IEEE](https://www.ieee.org/) standard for markup and hashing.

## Quick start

**Go:**

```go
import "github.com/art-media-platform/amp.SDK/stdlib/tag"

// Parse any inbound string (wire, file, user input).
name, _ := tag.Parse("Hello World!")
fmt.Println(name.Text)          // Hello.World   (case-preserved, for display)
fmt.Println(name.Canonic())     // hello.world   (folded form behind ID)
fmt.Println(name.ID.Base32())   // 7K4XGBQ7879D4F228U0CU1N627
fmt.Println(name.ID.AsLabel())  // 7K..N627  (compact debug label)

// Mint a unique time-based UID with entropy.
id := tag.NowID()
```

**C#:**

```cs
using art.media.platform;

// Parse any inbound string (wire, file, user input).
Name name = Name.Parse("Hello World!");
Debug.Log(name.Text);              // Hello.World   (case-preserved, for display)
Debug.Log(name.Canonic());         // hello.world   (folded form behind ID)
Debug.Log(name.ID.AsAsciiBase32);  // 7K4XGBQ7879D4F228U0CU1N627

// Mint a fresh time-based UID with entropy.
UID id = UID.Now();
```

## Types

A **`tag.Name`** pairs a UTF-8 expression with its 128-bit hash (a **`tag.UID`**):

```go
type UID  [2]uint64

type Name struct {
    ID   UID    // hash of any text expression
    Text string // optional case-preserved expression; canonize(Text) generates ID
}
```

`Text` is the expression as authored (`amp.law.PlanetEpoch`, not `amp.law.planetepoch`), so logs, attribute dumps, and debuggers stay self-documenting. Read `Text` by **default** — for display, labels, and identifiers (identity is the `ID`; a string re-parsed downstream re-folds to the same UID, so pre-folding it is wasted work). `Canonic()` recomputes the folded form (`canonize(Text)`) and allocates on every call — reach for it only when the folded bytes are the contract: normalizing arbitrary input, returning a resolve-style canonic result, or a case-insensitive match against a string whose case you do not control.

`Text` is **optional and may be dropped**. The UID is the sole identity, so a processor can match, route, and serve tags with `Text` stripped — fielding queries over opaque UIDs without learning what they name. Dropping `Text` before a request reaches an untrusted relay (e.g. `tag.DarkProjectsDivision.ClassifiedProjectTitle.Q3.2026` collapses to bare UIDs) is an information-leakage control; on the wire `Tag.Text` is an optional field, so omitting it is a no-op.

The hash is deterministic, cross-language, and small enough to compare with `==`, use as a map key, or fit in a database column. It has a short human-readable form via `id.Base32()` drawn from a 32-character alphabet that omits easily confused letters (such as `i`, `l`) — the same alphabet used by [Geohash](https://en.wikipedia.org/wiki/Geohash), borrowed purely for its readability properties (no geographic meaning here). This human-friendly alphabet is safe to read aloud, transcribe by hand, paste into a URL, or fit in a QR code. For compact log lines and debug output, `id.AsLabel()` returns a `first2..last4` short form (e.g. `6Z..800H`) — distinctive enough to tell IDs apart at a glance, short enough to fit anywhere.

The canonic word fold is intentionally lossy in ways that improve usability without compressing the namespace into anything close to dangerous.

A `UID` also doubles as a timestamp with discrete fixed precision: the first 6 bytes are UTC whole seconds, followed by 10 bytes (80 bits) of fractional precision. This means that a time-ordered UID (`NowID()`) is statistically universally unique *and* sortable as a wall-clock value.

Pre-computing tag UIDs at build time — so they're literals in your binary, not parsed at startup — is what [**forge**](https://github.com/art-media-platform/forge) is for. It reads a `.consts.sdl` source and emits typed `tag.Name` / `tag.UID` constants in Go and C# (with other languages coming), applying the same canonic rules described in this document.

## Tag Parsing

Two ways to produce a `Name` from a string, and the choice matters:

- **`tag.Parse(str)`** — for ANY string that came from outside the program (wire, file, CLI flag, user input). Auto-detects format: a 26-char Base32 string decodes back to its original UID; anything else is canonized. **Inbound UIDs round-trip intact** — they're not re-hashed.
- **`tag.NameFrom(str)`** — for hardcoded canonic expressions you constructed yourself (`"eth:" + addr`, `"a.profile.context"`). Always hashes through the canonic word fold.

Warning: calling `NameFrom` on an inbound Base32 string re-hashes the *string* instead of decoding it as a UID — and the failure is silent (you just get the wrong UID). When in doubt, use `Parse`.

## Canonic Forms

Tag literals are extracted from input text, reduced to a canonic dotted form, and finally hashed using Blake2S. The reduction is designed so a human typing the same words in different shapes lands on the same hash:

```
"Hello World!"      → hello.world                    (same UID)
"Hello, World!"     → hello.world                    (same UID)
"hello  world"      → hello.world                    (same UID)
"world  hello"      → world.hello                    (DIFFERENT UID)
```

The first three lines fold to the same canonic string (`hello.world`) and therefore the same UID — case, punctuation, and whitespace differences wash out.

Punctuation and other syntactic noise also washes out, except for URLs, described below.

For worked examples — various inputs run through the fold alongside their canonic forms and UIDs — see [`golden/welcome-to-tags.out.txt`](golden/welcome-to-tags.out.txt).

**Invariant:** for a plain name (no `/`, `:`, or `\`), a `tag.Name`'s `ID` is `UID_HashLiteral(canonize(Text))` — fold `Text` to its canonic string, hash it, get the UID. When a name carries a URL / identifier part, the name part and the URL part hash separately and combine, so scheme:identifier UIDs stay stable (see *URLs Preserved* and *Identity Preserved* below).

## Acronyms Preserved

ALL CAPS sequences are preserved on the assumption they are spoken letter by letter:

```
"Get the amp SDK today"    → get.the.amp.SDK.today
"Acronyms like NBA, NFL"   → acronyms.like.NBA.NFL
```

This keeps `Ada` (a name) distinct from `ADA` (the law) — meaningful in any search, identity, or routing context. Words that are not upper case are treated as spoken (phonetic), which also helps accessibility (sight or hearing impairments) and search across spelling variants.

## URLs Preserved

Tag expressions with a `/`, `:`, or `\` are treated as two parts:

- **Left** of the first separator → the **name** part, cleaned to canonic dotted form
- **Right** of the first separator → the **URL** part, kept exactly as-is (per [RFC 3986](https://datatracker.ietf.org/doc/html/rfc3986) — path/query case sensitivity belongs to the scheme owner)

The URL scheme itself is lowercased (RFC 3986 §3.1) but **otherwise preserved as-is** — dashes, dots, and plus signs inside the scheme are valid scheme grammar (`ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )`) and stay intact. The name + URL combination canonizes the words on the left side while preserving the URL exactly:

```
"https://example.com/path"                     → https://example.com/path
"HTTPS://example.com/path"                     → https://example.com/path (same UID)
"amp://planet/home"                            → amp://planet/home
"AMP://planet/home"                            → amp://planet/home (same UID)
"scheme://MixedCase/Preserved"                 → scheme://MixedCase/Preserved (path preserved verbatim)
"your-custom-scheme://preserved-url.com/path"  → your-custom-scheme://preserved-url.com/path
"Your-Custom-Scheme://preserved-url.com/path"  → your-custom-scheme://preserved-url.com/path (same UID)
"git+ssh://user@host/repo.git"                 → git+ssh://user@host/repo.git
"My App amp://planet/Home"                     → my.app.amp://planet/Home (name folded, URL exact)
```

## Identity Preserved

Wallet members, contract addresses, and similar identity URIs derive their UID by parsing a `scheme:identifier` expression — the URL form puts the scheme through the canonic word fold and hashes the identifier atomically. This matches [CAIP-10](https://github.com/ChainAgnostic/CAIPs/blob/main/CAIPs/caip-10.md) and [DID](https://www.w3.org/TR/did-1.0/) conventions.

The identifier (URL part) is hashed exact-as-is *by design*. Pre-lowercase it so [EIP-55](https://eips.ethereum.org/EIPS/eip-55) mixed-case addresses fold into the same UID:

```
"eth:0xabcdef1234567890..."    → eth:0xabcdef1234567890...
"Eth:0xabcdef1234567890..."    → eth:0xabcdef1234567890...   (same UID — scheme case-folded)
"ETH:0xabcdef1234567890..."    → eth:0xabcdef1234567890...   (same UID)

"eth:0xAbCdEf1234567890..."    → eth:0xAbCdEf1234567890...   (DIFFERENT UID — URL part is exact)
```


## Citation & Chaining

Tag UIDs can be cited as literals inside other tag expressions, allowing a tag's identity to incorporate references to other tags:

```
"We can cite 12VWSDH3ZB4W0ZY5RHVM9ZZGHP and 4VKB1MHN9J4YTYZPQ7HRZT5TBT as literals,
 demonstrating a convenient way to cryptographically chain and validate tags,
 allowing us to validate ancestry."
```

The cited UIDs become part of the new tag's content hash — a lightweight, cryptographically verifiable provenance chain. Geospatial tiles ([S2 cell IDs](https://s2geometry.io/)) can be cited the same way to bind tags to locations.

## JSON Support

`UID.MarshalJSON` / `UID.UnmarshalJSON` use the Base32 short form — quoted in JSON, with the zero UID rendered as the empty string (not JSON `null`, to preserve round-trip equality without forcing every wire field to be a pointer). UnmarshalJSON routes through `Parse`, so inbound Base32 UIDs round-trip intact.

# Forge

Most projects need named constants — string keys, version numbers, size limits, asset paths, feature-flag names — and only some of those are UIDs. [**forge**](https://github.com/art-media-platform/forge) generates them all from a single [`.consts.sdl`](https://github.com/art-media-platform/forge#the-solution) source: scalar constants (`string`, integer, and float types, plus hex and UUID literals) alongside hierarchical `tag.Name` / `tag.UID` identifiers, emitted as typed declarations in Go and C# (with other languages coming).

Declaring constants this way makes them compile-time literals in your binary — no startup parsing, no hand-mirroring the same value across languages, no drift. For tag UIDs specifically, the hash is pre-computed once at codegen time, so equivalent inputs collapse to the same value automatically.

