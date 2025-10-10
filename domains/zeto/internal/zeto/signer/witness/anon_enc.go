package witness

import (
	"context"
	"crypto/rand"
	"math/big"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
)

type FungibleEncWitnessInputs struct {
	FungibleWitnessInputs
	Enc *pb.ProvingRequestExtras_Encryption
}

func (inputs *FungibleEncWitnessInputs) Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	var nonce *big.Int
	if inputs.Enc != nil && inputs.Enc.EncryptionNonce != "" {
		n, ok := new(big.Int).SetString(inputs.Enc.EncryptionNonce, 10)
		if !ok {
			return nil, i18n.NewError(ctx, msgs.MsgErrorParseEncNonce)
		}
		nonce = n
	} else {
		nonce = crypto.NewEncryptionNonce()
	}
	// TODO: right now we generate the ephemeral key pair and throw away the private key,
	// need more thought on if more management of the key is needed
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		panic(i18n.NewError(ctx, msgs.MsgErrorGenerateRandBytes, err))
	}
	ephemeralKey := key.NewKeyEntryFromPrivateKeyBytes([32]byte(randomBytes))

	m, err := inputs.FungibleWitnessInputs.Assemble(ctx, keyEntry)
	if err != nil {
		return nil, err
	}

	m["encryptionNonce"] = nonce
	m["ecdhPrivateKey"] = ephemeralKey.PrivateKeyForZkp

	return m, nil
}
