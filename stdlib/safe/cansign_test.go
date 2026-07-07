package safe_test

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"strings"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"

	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519" // register the Poly25519 suite
)

// TestEnclave_CanSign proves CanSign reports the PRIVATE-half signing capability, distinct from
// FetchPubKey.  A generated SigningKey can sign; a public-only ADOPTED key (PrvKey nil — what a
// TOFU-declared peer key looks like) is still fetchable via FetchPubKey but CANNOT sign.  This is
// the over-report self-sovereign authorship must gate on CanSign, not FetchPubKey.
func TestEnclave_CanSign(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := safe.NewLocalTomeStore(filepath.Join(dir, "cansign.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("id"))
	defer guard.Close()
	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("cansign-test"))
	if err != nil {
		t.Fatalf("OpenEnclave: %v", err)
	}
	defer enc.Close(ctx)

	// (1) A generated SigningKey — private half held → CanSign true.
	full := tag.NewID()
	if _, err := enc.GenerateKey(full, safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fullRef := &safe.KeyRef{KeyringID_0: full[0], KeyringID_1: full[1], Type: safe.KeyType_SigningKey}
	if !enc.CanSign(fullRef) {
		t.Fatal("CanSign must be TRUE for a generated SigningKey (private half held)")
	}

	// (2) A public-only adopted key — import only the public half (PrvKey nil).
	kit, err := safe.CryptoKit(safe.Crypto.Poly25519.ID)
	if err != nil {
		t.Fatalf("CryptoKit: %v", err)
	}
	kp := safe.KeyPair{Pub: safe.PubKey{CryptoKitID: safe.Crypto.Poly25519.ID, KeyType: safe.KeyType_SigningKey}}
	if err := kit.Signing.Generate(rand.Reader, &kp); err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	pubOnly := tag.NewID()
	if err := enc.ImportKey(pubOnly, safe.KeyPair{Pub: kp.Pub}); err != nil { // Prv omitted ⇒ public-only
		t.Fatalf("ImportKey(public-only): %v", err)
	}
	pubRef := &safe.KeyRef{KeyringID_0: pubOnly[0], KeyringID_1: pubOnly[1], Type: safe.KeyType_SigningKey}
	if _, err := enc.FetchPubKey(pubRef); err != nil {
		t.Fatalf("FetchPubKey(public-only) must succeed — the over-report this guards against: %v", err)
	}
	if enc.CanSign(pubRef) {
		t.Fatal("CanSign must be FALSE for a public-only adopted key (no private half) — the durable signing gate")
	}

	// (3) SignRaw on the public-only key refuses loudly — never a garbage
	// signature (or kit panic) from an empty private half.
	if _, err := enc.SignRaw(pubRef, make([]byte, 32)); err == nil {
		t.Fatal("SignRaw must refuse a public-only key")
	} else if !strings.Contains(err.Error(), "public-only") {
		t.Fatalf("SignRaw refusal must name the public-only key state: %v", err)
	}
}
