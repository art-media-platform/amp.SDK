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

	for i := 0; i < 5; i++ {
		kit, err := GetCryptoKit(kitToTest)
		if err != nil {
			gTesting.Fatal(err)
		}
		testKit(kit, 32)
	}
}

func testKit(kit *CryptoKit, keyLen int) {
	msgLen := 0

	for i := int64(1); i < 37; i++ {
		testKitWithSizes(kit, keyLen, msgLen)
		step, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(7+37*i))
		msgLen += int(step.Int64())
	}
}

func testKitWithSizes(kit *CryptoKit, keyLen, msgLen int) {
	msg := make([]byte, msgLen)
	badMsg := make([]byte, msgLen)

	crypto_rand.Read(msg)

	msgOrig := make([]byte, msgLen)
	copy(msgOrig, msg)

	reader := crypto_rand.Reader

	if !bytes.Equal(msgOrig, msg) {
		gTesting.Fatal("initial msg check failed!?")
	}

	var crypt []byte

	kp := KeyPair{
		Pub: PubKey{CryptoKitID: kit.ID},
	}

	/*****************************************************
	** Symmetric test
	**/
	if kit.Encrypt != nil && kit.Decrypt != nil && kit.GenerateKey != nil {
		kp.Pub.KeyType = KeyType_SymmetricKey
		err := kit.GenerateKey(reader, keyLen, &kp)
		if err != nil {
			gTesting.Fatal(err)
		}

		crypt, err = kit.Encrypt(reader, msgOrig, kp.Prv)
		if err != nil {
			gTesting.Fatal(err)
		}

		if len(badMsg) != len(crypt) {
			badMsg = make([]byte, len(crypt))
		}

		msg, err = kit.Decrypt(crypt, kp.Prv)
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

			msg, err = kit.Decrypt(badMsg, kp.Prv)
			if err == nil {
				gTesting.Fatal("there should have been a decryption error!")
			}
		}
	}

	/*****************************************************
	** Asymmetric test (CryptoKit derives asymmetric keys from signing keys)
	**/
	if kit.EncryptFor != nil && kit.DecryptFrom != nil && kit.GenerateKey != nil {
		kp.Pub.KeyType = KeyType_SigningKey
		err := kit.GenerateKey(reader, keyLen, &kp)
		if err != nil {
			gTesting.Fatal(err)
		}

		recipient := KeyPair{
			Pub: PubKey{
				KeyType:     KeyType_SigningKey,
				CryptoKitID: kit.ID,
			},
		}

		err = kit.GenerateKey(reader, keyLen, &recipient)
		if err != nil {
			gTesting.Fatal(err)
		}

		crypt, err = kit.EncryptFor(reader, msgOrig, recipient.Pub.Bytes, kp.Prv)
		if err != nil {
			gTesting.Fatal(err)
		}

		if len(badMsg) != len(crypt) {
			badMsg = make([]byte, len(crypt))
		}

		msg, err = kit.DecryptFrom(crypt, kp.Pub.Bytes, recipient.Prv)
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

			msg, err = kit.DecryptFrom(badMsg, kp.Pub.Bytes, recipient.Prv)
			if err == nil {
				gTesting.Fatal("there should have been a decryption error!")
			}
		}
	}

	/*****************************************************
	** Signing test
	**/
	if kit.Sign != nil && kit.Verify != nil && kit.GenerateKey != nil {
		kp.Pub.KeyType = KeyType_SigningKey
		err := kit.GenerateKey(reader, keyLen, &kp)
		if err != nil {
			gTesting.Fatal(err)
		}

		crypt, err = kit.Sign(msgOrig, kp.Prv)
		if err != nil {
			gTesting.Fatal(err)
		}

		err = kit.Verify(crypt, msgOrig, kp.Pub.Bytes)
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

			err = kit.Verify(badMsg, msgOrig, kp.Pub.Bytes)
			if err == nil {
				gTesting.Fatal("there should have been a sig failed error!")
			}
		}
	}
}
