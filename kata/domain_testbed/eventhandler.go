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

package main

import (
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
)

func (tb *testbed) eventHandler() {
	for {
		select {
		case <-tb.ctx.Done():
			log.L(tb.ctx).Infof("Testbed event handler shutting down")
			return
		case msgFromDomain := <-tb.fromDomain.Channel:
			tb.handleFromDomain(msgFromDomain)
		}
	}
}

func (tb *testbed) handleFromDomain(msgToDomain commsbus.Message) {

}
