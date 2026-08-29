package amp

import (
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// ── Member re-key: quorum-authorized signing-identity re-issue ────────────────
//
// A member who lost their key is re-issued a new SigningKey + EncryptKey by the
// planet's founder quorum, on the record: a MemberEpoch carrying ReKey
// CoSignatures over the re-key digest below.  Same-MemberID re-key exists ONLY
// for identities whose canonic URI outlives the key (eth:, email:); a did:key
// MemberID IS the fold of its key and is refused structurally
// (AOM SD-did-identity.md §2, §12).  Authority model, vault admission, and ceremony:
// AOM SD-member-rekey.md.

// VerifyCoSignatureQuorum is the frame-agnostic quorum-verify core shared by
// planet genesis, epoch rotation, and member re-key: every CoSignature is
// checked over digest against its named signer's key, each signer counted at
// most once (a signer cannot manufacture a quorum by signing repeatedly), and
// the threshold enforced — required > 0 demands that many DISTINCT valid
// signatures, required <= 0 demands all declared signers, and an empty signer
// set never passes (fail-closed floor).  Returns the verified signer IDs.
// (AOM SD-security-sync.md §4.4, AOM SD-channel-governance.md §3, §8.)
func VerifyCoSignatureQuorum(signatures []*CoSignature, digest []byte, signerKeys map[tag.UID]safe.PubKey, required int) ([]tag.UID, error) {
	verified := make([]tag.UID, 0, len(signatures))
	seen := make(map[tag.UID]struct{}, len(signatures))
	for _, cosig := range signatures {
		if cosig == nil || cosig.MemberTag == nil {
			continue
		}
		signerID := cosig.MemberTag.UID()
		key, ok := signerKeys[signerID]
		if !ok {
			return nil, status.Code_BadRequest.Errorf("CoSignature from %s names no declared signer key", signerID.AsLabel())
		}
		if _, dup := seen[signerID]; dup {
			continue // count each signer once
		}
		if len(cosig.Signature) == 0 {
			return nil, status.Code_BadRequest.Errorf("empty CoSignature from %s", signerID.AsLabel())
		}
		if err := safe.VerifySignature(key.CryptoKitID, cosig.Signature, digest, key.Bytes); err != nil {
			return nil, status.Code_AuthFailed.Errorf("signer %s CoSignature failed: %v", signerID.AsLabel(), err)
		}
		seen[signerID] = struct{}{}
		verified = append(verified, signerID)
	}
	if required <= 0 {
		required = len(signerKeys) // 0 = all declared signers must sign (the strict default)
	}
	if required < 1 {
		required = 1 // never fail-open: no resolved signers (or a zero quorum) is unauthorized
	}
	if len(verified) < required {
		return nil, status.Code_AuthFailed.Errorf("quorum has %d valid signature(s), requires %d", len(verified), required)
	}
	return verified, nil
}

// reKeyFramePart returns one key's segment of the re-key digest — CryptoKit
// UID (16 bytes) ‖ pubkey bytes, the FounderFingerprint entry encoding.
// Identity is bytes: the kit and the key material bind, never a render.
func reKeyFramePart(ref *safe.KeyRef, role string) ([]byte, error) {
	if ref == nil || len(ref.PubKey) == 0 {
		return nil, status.Code_BadRequest.Errorf("amp: re-key record missing %s key", role)
	}
	part := ref.Kit().AppendTo(make([]byte, 0, 16+len(ref.PubKey)))
	return append(part, ref.PubKey...), nil
}

// ReKeyCoSignatureDigest returns the domain-separated digest each
// MemberEpoch.ReKey CoSignature signs and a verifier checks —
// SigningDomain_MemberReKey over (planetID, memberID, ReKeyPrior,
// SigningKey, EncryptKey), each part u32-length-framed by SigningParts, under
// the planet's hash policy.  Binding the retired prior key makes the digest
// the replay guard: a co-signed blob names exactly which key it retires, so it
// cannot re-install a dead key after a later rotation nor transplant to
// another member or planet.  All three keys are required — the lost device
// lost the wrap endpoint too, so a re-key without a fresh EncryptKey is
// malformed.  (AOM SD-member-rekey.md §3.)
func (me *MemberEpoch) ReKeyCoSignatureDigest(planetID tag.UID, hashKit safe.HashKitID) ([]byte, error) {
	if me == nil || me.MemberTag == nil || me.MemberTag.UID().IsNil() {
		return nil, status.Code_BadRequest.Error("amp: re-key record names no member")
	}
	if planetID.IsNil() {
		return nil, status.Code_BadRequest.Error("amp: re-key digest needs a planet")
	}
	prior, err := reKeyFramePart(me.ReKeyPrior, "prior signing")
	if err != nil {
		return nil, err
	}
	newSigning, err := reKeyFramePart(me.SigningKey, "new signing")
	if err != nil {
		return nil, err
	}
	newEncrypt, err := reKeyFramePart(me.EncryptKey, "new encrypt")
	if err != nil {
		return nil, err
	}
	memberID := me.MemberTag.UID()
	return safe.SigningDigest(hashKit, safe.SigningDomain_MemberReKey,
		planetID.AppendTo(nil),
		memberID.AppendTo(nil),
		prior,
		newSigning,
		newEncrypt,
	)
}

// VerifyReKeyQuorum verifies a re-key MemberEpoch's quorum: the structural
// gate (a did:key member is refused — its MemberID IS the fold of the key
// being retired, AOM SD-did-identity.md §2 — and the record must reinstate
// Status = Active, since Status rides outside the signed digest and must not
// become a submitter-controlled smuggling lane), then every ReKey CoSignature
// over ReKeyCoSignatureDigest against founderKeys with the genesis-frozen
// threshold (required <= 0 = all founders — the AOM SD-security-sync.md §4.4
// rotation mirror).  The
// ONE re-key verify site: the ACC authority gate and the vault key cache both
// call it.  What it does NOT check — and the vault must — is that ReKeyPrior
// equals the member's cached current SigningKey (the replay guard's grounding)
// and that the member was Suspended first.  (AOM SD-member-rekey.md §2-§4.)
func (me *MemberEpoch) VerifyReKeyQuorum(planetID tag.UID, hashKit safe.HashKitID, founderKeys map[tag.UID]safe.PubKey, required int) ([]tag.UID, error) {
	if me == nil || len(me.ReKey) == 0 {
		return nil, status.Code_BadRequest.Error("amp: not a re-key record (no ReKey CoSignatures)")
	}
	if me.Status != MemberStatus_Active {
		return nil, status.Code_BadRequest.Error("amp: a re-key record must reinstate Status = Active")
	}
	if me.MemberTag != nil && me.ReKeyPrior != nil {
		if foldID, ok := DIDKeyUID(me.ReKeyPrior.Kit(), me.ReKeyPrior.PubKey); ok && foldID == me.MemberTag.UID() {
			return nil, status.Code_AuthFailed.Error("amp: did:key member cannot be re-keyed — the MemberID is the fold of the key itself (SD-did-identity §2)")
		}
	}
	digest, err := me.ReKeyCoSignatureDigest(planetID, hashKit)
	if err != nil {
		return nil, err
	}
	return VerifyCoSignatureQuorum(me.ReKey, digest, founderKeys, required)
}
