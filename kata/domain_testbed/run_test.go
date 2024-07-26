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
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newUnitTestbed(t *testing.T) (url string, tb *testbed, done func()) {

	tb, err := newTestBed([]string{"unittestbed", "./sqlite.memory.config.yaml"})
	assert.NoError(t, err)
	tb.conf.DB.SQLite.MigrationsDir = "../db/migrations/sqlite"
	if err != nil {
		panic(err)
	}
	var serverErr error
	go func() {
		serverErr = tb.run()
	}()
	<-tb.ready

	return fmt.Sprintf("http://%s", tb.rpcServer.HTTPAddr()), tb, func() {
		select {
		case tb.sigc <- os.Kill:
		default:
		}
		<-tb.done
		assert.NoError(t, serverErr)
	}

}

func TestStartStop(t *testing.T) {
	_, _, done := newUnitTestbed(t)
	defer done()
}
