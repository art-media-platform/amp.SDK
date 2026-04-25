package safe

import (
	"context"
	"fmt"
)

// yubiGuard implements Guard using a YubiKey's PIV applet.
//
// Key agreement uses ECDH (P-256) on the YubiKey's PIV slot to derive a wrapping key,
// which then encrypts/decrypts the DEK using XChaCha20-Poly1305 + HKDF.
//
// Dependencies:
//
//	go get github.com/go-piv/piv-go/v2/piv
//
// Hardware pairing:
//
//	The YubiKey 5 series supports PIV, and USB-C models (e.g. YubiKey 5C NFC)
//	are ideal for a USB-C+storage combo dongle.
//
//	Bulk YubiKey + USB-C storage combo:
//	  - YubiKey 5C Nano or 5C NFC for the crypto engine
//	  - Pair with a high-capacity USB-C flash drive (e.g. Samsung FIT Plus 64GB–256GB)
//	  - Custom enclosure/keychain housing both in one unit
//	  - Suppliers: Yubico reseller program for bulk (500+ units), combined with
//	    a USB-C flash OEM (Kingston, Samsung, or Phison-based) for the storage side.
//	  - For merch/jewelry: CNC-milled aluminum or resin-cast housing with
//	    the YubiKey PCB and flash storage PCB side-by-side on a shared USB-C connector.
//
//	For active teens/20-somethings:
//	  - NFC ring or wristband with embedded NFC tag + companion app
//	    (the ring taps to authorize, the phone holds the encrypted tome)
//	  - Alternatively, a YubiKey 5 NFC in a custom silicone keychain/carabiner
//	    that clips to a bag or lanyard — functional fashion.
//
// Flow:
//
//	WrapDEK:
//	  1. Generate ephemeral EC key pair
//	  2. ECDH(ephemeral_priv, yubikey_pub) -> shared_secret
//	  3. HKDF(shared_secret, salt, info) -> wrapping_key
//	  4. XChaCha20-Poly1305.Seal(wrapping_key, DEK) -> cipherblob
//	  5. Store ephemeral_pub in WrappedDEK.EphemeralPubKey
//
//	UnwrapDEK:
//	  1. ECDH(yubikey_priv, ephemeral_pub) -> shared_secret  (computed on-card)
//	  2. HKDF(shared_secret, salt, info) -> wrapping_key
//	  3. XChaCha20-Poly1305.Open(wrapping_key, cipherblob) -> DEK
type yubiGuard struct {
	// piv *piv.YubiKey   // uncomment when go-piv is wired in
	// slot piv.Slot
	// pub  *ecdsa.PublicKey
	keyID []byte
}

var _ Guard = (*yubiGuard)(nil)

// NewYubiGuard opens a YubiKey and returns a Guard backed by its PIV slot.
//
// TODO: wire in go-piv/piv-go once hardware is available for testing.
//
//	func NewYubiGuard(pin string) (Guard, error) {
//	    cards, err := piv.Cards()
//	    if err != nil { return nil, err }
//	    if len(cards) == 0 { return nil, fmt.Errorf("safe: no YubiKey found") }
//
//	    yk, err := piv.Open(cards[0])
//	    if err != nil { return nil, err }
//
//	    cert, err := yk.Certificate(piv.SlotKeyManagement)
//	    if err != nil { return nil, err }
//
//	    pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
//	    if !ok { return nil, fmt.Errorf("safe: PIV slot does not contain an EC key") }
//
//	    // Derive a stable keyID from the public key
//	    h := sha256.Sum256(elliptic.Marshal(pub.Curve, pub.X, pub.Y))
//
//	    return &yubiGuard{
//	        piv:   yk,
//	        slot:  piv.SlotKeyManagement,
//	        pub:   pub,
//	        keyID: h[:16],
//	    }, nil
//	}
func NewYubiGuard() (Guard, error) {
	return nil, fmt.Errorf("safe: YubiKey support not yet implemented — install go-piv and connect hardware")
}

func (g *yubiGuard) Info(_ context.Context) (*GuardInfo, error) {
	return &GuardInfo{
		Provider:       "yubikey",
		Label:          "YubiKey PIV Guard",
		KeyID:          g.keyID,
		HardwareBacked: true,
		Removable:      true,
		RequiresPIN:    true,
		RequiresTouch:  true,
		ExportableRoot: false,
	}, nil
}

func (g *yubiGuard) WrapDEK(_ context.Context, dek []byte, aad []byte) (*WrappedDEK, error) {
	// TODO: implement ECDH key agreement + HKDF + AEAD seal
	//
	// 1. ephPriv, ephPub := elliptic.GenerateKey(elliptic.P256(), rand.Reader)
	// 2. shared := ecdh(ephPriv, g.pub)                          // go-piv SharedKey
	// 3. wrappingKey := hkdf(shared, salt, "safe.yubiGuard.WrapDEK")
	// 4. nonce, cipherblob := sealAEAD(wrappingKey, dek, aad)
	// 5. return &WrappedDEK{..., EphemeralPubKey: ephPub, ...}
	return nil, fmt.Errorf("safe: yubiGuard.WrapDEK not yet implemented")
}

func (g *yubiGuard) UnwrapDEK(_ context.Context, wrapped *WrappedDEK, aad []byte) ([]byte, error) {
	// TODO: implement on-card ECDH + HKDF + AEAD open
	//
	// 1. ephPub := unmarshalECPoint(wrapped.EphemeralPubKey)
	// 2. shared := g.piv.SharedKey(ephPub, piv.KeyAuth{PIN: pin})  // on-card ECDH
	// 3. wrappingKey := hkdf(shared, wrapped.Salt, "safe.yubiGuard.WrapDEK")
	// 4. dek := openAEAD(wrappingKey, wrapped.Nonce, wrapped.Cipherblob, aad)
	return nil, fmt.Errorf("safe: yubiGuard.UnwrapDEK not yet implemented")
}

func (g *yubiGuard) Close() error {
	// if g.piv != nil { return g.piv.Close() }
	return nil
}
