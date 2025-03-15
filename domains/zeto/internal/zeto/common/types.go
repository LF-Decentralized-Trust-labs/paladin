package common

import "github.com/hyperledger/firefly-signer/pkg/abi"

var ProofComponents = abi.ParameterArray{
	{Name: "pA", Type: "uint256[2]"},
	{Name: "pB", Type: "uint256[2][2]"},
	{Name: "pC", Type: "uint256[2]"},
}
