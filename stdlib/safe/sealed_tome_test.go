package safe_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// TestSealedTome_BadNonceLengthFailsClosed persists a wrong-length nonce in
// either nonce field of a sealed tome (the wrapped DEK's or the tome's) and
// requires both public openers to return an error and no live store.  The
// unchanged 24-byte case is the control: it reopens and recovers the key.
func TestSealedTome_BadNonceLengthFailsClosed(t *testing.T) {
	for _, kind := range []string{"identity", "epoch"} {
		for _, field := range []string{"wrapped", "payload"} {
			for _, size := range []int{0, safe.NonceSize - 1, safe.NonceSize, safe.NonceSize + 1} {
				t.Run(fmt.Sprintf("%s/%s/%d", kind, field, size), func(t *testing.T) {
					defer func() {
						if recovered := recover(); recovered != nil {
							t.Errorf("tome open panicked instead of returning an error: %v", recovered)
						}
					}()
					ctx := t.Context()
					pathname := filepath.Join(t.TempDir(), "nonce.tome")
					store := safe.NewLocalTomeStore(pathname)
					guard := safe.NewFileGuard([]byte("sealed-tome-root"), []byte("sealed-tome"))
					defer guard.Close()
					aad := []byte("sealed-tome")
					keyringID := tag.NowID()
					epochID := tag.NowID()
					keyBytes := bytes.Repeat([]byte{0x5a}, safe.DEKSize)
					defer safe.Zero(keyBytes)

					if kind == "identity" {
						enc, err := safe.OpenEnclave(ctx, store, guard, aad)
						if err != nil {
							t.Fatal(err)
						}
						_, err = enc.GenerateKey(ctx, keyringID, safe.KeySpec{
							CryptoKitID: safe.Crypto.Poly25519.ID,
							KeyType:     safe.KeyType_SigningKey,
						})
						closeErr := enc.Close(ctx)
						if err != nil || closeErr != nil {
							t.Fatalf("prime identity tome: generate=%v close=%v", err, closeErr)
						}
					} else {
						keys, err := safe.OpenEpochKeyStore(ctx, store, guard, aad)
						if err != nil {
							t.Fatal(err)
						}
						err = keys.PutKey(ctx, safe.Scope(keyringID), safe.SymKey{
							CryptoKitID: safe.Crypto.Poly25519.ID,
							EpochID:     epochID,
							Role:        safe.KeyRole_ContentKey,
							Bytes:       keyBytes,
						})
						closeErr := keys.Close(ctx)
						if err != nil || closeErr != nil {
							t.Fatalf("prime epoch tome: put=%v close=%v", err, closeErr)
						}
					}

					sealed, err := store.Load(ctx)
					if err != nil || sealed == nil || sealed.WrappedDEK == nil {
						t.Fatalf("load primed tome: %v", err)
					}
					if size != safe.NonceSize {
						if field == "wrapped" {
							sealed.WrappedDEK.Nonce = make([]byte, size)
						} else {
							sealed.TomeNonce = make([]byte, size)
						}
					}
					if err := store.Save(ctx, sealed); err != nil {
						t.Fatal(err)
					}

					if kind == "identity" {
						enc, err := safe.OpenEnclave(ctx, store, guard, aad)
						if size != safe.NonceSize {
							if err == nil || enc != nil {
								t.Fatal("malformed nonce produced a live identity enclave")
							}
						} else {
							if err != nil {
								t.Fatal(err)
							}
							_, fetchErr := enc.FetchPubKey(&safe.KeyRef{
								KeyringID_0: keyringID[0],
								KeyringID_1: keyringID[1],
								Type:        safe.KeyType_SigningKey,
							})
							closeErr := enc.Close(ctx)
							if fetchErr != nil || closeErr != nil {
								t.Fatalf("valid identity reopen: fetch=%v close=%v", fetchErr, closeErr)
							}
						}
					} else {
						keys, err := safe.OpenEpochKeyStore(ctx, store, guard, aad)
						if size != safe.NonceSize {
							if err == nil || keys != nil {
								t.Fatal("malformed nonce produced a live epoch store")
							}
						} else {
							if err != nil {
								t.Fatal(err)
							}
							key, getErr := keys.GetKey(safe.Scope(keyringID), epochID, safe.KeyRole_ContentKey)
							matches := bytes.Equal(key.Bytes, keyBytes)
							key.Zero()
							closeErr := keys.Close(ctx)
							if getErr != nil || closeErr != nil || !matches {
								t.Fatalf("valid epoch reopen: get=%v close=%v matches=%v", getErr, closeErr, matches)
							}
						}
					}
				})
			}
		}
	}
}
