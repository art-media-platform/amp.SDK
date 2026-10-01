package amp_test

import (
	"crypto/rand"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"google.golang.org/protobuf/proto"
)

// TestEpochVerbatim_Roundtrip exercises the three-layer verbatim-signed model end
// to end: assemble (marshal once) -> sign the FRAME -> verify -> tamper-detect ->
// charter continuity across a rotation -> Terminal-seal rejection.  This proves
// the sign/verify mechanics actually work, not just compile.
func TestEpochVerbatim_Roundtrip(t *testing.T) {
	kitID := safe.Crypto.Poly25519.ID
	kit, err := safe.CryptoKit(kitID)
	if err != nil {
		t.Fatal(err)
	}

	uid := func(hi, lo uint64) *amp.Tag { return &amp.Tag{UID_0: hi, UID_1: lo} }

	charter := &amp.PlanetCharter{
		CharterSchema:             1,
		PlanetID:                  uid(0xABCD, 0xEF01),
		GenesisEpoch:              uid(100, 200),
		Privacy:                   amp.PrivacyMode_Confidential,
		Declaration:               &amp.Tags{Head: uid(1, 1)},
		Founders:                  []*amp.Tag{uid(0xF1, 0)},
		GenesisRequiredSignatures: 1,
	}
	genesisTerms := &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      uid(100, 200), // == GenesisEpoch
		EpochHeight:   0,
		CryptoKitID_0: kitID[0],
		CryptoKitID_1: kitID[1],
		Label:         "Genesis",
		Mark:          &amp.BrandMark{Identity: &amp.BrandIdentity{AppName: "Hexosphere"}},
	}

	// 1) Assemble: marshal charter+terms once, bind CharterHash into Terms.
	env, err := amp.AssembleEpoch(charter, genesisTerms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatalf("AssembleEpoch: %v", err)
	}

	// 2) Sign the epoch cosign digest (domain-separated over the FRAME), append a CoSignature.
	digest, err := env.CoSignatureDigest()
	if err != nil {
		t.Fatalf("CoSignatureDigest: %v", err)
	}
	kp := safe.KeyPair{Pub: safe.PubKey{CryptoKitID: kitID, KeyType: safe.KeyType_SigningKey}}
	if err := kit.Signing.Generate(rand.Reader, &kp); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	sig, err := kit.Signing.Sign(digest, kp.Prv)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	pub := kp.Pub.Bytes
	cosig := &amp.CoSignature{MemberTag: uid(0xF1, 0), Signature: sig}

	// 3) Verify succeeds over the stored bytes.
	if err := env.VerifyCoSignature(cosig, pub, kitID); err != nil {
		t.Fatalf("VerifyCoSignature (valid): %v", err)
	}

	// 4) Tamper a Charter byte -> verify must fail (signed-bytes integrity).
	bad := proto.Clone(env).(*amp.PlanetEpoch)
	bad.Charter[len(bad.Charter)-1] ^= 0xFF
	if err := bad.VerifyCoSignature(cosig, pub, kitID); err == nil {
		t.Fatal("VerifyCoSignature accepted a tampered Charter")
	}

	// 5) Genesis charter continuity: CharterHash matches carried Charter; no prev.
	if err := env.VerifyCharterContinuity(nil); err != nil {
		t.Fatalf("genesis VerifyCharterContinuity: %v", err)
	}

	// 6) Rotation: same charter bytes, height+1, PreviousEpoch -> genesis.
	rotTerms := &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      uid(101, 201),
		PreviousEpoch: uid(100, 200),
		EpochHeight:   1,
		CryptoKitID_0: kitID[0],
		CryptoKitID_1: kitID[1],
		Label:         "Rotation 1",
		Mark:          &amp.BrandMark{Identity: &amp.BrandIdentity{AppName: "Hexosphere"}},
	}
	rot, err := amp.AssembleEpoch(charter, rotTerms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatalf("AssembleEpoch(rot): %v", err)
	}
	if err := rot.VerifyCharterContinuity(env); err != nil {
		t.Fatalf("rotation VerifyCharterContinuity: %v", err)
	}

	// 7) Terminal seal the genesis epoch -> no successor may chain off it.
	sealedTerms := proto.Clone(genesisTerms).(*amp.EpochTerms)
	sealedTerms.Seal = amp.SealState_Sealed
	sealed, err := amp.AssembleEpoch(charter, sealedTerms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatalf("AssembleEpoch(sealed): %v", err)
	}
	if err := rot.VerifyCharterContinuity(sealed); err == nil {
		t.Fatal("VerifyCharterContinuity accepted a successor chaining off a Terminal epoch")
	}

	t.Log("verbatim epoch: assemble/sign/verify/tamper/continuity/terminal-seal all OK")
}

// TestVerifyCharterContinuity_HashKitStable pins the invariant that a planet's
// HashKit is stable across its epoch chain (AOM SD-security-sync.md §5.3.2).  A
// rotation that carries the prior epoch's hash forward (what RotateEpoch does:
// AssembleEpoch with prevTerms.EffectiveHashKit()) verifies; one that changes
// the hash — e.g. a reset to the default Blake2s on a SHA3 genesis — is
// rejected, since a deliberate hash migration is a deferred capability.
func TestVerifyCharterContinuity_HashKitStable(t *testing.T) {
	uid := func(hi, lo uint64) *amp.Tag { return &amp.Tag{UID_0: hi, UID_1: lo} }

	charter := &amp.PlanetCharter{
		CharterSchema:             1,
		PlanetID:                  uid(0x5A11, 0x7A6),
		GenesisEpoch:              uid(100, 200),
		Privacy:                   amp.PrivacyMode_Confidential,
		GenesisRequiredSignatures: 1,
	}
	// Genesis under a NON-default hash policy (SHA3-256).
	genesis, err := amp.AssembleEpoch(charter, &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      uid(100, 200),
		EpochHeight:   0,
		CryptoKitID_0: safe.Crypto.Poly25519.ID[0],
		CryptoKitID_1: safe.Crypto.Poly25519.ID[1],
	}, safe.HashKitID_SHA3_256)
	if err != nil {
		t.Fatalf("AssembleEpoch(genesis): %v", err)
	}

	// A well-formed successor (same charter, height+1, points at genesis); every
	// continuity field below is valid, so the ONLY rejection cause is the HashKit.
	rotTerms := func() *amp.EpochTerms {
		return &amp.EpochTerms{
			TermsSchema:   1,
			EpochTag:      uid(101, 201),
			PreviousEpoch: uid(100, 200),
			EpochHeight:   1,
			CryptoKitID_0: safe.Crypto.Poly25519.ID[0],
			CryptoKitID_1: safe.Crypto.Poly25519.ID[1],
		}
	}

	// Carry the hash forward (SHA3 → SHA3): the chain is continuity-valid.
	prevTerms, _ := genesis.ParsedTerms()
	carried, err := amp.AssembleEpoch(charter, rotTerms(), prevTerms.EffectiveHashKit())
	if err != nil {
		t.Fatalf("AssembleEpoch(carried): %v", err)
	}
	if err := carried.VerifyCharterContinuity(genesis); err != nil {
		t.Fatalf("hash-stable rotation rejected: %v", err)
	}

	// Change the hash (SHA3 → Blake2s, the default): must be rejected.
	changed, err := amp.AssembleEpoch(charter, rotTerms(), safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatalf("AssembleEpoch(changed): %v", err)
	}
	if err := changed.VerifyCharterContinuity(genesis); err == nil {
		t.Fatal("VerifyCharterContinuity accepted a HashKit change across the epoch chain")
	}
	t.Log("epoch chain pins HashKit: carry-forward verifies, change rejected")
}

// TestVerifyCharterContinuity_Refusals pins each continuity check on its own:
// from one valid genesis and rotation, every variant below carries exactly one
// defect and must be refused AuthFailed.  The Sealed-predecessor and HashKit
// refusals are TestEpochVerbatim_Roundtrip step 7 and
// TestVerifyCharterContinuity_HashKitStable.
func TestVerifyCharterContinuity_Refusals(t *testing.T) {
	uid := func(hi, lo uint64) *amp.Tag {
		return &amp.Tag{UID_0: hi, UID_1: lo}
	}
	hashKit := safe.HashKitID_Blake2s_256
	assemble := func(charter *amp.PlanetCharter,
		terms *amp.EpochTerms) *amp.PlanetEpoch {
		t.Helper()
		env, err := amp.AssembleEpoch(charter, terms, hashKit)
		if err != nil {
			t.Fatal(err)
		}
		return env
	}
	charter := &amp.PlanetCharter{
		CharterSchema: 1,
		PlanetID:      uid(0xC0, 0x01),
		GenesisEpoch:  uid(100, 200),
		Declaration:   &amp.Tags{Head: &amp.Tag{Text: "As founded"}},
		Founders:      []*amp.Tag{uid(0xF1, 0)},
	}
	rival := proto.Clone(charter).(*amp.PlanetCharter)
	rival.Declaration = &amp.Tags{Head: &amp.Tag{Text: "As rewritten"}}

	genesisTerms := func() *amp.EpochTerms {
		return &amp.EpochTerms{
			TermsSchema: 1,
			EpochTag:    uid(100, 200),
		}
	}
	rotTerms := func() *amp.EpochTerms {
		return &amp.EpochTerms{
			TermsSchema:   1,
			EpochTag:      uid(101, 201),
			PreviousEpoch: uid(100, 200),
			EpochHeight:   1,
		}
	}
	genesis := assemble(charter, genesisTerms())
	rotation := assemble(charter, rotTerms())
	if err := rotation.VerifyCharterContinuity(genesis); err != nil {
		t.Fatalf("the valid rotation is refused: %v", err)
	}

	// (a) Terms.CharterHash commits to other bytes than the carried Charter:
	// the rival's Terms ride with the founded Charter.
	hashOverOther := &amp.PlanetEpoch{
		Charter: genesis.Charter,
		Terms:   assemble(rival, rotTerms()).Terms,
	}
	genesisHashOverOther := &amp.PlanetEpoch{
		Charter: genesis.Charter,
		Terms:   assemble(rival, genesisTerms()).Terms,
	}
	wrongPrevious := rotTerms()
	wrongPrevious.PreviousEpoch = uid(100, 999)
	skipHeight := rotTerms()
	skipHeight.EpochHeight = 2
	sameHeight := rotTerms()
	sameHeight.EpochHeight = 0

	for _, variant := range []struct {
		name  string
		epoch *amp.PlanetEpoch
		prev  *amp.PlanetEpoch
	}{
		{
			name:  "(a) CharterHash over other bytes",
			epoch: hashOverOther,
			prev:  genesis,
		},
		{
			name:  "(a) CharterHash over other bytes, genesis",
			epoch: genesisHashOverOther,
			prev:  nil,
		},
		{
			name:  "(b) rival self-consistent Charter",
			epoch: assemble(rival, rotTerms()),
			prev:  genesis,
		},
		{
			name:  "(c) PreviousEpoch names another epoch",
			epoch: assemble(charter, wrongPrevious),
			prev:  genesis,
		},
		{
			name:  "(d) EpochHeight = predecessor + 2",
			epoch: assemble(charter, skipHeight),
			prev:  genesis,
		},
		{
			name:  "(d) EpochHeight = predecessor",
			epoch: assemble(charter, sameHeight),
			prev:  genesis,
		},
	} {
		err := variant.epoch.VerifyCharterContinuity(variant.prev)
		if code := status.GetCode(err); code != status.Code_AuthFailed {
			t.Errorf("%s: want AuthFailed, got %v (%v)",
				variant.name, code, err)
		}
	}
}

// TestEpochTerms_IsGenesis_ZeroUIDPrevious pins IsGenesis at the wire edge: a
// PreviousEpoch that is present but carries the zero UID (`1a 00`, an empty
// Tag) names no predecessor, so the terms read as genesis.
func TestEpochTerms_IsGenesis_ZeroUIDPrevious(t *testing.T) {
	terms := &amp.EpochTerms{}
	wire := []byte{0x08, 0x01, 0x1a, 0x00} // TermsSchema 1, PreviousEpoch {}
	if err := proto.Unmarshal(wire, terms); err != nil {
		t.Fatal(err)
	}
	if terms.PreviousEpoch == nil {
		t.Fatal("decoded terms dropped the empty PreviousEpoch")
	}
	if !terms.IsGenesis() {
		t.Error("a zero-UID PreviousEpoch must read as genesis")
	}
	if !(*amp.EpochTerms)(nil).IsGenesis() {
		t.Error("nil terms must read as genesis")
	}
	terms.PreviousEpoch = &amp.Tag{UID_0: 100, UID_1: 200}
	if terms.IsGenesis() {
		t.Error("terms naming a predecessor must not read as genesis")
	}
}
