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

package extensions

import (
	"context"

	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
)

const ZkpKeyStoreSigner = "zkp"

type zkpExt struct{}

func NewZkpSignerExtension() api.Extension {
	return &zkpExt{}
}

func (z *zkpExt) KeyStore(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error) {
	if config.Type != ZkpKeyStoreSigner {
		return nil, nil
	}

	kss, err := NewZetoKeystoreSigner(ctx, config)
	if err != nil {
		return nil, err
	}
	return kss, nil
}
