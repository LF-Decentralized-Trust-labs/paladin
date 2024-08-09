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

package types

import "github.com/hyperledger/firefly-signer/pkg/abi"

// The serializer we should use in all places that go from ABI validated data,
// back down to JSON that might be:
// 1) Stored in the state store
// 2) Passed to end-users over a JSON/RPC API
// 3) Passed to domain plugins over a gRPC API
func StandardABISerializer() *abi.Serializer {
	return abi.NewSerializer().
		SetFormattingMode(abi.FormatAsObjects).
		SetIntSerializer(abi.Base10StringIntSerializer).
		SetFloatSerializer(abi.Base10StringFloatSerializer).
		SetByteSerializer(abi.HexByteSerializer0xPrefix)
}
