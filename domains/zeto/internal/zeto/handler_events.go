package zeto

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/node"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/smt"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (z *Zeto) handleMintEvent(ctx context.Context, tree core.SparseMerkleTree, storage smt.StatesStorage, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var mint MintEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &mint); err == nil {
		txID := decodeTransactionData(mint.Data)
		if txID == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for mint event: %s. Skip to the next event", mint.Data)
			return nil
		}
		res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
			TransactionId: txID.String(),
			Location:      ev.Location,
		})
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txID, mint.Outputs)...)
		if common.IsNullifiersToken(tokenName) {
			err := z.updateMerkleTree(ctx, tree, storage, txID, mint.Outputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOMint", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal mint event: %s", err)
	}
	return nil
}

func (z *Zeto) handleTransferEvent(ctx context.Context, tree core.SparseMerkleTree, storage smt.StatesStorage, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var transfer TransferEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
		txID := decodeTransactionData(transfer.Data)
		if txID == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for transfer event: %s. Skip to the next event", transfer.Data)
			return nil
		}
		res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
			TransactionId: txID.String(),
			Location:      ev.Location,
		})
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txID, transfer.Inputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txID, transfer.Outputs)...)
		if common.IsNullifiersToken(tokenName) {
			err := z.updateMerkleTree(ctx, tree, storage, txID, transfer.Outputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOTransfer", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal transfer event: %s", err)
	}
	return nil
}

func (z *Zeto) handleTransferWithEncryptionEvent(ctx context.Context, tree core.SparseMerkleTree, storage smt.StatesStorage, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var transfer TransferWithEncryptedValuesEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
		txID := decodeTransactionData(transfer.Data)
		if txID == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for transfer event: %s. Skip to the next event", transfer.Data)
			return nil
		}
		res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
			TransactionId: txID.String(),
			Location:      ev.Location,
		})
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txID, transfer.Inputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txID, transfer.Outputs)...)
		if common.IsNullifiersToken(tokenName) {
			err := z.updateMerkleTree(ctx, tree, storage, txID, transfer.Outputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOTransferWithEncryptedValues", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal transfer event: %s", err)
	}
	return nil
}

func (z *Zeto) handleWithdrawEvent(ctx context.Context, tree core.SparseMerkleTree, storage smt.StatesStorage, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var withdraw WithdrawEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &withdraw); err == nil {
		txID := decodeTransactionData(withdraw.Data)
		if txID == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for withdraw event: %s. Skip to the next event", withdraw.Data)
			return nil
		}
		res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
			TransactionId: txID.String(),
			Location:      ev.Location,
		})
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txID, withdraw.Inputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txID, []tktypes.HexUint256{withdraw.Output})...)
		if common.IsNullifiersToken(tokenName) {
			err := z.updateMerkleTree(ctx, tree, storage, txID, []tktypes.HexUint256{withdraw.Output})
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOWithdraw", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal withdraw event: %s", err)
	}
	return nil
}

func (z *Zeto) handleLockedEvent(ctx context.Context, ev *prototk.OnChainEvent, res *prototk.HandleEventBatchResponse) error {
	var lock LockedEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &lock); err == nil {
		txID := decodeTransactionData(lock.Data)
		if txID == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for lock event: %s. Skip to the next event", lock.Data)
			return nil
		}
		res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
			TransactionId: txID.String(),
			Location:      ev.Location,
		})
	} else {
		log.L(ctx).Errorf("Failed to unmarshal lock event: %s", err)
	}
	return nil
}

func (z *Zeto) updateMerkleTree(ctx context.Context, tree core.SparseMerkleTree, storage smt.StatesStorage, txID tktypes.HexBytes, outputs []tktypes.HexUint256) error {
	storage.SetTransactionId(txID.HexString0xPrefix())
	for _, out := range outputs {
		if out.NilOrZero() {
			continue
		}
		err := z.addOutputToMerkleTree(ctx, tree, out)
		if err != nil {
			return err
		}
	}
	return nil
}

func (z *Zeto) addOutputToMerkleTree(ctx context.Context, tree core.SparseMerkleTree, output tktypes.HexUint256) error {
	idx, err := node.NewNodeIndexFromBigInt(output.Int())
	if err != nil {
		return i18n.NewError(ctx, msgs.MsgErrorNewNodeIndex, output.String(), err)
	}
	n := node.NewIndexOnly(idx)
	leaf, err := node.NewLeafNode(n)
	if err != nil {
		return i18n.NewError(ctx, msgs.MsgErrorNewLeafNode, err)
	}
	err = tree.AddLeaf(leaf)
	if err != nil {
		return i18n.NewError(ctx, msgs.MsgErrorAddLeafNode, err)
	}
	return nil
}

func parseStatesFromEvent(txID tktypes.HexBytes, states []tktypes.HexUint256) []*prototk.StateUpdate {
	refs := make([]*prototk.StateUpdate, len(states))
	for i, state := range states {
		refs[i] = &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txID.String(),
		}
	}
	return refs
}

func formatErrors(errors []string) string {
	msg := fmt.Sprintf("(failures=%d)", len(errors))
	for i, err := range errors {
		msg = fmt.Sprintf("%s. [%d]%s", msg, i, err)
	}
	return msg
}
