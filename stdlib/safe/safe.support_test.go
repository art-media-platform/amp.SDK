package safe

import (
	"bytes"
	crypto_rand "crypto/rand"
	"math/big"
	math_rand "math/rand"
	"testing"
)

var gTesting *testing.T

func CryptoKitTest(kitToTest CryptoKitID, t *testing.T) {
	gTesting = t

	for i := 0; i < 200; i++ {
		kit, err := GetCryptoKit(kitToTest)
		if err != nil {
			gTesting.Fatal(err)
		}
		testKit(kit, 32)
	}
}

func testKit(kit *CryptoKit, inKeyLen int) {
	msgLen := 0 // start at 0

	for i := int64(1); i < 37; i++ {
		testKitWithSizes(kit, inKeyLen, msgLen)
		step, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(7+37*i))
		msgLen += int(step.Int64())
	}
}

func testKitWithSizes(kit *CryptoKit, inKeyLen, inMsgLen int) {
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

	entry := KeyEntry{
		KeyInfo: &KeyInfo{
			CryptoKitID: kit.ID,
		},
	}

	/*****************************************************
	** Symmetric test
	**/
	if kit.Encrypt != nil && kit.Decrypt != nil && kit.GenerateKey != nil {
		entry.KeyInfo.KeyType = KeyType_SymmetricKey
		err := kit.GenerateKey(reader, inKeyLen, &entry)
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
			if err == nil {
				gTesting.Fatal("there should have been a decryption error!")
			}
		}
	}

	/*****************************************************
	** Asymmetric test
	**/
	if kit.EncryptFor != nil && kit.DecryptFrom != nil && kit.GenerateKey != nil {
		entry.KeyInfo.KeyType = KeyType_AsymmetricKey
		err := kit.GenerateKey(reader, inKeyLen, &entry)
		if err != nil {
			gTesting.Fatal(err)
		}

		recipient := KeyEntry{
			KeyInfo: &KeyInfo{
				KeyType:     KeyType_AsymmetricKey,
				CryptoKitID: kit.ID,
			},
		}

		err = kit.GenerateKey(reader, inKeyLen, &recipient)
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
			if err == nil {
				gTesting.Fatal("there should have been a decryption error!")
			}
		}
	}

	/*****************************************************
	** Signing test
	**/
	if kit.Sign != nil && kit.Verify != nil && kit.GenerateKey != nil {
		entry.KeyInfo.KeyType = KeyType_SigningKey
		err := kit.GenerateKey(reader, inKeyLen, &entry)
		if err != nil {
			gTesting.Fatal(err)
		}

		crypt, err = kit.Sign(msgOrig, entry.PrivKey)
		if err != nil {
			gTesting.Fatal(err)
		}

		err = kit.Verify(crypt, msgOrig, entry.KeyInfo.PubKey)
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

			err = kit.Verify(badMsg, msgOrig, entry.KeyInfo.PubKey)
			if err == nil {
				gTesting.Fatal("there should have been a sig failed error!")
			}
		}
	}
}
