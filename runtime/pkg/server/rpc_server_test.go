// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewRPCServer(t *testing.T) {
	ctx := context.Background()

	rpcServer, err := NewRPCServer(ctx)
	if err != nil {
		t.Fatalf("Error creating RPC server: %v", err)
	}

	testServer := httptest.NewServer(rpcServer)
	defer testServer.Close()

	requestBody := `{
		"method": "pld.SubmitTransaction",
		"params": [{}], // Replace with actual parameters if needed
		"id": 1
	}`
	resp, err := http.Post(testServer.URL, "application/json", strings.NewReader(requestBody))
	if err != nil {
		t.Fatalf("Error making POST request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status OK, got %v", resp.StatusCode)
	}
}

func TestNewRPCServerFailures(t *testing.T) {
	ctx := context.Background()

	rpcServer, err := NewRPCServer(ctx)
	if err != nil {
		t.Fatalf("Error creating RPC server: %v", err)
	}

	testServer := httptest.NewServer(rpcServer)
	defer testServer.Close()

	requestBody := `{
		"method": "pld.SubmitTransaction",
		"params": [{}], // Replace with actual parameters if needed
		"id": 1
	}`
	resp, err := http.Post(testServer.URL, "application/json", strings.NewReader(requestBody))
	if err != nil {
		t.Fatalf("Error making POST request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status OK, got %v", resp.StatusCode)
	}
}
