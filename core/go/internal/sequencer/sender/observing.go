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

package sender

import "context"

func (s *sender) applyHeartbeatReceived(ctx context.Context) error {
	s.timeOfMostRecentHeartbeat = s.clock.Now()
	return nil
}

func guard_HeartbeatThresholdExceeded(ctx context.Context, s *sender) bool {
	if s.timeOfMostRecentHeartbeat == nil {
		//we have never seen a heartbeat so that was a really long time ago, certainly longer than any threshold
		return true
	}
	if s.clock.HasExpired(s.timeOfMostRecentHeartbeat, s.heartbeatThresholdMs) {
		return true
	}
	return false
}
