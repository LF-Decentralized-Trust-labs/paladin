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

package zkp

import (
	"fmt"
	"os"
	"path"

	"github.com/iden3/go-rapidsnark/witness/v2"
	"github.com/iden3/go-rapidsnark/witness/wasmer"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
)

func loadCircuit(circuitName string, config api.SnarkProverConfig) (witness.Calculator, []byte, error) {
	if config.CircuitsDir == "" {
		return nil, []byte{}, fmt.Errorf("CIRCUITS_ROOT not set")
	}
	if config.ProvingKeysDir == "" {
		return nil, []byte{}, fmt.Errorf("PROVING_KEYS_ROOT not set")
	}

	// load the wasm file for the circuit
	wasmBytes, err := os.ReadFile(path.Join(config.CircuitsDir, fmt.Sprintf("%s_js", circuitName), fmt.Sprintf("%s.wasm", circuitName)))
	if err != nil {
		return nil, []byte{}, err
	}

	// create the prover
	zkeyBytes, err := os.ReadFile(path.Join(config.ProvingKeysDir, fmt.Sprintf("%s.zkey", circuitName)))
	if err != nil {
		return nil, []byte{}, err
	}

	// create the calculator
	var ops []witness.Option
	ops = append(ops, witness.WithWasmEngine(wasmer.NewCircom2WitnessCalculator))
	calc, err := witness.NewCalculator(wasmBytes, ops...)
	if err != nil {
		return nil, []byte{}, err
	}

	return calc, zkeyBytes, err
}
