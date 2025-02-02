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

package common

import "time"

// TODO consider abstracting even more here so that we are never exposing a Time object, but instead some abstract token that might be related to real (seconds and milliseconds) time units or might just be related to some nebulous measurement of time like "heartbeats" or "clicks" (as a sub division of heartbeats)
type Clock interface {
	//wrapper of time.Now()
	//primarily to allow artificial clocks to be injected for testing
	Now() time.Time
}
type realClock struct{}

func (c *realClock) Now() time.Time {
	return time.Now()
}
func RealClock() Clock {
	return &realClock{}
}
