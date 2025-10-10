/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package smt

import (
	"context"
	"encoding/json"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/node"
)

type StatesStorage interface {
	core.Storage
	GetNewStates() ([]*prototk.NewConfirmedState, error)
	SetTransactionId(txId string)
}

// The storage object is used in a batch context, across multiple events.
// the new states are not committed until the entire batch of events are processed.
// On the other hand, the Tx session in Zeto SMT implementation is on a per AddLeaf() basis.
// Therefore we need to have an in-memory implementation of Tx objects
type statesStorage struct {
	CoreInterface     plugintk.DomainCallbacks
	smtName           string
	stateQueryContext string
	rootSchemaId      string
	nodeSchemaId      string
	pendingNodesTx    *nodesTx
	rootNode          *smtRootNode
	committedNewNodes map[core.NodeRef]*smtNode
}

// this corresponds to the new nodes resulted from the execution of
// AddLeaf() in the Zeto SMT implementation
type nodesTx struct {
	transactionId string
	inflightRoot  core.NodeRef
	inflightNodes map[core.NodeRef]core.Node
}

type smtRootNode struct {
	root core.NodeRef
	txId string
}

type smtNode struct {
	node core.Node
	txId string
}

func (n *nodesTx) getNode(ref core.NodeRef) (core.Node, error) {
	if node, ok := n.inflightNodes[ref]; ok {
		return node, nil
	}
	return nil, core.ErrNotFound
}

func NewStatesStorage(c plugintk.DomainCallbacks, smtName, stateQueryContext, rootSchemaId, nodeSchemaId string) StatesStorage {
	return &statesStorage{
		CoreInterface:     c,
		smtName:           smtName,
		stateQueryContext: stateQueryContext,
		rootSchemaId:      rootSchemaId,
		nodeSchemaId:      nodeSchemaId,
		committedNewNodes: make(map[core.NodeRef]*smtNode),
	}
}

func (s *statesStorage) SetTransactionId(txId string) {
	if s.pendingNodesTx == nil {
		s.pendingNodesTx = &nodesTx{
			inflightNodes: make(map[core.NodeRef]core.Node),
		}
	}
	s.pendingNodesTx.transactionId = txId
}

func (s *statesStorage) GetNewStates() ([]*prototk.NewConfirmedState, error) {
	var newStates []*prototk.NewConfirmedState
	ctx := context.Background()
	if s.rootNode != nil {
		newRootNodeState, err := s.makeNewStateFromRootNode(ctx, s.rootNode)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorNewStateFromCommittedRoot, err)
		}
		newStates = append(newStates, newRootNodeState)
	}
	for _, node := range s.committedNewNodes {
		newNodeState, err := s.makeNewStateFromTreeNode(ctx, node)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorNewStateFromCommittedNode, err)
		}
		newStates = append(newStates, newNodeState)
	}
	return newStates, nil
}

func (s *statesStorage) GetRootNodeRef() (core.NodeRef, error) {
	if s.pendingNodesTx != nil && s.pendingNodesTx.inflightRoot != nil {
		return s.pendingNodesTx.inflightRoot, nil
	}

	if s.rootNode != nil {
		return s.rootNode.root, nil
	}

	queryBuilder := query.NewQueryBuilder().
		Limit(1).
		Sort(".created DESC").
		Equal("smtName", s.smtName)

	ctx := context.Background()
	res, err := s.CoreInterface.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: s.stateQueryContext,
		SchemaId:          s.rootSchemaId,
		QueryJson:         queryBuilder.Query().String(),
	})
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorQueryAvailStates, err)
	}

	if len(res.States) == 0 {
		return nil, core.ErrNotFound
	}

	var root types.MerkleTreeRoot
	err = json.Unmarshal([]byte(res.States[0].DataJson), &root)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalRootIdx, err)
	}

	idx, err := node.NewNodeIndexFromHex(root.RootIndex.HexString())
	return idx, err
}

func (s *statesStorage) UpsertRootNodeRef(root core.NodeRef) error {
	if s.pendingNodesTx == nil {
		s.pendingNodesTx = &nodesTx{
			inflightNodes: make(map[core.NodeRef]core.Node),
		}
	}
	s.pendingNodesTx.inflightRoot = root
	return nil
}

func (s *statesStorage) GetNode(ref core.NodeRef) (core.Node, error) {
	// the node's reference key (not the index) is used as the key to
	// store the node in the DB
	refKey := ref.Hex()

	// first check if the node is in the committed nodes cache
	if n, ok := s.committedNewNodes[ref]; ok {
		return n.node, nil
	}
	// next check if the node is in the inflight nodes cache
	if s.pendingNodesTx != nil {
		if n, err := s.pendingNodesTx.getNode(ref); err == nil {
			return n, nil
		}
	}

	queryBuilder := query.NewQueryBuilder().
		Limit(1).
		Sort(".created").
		Equal("refKey", refKey)

	ctx := context.Background()
	res, err := s.CoreInterface.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: s.stateQueryContext,
		SchemaId:          s.nodeSchemaId,
		QueryJson:         queryBuilder.Query().String(),
	})
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorQueryAvailStates, err)
	}
	if len(res.States) == 0 {
		return nil, core.ErrNotFound
	}
	var n types.MerkleTreeNode
	err = json.Unmarshal([]byte(res.States[0].DataJson), &n)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalSMTNode, err)
	}

	var newNode core.Node
	nodeType := core.NodeTypeFromByte(n.Type[:][0])
	switch nodeType {
	case core.NodeTypeLeaf:
		idx, err1 := node.NewNodeIndexFromHex(n.Index.HexString())
		if err1 != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorNewNodeIndex, err1)
		}
		v := node.NewIndexOnly(idx)
		newNode, err = node.NewLeafNode(v)
	case core.NodeTypeBranch:
		leftChild, err1 := node.NewNodeIndexFromHex(n.LeftChild.HexString())
		if err1 != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorNewNodeIndex, err1)
		}
		rightChild, err2 := node.NewNodeIndexFromHex(n.RightChild.HexString())
		if err2 != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorNewNodeIndex, err2)
		}
		newNode, err = node.NewBranchNode(leftChild, rightChild)
	}
	return newNode, err
}

func (s *statesStorage) InsertNode(n core.Node) error {
	s.pendingNodesTx.inflightNodes[n.Ref()] = n

	return nil
}

func (s *statesStorage) BeginTx() (core.Transaction, error) {
	// reset the inflight nodes cache
	if s.pendingNodesTx != nil {
		s.pendingNodesTx.inflightNodes = make(map[core.NodeRef]core.Node)
	} else {
		s.pendingNodesTx = &nodesTx{
			inflightNodes: make(map[core.NodeRef]core.Node),
		}
	}
	return s, nil
}

func (s *statesStorage) Commit() error {
	// here we merge the inflight nodes in the pending Tx with the committed new nodes
	s.rootNode = &smtRootNode{
		root: s.pendingNodesTx.inflightRoot,
		txId: s.pendingNodesTx.transactionId,
	}
	for ref, node := range s.pendingNodesTx.inflightNodes {
		s.committedNewNodes[ref] = &smtNode{
			node: node,
			txId: s.pendingNodesTx.transactionId,
		}
	}
	// reset the inflight nodes cache
	s.pendingNodesTx.inflightNodes = make(map[core.NodeRef]core.Node)
	s.pendingNodesTx.inflightRoot = nil
	return nil
}

func (s *statesStorage) Rollback() error {
	// reset the inflight nodes cache
	s.pendingNodesTx = &nodesTx{
		inflightNodes: make(map[core.NodeRef]core.Node),
	}
	return nil
}

func (s *statesStorage) Close() {
	// not needed for this implementation because
	// there are no resources to close
}

func (s *statesStorage) makeNewStateFromTreeNode(ctx context.Context, n *smtNode) (*prototk.NewConfirmedState, error) {
	node := n.node
	// we clone the node so that the value properties are not saved
	refBytes, err := pldtypes.ParseBytes32(node.Ref().Hex())
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorParseNodeRef, err)
	}
	newNode := &types.MerkleTreeNode{
		RefKey: refBytes,
		Type:   pldtypes.HexBytes([]byte{node.Type().ToByte()}),
	}
	if node.Type() == core.NodeTypeBranch {
		leftBytes, err1 := pldtypes.ParseBytes32(node.LeftChild().Hex())
		if err1 != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorParseNodeRef, err1)
		}
		rightBytes, err2 := pldtypes.ParseBytes32(node.RightChild().Hex())
		if err2 != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorParseNodeRef, err2)
		}
		newNode.LeftChild = leftBytes
		newNode.RightChild = rightBytes
	} else if node.Type() == core.NodeTypeLeaf {
		idxBytes, err := pldtypes.ParseBytes32(node.Index().Hex())
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorParseNodeRef, err)
		}
		newNode.Index = idxBytes
	}

	data, _ := json.Marshal(newNode)
	hash, err := newNode.Hash(s.smtName)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashSMTNode, err)
	}
	newNodeState := &prototk.NewConfirmedState{
		Id:            &hash,
		SchemaId:      s.nodeSchemaId,
		StateDataJson: string(data),
		TransactionId: n.txId,
	}
	return newNodeState, nil
}

func (s *statesStorage) makeNewStateFromRootNode(ctx context.Context, rootNode *smtRootNode) (*prototk.NewConfirmedState, error) {
	root := rootNode.root
	bytes, err := pldtypes.ParseBytes32(root.Hex())
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorParseRootNodeIdx, err)
	}
	newRoot := &types.MerkleTreeRoot{
		SmtName:   s.smtName,
		RootIndex: bytes,
	}
	data, err := json.Marshal(newRoot)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUpsertRootNode, err)
	}
	hash, err := newRoot.Hash()
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashSMTNode, err)
	}
	newRootState := &prototk.NewConfirmedState{
		Id:            &hash,
		SchemaId:      s.rootSchemaId,
		StateDataJson: string(data),
		TransactionId: rootNode.txId,
	}
	return newRootState, nil
}
