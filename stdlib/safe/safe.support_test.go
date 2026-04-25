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
		kit, err := GetKit(kitToTest)
		if err != nil {
			gTesting.Fatal(err)
		}
		testKit(kit, 32)
	}
}

func testKit(kit *KitSpec, keyLen int) {
	msgLen := 0

	for i := int64(1); i < 37; i++ {
		testKitWithSizes(kit, keyLen, msgLen)
		step, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(7+37*i))
		msgLen += int(step.Int64())
	}
}

func testKitWithSizes(kit *KitSpec, keyLen, msgLen int) {
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

	/*****************************************************
	** Symmetric test (kit-agnostic — uses package-level AEAD primitives)
	**/
	{
		symKey := make([]byte, DEKSize)
		crypto_rand.Read(symKey)

		nonce, ct, err := SealAEAD(reader, symKey, msgOrig, nil)
		if err != nil {
			gTesting.Fatal(err)
		}
		crypt = append(nonce, ct...)

		if len(badMsg) != len(crypt) {
			badMsg = make([]byte, len(crypt))
		}

		msg, err = OpenAEAD(symKey, crypt[:NonceSize], crypt[NonceSize:], nil)
		if err != nil {
			gTesting.Fatal(err)
		}
		if !bytes.Equal(msg, msgOrig) {
			gTesting.Fatal("symmetric decrypt failed check")
		}

		for k := 0; k < 100; k++ {
			rndPos := math_rand.Int31n(int32(len(crypt)))
			rndAdj := 1 + byte(math_rand.Int31n(254))
			copy(badMsg, crypt)
			badMsg[rndPos] += rndAdj

			_, err = OpenAEAD(symKey, badMsg[:NonceSize], badMsg[NonceSize:], nil)
			if err == nil {
				gTesting.Fatal("there should have been a decryption error!")
			}
		}
	}

	/*****************************************************
	** Asymmetric test (EncryptOps capability)
	**/
	if kit.Encrypt != nil && kit.Encrypt.Seal != nil && kit.Encrypt.Open != nil && kit.Encrypt.Generate != nil {
		kp := KeyPair{Pub: PubKey{KeyType: KeyType_AsymmetricKey, CryptoKitID: kit.ID}}
		if err := kit.Encrypt.Generate(reader, &kp); err != nil {
			gTesting.Fatal(err)
		}

		recipient := KeyPair{Pub: PubKey{KeyType: KeyType_AsymmetricKey, CryptoKitID: kit.ID}}
		if err := kit.Encrypt.Generate(reader, &recipient); err != nil {
			gTesting.Fatal(err)
		}

		var err error
		crypt, err = kit.Encrypt.Seal(reader, msgOrig, recipient.Pub.Bytes, kp.Prv)
		if err != nil {
			gTesting.Fatal(err)
		}

		if len(badMsg) != len(crypt) {
			badMsg = make([]byte, len(crypt))
		}

		msg, err = kit.Encrypt.Open(crypt, kp.Pub.Bytes, recipient.Prv)
		if err != nil {
			gTesting.Fatal(err)
		}
		if !bytes.Equal(msg, msgOrig) {
			gTesting.Fatal("asymmetric decrypt failed check")
		}

		for k := 0; k < 100; k++ {
			rndPos := math_rand.Int31n(int32(len(crypt)))
			rndAdj := 1 + byte(math_rand.Int31n(254))
			copy(badMsg, crypt)
			badMsg[rndPos] += rndAdj

			_, err = kit.Encrypt.Open(badMsg, kp.Pub.Bytes, recipient.Prv)
			if err == nil {
				gTesting.Fatal("there should have been a decryption error!")
			}
		}
	}

	/*****************************************************
	** Signing test (SignOps capability)
	**/
	if kit.Signing != nil && kit.Signing.Sign != nil && kit.Signing.Verify != nil && kit.Signing.Generate != nil {
		kp := KeyPair{Pub: PubKey{KeyType: KeyType_SigningKey, CryptoKitID: kit.ID}}
		if err := kit.Signing.Generate(reader, &kp); err != nil {
			gTesting.Fatal(err)
		}

		var err error
		crypt, err = kit.Signing.Sign(msgOrig, kp.Prv)
		if err != nil {
			gTesting.Fatal(err)
		}

		if err := kit.Signing.Verify(crypt, msgOrig, kp.Pub.Bytes); err != nil {
			gTesting.Fatal(err)
		}

		if len(badMsg) != len(crypt) {
			badMsg = make([]byte, len(crypt))
		}

		for k := 0; k < 100; k++ {
			rndPos := math_rand.Int31n(int32(len(crypt)))
			rndAdj := 1 + byte(math_rand.Int31n(254))
			copy(badMsg, crypt)
			badMsg[rndPos] += rndAdj

			err = kit.Signing.Verify(badMsg, msgOrig, kp.Pub.Bytes)
			if err == nil {
				gTesting.Fatal("there should have been a sig failed error!")
			}
		}
	}
}
