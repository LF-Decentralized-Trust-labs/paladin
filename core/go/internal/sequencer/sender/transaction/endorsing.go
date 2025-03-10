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
package transaction

import (
	"context"
	"errors"
)

// Actions can be specified for transition to a state either as the OnTransitionTo function that will run for all transitions to that state or as the On field in the Transition struct if the action applies
// for a specific transition
func action_ResendAssembleSuccessResponse(ctx context.Context, txn *Transaction) error {
	return errors.New("not implemented")
}

func guard_AssembleRequestMatchesPreviousResponse(ctx context.Context, txn *Transaction) bool {
	return false

}
