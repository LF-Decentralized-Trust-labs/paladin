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

package noto

import (
	"log"
	"reflect"

	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
)

func HandleDomainMessage(message *pb.Message) error {
	body, err := message.Body.UnmarshalNew()
	if err != nil {
		return err
	}

	switch m := body.(type) {
	case *pb.ConfigureDomainRequest:
		log.Printf("Configuring domain: %s", m.Name)
	default:
		log.Printf("Unknown type: %s", reflect.TypeOf(m))
	}

	return nil
}
