package ski

import (
	"bytes"
	crypto_rand "crypto/rand"
	"math/big"
	math_rand "math/rand"

	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
)

var gTesting *testing.T

func TestCryptoKits(kitsToTest []CryptoKitID, t *testing.T) {
	gTesting = t

	for _, kitID := range kitsToTest {
		for i := 0; i < 200; i++ {
			kit, err := GetCryptoKit(kitID)
			if err != nil {
				gTesting.Fatal(err)
			}
			testKit(kit, 32)
		}
	}
}

func testKit(kit CryptoKit, inKeyLen int) {
	msgLen := 0 // start at 0

	for i := int64(1); i < 37; i++ {
		testKitWithSizes(kit, inKeyLen, msgLen)
		step, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(7+37*i))
		msgLen += int(step.Int64())
	}
}

func testKitWithSizes(kit CryptoKit, inKeyLen, inMsgLen int) {
	msg := make([]byte, inMsgLen)
	badMsg := make([]byte, inMsgLen)

	crypto_rand.Read(msg)

	msgOrig := make([]byte, inMsgLen)
	copy(msgOrig, msg)

	reader := crypto_rand.Reader

	if !bytes.Equal(msgOrig, msg) {
		gTesting.Fatal("initial msg check failed!?")
	}

	var crypt []byte

	/*****************************************************
	** Symmetric password test
	**/
	{
		passLen := 1 + math_rand.Int31n(30)
		pass := make([]byte, passLen)
		crypto_rand.Read(pass)

		crypt, err := kit.EncryptUsingPassword(reader, msgOrig, pass)
		if !amp.IsError(err, amp.ErrCode_Unimplemented) {
			if err != nil {
				gTesting.Fatal(err)
			}

			if len(badMsg) != len(crypt) {
				badMsg = make([]byte, len(crypt))
			}

			msg, err = kit.DecryptUsingPassword(crypt, pass)
			if err != nil {
				gTesting.Fatal(err)
			}
			if !bytes.Equal(msg, msgOrig) {
				gTesting.Fatal("symmetric decrypt failed check")
			}
		}
	}

	entry := KeyEntry{
		KeyInfo: &KeyInfo{
			CryptoKitID: kit.CryptoKitID(),
		},
	}

	/*****************************************************
	** Symmetric test
	**/
	{
		entry.KeyInfo.KeyType = KeyType_SymmetricKey
		err := kit.GenerateNewKey(inKeyLen, reader, &entry)
		if !amp.IsError(err, amp.ErrCode_Unimplemented) {
			if err != nil {
				gTesting.Fatal(err)
			}

			crypt, err = kit.Encrypt(reader, msgOrig, entry.PrivKey)
			if err != nil {
				gTesting.Fatal(err)
			}

			if len(badMsg) != len(crypt) {
				badMsg = make([]byte, len(crypt))
			}

			msg, err = kit.Decrypt(crypt, entry.PrivKey)
			if err != nil {
				gTesting.Fatal(err)
			}
			if !bytes.Equal(msg, msgOrig) {
				gTesting.Fatal("symmetric decrypt failed check")
			}

			// Vary the data slightly to test
			for k := 0; k < 100; k++ {

				rndPos := math_rand.Int31n(int32(len(crypt)))
				rndAdj := 1 + byte(math_rand.Int31n(254))
				copy(badMsg, crypt)
				badMsg[rndPos] += rndAdj

				msg, err = kit.Decrypt(badMsg, entry.PrivKey)
				if msg != nil {
					gTesting.Fatal("should have got a nil msg")
				}
				if err == nil {
					gTesting.Fatal("there should have been a decryption error!")
				}
			}
		}
	}

	/*****************************************************
	** Asymmetric test
	**/
	{
		entry.KeyInfo.KeyType = KeyType_AsymmetricKey
		err := kit.GenerateNewKey(inKeyLen, reader, &entry)
		if !amp.IsError(err, amp.ErrCode_Unimplemented) {
			if err != nil {
				gTesting.Fatal(err)
			}

			recipient := KeyEntry{
				KeyInfo: &KeyInfo{},
			}
			*recipient.KeyInfo = *entry.KeyInfo

			err = kit.GenerateNewKey(inKeyLen, reader, &recipient)
			if err != nil {
				gTesting.Fatal(err)
			}

			crypt, err = kit.EncryptFor(reader, msgOrig, recipient.KeyInfo.PubKey, entry.PrivKey)
			if err != nil {
				gTesting.Fatal(err)
			}

			if len(badMsg) != len(crypt) {
				badMsg = make([]byte, len(crypt))
			}

			msg, err = kit.DecryptFrom(crypt, entry.KeyInfo.PubKey, recipient.PrivKey)
			if err != nil {
				gTesting.Fatal(err)
			}
			if !bytes.Equal(msg, msgOrig) {
				gTesting.Fatal("asymmetric decrypt failed check")
			}

			// Vary the data slightly to test
			for k := 0; k < 100; k++ {
				rndPos := math_rand.Int31n(int32(len(crypt)))
				rndAdj := 1 + byte(math_rand.Int31n(254))
				copy(badMsg, crypt)
				badMsg[rndPos] += rndAdj

				msg, err = kit.DecryptFrom(badMsg, entry.KeyInfo.PubKey, recipient.PrivKey)
				if msg != nil {
					gTesting.Fatal("should have got a nil msg")
				}
				if err == nil {
					gTesting.Fatal("there should have been a decryption error!")
				}
			}
		}
	}

	/*****************************************************
	** Signing test
	**/
	{
		entry.KeyInfo.KeyType = KeyType_SigningKey
		err := kit.GenerateNewKey(inKeyLen, reader, &entry)
		if !amp.IsError(err, amp.ErrCode_Unimplemented) {
			if err != nil {
				gTesting.Fatal(err)
			}

			crypt, err = kit.Sign(msgOrig, entry.PrivKey)
			if err != nil {
				gTesting.Fatal(err)
			}

			err = kit.VerifySignature(crypt, msgOrig, entry.KeyInfo.PubKey)
			if err != nil {
				gTesting.Fatal(err)
			}

			if len(badMsg) != len(crypt) {
				badMsg = make([]byte, len(crypt))
			}

			// Vary the data slightly to test
			for k := 0; k < 100; k++ {
				rndPos := math_rand.Int31n(int32(len(crypt)))
				rndAdj := 1 + byte(math_rand.Int31n(254))
				copy(badMsg, crypt)
				badMsg[rndPos] += rndAdj

				err = kit.VerifySignature(badMsg, msgOrig, entry.KeyInfo.PubKey)
				if !amp.IsError(err, amp.ErrCode_VerifySignatureFailed) {
					gTesting.Fatal("there should have been a sig failed error!")
				}
			}
		}
	}
}
