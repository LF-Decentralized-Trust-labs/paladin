/*
 * Copyright © 2025 Kaleido, Inc.
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

package domain

import (
	"context"
	"errors"
	"testing"

	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestMockDomainCallbacks_FindAvailableStates(t *testing.T) {
	tests := []struct {
		name           string
		mockFunc       func() (*pb.FindAvailableStatesResponse, error)
		expectedStates []*pb.StoredState
		expectedError  error
	}{
		{
			name: "successful response",
			mockFunc: func() (*pb.FindAvailableStatesResponse, error) {
				return &pb.FindAvailableStatesResponse{
					States: []*pb.StoredState{
						{
							Id:        "state1",
							SchemaId:  "schema1",
							CreatedAt: 123456789,
							DataJson:  `{"key": "value"}`,
							Locks:     []*pb.StateLock{},
						},
					},
				}, nil
			},
			expectedStates: []*pb.StoredState{
				{
					Id:        "state1",
					SchemaId:  "schema1",
					CreatedAt: 123456789,
					DataJson:  `{"key": "value"}`,
					Locks:     []*pb.StateLock{},
				},
			},
			expectedError: nil,
		},
		{
			name: "error response",
			mockFunc: func() (*pb.FindAvailableStatesResponse, error) {
				return nil, errors.New("database error")
			},
			expectedStates: nil,
			expectedError:  errors.New("database error"),
		},
		{
			name: "empty states response",
			mockFunc: func() (*pb.FindAvailableStatesResponse, error) {
				return &pb.FindAvailableStatesResponse{
					States: []*pb.StoredState{},
				}, nil
			},
			expectedStates: []*pb.StoredState{},
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callbacks := &MockDomainCallbacks{
				MockFindAvailableStates: tt.mockFunc,
			}

			req := &pb.FindAvailableStatesRequest{
				StateQueryContext: "test-context",
				SchemaId:          "test-schema",
				QueryJson:         `{"filter": "test"}`,
			}

			result, err := callbacks.FindAvailableStates(context.Background(), req)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedStates, result.States)
			}
		})
	}
}

func TestMockDomainCallbacks_LocalNodeName(t *testing.T) {
	tests := []struct {
		name          string
		mockFunc      func() (*pb.LocalNodeNameResponse, error)
		expectedName  string
		expectedError error
	}{
		{
			name: "successful response",
			mockFunc: func() (*pb.LocalNodeNameResponse, error) {
				return &pb.LocalNodeNameResponse{
					Name: "test-node-1",
				}, nil
			},
			expectedName:  "test-node-1",
			expectedError: nil,
		},
		{
			name: "error response",
			mockFunc: func() (*pb.LocalNodeNameResponse, error) {
				return nil, errors.New("network error")
			},
			expectedName:  "",
			expectedError: errors.New("network error"),
		},
		{
			name: "empty name response",
			mockFunc: func() (*pb.LocalNodeNameResponse, error) {
				return &pb.LocalNodeNameResponse{
					Name: "",
				}, nil
			},
			expectedName:  "",
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callbacks := &MockDomainCallbacks{
				MockLocalNodeName: tt.mockFunc,
			}

			result, err := callbacks.LocalNodeName(context.Background(), &pb.LocalNodeNameRequest{})

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedName, result.Name)
			}
		})
	}
}

func TestMockDomainCallbacks_Unimplemented(t *testing.T) {
	callbacks := &MockDomainCallbacks{}

	result1, err := callbacks.EncodeData(context.Background(), &pb.EncodeDataRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result1)

	result2, err := callbacks.RecoverSigner(context.Background(), &pb.RecoverSignerRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result2)

	result3, err := callbacks.DecodeData(context.Background(), &pb.DecodeDataRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result3)

	result4, err := callbacks.SendTransaction(context.Background(), &pb.SendTransactionRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result4)

	result6, err := callbacks.GetStatesByID(context.Background(), &pb.GetStatesByIDRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result6)
}
