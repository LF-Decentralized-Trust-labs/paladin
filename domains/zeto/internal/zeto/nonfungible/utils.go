package nonfungible

import (
	"context"

	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

var (
	encodeTransactionDataFunc func(context.Context, *pb.TransactionSpecification) (tktypes.HexBytes, error) = common.EncodeTransactionData
	encodeProofFunc           func(proof *corepb.SnarkProof) map[string]interface{}                         = common.EncodeProof
	findVerifierFunc          func(string, string, string, []*pb.ResolvedVerifier) *pb.ResolvedVerifier     = domain.FindVerifier
	findAttestationFunc       func(string, []*pb.AttestationResult) *pb.AttestationResult                   = domain.FindAttestation
)

type baseHandler struct {
	name string
}

func (h *baseHandler) getAlgoZetoSnarkBJJ() string {
	return getAlgoZetoSnarkBJJ(h.name)
}

func getAlgoZetoSnarkBJJ(name string) string {
	return zetosignerapi.AlgoDomainZetoSnarkBJJ(name)
}
