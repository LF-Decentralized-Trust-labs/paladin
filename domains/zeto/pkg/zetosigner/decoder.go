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

package zetosigner

import (
	"fmt"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	pb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"google.golang.org/protobuf/proto"
)

func decodeProvingRequest(payload []byte) (*pb.ProvingRequest, interface{}, error) {
	inputs := pb.ProvingRequest{}
	// Unmarshal payload into inputs
	err := proto.Unmarshal(payload, &inputs)
	if err != nil {
		return nil, nil, err
	}
	if inputs.CircuitId == constants.CIRCUIT_ANON_ENC {
		encExtras := pb.ProvingRequestExtras_Encryption{
			EncryptionNonce: "",
		}
		if len(inputs.Extras) > 0 {
			err := proto.Unmarshal(inputs.Extras, &encExtras)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to unmarshal proving request extras for circuit %s. %s", inputs.CircuitId, err)
			}
		}
		return &inputs, &encExtras, nil
	} else if inputs.CircuitId == constants.CIRCUIT_ANON_NULLIFIER {
		var nullifierExtras pb.ProvingRequestExtras_Nullifiers
		err := proto.Unmarshal(inputs.Extras, &nullifierExtras)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal proving request extras for circuit %s. %s", inputs.CircuitId, err)
		}
		return &inputs, &nullifierExtras, nil
	}
	return &inputs, nil, nil
}