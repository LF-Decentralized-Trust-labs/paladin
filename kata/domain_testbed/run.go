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
	"os"

	"github.com/kaleido-io/paladin/kata/internal/commsbus"
)

// The domain testbed is a simple app that runs a comms bus, and simply passes request/response
// synchronous calls from one input gRPC UnixDomainSocket to another. There are three in total:
// 1) testapp->domain: We host the TestBed_ToDomain RPC interface
// 2) domain: We host and deliver async events to/from the domain
// 3) doman->testapp: The test app hosts the gRPC server, and we send it sync requests
func main() {

	commsBus := commsbus.NewCommsBus()

	os.Exit(0)
}
