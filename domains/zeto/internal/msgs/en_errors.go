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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"golang.org/x/text/language"
)

const zetoPrefix = "PD21"

var registered sync.Once
var pde = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix(zetoPrefix, "Zeto Domain")
	})
	if !strings.HasPrefix(key, zetoPrefix) {
		panic(fmt.Errorf("must have prefix '%s': %s", zetoPrefix, key))
	}
	return i18n.PDE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	MsgContractNotFound                      = pde("PD210000", "Contract '%s' not found")
	MsgErrorDecodeBJJKey                     = pde("PD210001", "Failed to decode babyjubjub key. %s")
	MsgErrorParseDomainConfig                = pde("PD210002", "Failed to parse domain config json. %s")
	MsgErrorConfigZetoDomain                 = pde("PD210003", "Failed to configure Zeto domain. %s")
	MsgErrorMarshalZetoEventAbis             = pde("PD210004", "Failed to marshal Zeto event abis. %s")
	MsgErrorValidateInitDeployParams         = pde("PD210005", "Failed to validate init deploy parameters. %s")
	MsgErrorValidatePrepDeployParams         = pde("PD210006", "Failed to validate prepare deploy parameters. %s")
	MsgErrorFindCircuitId                    = pde("PD210007", "Failed to find circuit ID based on the token name. %s")
	MsgErrorValidateInitTxSpec               = pde("PD210008", "Failed to validate init transaction spec. %s")
	MsgErrorValidateAssembleTxSpec           = pde("PD210009", "Failed to validate assemble transaction spec. %s")
	MsgErrorValidateEndorseTxParams          = pde("PD210010", "Failed to validate endorse transaction spec. %s")
	MsgErrorValidatePrepTxSpec               = pde("PD210011", "Failed to validate prepare transaction spec. %s")
	MsgErrorUnmarshalFuncAbi                 = pde("PD210012", "Failed to unmarshal function abi json. %s")
	MsgErrorDecodeDomainConfig               = pde("PD210013", "Failed to decode domain config. %s")
	MsgUnknownFunction                       = pde("PD210014", "Unknown function: %s")
	MsgErrorValidateFuncParams               = pde("PD210015", "Failed to validate function params. %s")
	MsgUnexpectedFuncSignature               = pde("PD210016", "Unexpected signature for function '%s': expected='%s', actual='%s'")
	MsgErrorDecodeContractAddress            = pde("PD210017", "Failed to decode contract address. %s")
	MsgErrorAbiDecodeDomainInstanceConfig    = pde("PD210018", "Failed to abi decode domain instance config bytes. %s")
	MsgErrorNewSmt                           = pde("PD210019", "Failed to create Merkle tree for %s: %s")
	MsgErrorHandleEvents                     = pde("PD210020", "Failed to handle events %s")
	MsgErrorGetNewSmtStates                  = pde("PD210021", "Failed to get new states for Merkle tree %s: %s")
	MsgErrorGetVerifier                      = pde("PD210022", "Failed to get verifier. %s")
	MsgErrorSign                             = pde("PD210023", "Failed to sign. %s")
	MsgNoTransferParams                      = pde("PD210024", "No transfer parameters provided")
	MsgNoParamTo                             = pde("PD210025", "Parameter 'to' is required (index=%d)")
	MsgNoParamAmount                         = pde("PD210026", "Parameter 'amount' is required (index=%d)")
	MsgParamAmountInRange                    = pde("PD210027", "Parameter 'amount' must be in the range (0, 2^100) (index=%d)")
	MsgErrorParseTxId                        = pde("PD210028", "Failed to parse transaction id. %s")
	MsgErrorMarshalZetoCoinSchemaAbi         = pde("PD210029", "Failed to marshal Zeto Coin schema abi. %s")
	MsgErrorMarshalMerkleTreeRootSchemaAbi   = pde("PD210030", "Failed to marshal Merkle Tree Root schema abi. %s")
	MsgErrorMarshalMerkleTreeNodeSchemaAbi   = pde("PD210031", "Failed to marshal Merkle Tree Node schema abi. %s")
	MsgErrorQueryAvailCoins                  = pde("PD210032", "Failed to query the state store for available coins. %s")
	MsgInsufficientFunds                     = pde("PD210033", "Insufficient funds (available=%s)")
	MsgInvalidCoin                           = pde("PD210034", "Coin %s is invalid: %s")
	MsgMaxCoinsReached                       = pde("PD210035", "Need more than maximum number (%d) of coins to fulfill the transfer amount total")
	MsgErrorResolveVerifier                  = pde("PD210036", "Failed to resolve verifier: %s")
	MsgErrorLoadOwnerPubKey                  = pde("PD210037", "Failed load owner public key. %s")
	MsgErrorCreateNewState                   = pde("PD210038", "Failed to create new state. %s")
	MsgErrorPrepTxInputs                     = pde("PD210039", "Failed to prepare transaction inputs. %s")
	MsgErrorPrepTxOutputs                    = pde("PD210040", "Failed to prepare transaction outputs. %s")
	MsgErrorPrepTxChange                     = pde("PD210041", "Failed to prepare outputs for change coins. %s")
	MsgErrorFormatProvingReq                 = pde("PD210042", "Failed to format proving request. %s")
	MsgErrorFindSenderAttestation            = pde("PD210043", "Did not find 'sender' attestation")
	MsgErrorUnmarshalProvingRes              = pde("PD210044", "Failed to unmarshal proving response. %s")
	MsgErrorParseInputStates                 = pde("PD210045", "Failed to parse input states. %s")
	MsgErrorHashInputState                   = pde("PD210046", "Failed to create Poseidon hash for an input coin. %s")
	MsgErrorParseOutputStates                = pde("PD210047", "Failed to parse output states. %s")
	MsgErrorHashOutputState                  = pde("PD210048", "Failed to create Poseidon hash for an output coin. %s")
	MsgErrorEncodeTxData                     = pde("PD210049", "Failed to encode transaction data. %s")
	MsgErrorMarshalPrepedParams              = pde("PD210050", "Failed to marshal prepared params to JSON. %s")
	MsgErrorFindTokenAbi                     = pde("PD210051", "Failed to find abi for the token contract %s. %s")
	MsgErrorGenerateMTP                      = pde("PD210052", "Failed to generate merkle proofs. %s")
	MsgErrorMarshalExtraObj                  = pde("PD210053", "Failed to marshal the extras object in the proving request. %s")
	MsgErrorNewLeafNode                      = pde("PD210054", "Failed to create new leaf node. %s")
	MsgErrorQueryLeafNode                    = pde("PD210055", "Failed to query the smt DB for leaf node (ref=%s). %s")
	MsgErrorNewNodeIndex                     = pde("PD210056", "Failed to create new node index from hash. %s")
	MsgErrorHashMismatch                     = pde("PD210057", "Coin (ref=%s) found in the merkle tree but the persisted hash %s (index=%s) did not match the expected hash %s (index=%s)")
	MsgErrorConvertToCircomProof             = pde("PD210058", "Failed to convert to circom verifier proof. %s")
	MsgErrorUnmarshalLockParams              = pde("PD210059", "Failed to unmarshal lock parameters. %s")
	MsgErrorDecodeTransferCall               = pde("PD210060", "Failed to decode the transfer call. %s")
	MsgErrorUpdateSMT                        = pde("PD210061", "Failed to update merkle tree for the %s event. %s")
	MsgErrorAddLeafNode                      = pde("PD210062", "Failed to add leaf node. %s")
	MsgErrorNewStateFromCommittedRoot        = pde("PD210063", "Failed to create new state from committed merkle tree root node. %s")
	MsgErrorNewStateFromCommittedNode        = pde("PD210064", "Failed to create new state from committed merkle tree node. %s")
	MsgErrorQueryAvailStates                 = pde("PD210065", "Failed to find available states for the merkle tree. %s")
	MsgErrorUnmarshalRootIdx                 = pde("PD210066", "Failed to unmarshal root node index. %s")
	MsgErrorUnmarshalSMTNode                 = pde("PD210067", "Failed to unmarshal Merkle Tree Node from state json. %s")
	MsgErrorParseNodeRef                     = pde("PD210068", "Failed to parse node reference. %s")
	MsgErrorHashSMTNode                      = pde("PD210069", "Failed to hash merkle tree node. %s")
	MsgErrorParseRootNodeIdx                 = pde("PD210070", "Failed to parse root node index. %s")
	MsgErrorUpsertRootNode                   = pde("PD210071", "Failed to upsert root node. %s")
	MsgInvalidCompressedPubkeyLen            = pde("PD210072", "Invalid compressed public key length: %d")
	MsgInvalidPrivkeyLen                     = pde("PD210073", "Invalid key length: %d")
	MsgInvalidConfigCircuitRoot              = pde("PD210074", "Circuits root must be set via the configuration file")
	MsgInvalidConfigProvingKeysRoot          = pde("PD210075", "Proving keys root must be set via the configuration file")
	MsgErrorUnmarshalProvingReqExtras        = pde("PD210076", "Failed to unmarshal proving request extras for circuit %s. %s")
	MsgErrorParseEncNonce                    = pde("PD210077", "Failed to parse encryption nonce")
	MsgErrorGenerateRandBytes                = pde("PD210078", "Failed to generate random bytes for encryption key. %s")
	MsgErrorCalcNullifier                    = pde("PD210079", "Failed to calculate nullifier. %s")
	MsgErrorDecodeRootExtras                 = pde("PD210080", "Failed to decode root value in extras")
	MsgErrorDecodeMTPNodeExtras              = pde("PD210081", "Failed to decode node in merkle proof in extras")
	MsgErrorParseInputSalt                   = pde("PD210082", "Failed to parse input salt")
	MsgErrorParseOutputSalt                  = pde("PD210083", "Failed to parse output salt")
	MsgErrorParseInputCommitment             = pde("PD210084", "Failed to parse input commitment")
	MsgNotImplemented                        = pde("PD210085", "Not implemented")
	MsgErrorStateHashMismatch                = pde("PD210086", "State hash mismatch (hashed vs. received): %s != %s")
	MsgErrorUnmarshalStateData               = pde("PD210087", "Failed to unmarshal state data. %s")
	MsgErrorSignAlgoMismatch                 = pde("PD210088", "'%s' does not match supported algorithm '%s'")
	MsgErrorVerifierTypeMismatch             = pde("PD210089", "'%s' does not match supported verifierTypes '%s'")
	MsgErrorPayloadTypeMismatch              = pde("PD210090", "'%s' does not match supported payloadType '%s'")
	MsgErrorMissingCircuitID                 = pde("PD210091", "circuit ID is required")
	MsgErrorMissingInputCommitments          = pde("PD210092", "input commitments are required")
	MsgErrorMissingInputValues               = pde("PD210093", "input values are required")
	MsgErrorMissingInputSalts                = pde("PD210094", "input salts are required")
	MsgErrorInputsDiffLength                 = pde("PD210095", "input commitments, values, and salts must have the same length")
	MsgErrorMissingOutputValues              = pde("PD210096", "output values are required")
	MsgErrorMissingOutputOwners              = pde("PD210097", "output owner keys are required")
	MsgErrorOutputsDiffLength                = pde("PD210098", "output values and owner keys must have the same length")
	MsgErrorAssembleInputs                   = pde("PD210099", "failed to assemble private inputs for witness calculation. %s")
	MsgErrorCalcWitness                      = pde("PD210100", "failed to calculate the witness. %s")
	MsgErrorGenerateProof                    = pde("PD210101", "failed to generate proof. %s")
	MsgNoDomainReceipt                       = pde("PD210102", "Not implemented. See state receipt for coin transfers")
	MsgUnknownSignPayload                    = pde("PD210103", "Sign payload type '%s' not recognized")
	MsgNullifierGenerationFailed             = pde("PD210104", "Failed to generate nullifier for coin")
	MsgErrorDecodeDepositCall                = pde("PD210105", "Failed to decode the deposit call. %s")
	MsgErrorDecodeWithdrawCall               = pde("PD210106", "Failed to decode the withdraw call. %s")
	MsgParamTotalAmountInRange               = pde("PD210107", "Total amount must be in the range (0, 2^100)")
	MsgStatesNotFound                        = pde("PD210108", "States not found: %s")
	MsgErrorHashState                        = pde("PD210109", "Failed to create Poseidon hash for token. %s")
	MsgErrorTokenIDToString                  = pde("PD210111", "Failed to convert token ID to string. %s")
	MsgInvalidUTXO                           = pde("PD210112", "Failed to set UTXO. %s")
	MsgErrorNoTokensForTransfer              = pde("PD210113", "Error no tokens to transfer")
	MsgNoParamTokenID                        = pde("PD210114", "Parameter 'tokenID' is required (index=%d)")
	MsgNoParamURI                            = pde("PD210115", "Parameter 'uri' is required (index=%d)")
	MsgParamTokenIDNotEmpty                  = pde("PD210116", "Parameter 'tokenID' is not empty (index=%d)")
	MsgErrorParseFieldModulus                = pde("PD210117", "Failed to parse field modulus.")
	MsgErrorGenerateRandomNumber             = pde("PD210118", "Failed to generate random number")
	MsgErrorMarshalValuesFungible            = pde("PD210119", "Failed to marshal TokenSecrets_Fungible")
	MsgErrorMarshalValuesNonFungible         = pde("PD210120", "Failed to marshal TokenSecrets_NonFungible")
	MsgErrorUnmarshalTokenSecretsFungible    = pde("PD210121", "Failed to unmarshal TokenSecrets_Fungible. %s")
	MsgErrorUnmarshalTokenSecretsNonFungible = pde("PD210122", "Failed to unmarshal TokenSecrets_NonFungible. %s")
	MsgErrorTokenTypeMismatch                = pde("PD210123", "Token type mismatch. Actual: %s, Expected: %s")
	MsgErrorProvingReqCommonNil              = pde("PD210124", "Proving request common is nil")
	MsgErrorLockDelegateNotFound             = pde("PD210125", "lock delegate not found by ID in the local wallet: %s")
	MsgErrorMissingLockInputs                = pde("PD210126", "locked inputs are required")
	MsgErrorQueryLockedInputs                = pde("PD210127", "Failed to query the state store for locked inputs. Expected: %d. Found: %d")
	MsgErrorInputNotLocked                   = pde("PD210128", "Input %s is not locked")
	MsgErrorInsufficientInputAmount          = pde("PD210129", "Insufficient input amount (total=%s) for the transfers (total=%s)")
	MsgErrorNoLockedInputs                   = pde("PD210130", "No locked inputs found")
	MsgErrorParseInfoStates                  = pde("PD210131", "Failed to parse info states. %s")
	MsgErrorDecodeDelegateExtras             = pde("PD210132", "Failed to decode delegate in extras. %s")
	MsgErrorMissingLockDelegate              = pde("PD210133", "lock delegate is required")
	MsgFailedToQueryStatesById               = pde("PD210134", "Failed to query states by IDs. Wanted: %d, Found: %d")
	MsgNoParamAccount                        = pde("PD210135", "Parameter 'account' is required")
	MsgErrorValidateInitCallTxSpec           = pde("PD210136", "Failed to validate init call transaction spec. %s")
	MsgErrorValidateExecCallTxSpec           = pde("PD210137", "Failed to validate execute call transaction spec. %s")
	MsgErrorGetAccountBalance                = pde("PD210138", "Failed to get account balance. %s")
	MsgErrorHandlerImplementationNotFound    = pde("PD210139", "Handler implementation not found. %s")
	MsgUnknownSmtType                        = pde("PD210140", "Unknown states merkle tree type: %d")
	MsgErrorDecodePublicKeyFromHex           = pde("PD210141", "Failed to decode public key from compressed hex. %s")
	MsgErrorDecodePrivateKey                 = pde("PD210142", "Failed to decode private key. %s")
)
