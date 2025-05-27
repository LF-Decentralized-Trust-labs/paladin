package common

import (
	"math/big"

	"github.com/iden3/go-iden3-crypto/babyjub"
)

type TestUser struct {
	PrivateKey       *babyjub.PrivateKey
	PublicKey        *babyjub.PublicKey
	PrivateKeyBigInt *big.Int
}

func NewTestKeypair() *TestUser {
	babyJubjubPrivKey := babyjub.NewRandPrivKey()
	babyJubjubPubKey := babyJubjubPrivKey.Public()
	// convert the private key to big.Int for use inside circuits
	privKeyBigInt := babyjub.SkToBigInt(&babyJubjubPrivKey)

	return &TestUser{
		PrivateKey:       &babyJubjubPrivKey,
		PublicKey:        babyJubjubPubKey,
		PrivateKeyBigInt: privKeyBigInt,
	}
}
