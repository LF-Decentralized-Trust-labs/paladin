package secp256k1

import (
	"context"

	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/kata/pkg/signer/common"
)

type sepc256k1Signer struct{}

func Register(registry map[string]api.InMemorySigner) {
	signer := &sepc256k1Signer{}
	registry[api.Algorithm_ECDSA_SECP256K1_PLAINBYTES] = signer
}

func (s *sepc256k1Signer) Sign(ctx context.Context, privateKey []byte, req *proto.SignRequest) (*proto.SignResponse, error) {
	kp, _ := secp256k1.NewSecp256k1KeyPair(privateKey)
	sig, err := kp.SignDirect(req.Payload)
	if err == nil {
		return &proto.SignResponse{Payload: common.CompactRSV(sig)}, nil
	}
	return nil, err
}
