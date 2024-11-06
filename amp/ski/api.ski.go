// Package ski (Secure Key Interface) contains crypto abstractions and pluggable interfaces
package ski

import (
	"io"
	"sync"

	"github.com/art-media-platform/amp.SDK/amp"
)

/*****************************************************
** CryptoKit registration
**/

// gCryptoKitRegistry maps a CryptoKitID to an available ("registered") implementation
var gCryptoKitRegistry struct {
	sync.RWMutex
	Lookup map[CryptoKitID]CryptoKit
}

// CryptoKit is a generic pluggable interface that any crypto package can implement.
// It can even be partially implemented (just set nil values for funcs not implemented).
// All calls are assumed to be threadsafe.
type CryptoKit interface {

	// CryptoKitID univeserally identifies a specific crypto suite and version.
	CryptoKitID() CryptoKitID

	// Pre: ioEntry.KeyType, .KeyDomain, .CryptoKitID, and .TimeCreated are set.
	// inRequestedKeySz is the requested length of the private key (ignored for some implementations)
	GenerateNewKey(
		inRequestedKeySz int,
		ioRand io.Reader,
		ioEntry *KeyEntry,
	) error

	/*****************************************************
	** Symmetric encryption (via arbitrary password)
	**/

	// Encrypts a buffer using any arbitrary-length password
	EncryptUsingPassword(
		ioRand io.Reader,
		inMsg []byte,
		inPwd []byte,
	) ([]byte, error)

	// Decrypt a buffer encrypted via EncryptUsingPassword()
	DecryptUsingPassword(
		inMsg []byte,
		inPwd []byte,
	) ([]byte, error)

	/*****************************************************
	** Symmetric encryption
	**/

	Encrypt(
		ioRand io.Reader,
		inMsg []byte,
		inKey []byte,
	) ([]byte, error)

	Decrypt(
		inMsg []byte,
		inKey []byte,
	) ([]byte, error)

	/*****************************************************
	** Asymmetric encryption
	**/

	EncryptFor(
		ioRand io.Reader,
		inMsg []byte,
		inPeerPubKey []byte,
		inPrivKey []byte,
	) ([]byte, error)

	DecryptFrom(
		inMsg []byte,
		inPeerPubKey []byte,
		inPrivKey []byte,
	) ([]byte, error)

	/*****************************************************
	** Signing & Verification
	**/

	Sign(
		inDigest []byte,
		inSignerPrivKey []byte,
	) ([]byte, error)

	VerifySignature(
		inSig []byte,
		inDigest []byte,
		inSignerPubKey []byte,
	) error
}

// RegisterCryptoKit is convenience fuction that registers the given provider so it can be invoked via ski.StartSession()
func RegisterCryptoKit(
	inKit CryptoKit,
) error {
	var err error
	gCryptoKitRegistry.Lock()
	if gCryptoKitRegistry.Lookup == nil {
		gCryptoKitRegistry.Lookup = map[CryptoKitID]CryptoKit{}
	}
	kitID := inKit.CryptoKitID()
	existing := gCryptoKitRegistry.Lookup[kitID]
	if existing == nil {
		gCryptoKitRegistry.Lookup[kitID] = inKit
	} else if existing != inKit {
		err = amp.ErrCode_UnrecognizedCryptoKit.Errorf("the CryptoKit %d (%s) is already registered", kitID, kitID.String())
	}
	gCryptoKitRegistry.Unlock()

	return err
}

/*****************************************************
** CryptoKit convenience functions
**/

// GetCryptoKit fetches a given registered crypto module for use
// If the associated CryptoKit has not been registered, an error is returned.
func GetCryptoKit(
	inCryptoKitID CryptoKitID,
) (CryptoKit, error) {

	gCryptoKitRegistry.RLock()
	kit := gCryptoKitRegistry.Lookup[inCryptoKitID]
	gCryptoKitRegistry.RUnlock()

	if kit == nil {
		return nil, amp.ErrCode_CryptoKitAlreadyRegistered.Errorf("CryptoKit %d not found", inCryptoKitID)
	}

	return kit, nil
}

// VerifySignature is a convenience function that performs signature validation for any registered CryptoKit.
// Returns nil err if the signature of inDigest plus the signer's private key matches the given signature.
// This function is threadsafe.
func VerifySignature(
	inCryptoKitID CryptoKitID,
	inSig []byte,
	inDigest []byte,
	inSignerPubKey []byte,
) error {

	kit, err := GetCryptoKit(inCryptoKitID)
	if err != nil {
		return err
	}

	err = kit.VerifySignature(
		inSig,
		inDigest,
		inSignerPubKey,
	)
	return err
}

/*****************************************************
** ski.EnclaveSession
**/

// EnclaveSession provides crypto services from an opaque crypto services provider
type EnclaveSession interface {

	// Merges all keys in the given KeyTome with this host KeyTome.
	// See docs for KeyTome.MergeTome() on how error conditions are addressed.
	// Note: incoming duplicate key entries are ignored/dropped.
	//ImportKeys(srcTome *KeyTome) error

	// Generates a new KeyEntry for each entry in srcTome (based on the entry's KeyType and CryptoKitID, ignoring the rest) and merges it
	// with the host KeyTome. A copy of each newly generated entry (except for PrivKey) is placed into result KeyTome.
	// See "KeyGen mode" notes where KeyEntry is declared.
	GenerateKeys(srcTome *KeyTome) (*KeyTome, error)

	// Returns a info about a key for the referenced key.
	// If len(inKeyRef.PubKey) == 0, then the newest KeyEntry in the implied Keyring is returned.
	FetchKeyInfo(inKeyRef *KeyRef) (*KeyInfo, error)

	// Performs signing, encryption, and decryption.
	DoCryptOp(inArgs *CryptOpArgs) (*CryptOpOut, error)

	// EndSession ends this session, resulting in the host Provider to call its inOnSessionEnded() callback followed by inOnCompletion.
	// Following a call to EndSession(), no more calls into this interface should be made.
	EndSession(reason string)
}
