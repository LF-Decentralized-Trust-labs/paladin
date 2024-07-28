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
//nolint
package main

import "C"
import (
	"github.com/kaleido-io/paladin/kata/test/plugins/transport/A/pkg/provider"
)

func BuildInfo() string {
	return provider.BuildInfo()
}

// CreateInstance implements plugins.TransportProvider and starts a new gRPC listener on the given socket address with the name of the provider as destination
func InitializeTransportProvider(socketAddress string, listenerDestination string) error {
	return provider.InitializeTransportProvider(socketAddress, listenerDestination)
}

func main() {}
