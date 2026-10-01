package amp_test

// planet_epoch_canonical_test.go holds the PlanetEpoch freeze fixtures: the
// FRAME a CoSignature covers, its CoSignature digest, and one fixed-key
// CoSignature (SD-canonization-spec §2.2.1, §4).
//
// The bytes below are normative.  Any implementation reproducing them is
// conformant; any implementation diverging is not.
//
// Each expected value was computed outside Go, by the method its comment
// names: FRAMEs by a hand-written proto3 encoder (schema transcribed from
// amp.core.proto and safe.proto, fields ascending, proto3 zero values
// omitted), digests by Python hashlib, the signature by OpenSSL Ed25519.
// Pre-freeze, a fixture changes only for a deliberate layout amendment: its
// value is re-derived by such an oracle and the commit states the reason.
// Post-freeze, a fixture is never modified; a new layout adds a new fixture.

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// goldenFrameHex is the FRAME of goldenEpoch: u64BE(len Charter), the Charter
// in 32-byte lines, u64BE(len Terms), the Terms.  Whitespace is layout only.
// Oracle: the proto3 encoder over the goldenLayers values; 996 bytes.
const goldenFrameHex = `
	0000000000000180
	080112121973370000000000002178030000000000001a1219aa000000000000
	0021bb00000000000000221219cc0000000000000021dd000000000000002ac8
	010a1219e10000000000000021e200000000000000121219e300000000000000
	21e4000000000000001934120000000000002178560000000000002880c9e9b2
	06328201e29c992ed791d6b0d6bcd7a8d6b5d790d7a9d6b4d781d696d799d7aa
	2ed791d6b8d6bcd7a8d6b8d6a3d7902ed790d6b1d79cd6b9d794d6b4d691d799
	d79d2ed790d6b5d6a5d7aa2ed794d6b7d7a9d6b8d6bcd781d79ed6b7d696d799
	d6b4d79d2ed795d6b0d790d6b5d6a5d7aa2ed794d6b8d790d6b8d6bdd7a8d6b6
	d7a52ee29c99a0060130073a5d0a12e2010f4520506c75726962757320556e75
	6d2208e20105656e2d555332160a14e201114f7574206f66206d616e792c206f
	6e652e32250a23e201204d656d6265727320617574686f723b20616e796f6e65
	206d617920726561642e421219fe0f00000000000021ff0f0000000000004801
	0000000000000254
	0801121219aaaa00000000000021bbbb0000000000001a121911110000000000
	0021222200000000000020073220b55f2acbfc80f01d73a3ac55a5e21d08694e
	375459a1bd03def097c6a6c7806d39bdcc3ab62bc98d18419fa0a3fff85320e2
	520a526f746174696f6e20375a80010a4f0a07416d706c69667912126172742e
	6d656469612e706c6174666f726d1a0c706c616e65742e746f6f6c7322064869
	204d6f6d2a06616d703a2f2f321219900000000000000021a000000000000000
	3a2d0a121950000000000000002160000000000000002217c20109696d616765
	2f706e67ea010889504e470d0a1a0a6212191000000000000000212000000000
	0000006a4d19300000000000000021400000000000000080016f8801de019001
	cd02a8010bc20109696d6167652f706e67d20112616d703a2f2f706c616e6574
	2f696e646578e20109496e6465782e5461678201121970000000000000002180
	00000000000000900103a00101a80102b2015a09a10000000000000011a20000
	000000000019b10000000000000021b20000000000000029c100000000000000
	31c20000000000000039d10000000000000041d20000000000000049e5000000
	0000000051e600000000000000ca016908808040108080803218904e2080a305
	2880f52430ac0238e80740644880f52450808080fa0158901c621e0a03746370
	12177661756c742e706c616e65742e746f6f6c733a3531393362170a03756470
	12103230332e302e3131332e373a3531393368808080808002800280ceda038a
	0212191c00000000000000211d00000000000000
`

// goldenCharterHashHex = blake2s-256(Charter), the CharterHash in the Terms.
const goldenCharterHashHex = "b55f2acbfc80f01d73a3ac55a5e21d08" +
	"694e375459a1bd03def097c6a6c7806d"

// goldenDigestHex is goldenEpoch's CoSignatureDigest:
// blake2s-256(0x10 ‖ "amp.sig.epoch.v1" ‖ u32BE(996) ‖ FRAME).
const goldenDigestHex = "365d8d25a8b73b5b6a42d523a0680cc1" +
	"3f55dae848bd54a3c82d76608ef54e31"

// goldenFrameSHA3Hex is the FRAME of goldenEpochUnder(HashKitID_SHA3_256): the
// Terms carry HashKit (field 5, `28 03`) and CharterHash = sha3-256(Charter).
// Oracle: the proto3 encoder; 998 bytes.
const goldenFrameSHA3Hex = `
	0000000000000180
	080112121973370000000000002178030000000000001a1219aa000000000000
	0021bb00000000000000221219cc0000000000000021dd000000000000002ac8
	010a1219e10000000000000021e200000000000000121219e300000000000000
	21e4000000000000001934120000000000002178560000000000002880c9e9b2
	06328201e29c992ed791d6b0d6bcd7a8d6b5d790d7a9d6b4d781d696d799d7aa
	2ed791d6b8d6bcd7a8d6b8d6a3d7902ed790d6b1d79cd6b9d794d6b4d691d799
	d79d2ed790d6b5d6a5d7aa2ed794d6b7d7a9d6b8d6bcd781d79ed6b7d696d799
	d6b4d79d2ed795d6b0d790d6b5d6a5d7aa2ed794d6b8d790d6b8d6bdd7a8d6b6
	d7a52ee29c99a0060130073a5d0a12e2010f4520506c75726962757320556e75
	6d2208e20105656e2d555332160a14e201114f7574206f66206d616e792c206f
	6e652e32250a23e201204d656d6265727320617574686f723b20616e796f6e65
	206d617920726561642e421219fe0f00000000000021ff0f0000000000004801
	0000000000000256
	0801121219aaaa00000000000021bbbb0000000000001a121911110000000000
	002122220000000000002007280332205e0aa9e4673525a4540192512804d8f0
	3e80ce1a5d6dc6ea1e4f0cf8b1e19b3339bdcc3ab62bc98d18419fa0a3fff853
	20e2520a526f746174696f6e20375a80010a4f0a07416d706c69667912126172
	742e6d656469612e706c6174666f726d1a0c706c616e65742e746f6f6c732206
	4869204d6f6d2a06616d703a2f2f321219900000000000000021a00000000000
	00003a2d0a121950000000000000002160000000000000002217c20109696d61
	67652f706e67ea010889504e470d0a1a0a621219100000000000000021200000
	00000000006a4d19300000000000000021400000000000000080016f8801de01
	9001cd02a8010bc20109696d6167652f706e67d20112616d703a2f2f706c616e
	65742f696e646578e20109496e6465782e546167820112197000000000000000
	218000000000000000900103a00101a80102b2015a09a10000000000000011a2
	0000000000000019b10000000000000021b20000000000000029c10000000000
	000031c20000000000000039d10000000000000041d20000000000000049e500
	00000000000051e600000000000000ca016908808040108080803218904e2080
	a3052880f52430ac0238e80740644880f52450808080fa0158901c621e0a0374
	637012177661756c742e706c616e65742e746f6f6c733a3531393362170a0375
	647012103230332e302e3131332e373a3531393368808080808002800280ceda
	038a0212191c00000000000000211d00000000000000
`

// goldenCharterHashSHA3Hex = sha3-256(Charter).
const goldenCharterHashSHA3Hex = "5e0aa9e4673525a4540192512804d8f0" +
	"3e80ce1a5d6dc6ea1e4f0cf8b1e19b33"

// goldenDigestSHA3Hex is the SHA3 variant's CoSignatureDigest:
// sha3-256(0x10 ‖ "amp.sig.epoch.v1" ‖ u32BE(998) ‖ FRAME).
const goldenDigestSHA3Hex = "c15f7575d99dd546fee34cb99e1705bd" +
	"b6d70d539eb2a845d0b063750249c056"

// genesisFrameHex is the FRAME of genesisLayers under the default Blake2s_256.
// Oracle: the proto3 encoder; 343 bytes.
const genesisFrameHex = `
	00000000000000a1
	080112121907e5b4c2613a8f0d2171605f4e3d2c1b9a1a1219e5d4c3b2a1f092
	012178695a4b3c2d1e0f3a610a19e20116546865204865786f73706865726520
	436f6d70616374321a0a18e201155765206d616b652061727420746f67657468
	65722e32280a26e20123576861742061206d656d62657220617574686f727320
	7374617973207468656972732e42121968241f9d7a5e3b0c21ce8a4602df9b57
	13
	00000000000000a6
	0801121219e5d4c3b2a1f092012178695a4b3c2d1e0f3220afe2bf9fb54f234b
	823401e30285e6f638c651e44a84274a90295c32c3c5ee26396fda3447b83491
	6241e6e112bbd10a87cc520a4865786f73706865726562121907e5b4c2613a8f
	0d2171605f4e3d2c1b9aca0139621e0a0374637012177661756c742e706c616e
	65742e746f6f6c733a3531393362170a0374637012103230332e302e3131332e
	373a35313933
`

// genesisDigestHex = blake2s-256(0x10 ‖ "amp.sig.epoch.v1" ‖ u32BE(343) ‖
// FRAME).
const genesisDigestHex = "e3c9a9eedc22b95135994f11e6cf1ce1" +
	"178ec1ec9c8f6747ee549947f30b70ce"

// RFC 8032 §7.1 TEST 1: the Ed25519 secret key (seed) and its public key.
const (
	rfc8032Test1SeedHex = "9d61b19deffd5a60ba844af492ec2cc4" +
		"4449c5697b326919703bac031cae7f60"
	rfc8032Test1PubHex = "d75a980182b10ab7d54bfed3c964073a" +
		"0ee172f3daa62325af021a68f707511a"
)

// goldenCoSignatureHex is the RFC 8032 TEST 1 key's Ed25519 signature over
// goldenDigestHex.  Oracle: OpenSSL 3.6 `pkeyutl -sign -rawin` (the same
// method reproduces RFC 8032 TEST 2).
const goldenCoSignatureHex = "c3c9e1c4c189384140a3716b0b306b0d" +
	"f52abba31b79cde6c44e59ea785d7bea" +
	"1621ab78eb5f65a3b44c289f3a6d64f1" +
	"6dd8848b53cee8637da03e50a64e8e0c"

// goldenLayers returns the golden epoch's layers: a fully-populated epoch
// envelope (rotation-shaped: PreviousEpoch and EpochHeight are set, so
// IsGenesis is false).  Every field of PlanetCharter, EpochTerms and each
// message type they nest is set in at least one instance, except
// EpochTerms.HashKit and CharterHash, which AssembleEpoch sets
// (TestGoldenEpoch_EveryFieldOnWire).
func goldenLayers() (*amp.PlanetCharter, *amp.EpochTerms) {
	charter := &amp.PlanetCharter{
		CharterSchema: 1,
		PlanetID:      tagWithUID(0x3773, 888),
		GenesisEpoch:  tagWithUID(0xAA, 0xBB),
		ParentPlanet:  tagWithUID(0xCC, 0xDD),
		Origin: &amp.PlanetOrigin{
			FromPlanet:          tagWithUID(0xE1, 0xE2),
			FromEpoch:           tagWithUID(0xE3, 0xE4),
			FromChronicleHead_0: 0x1234,
			FromChronicleHead_1: 0x5678,
			ForkTime:            1717200000,
			Label:               "✙.בְּרֵאשִׁ֖ית.בָּרָ֣א.אֱלֹהִ֑ים.אֵ֥ת.הַשָּׁמַ֖יִם.וְאֵ֥ת.הָאָֽרֶץ.✙",
			Version:             1,
		},
		Privacy: amp.PrivacyMode_Public,
		Declaration: &amp.Tags{
			Head:    &amp.Tag{Text: "E Pluribus Unum"},
			SubTags: []*amp.Tag{{Text: "en-US"}},
			Children: []*amp.Tags{
				{Head: &amp.Tag{Text: "Out of many, one."}},
				{Head: &amp.Tag{Text: "Members author; anyone may read."}},
			},
		},
		Founders:                  []*amp.Tag{tagWithUID(0xFFE, 0xFFF)},
		GenesisRequiredSignatures: 1,
	}
	terms := &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      tagWithUID(0xAAAA, 0xBBBB),
		PreviousEpoch: tagWithUID(0x1111, 0x2222),
		EpochHeight:   7,
		CryptoKitID_0: safe.Crypto.P256.ID[0],
		CryptoKitID_1: safe.Crypto.P256.ID[1],
		Label:         "Rotation 7",
		Mark: &amp.BrandMark{
			Identity: &amp.BrandIdentity{
				AppName:    "Amplify",
				OrgName:    "art.media.platform",
				AppDomain:  "planet.tools",
				AppDesc:    "Hi Mom",
				URLSchemes: []string{"amp://"},
				NamedBy:    tagWithUID(0x90, 0xA0),
			},
			Glyphs: &amp.Tags{
				Head: tagWithUID(0x50, 0x60),
				SubTags: []*amp.Tag{{
					ContentTypeRaw: "image/png",
					Data: []byte{
						0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
					},
				}},
			},
		},
		Foyer: tagWithUID(0x10, 0x20),
		Index: &amp.Tag{
			UID_0:          0x30,
			UID_1:          0x40,
			I:              111,
			J:              222,
			K:              333,
			Units:          amp.Units_Meters,
			ContentTypeRaw: "image/png",
			URI:            "amp://planet/index",
			Text:           "Index.Tag",
		},
		GovernanceGroup:    tagWithUID(0x70, 0x80),
		RequiredSignatures: 3,
		Seal:               amp.SealState_Paused,
		Admission:          amp.MemberAdmission_AdmissionOpen,
		CodexEdition: &amp.Address{
			PlanetID_0: 0xA1,
			PlanetID_1: 0xA2,
			NodeID_0:   0xB1,
			NodeID_1:   0xB2,
			AttrID_0:   0xC1,
			AttrID_1:   0xC2,
			ItemID_0:   0xD1,
			ItemID_1:   0xD2,
			EditID_0:   0xE5,
			EditID_1:   0xE6,
		},
		VaultConfig: &amp.VaultConfig{
			MaxTxMsgSize:            1 << 20,
			MaxBytesPerWindow:       100 << 20,
			MaxTxPerWindow:          10000,
			RateLimitWindowSecs:     86400,
			QuarantineRetentionSecs: 7 * 86400,
			MaxFutureSkewSecs:       300,
			MaxPendingPerEpoch:      1000,
			MaxPendingEpochs:        100,
			BootstrapTTLSecs:        7 * 86400,
			MaxBlobBytesPerWindow:   500 << 20,
			BlobRateLimitWindowSecs: 3600,
			VaultAddrs: []*amp.VaultAddr{
				{
					Transport: "tcp",
					Address:   []byte("vault.planet.tools:5193"),
				},
				{
					Transport: "udp",
					Address:   []byte("203.0.113.7:5193"),
				},
			},
			MaxBlobBytesPerObject: 64 << 30,
		},
		MaxGracePeriod: 90 * 86400,
		LicenseClass:   tagWithUID(0x1C, 0x1D),
	}
	return charter, terms
}

// genesisLayers returns a genesis shaped the way amp.planet app.home
// assembleGenesis (home.genesis.go) builds one from a genesis pin: one
// founder, Confidential (the zero PrivacyMode), a declaration as
// parseDeclaration emits it (Head + one Child per clause), Foyer = the planet,
// a Poly25519 suite, two bootstrap VaultAddrs, no PreviousEpoch, height 0.
// GenesisRequiredSignatures (0 = all founders) and Admission are unset, as a
// genesis pin leaves them.  The UIDs are fixed stand-ins for NewID / NowID.
func genesisLayers() (*amp.PlanetCharter, *amp.EpochTerms) {
	planet := tagWithUID(0x0D8F3A61C2B4E507, 0x9A1B2C3D4E5F6071)
	epoch := tagWithUID(0x0192F0A1B2C3D4E5, 0x0F1E2D3C4B5A6978)
	charter := &amp.PlanetCharter{
		CharterSchema: 1,
		PlanetID:      planet,
		GenesisEpoch:  epoch,
		Privacy:       amp.PrivacyMode_Confidential,
		Declaration: &amp.Tags{
			Head: &amp.Tag{Text: "The Hexosphere Compact"},
			Children: []*amp.Tags{
				{Head: &amp.Tag{Text: "We make art together."}},
				{Head: &amp.Tag{Text: "What a member authors stays theirs."}},
			},
		},
		Founders: []*amp.Tag{
			tagWithUID(0x0C3B5E7A9D1F2468, 0x13579BDF02468ACE),
		},
	}
	terms := &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      epoch,
		CryptoKitID_0: safe.Crypto.Poly25519.ID[0],
		CryptoKitID_1: safe.Crypto.Poly25519.ID[1],
		Label:         "Hexosphere",
		Foyer:         planet,
		VaultConfig: &amp.VaultConfig{
			VaultAddrs: []*amp.VaultAddr{
				{
					Transport: "tcp",
					Address:   []byte("vault.planet.tools:5193"),
				},
				{
					Transport: "tcp",
					Address:   []byte("203.0.113.7:5193"),
				},
			},
		},
	}
	return charter, terms
}

// goldenEpochUnder assembles the golden layers under hashKit, marshaled once
// via AssembleEpoch, so its FRAME is the verbatim signed artifact.
func goldenEpochUnder(t *testing.T, hashKit safe.HashKitID) *amp.PlanetEpoch {
	t.Helper()
	charter, terms := goldenLayers()
	env, err := amp.AssembleEpoch(charter, terms, hashKit)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

// goldenEpoch is the golden epoch under the default Blake2s_256.  Shared by
// the layout, digest, and exclusion tests.
func goldenEpoch(t *testing.T) *amp.PlanetEpoch {
	t.Helper()
	return goldenEpochUnder(t, safe.HashKitID_Blake2s_256)
}

// fixtureBytes decodes a hex fixture; whitespace is layout only.
func fixtureBytes(t *testing.T, hexText string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(strings.Join(strings.Fields(hexText), ""))
	if err != nil {
		t.Fatalf("bad hex fixture: %v", err)
	}
	return raw
}

// epochFromFrame splits a FRAME at its u64BE length prefixes, independently
// of amp.EpochFrame, into an envelope carrying the two verbatim layers.  The
// FRAME omits the envelope's EpochTag copy; it is restored from the Terms, as
// AssembleEpoch sets it.
func epochFromFrame(t *testing.T, frame []byte) *amp.PlanetEpoch {
	t.Helper()
	rest := frame
	layers := make([][]byte, 0, 2)
	for range 2 {
		if len(rest) < 8 {
			t.Fatalf("FRAME truncated in a length prefix")
		}
		size := binary.BigEndian.Uint64(rest[:8])
		rest = rest[8:]
		if size > uint64(len(rest)) {
			t.Fatalf("FRAME layer of %d bytes overruns the FRAME", size)
		}
		layers = append(layers, rest[:size])
		rest = rest[size:]
	}
	if len(rest) != 0 {
		t.Fatalf("FRAME carries %d trailing bytes", len(rest))
	}
	env := &amp.PlanetEpoch{
		Charter: layers[0],
		Terms:   layers[1],
	}
	terms, err := env.ParsedTerms()
	if err != nil {
		t.Fatalf("FRAME Terms do not parse: %v", err)
	}
	env.EpochTag = terms.EpochTag
	return env
}

// assertFrame fails unless env's FRAME equals the fixture byte for byte.
func assertFrame(t *testing.T, env *amp.PlanetEpoch, fixtureHex string) {
	t.Helper()
	frame, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	gotHex := hex.EncodeToString(frame)
	wantHex := hex.EncodeToString(fixtureBytes(t, fixtureHex))
	if gotHex != wantHex {
		t.Errorf("\nFRAME layout drift\n got:  %s\nwant: %s\n\n"+
			"Every signature over a FRAME of the prior layout stops "+
			"verifying; see the fixture rules atop this file.",
			gotHex, wantHex)
	}
}

// assertDigest fails unless env's CoSignatureDigest equals wantHex.
func assertDigest(t *testing.T, env *amp.PlanetEpoch, wantHex string) {
	t.Helper()
	digest, err := env.CoSignatureDigest()
	if err != nil {
		t.Fatal(err)
	}
	if gotHex := hex.EncodeToString(digest); gotHex != wantHex {
		t.Errorf("\nCoSignatureDigest drift\n got:  %s\nwant: %s",
			gotHex, wantHex)
	}
}

// TestPlanetEpoch_Frame_GoldenFixture pins, in the encoder direction, the
// verbatim FRAME that PlanetEpoch.CoSignatures cover:
//
//	u64BE(len Charter) || Charter || u64BE(len Terms) || Terms
//
// Authority does not depend on cross-language proto-marshal stability: each
// layer is marshaled once and signed and verified as the stored bytes.  The
// fixture locks the layout (field numbers, wire types, framing), so a field
// renumber or a framing change fails here.  A failure is either a defect to
// fix or a deliberate layout amendment, handled by the fixture rules atop
// this file.
func TestPlanetEpoch_Frame_GoldenFixture(t *testing.T) {
	assertFrame(t, goldenEpoch(t), goldenFrameHex)
}

// TestPlanetEpoch_Frame_GoldenDecode pins the decode direction, the authority
// path: the fixed FRAME bytes parse to the golden field values, with no
// unknown fields.  It holds under any legal re-ordering of a marshaler's
// output, which the encoder-direction fixture would not.
func TestPlanetEpoch_Frame_GoldenDecode(t *testing.T) {
	for _, fixture := range []struct {
		hashKit     safe.HashKitID
		frameHex    string
		charterHash string
	}{
		{
			hashKit:     safe.HashKitID_Blake2s_256,
			frameHex:    goldenFrameHex,
			charterHash: goldenCharterHashHex,
		},
		{
			hashKit:     safe.HashKitID_SHA3_256,
			frameHex:    goldenFrameSHA3Hex,
			charterHash: goldenCharterHashSHA3Hex,
		},
	} {
		env := epochFromFrame(t, fixtureBytes(t, fixture.frameHex))
		charter, err := env.ParsedCharter()
		if err != nil {
			t.Fatal(err)
		}
		terms, err := env.ParsedTerms()
		if err != nil {
			t.Fatal(err)
		}
		wantCharter, wantTerms := goldenLayers()
		wantTerms.HashKit = fixture.hashKit
		wantTerms.CharterHash = fixtureBytes(t, fixture.charterHash)
		if !proto.Equal(charter, wantCharter) {
			t.Errorf("%v: decoded Charter differs from the golden values\n"+
				" got:  %v\nwant: %v", fixture.hashKit, charter, wantCharter)
		}
		if !proto.Equal(terms, wantTerms) {
			t.Errorf("%v: decoded Terms differ from the golden values\n"+
				" got:  %v\nwant: %v", fixture.hashKit, terms, wantTerms)
		}
		if err := env.VerifyCharterContinuity(nil); err != nil {
			t.Errorf("%v: CharterHash does not bind the Charter: %v",
				fixture.hashKit, err)
		}
	}
}

// TestPlanetEpoch_Frame_GoldenFixture_SHA3 pins the FRAME and digest under a
// non-default HashKit: the only golden with EpochTerms.HashKit on the wire
// (proto3 omits the Blake2s_256 = 0 default), and the only one whose digest
// depends on CoSignatureDigest hashing under Terms.EffectiveHashKit.
func TestPlanetEpoch_Frame_GoldenFixture_SHA3(t *testing.T) {
	assertFrame(t, goldenEpochUnder(t, safe.HashKitID_SHA3_256),
		goldenFrameSHA3Hex)
	env := epochFromFrame(t, fixtureBytes(t, goldenFrameSHA3Hex))
	assertDigest(t, env, goldenDigestSHA3Hex)
}

// TestPlanetEpoch_CoSignatureDigest_Golden pins the 32 bytes a co-signer
// signs (SD-canonization-spec §2.2.1): the SigningDomain_EpochCoSign tag, the
// u32BE FRAME length, and the hash, over the fixed FRAME bytes.  A change
// strands every stored CoSignature.
func TestPlanetEpoch_CoSignatureDigest_Golden(t *testing.T) {
	env := epochFromFrame(t, fixtureBytes(t, goldenFrameHex))
	assertDigest(t, env, goldenDigestHex)
}

// TestPlanetEpoch_CoSignature_Golden pins one CoSignature over a fixed key:
// the RFC 8032 TEST 1 seed signs goldenDigestHex under the Poly25519 kit
// (Ed25519, deterministic), and VerifyCoSignature accepts the literal over
// the fixed FRAME.
func TestPlanetEpoch_CoSignature_Golden(t *testing.T) {
	prv := ed25519.NewKeyFromSeed(fixtureBytes(t, rfc8032Test1SeedHex))
	pub := fixtureBytes(t, rfc8032Test1PubHex)
	if !bytes.Equal(prv.Public().(ed25519.PublicKey), pub) {
		t.Fatal("RFC 8032 TEST 1 seed does not derive its public key")
	}
	kit, err := safe.CryptoKit(safe.Crypto.Poly25519.ID)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kit.Signing.Sign(fixtureBytes(t, goldenDigestHex), prv)
	if err != nil {
		t.Fatal(err)
	}
	if gotHex := hex.EncodeToString(sig); gotHex != goldenCoSignatureHex {
		t.Errorf("\nCoSignature drift\n got:  %s\nwant: %s",
			gotHex, goldenCoSignatureHex)
	}

	cosig := &amp.CoSignature{
		MemberTag: tagWithUID(0xFFE, 0xFFF),
		Signature: fixtureBytes(t, goldenCoSignatureHex),
	}
	env := epochFromFrame(t, fixtureBytes(t, goldenFrameHex))
	err = env.VerifyCoSignature(cosig, pub, safe.Crypto.Poly25519.ID)
	if err != nil {
		t.Errorf("VerifyCoSignature refused the golden CoSignature: %v", err)
	}
	sha3Env := epochFromFrame(t, fixtureBytes(t, goldenFrameSHA3Hex))
	err = sha3Env.VerifyCoSignature(cosig, pub, safe.Crypto.Poly25519.ID)
	if err == nil {
		t.Error("the golden CoSignature verified over another FRAME")
	}
}

// TestPlanetEpoch_GenesisFrame_Golden pins a production-shaped genesis
// (genesisLayers): it reads as genesis, its CharterHash binds its Charter,
// and its FRAME and digest match the fixture.
func TestPlanetEpoch_GenesisFrame_Golden(t *testing.T) {
	charter, terms := genesisLayers()
	assembled, err := amp.AssembleEpoch(charter, terms,
		safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	assertFrame(t, assembled, genesisFrameHex)

	env := epochFromFrame(t, fixtureBytes(t, genesisFrameHex))
	assertDigest(t, env, genesisDigestHex)
	parsedTerms, err := env.ParsedTerms()
	if err != nil {
		t.Fatal(err)
	}
	if !parsedTerms.IsGenesis() {
		t.Error("the genesis fixture does not read as genesis")
	}
	if parsedTerms.EpochHeight != 0 {
		t.Errorf("genesis EpochHeight = %d, want 0", parsedTerms.EpochHeight)
	}
	parsedCharter, err := env.ParsedCharter()
	if err != nil {
		t.Fatal(err)
	}
	if parsedCharter.IsPublic() {
		t.Error("the genesis fixture reads as Public; it is Confidential")
	}
	if err := env.VerifyCharterContinuity(nil); err != nil {
		t.Errorf("genesis VerifyCharterContinuity(nil): %v", err)
	}
}

// TestGoldenEpoch_EveryFieldOnWire proves the golden pins every field's wire
// layout: each field of PlanetCharter, EpochTerms and every message they nest
// is present in the decoded golden FRAME or its SHA3 variant (which alone
// carries HashKit, since proto3 omits the Blake2s_256 = 0 default).  No field
// is excluded.  A field added to any of these messages fails here until the
// golden sets it and its fixtures are re-derived.
func TestGoldenEpoch_EveryFieldOnWire(t *testing.T) {
	present := map[protoreflect.FullName]bool{}
	for _, frameHex := range []string{goldenFrameHex, goldenFrameSHA3Hex} {
		env := epochFromFrame(t, fixtureBytes(t, frameHex))
		charter, err := env.ParsedCharter()
		if err != nil {
			t.Fatal(err)
		}
		terms, err := env.ParsedTerms()
		if err != nil {
			t.Fatal(err)
		}
		markPresent(t, charter.ProtoReflect(), present)
		markPresent(t, terms.ProtoReflect(), present)
	}

	declared := map[protoreflect.FullName]bool{}
	visited := map[protoreflect.FullName]bool{}
	collectFields((&amp.PlanetCharter{}).ProtoReflect().Descriptor(),
		declared, visited)
	collectFields((&amp.EpochTerms{}).ProtoReflect().Descriptor(),
		declared, visited)

	if len(declared) == 0 {
		t.Fatal("no fields declared; the descriptor walk is broken")
	}
	missing := []string{}
	for name := range declared {
		if !present[name] {
			missing = append(missing, string(name))
		}
	}
	slices.Sort(missing)
	for _, name := range missing {
		t.Errorf("%s is not on the golden wire", name)
	}
}

// markPresent records every field populated in msg and its nested messages,
// and fails on unknown fields.
func markPresent(t *testing.T, msg protoreflect.Message,
	present map[protoreflect.FullName]bool) {
	t.Helper()
	if unknown := msg.GetUnknown(); len(unknown) > 0 {
		t.Errorf("%s carries %d unknown bytes",
			msg.Descriptor().FullName(), len(unknown))
	}
	msg.Range(func(field protoreflect.FieldDescriptor,
		value protoreflect.Value) bool {
		present[field.FullName()] = true
		if field.Message() == nil {
			return true
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				markPresent(t, list.Get(i).Message(), present)
			}
			return true
		}
		markPresent(t, value.Message(), present)
		return true
	})
}

// collectFields records every field of desc and of each message type it
// reaches.
func collectFields(desc protoreflect.MessageDescriptor,
	declared, visited map[protoreflect.FullName]bool) {
	if visited[desc.FullName()] {
		return
	}
	visited[desc.FullName()] = true
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		declared[field.FullName()] = true
		if field.Message() != nil {
			collectFields(field.Message(), declared, visited)
		}
	}
}

// TestPlanetEpoch_Frame_ExcludesCoSignatures proves the FRAME is stable under
// the fields meant to grow after signing (Signatures, Witnesses) and breaks
// under a byte tamper of a signed layer.
func TestPlanetEpoch_Frame_ExcludesCoSignatures(t *testing.T) {
	env := goldenEpoch(t)

	frame, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}

	// Re-reading the stored bytes yields the identical FRAME.
	frame2, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(frame) != hex.EncodeToString(frame2) {
		t.Fatal("SignedBytes must be stable across calls")
	}

	// Signatures + Witnesses (appended after signing) do NOT affect the FRAME.
	env.Signatures = []*amp.CoSignature{
		{MemberTag: tagWithUID(1, 2), Signature: []byte{0xff, 0xfe}},
	}
	env.Witnesses = []*amp.CoSignature{
		{MemberTag: tagWithUID(3, 4), Signature: []byte{0xfd, 0xfc}},
	}
	withSigs, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(frame) != hex.EncodeToString(withSigs) {
		t.Fatal("Signatures + Witnesses must NOT affect the FRAME")
	}

	// Tampering a signed layer changes the FRAME.
	env.Terms[len(env.Terms)-1] ^= 0xFF
	tampered, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(frame) == hex.EncodeToString(tampered) {
		t.Fatal("tampering Terms must change the FRAME")
	}
}

func tagWithUID(hi, lo uint64) *amp.Tag {
	return &amp.Tag{UID_0: hi, UID_1: lo}
}
