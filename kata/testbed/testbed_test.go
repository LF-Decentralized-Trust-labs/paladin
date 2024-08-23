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
	"path"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/kata/internal/componentmgr"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/httpserver"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/internal/rpcclient"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func newUnitTestbed(t *testing.T, initFunctions ...func(c components.AllComponents) error) (url string, tb *testbed, done func()) {
	logrus.SetLevel(logrus.DebugLevel)

	tb = newTestBed(initFunctions...)
	err := tb.setupConfig([]string{"unittestbed", "./sqlite.memory.config.yaml"})

	assert.NoError(t, err)
	if err != nil {
		panic(err)
	}
	// Tweak config to work from in test dir, while leaving it so it still works for commandline on disk
	tb.conf.DB.SQLite.MigrationsDir = "../db/migrations/sqlite"
	serverErr := make(chan error)
	go func() {
		serverErr <- tb.run()
	}()
	err = <-tb.ready
	assert.NoError(t, err)

	return fmt.Sprintf("http://%s", tb.components.RPCServer().HTTPAddr()), tb, func() {
		select {
		case tb.sigc <- os.Kill:
		default:
		}
		select {
		case err := <-serverErr:
			assert.NoError(t, err)
		case <-time.After(2 * time.Second):
			assert.Fail(t, "timeout on shutdown")
		}
	}

}

func TestCommandLineRunFail(t *testing.T) {
	var exitCode int
	exitProcess = func(code int) { exitCode = code }
	origArgs := os.Args
	defer func() {
		os.Args = origArgs
	}()

	failConfig := path.Join(t.TempDir(), "broken.config.yaml")
	os.Args = []string{"unittestbed", failConfig}
	err := os.WriteFile(failConfig, ([]byte)(`{
		"rpc": {
	    	http: {
				"host": ":::::::wrong",
				"port": -12345
			}
		}
	}`), 0644)
	assert.NoError(t, err)

	main()
	assert.Equal(t, 1, exitCode)
}

func TestBadConfig(t *testing.T) {
	err := newTestBed().setupConfig([]string{"unittestbed", t.TempDir()})
	assert.Error(t, err)
}

func TestTempSocketFileFail(t *testing.T) {
	tempDir := t.TempDir()
	thisIsAFile := path.Join(tempDir, "a.file")
	err := os.WriteFile(thisIsAFile, []byte{}, 0644)
	assert.NoError(t, err)
	_, err = (&testbed{conf: &componentmgr.Config{TempDir: &thisIsAFile}}).tempSocketFile()
	assert.Error(t, err)
}

func TestRunCommsBusError(t *testing.T) {
	tb := newTestBed()
	tb.conf = &componentmgr.Config{
		DB: persistence.Config{
			Type: "sqlite",
			SQLite: persistence.SQLiteConfig{SQLDBConfig: persistence.SQLDBConfig{
				URI:           ":memory:",
				MigrationsDir: "./sqlite.memory.config.yaml",
			}},
		},
		PluginControllerConfig: plugins.PluginControllerConfig{
			GRPC: plugins.GRPCConfig{
				Address: "unix:" + t.TempDir(),
			},
		},
	}
	err := tb.run()
	assert.Regexp(t, "Comms bus", err)
}

func TestRunRPCError(t *testing.T) {
	tb := newTestBed()
	tb.conf = &componentmgr.Config{
		DB: persistence.Config{
			Type: "sqlite",
			SQLite: persistence.SQLiteConfig{SQLDBConfig: persistence.SQLDBConfig{
				URI:           ":memory:",
				MigrationsDir: "./sqlite.memory.config.yaml",
			}},
		},
		PluginControllerConfig: plugins.PluginControllerConfig{
			GRPC: plugins.GRPCConfig{
				Address: "unix:" + path.Join(t.TempDir(), "socket.file"),
			},
		},
		RPCServer: rpcserver.Config{
			HTTP: rpcserver.HTTPEndpointConfig{
				Config: httpserver.Config{
					Port:    confutil.P(-1),
					Address: confutil.P("::::wrong"),
				},
			},
		},
	}
	err := tb.run()
	assert.Regexp(t, "RPC", err)
}

func TestRunBlockIndexerError(t *testing.T) {
	tb := newTestBed()
	tb.conf = &componentmgr.Config{
		DB: persistence.Config{
			Type: "sqlite",
			SQLite: persistence.SQLiteConfig{SQLDBConfig: persistence.SQLDBConfig{
				URI:           ":memory:",
				MigrationsDir: "./sqlite.memory.config.yaml",
			}},
		},
		PluginControllerConfig: plugins.PluginControllerConfig{
			GRPC: plugins.GRPCConfig{
				Address: "unix:" + path.Join(t.TempDir(), "socket.file"),
			},
		},
		RPCServer: rpcserver.Config{
			HTTP: rpcserver.HTTPEndpointConfig{Disabled: true},
			WS:   rpcserver.WSEndpointConfig{Disabled: true},
		},
		Blockchain: ethclient.Config{
			WS: rpcclient.WSConfig{
				HTTPConfig: rpcclient.HTTPConfig{
					URL: "!!!! wrongness",
				},
			},
		},
	}
	err := tb.run()
	assert.Regexp(t, "Blockchain", err)
}
