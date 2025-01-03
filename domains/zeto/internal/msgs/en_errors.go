// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package msgs

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"golang.org/x/text/language"
)

const zetoPrefix = "PD21"

var registered sync.Once
var ffe = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix(zetoPrefix, "Zeto Domain")
	})
	if !strings.HasPrefix(key, zetoPrefix) {
		panic(fmt.Errorf("must have prefix '%s': %s", zetoPrefix, key))
	}
	return i18n.FFE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	MsgContractNotFound                    = ffe("PD210000", "Contract '%s' not found")
	MsgErrorDecodeBJJKey                   = ffe("PD210001", "Failed to decode babyjubjub key. %s")
	MsgErrorParseDomainConfig              = ffe("PD210002", "Failed to parse domain config json. %s")
	MsgErrorConfigZetoDomain               = ffe("PD210003", "Failed to configure Zeto domain. %s")
	MsgErrorMarshalZetoEventAbis           = ffe("PD210004", "Failed to marshal Zeto event abis. %s")
	MsgErrorValidateInitDeployParams       = ffe("PD210005", "Failed to validate init deploy parameters. %s")
	MsgErrorValidatePrepDeployParams       = ffe("PD210006", "Failed to validate prepare deploy parameters. %s")
	MsgErrorFindCircuitId                  = ffe("PD210007", "Failed to find circuit ID based on the token name. %s")
	MsgErrorValidateInitTxSpec             = ffe("PD210008", "Failed to validate init transaction spec. %s")
	MsgErrorValidateAssembleTxSpec         = ffe("PD210009", "Failed to validate assemble transaction spec. %s")
	MsgErrorValidateEndorseTxParams        = ffe("PD210010", "Failed to validate endorse transaction spec. %s")
	MsgErrorValidatePrepTxSpec             = ffe("PD210011", "Failed to validate prepare transaction spec. %s")
	MsgErrorUnmarshalFuncAbi               = ffe("PD210012", "Failed to unmarshal function abi json. %s")
	MsgErrorDecodeDomainConfig             = ffe("PD210013", "Failed to decode domain config. %s")
	MsgUnknownFunction                     = ffe("PD210014", "Unknown function: %s")
	MsgErrorValidateFuncParams             = ffe("PD210015", "Failed to validate function params. %s")
	MsgUnexpectedFuncSignature             = ffe("PD210016", "Unexpected signature for function '%s': expected='%s', actual='%s'")
	MsgErrorDecodeContractAddress          = ffe("PD210017", "Failed to decode contract address. %s")
	MsgErrorAbiDecodeDomainInstanceConfig  = ffe("PD210018", "Failed to abi decode domain instance config bytes. %s")
	MsgErrorNewSmt                         = ffe("PD210019", "Failed to create Merkle tree for %s: %s")
	MsgErrorHandleEvents                   = ffe("PD210020", "Failed to handle events %s")
	MsgErrorGetNewSmtStates                = ffe("PD210021", "Failed to get new states for Merkle tree %s: %s")
	MsgErrorGetVerifier                    = ffe("PD210022", "Failed to get verifier. %s")
	MsgErrorSign                           = ffe("PD210023", "Failed to sign. %s")
	MsgNoTransferParams                    = ffe("PD210024", "No transfer parameters provided")
	MsgNoParamTo                           = ffe("PD210025", "Parameter 'to' is required (index=%d)")
	MsgNoParamAmount                       = ffe("PD210026", "Parameter 'amount' is required (index=%d)")
	MsgParamAmountInRange                  = ffe("PD210027", "Parameter 'amount' must be in the range (0, 2^100) (index=%d)")
	MsgErrorParseTxId                      = ffe("PD210028", "Failed to parse transaction id. %s")
	MsgErrorMarshalZetoCoinSchemaAbi       = ffe("PD210029", "Failed to marshal Zeto Coin schema abi. %s")
	MsgErrorMarshalMerkleTreeRootSchemaAbi = ffe("PD210030", "Failed to marshal Merkle Tree Root schema abi. %s")
	MsgErrorMarshalMerkleTreeNodeSchemaAbi = ffe("PD210031", "Failed to marshal Merkle Tree Node schema abi. %s")
	MsgErrorQueryAvailCoins                = ffe("PD210032", "Failed to query the state store for available coins. %s")
	MsgInsufficientFunds                   = ffe("PD210033", "Insufficient funds (available=%s)")
	MsgInvalidCoin                         = ffe("PD210034", "Coin %s is invalid: %s")
	MsgMaxCoinsReached                     = ffe("PD210035", "Need more than maximum number (%d) of coins to fulfill the transfer amount total")
	MsgErrorResolveVerifier                = ffe("PD210036", "Failed to resolve verifier: %s")
	MsgErrorLoadOwnerPubKey                = ffe("PD210037", "Failed load owner public key. %s")
	MsgErrorCreateNewState                 = ffe("PD210038", "Failed to create new state. %s")
	MsgErrorPrepTxInputs                   = ffe("PD210039", "Failed to prepare transaction inputs. %s")
	MsgErrorPrepTxOutputs                  = ffe("PD210040", "Failed to prepare transaction outputs. %s")
	MsgErrorPrepTxChange                   = ffe("PD210041", "Failed to prepare outputs for change coins. %s")
	MsgErrorFormatProvingReq               = ffe("PD210042", "Failed to format proving request. %s")
	MsgErrorFindSenderAttestation          = ffe("PD210043", "Did not find 'sender' attestation")
	MsgErrorUnmarshalProvingRes            = ffe("PD210044", "Failed to unmarshal proving response. %s")
	MsgErrorParseInputStates               = ffe("PD210045", "Failed to parse input states. %s")
	MsgErrorHashInputState                 = ffe("PD210046", "Failed to create Poseidon hash for an input coin. %s")
	MsgErrorParseOutputStates              = ffe("PD210047", "Failed to parse output states. %s")
	MsgErrorHashOutputState                = ffe("PD210048", "Failed to create Poseidon hash for an output coin. %s")
	MsgErrorEncodeTxData                   = ffe("PD210049", "Failed to encode transaction data. %s")
	MsgErrorMarshalPrepedParams            = ffe("PD210050", "Failed to marshal prepared params to JSON. %s")
	MsgErrorFindTokenAbi                   = ffe("PD210051", "Failed to find abi for the token contract %s. %s")
	MsgErrorGenerateMTP                    = ffe("PD210052", "Failed to generate merkle proofs. %s")
	MsgErrorMarshalExtraObj                = ffe("PD210053", "Failed to marshal the extras object in the proving request. %s")
	MsgErrorNewLeafNode                    = ffe("PD210054", "Failed to create new leaf node. %s")
	MsgErrorQueryLeafNode                  = ffe("PD210055", "Failed to query the smt DB for leaf node (ref=%s). %s")
	MsgErrorNewNodeIndex                   = ffe("PD210056", "Failed to create new node index from hash. %s")
	MsgErrorHashMismatch                   = ffe("PD210057", "Coin (ref=%s) found in the merkle tree but the persisted hash %s (index=%s) did not match the expected hash %s (index=%s)")
	MsgErrorConvertToCircomProof           = ffe("PD210058", "Failed to convert to circom verifier proof. %s")
	MsgErrorUnmarshalLockProofParams       = ffe("PD210059", "Failed to unmarshal lockProof parameters. %s")
	MsgErrorDecodeTransferCall             = ffe("PD210060", "Failed to decode the transfer call. %s")
	MsgErrorUpdateSMT                      = ffe("PD210061", "Failed to update merkle tree for the %s event. %s")
	MsgErrorAddLeafNode                    = ffe("PD210062", "Failed to add leaf node. %s")
	MsgErrorNewStateFromCommittedRoot      = ffe("PD210063", "Failed to create new state from committed merkle tree root node. %s")
	MsgErrorNewStateFromCommittedNode      = ffe("PD210064", "Failed to create new state from committed merkle tree node. %s")
	MsgErrorQueryAvailStates               = ffe("PD210065", "Failed to find available states for the merkle tree. %s")
	MsgErrorUnmarshalRootIdx               = ffe("PD210066", "Failed to unmarshal root node index. %s")
	MsgErrorUnmarshalSMTNode               = ffe("PD210067", "Failed to unmarshal Merkle Tree Node from state json. %s")
	MsgErrorParseNodeRef                   = ffe("PD210068", "Failed to parse node reference. %s")
	MsgErrorHashSMTNode                    = ffe("PD210069", "Failed to hash merkle tree node. %s")
	MsgErrorParseRootNodeIdx               = ffe("PD210070", "Failed to parse root node index. %s")
	MsgErrorUpsertRootNode                 = ffe("PD210071", "Failed to upsert root node. %s")
	MsgInvalidCompressedPubkeyLen          = ffe("PD210072", "Invalid compressed public key length: %d")
	MsgInvalidPrivkeyLen                   = ffe("PD210073", "Invalid key length: %d")
	MsgInvalidConfigCircuitRoot            = ffe("PD210074", "Circuits root must be set via the configuration file")
	MsgInvalidConfigProvingKeysRoot        = ffe("PD210075", "Proving keys root must be set via the configuration file")
	MsgErrorUnmarshalProvingReqExtras      = ffe("PD210076", "Failed to unmarshal proving request extras for circuit %s. %s")
	MsgErrorParseEncNonce                  = ffe("PD210077", "Failed to parse encryption nonce")
	MsgErrorGenerateRandBytes              = ffe("PD210078", "Failed to generate random bytes for encryption key. %s")
	MsgErrorCalcNullifier                  = ffe("PD210079", "Failed to calculate nullifier. %s")
	MsgErrorDecodeRootExtras               = ffe("PD210080", "Failed to decode root value in extras")
	MsgErrorDecodeMTPNodeExtras            = ffe("PD210081", "Failed to decode node in merkle proof in extras")
	MsgErrorParseInputSalt                 = ffe("PD210082", "Failed to parse input salt")
	MsgErrorParseOutputSalt                = ffe("PD210083", "Failed to parse output salt")
	MsgErrorParseInputCommitment           = ffe("PD210084", "Failed to parse input commitment")
	MsgNotImplemented                      = ffe("PD210085", "Not implemented")
	MsgErrorStateHashMismatch              = ffe("PD210086", "State hash mismatch (hashed vs. received): %s != %s")
	MsgErrorUnmarshalStateData             = ffe("PD210087", "Failed to unmarshal state data. %s")
	MsgErrorSignAlgoMismatch               = ffe("PD210088", "'%s' does not match supported algorithm '%s'")
	MsgErrorVerifierTypeMismatch           = ffe("PD210089", "'%s' does not match supported verifierType '%s'")
	MsgErrorPayloadTypeMismatch            = ffe("PD210090", "'%s' does not match supported payloadType '%s'")
	MsgErrorMissingCircuitID               = ffe("PD210091", "circuit ID is required")
	MsgErrorMissingInputCommitments        = ffe("PD210092", "input commitments are required")
	MsgErrorMissingInputValues             = ffe("PD210093", "input values are required")
	MsgErrorMissingInputSalts              = ffe("PD210094", "input salts are required")
	MsgErrorInputsDiffLength               = ffe("PD210095", "input commitments, values, and salts must have the same length")
	MsgErrorMissingOutputValues            = ffe("PD210096", "output values are required")
	MsgErrorMissingOutputOwners            = ffe("PD210097", "output owner keys are required")
	MsgErrorOutputsDiffLength              = ffe("PD210098", "output values and owner keys must have the same length")
	MsgErrorAssembleInputs                 = ffe("PD210099", "failed to assemble private inputs for witness calculation. %s")
	MsgErrorCalcWitness                    = ffe("PD210100", "failed to calculate the witness. %s")
	MsgErrorGenerateProof                  = ffe("PD210101", "failed to generate proof. %s")
	MsgNoDomainReceipt                     = ffe("PD210102", "Not implemented. See state receipt for coin transfers")
	MsgUnknownSignPayload                  = ffe("PD210103", "Sign payload type '%s' not recognized")
	MsgNullifierGenerationFailed           = ffe("PD210104", "Failed to generate nullifier for coin")
	MsgErrorDecodeDepositCall              = ffe("PD210105", "Failed to decode the deposit call. %s")
	MsgErrorDecodeWithdrawCall             = ffe("PD210106", "Failed to decode the withdraw call. %s")
	MsgParamTotalAmountInRange             = ffe("PD210107", "Total amount must be in the range (0, 2^100)")
)
