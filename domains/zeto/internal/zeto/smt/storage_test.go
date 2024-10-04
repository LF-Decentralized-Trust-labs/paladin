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
	"errors"
	"math/big"
	"testing"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/node"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

type testDomainCallbacks struct {
	returnFunc func() (*prototk.FindAvailableStatesResponse, error)
}

func (dc *testDomainCallbacks) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	return dc.returnFunc()
}

func (dc *testDomainCallbacks) EncodeData(ctx context.Context, req *prototk.EncodeDataRequest) (*prototk.EncodeDataResponse, error) {
	return nil, nil
}
func (dc *testDomainCallbacks) RecoverSigner(ctx context.Context, req *prototk.RecoverSignerRequest) (*prototk.RecoverSignerResponse, error) {
	return nil, nil
}

func (dc *testDomainCallbacks) DecodeData(context.Context, *prototk.DecodeDataRequest) (*prototk.DecodeDataResponse, error) {
	return nil, nil
}

func returnCustomError() (*prototk.FindAvailableStatesResponse, error) {
	return nil, errors.New("test error")
}

func returnEmptyStates() (*prototk.FindAvailableStatesResponse, error) {
	return &prototk.FindAvailableStatesResponse{}, nil
}

func returnBadData() (*prototk.FindAvailableStatesResponse, error) {
	return &prototk.FindAvailableStatesResponse{
		States: []*prototk.StoredState{
			{
				DataJson: "bad data",
			},
		},
	}, nil
}

func returnNode(t int) func() (*prototk.FindAvailableStatesResponse, error) {
	var data []byte
	if t == 0 {
		data, _ = json.Marshal(map[string]string{"rootIndex": "0x1234567890123456789012345678901234567890123456789012345678901234"})
	} else if t == 1 {
		data, _ = json.Marshal(map[string]string{
			"index":      "0x197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521",
			"leftChild":  "0x0000000000000000000000000000000000000000000000000000000000000000",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"type":       "0x02", // leaf node
		})
	} else if t == 2 {
		data, _ = json.Marshal(map[string]string{
			"leftChild":  "0x197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0xd23ae67af3b0e9f4854eb76954c27c7607b2a37b633d6b107e607cee460a6425",
			"type":       "0x01", // branch node
		})
	} else if t == 3 {
		data, _ = json.Marshal(map[string]string{
			"leftChild":  "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0xd23ae67af3b0e9f4854eb76954c27c7607b2a37b633d6b107e607cee460a6425",
			"type":       "0x01", // branch node
		})
	} else if t == 4 {
		data, _ = json.Marshal(map[string]string{
			"leftChild":  "0x197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"type":       "0x01", // branch node
		})
	} else if t == 5 {
		data, _ = json.Marshal(map[string]string{
			"index":      "baddata",
			"leftChild":  "0x0000000000000000000000000000000000000000000000000000000000000000",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"type":       "0x02", // leaf node
		})
	} else if t == 6 {
		data, _ = json.Marshal(map[string]string{
			"index":      "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"leftChild":  "0x0000000000000000000000000000000000000000000000000000000000000000",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"type":       "0x02", // leaf node
		})
	}
	return func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					DataJson: string(data),
				},
			},
		}, nil
	}
}

func TestStorage(t *testing.T) {
	contractAddress := tktypes.RandAddress().Address0xHex()

	storage, smt, err := New(&testDomainCallbacks{returnFunc: returnCustomError}, "test", contractAddress, "root-schema", "node-schema")
	assert.EqualError(t, err, "failed to find available states. test error")
	assert.NotNil(t, storage)
	assert.Nil(t, smt)

	storage, smt, err = New(&testDomainCallbacks{returnFunc: returnEmptyStates}, "test", contractAddress, "root-schema", "node-schema")
	assert.NoError(t, err)
	assert.NotNil(t, storage)
	assert.NotNil(t, smt)
	assert.Equal(t, 0, big.NewInt(0).Cmp(storage.(*statesStorage).rootNode.BigInt()))
	assert.Equal(t, 1, len(storage.(*statesStorage).newNodes))
	var root MerkleTreeRoot
	err = json.Unmarshal([]byte(storage.(*statesStorage).newNodes[0].StateDataJson), &root)
	assert.NoError(t, err)
	assert.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", root.RootIndex.String())

	storage, smt, err = New(&testDomainCallbacks{returnFunc: returnBadData}, "test", contractAddress, "root-schema", "node-schema")
	assert.EqualError(t, err, "failed to unmarshal root node index. invalid character 'b' looking for beginning of value")
	assert.NotNil(t, storage)
	assert.Nil(t, smt)

	storage, smt, err = New(&testDomainCallbacks{returnFunc: returnNode(0)}, "test", contractAddress, "root-schema", "node-schema")
	assert.NoError(t, err)
	assert.NotNil(t, storage)
	assert.NotNil(t, smt)
	assert.Equal(t, "1234567890123456789012345678901234567890123456789012345678901234", storage.(*statesStorage).rootNode.Hex())

	assert.Empty(t, storage.(*statesStorage).GetNewStates())
	idx, err := storage.(*statesStorage).GetRootNodeIndex()
	assert.NoError(t, err)
	assert.NotEmpty(t, idx)
}

func TestGetNode(t *testing.T) {
	contractAddress := tktypes.RandAddress().Address0xHex()
	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234))

	storage := NewStatesStorage(&testDomainCallbacks{returnFunc: returnCustomError}, "test", contractAddress, "root-schema", "node-schema")
	_, err := storage.GetNode(idx)
	assert.EqualError(t, err, "failed to find available states. test error")

	storage = NewStatesStorage(&testDomainCallbacks{returnFunc: returnEmptyStates}, "test", contractAddress, "root-schema", "node-schema")
	_, err = storage.GetNode(idx)
	assert.EqualError(t, err, core.ErrNotFound.Error())

	storage = NewStatesStorage(&testDomainCallbacks{returnFunc: returnNode(1)}, "test", contractAddress, "root-schema", "node-schema")
	n, err := storage.GetNode(idx)
	assert.NoError(t, err)
	assert.NotNil(t, n)
	assert.Equal(t, "197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521", n.Index().Hex())
	assert.Equal(t, core.NodeTypeLeaf, n.Type())

	storage = NewStatesStorage(&testDomainCallbacks{returnFunc: returnNode(2)}, "test", contractAddress, "root-schema", "node-schema")
	n, err = storage.GetNode(idx)
	assert.NoError(t, err)
	assert.NotNil(t, n)
	assert.Empty(t, n.Index())
	assert.Equal(t, "197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521", n.LeftChild().Hex())

	storage = NewStatesStorage(&testDomainCallbacks{returnFunc: returnNode(3)}, "test", contractAddress, "root-schema", "node-schema")
	_, err = storage.GetNode(idx)
	assert.EqualError(t, err, "inputs values not inside Finite Field")

	storage = NewStatesStorage(&testDomainCallbacks{returnFunc: returnNode(4)}, "test", contractAddress, "root-schema", "node-schema")
	_, err = storage.GetNode(idx)
	assert.EqualError(t, err, "inputs values not inside Finite Field")

	storage = NewStatesStorage(&testDomainCallbacks{returnFunc: returnNode(5)}, "test", contractAddress, "root-schema", "node-schema")
	_, err = storage.GetNode(idx)
	assert.ErrorContains(t, err, "failed to unmarshal Merkle Tree Node from state json. PD020007: Invalid hex")

	storage = NewStatesStorage(&testDomainCallbacks{returnFunc: returnNode(6)}, "test", contractAddress, "root-schema", "node-schema")
	_, err = storage.GetNode(idx)
	assert.ErrorContains(t, err, "failed to unmarshal Merkle Tree Node from state json. PD020008: Failed to parse value as 32 byte hex string")
}

func TestInsertNode(t *testing.T) {
	contractAddress := tktypes.RandAddress().Address0xHex()
	storage := NewStatesStorage(&testDomainCallbacks{returnFunc: returnEmptyStates}, "test", contractAddress, "root-schema", "node-schema")
	assert.NotNil(t, storage)
	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234))
	n, _ := node.NewLeafNode(node.NewIndexOnly(idx))
	err := storage.InsertNode(n)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(storage.(*statesStorage).newNodes))

	n, _ = node.NewBranchNode(idx, idx)
	err = storage.InsertNode(n)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(storage.(*statesStorage).newNodes))
}

func TestUnimplementedMethods(t *testing.T) {
	contractAddress := tktypes.RandAddress().Address0xHex()
	storage := NewStatesStorage(&testDomainCallbacks{returnFunc: returnEmptyStates}, "test", contractAddress, "root-schema", "node-schema")
	assert.NotNil(t, storage)
	assert.Nil(t, storage.(*statesStorage).Commit())
	assert.Nil(t, storage.(*statesStorage).Rollback())
	storage.(*statesStorage).Close()
}
