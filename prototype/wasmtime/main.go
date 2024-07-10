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
	_ "embed"
	"fmt"
	"net/http"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/gorilla/mux"
	"github.com/gorilla/rpc"
	"github.com/gorilla/rpc/json"
)

//go:embed domains/domainC/domainC.wasm
var domainCWASM []byte

type TokenCDomain struct{}

type TokenCTransferInput struct {
	To    string `json:"to"`
	Value int    `json:"value"`
}

type InvokeArgsC struct {
	Function string              `json:"function"`
	Input    TokenCTransferInput `json:"input"`
}

type ResponseC struct {
	Result uint64
}

func (t *TokenCDomain) Invoke(r *http.Request, args *InvokeArgsC, result *ResponseC) error {
	// ctx := r.Context()

	*result = ResponseC{}
	return nil
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {

	engine := wasmtime.NewEngine()
	store := wasmtime.NewStore(engine)
	module, err := wasmtime.NewModule(engine, domainCWASM)
	//This fails with `Invalid input WebAssembly code at offset 4: unknown binary version` because the go port of wasmtime doesn't
	//support component model yet
	check(err)
	instance, err := wasmtime.NewInstance(store, module, []wasmtime.AsExtern{})
	check(err)

	gcd := instance.GetExport(store, "gcd").Func()
	val, err := gcd.Call(store, 6, 27)
	check(err)
	fmt.Printf("gcd(6, 27) = %d\n", val.(int32))

	rpcServer := rpc.NewServer()

	rpcServer.RegisterCodec(json.NewCodec(), "application/json")
	rpcServer.RegisterCodec(json.NewCodec(), "application/json;charset=UTF-8")

	tokenC := new(TokenCDomain)

	rpcServer.RegisterService(tokenC, "TokenC")

	router := mux.NewRouter()
	router.Handle("/", rpcServer)
	http.ListenAndServe(":1337", router)
}
