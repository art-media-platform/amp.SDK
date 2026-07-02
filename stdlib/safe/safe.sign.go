package safe

// safe.sign.go — domain separation for signatures.
//
// One signing key authors several distinct things: a login proof, a TxMsg author
// seal, a PlanetEpoch co-signature, an SSO member token, a node handshake proof.
// A raw signature is only "this key signed these bytes" — it carries no purpose —
// so absent a bound context a signature made for one purpose can be replayed as
// another (a host that chooses a login challenge could lift the proof into a
// forged author seal).  Every signature therefore binds a SigningDomain: the
// signed bytes begin with a length-prefixed domain tag, so cross-context reuse is
// impossible by construction rather than by luck of differing payload shapes.
//
// This is the signature-side twin of the HKDF purpose strings that already
// domain-separate derived keys (DeriveSubKey).

// SigningDomain tags one signing context.  This is the single closed registry of
// contexts; their distinctness is the security property, so they live in one
// place to be audited together.  A value is a permanent wire commitment — never
// reuse, renumber, or repurpose one; retire by adding a new .vN.
type SigningDomain string

const (
	SigningDomain_Login        SigningDomain = "amp.sig.login.v1"  // member login proof-of-possession over the host challenge
	SigningDomain_TxAuthor     SigningDomain = "amp.sig.tx.v1"     // TxMsg author seal over the wire (SealTx / TxSignedDigest)
	SigningDomain_EpochCoSign  SigningDomain = "amp.sig.epoch.v1"  // PlanetEpoch founder/admin co-signature over the epoch frame
	SigningDomain_MemberToken  SigningDomain = "amp.sig.token.v1"  // SSO bearer token — signed as a text MESSAGE (external-wallet SIWE compatible), not a SigningDigest (app.www)
	SigningDomain_VaultNode    SigningDomain = "amp.sig.vault.v1"  // node/device vault-transport handshake proof (vault.identity)
	SigningDomain_InviteRedeem SigningDomain = "amp.sig.invite.v1" // invite redemption proof — RedeemKey binds a redemption to its invite policy (app.invite)
)

// AllSigningDomains enumerates every registered SigningDomain — the audit
// surface: the distinctness, cross-rejection, and length-bound tests iterate
// it, so a new domain is covered the moment it is added here.
var AllSigningDomains = []SigningDomain{
	SigningDomain_Login,
	SigningDomain_TxAuthor,
	SigningDomain_EpochCoSign,
	SigningDomain_MemberToken,
	SigningDomain_VaultNode,
	SigningDomain_InviteRedeem,
}

// SigningDomainTag returns the length-prefixed domain bytes that prefix every
// signed payload for domain — u8(len) || domain.  The length prefix makes the
// boundary between the tag and the payload unambiguous, so no domain can be a
// prefix of another's message.  Callers that hash through a CryptoProvider (the
// TxMsg author path) prepend this as the first hashed segment; SigningDigest
// applies it directly — both yield the same bytes.
func SigningDomainTag(domain SigningDomain) []byte {
	tag := make([]byte, 0, 1+len(domain))
	tag = append(tag, byte(len(domain)))
	tag = append(tag, domain...)
	return tag
}

// SigningDigest binds domain into a fixed 32-byte digest over parts under
// hashKit (0 = default Blake2s_256): H( SigningDomainTag(domain) || parts... ).
// It is the one authoritative definition of what a domain-separated signature
// covers — the signer passes the result to Enclave.SignRaw, the verifier to
// VerifySignature.  Fixed-size so it is kit-agnostic (every CryptoKit signs a
// 32-byte digest, re-hashing internally) and cheap for large payloads (the wire
// is hashed once here, not again in full by the kit).
func SigningDigest(hashKit HashKitID, domain SigningDomain, parts ...[]byte) ([]byte, error) {
	hk, err := NewHashKit(hashKit)
	if err != nil {
		return nil, err
	}
	hk.Hasher.Write(SigningDomainTag(domain))
	for _, part := range parts {
		hk.Hasher.Write(part)
	}
	return hk.Hasher.Sum(nil), nil
}

// SignDomain signs a domain-separated digest over parts with ref's key — the
// preferred signing entry point, since the domain is mandatory.  (Enclave.SignRaw
// over a caller-supplied digest remains for the client-side seal, where the node
// computes the SigningDigest and ships it for the off-node key to sign.)
func SignDomain(enc Enclave, ref *KeyRef, hashKit HashKitID, domain SigningDomain, parts ...[]byte) ([]byte, error) {
	digest, err := SigningDigest(hashKit, domain, parts...)
	if err != nil {
		return nil, err
	}
	return enc.SignRaw(ref, digest)
}

// VerifyDomain verifies sig over the domain-separated digest of parts against
// signerPubKey under cryptoKit — the verifier twin of SignDomain.
func VerifyDomain(cryptoKit CryptoKitID, hashKit HashKitID, domain SigningDomain, sig, signerPubKey []byte, parts ...[]byte) error {
	digest, err := SigningDigest(hashKit, domain, parts...)
	if err != nil {
		return err
	}
	return VerifySignature(cryptoKit, sig, digest, signerPubKey)
}
