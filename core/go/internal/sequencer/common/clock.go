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

import (
	"context"
	"time"
)

type Clock interface {
	//wrapper of time.Now()
	//primarily to allow artificial clocks to be injected for testing
	Now() Time
	HasExpired(Time, Duration) bool
	Duration(milliseconds int) Duration
	ScheduleInterval(context.Context, Duration, func()) (cancel func())
}

type Duration interface {
}

type Time interface {
}

type realClock struct{}

func (c *realClock) Duration(milliseconds int) Duration {
	return time.Duration(milliseconds) * time.Millisecond
}
func (c *realClock) Now() Time {
	return time.Now()
}

func RealClock() Clock {
	return &realClock{}
}

func (c *realClock) HasExpired(start Time, duration Duration) bool {
	realStart := start.(time.Time)
	realDuration := duration.(time.Duration)
	return time.Now().After(realStart.Add(realDuration))
}

func (c *realClock) ScheduleInterval(ctx context.Context, duration Duration, f func()) (cancel func()) {
	realDuration := duration.(time.Duration)
	ticker := time.NewTicker(realDuration)
	go func() {
		<-ticker.C
		f()
	}()
	return func() {
		ticker.Stop()
	}
}

type FakeClockForTesting struct {
	currentTime int
}

type fakeTime struct {
	milliseconds int
}

type fakeDuration struct {
	milliseconds int
}

// On the fake clock, time is just a number
func (c *FakeClockForTesting) Advance(advance int) {
	c.currentTime += advance
}

func (c *FakeClockForTesting) Now() Time {
	return &fakeTime{c.currentTime}
}

func (c *FakeClockForTesting) HasExpired(start Time, duration Duration) bool {
	startMillis := start.(*fakeTime).milliseconds
	durationMillis := duration.(*fakeDuration).milliseconds
	nowMillis := c.currentTime
	return nowMillis > startMillis+durationMillis

}

func (c *FakeClockForTesting) ScheduleInterval(context.Context, Duration, func()) (cancel func()) {
	return func() {}
}

func (c *FakeClockForTesting) Duration(milliseconds int) Duration {
	return &fakeDuration{milliseconds}
}
