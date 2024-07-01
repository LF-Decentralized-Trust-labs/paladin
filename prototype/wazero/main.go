package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/rpc"
	"github.com/gorilla/rpc/json"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed domains/domainA/domainA.wasm
var domainAWASM []byte

//go:embed domains/domainB/domainB.wasm
var domainBWASM []byte

type TokenADomain struct{}
type TokenBDomain struct{}

type TokenATransferInput struct {
	Foo uint64 `json:"foo"`
	Bar uint64 `json:"bar"`
}

type TokenBTransferInput struct {
	Foo uint64 `json:"foo"`
	Bar uint64 `json:"bar"`
}

type InvokeArgsA struct {
	Function string              `json:"function"`
	Input    TokenATransferInput `json:"input"`
}

type InvokeArgsB struct {
	Function string              `json:"function"`
	Input    TokenBTransferInput `json:"input"`
}

type Response struct {
	Result uint64
}

var domainA api.Module
var domainB api.Module

func (t *TokenADomain) Invoke(r *http.Request, args *InvokeArgsA, result *Response) error {
	ctx := r.Context()
	assemble := domainA.ExportedFunction("assemble")
	results, err := assemble.Call(ctx, args.Input.Bar, args.Input.Foo)
	if err != nil {
		log.Panicf("failed to call assemble: %v", err)
	}

	fmt.Printf("%d\n", results[0])

	*result = Response{Result: results[0]}
	return nil
}

func (t *TokenBDomain) Invoke(r *http.Request, args *InvokeArgsB, result *Response) error {
	ctx := r.Context()
	assemble := domainB.ExportedFunction("assemble")
	results, err := assemble.Call(ctx, args.Input.Bar, args.Input.Foo)
	if err != nil {
		log.Panicf("failed to call assemble: %v", err)
	}

	fmt.Printf("%d\n", results[0])

	*result = Response{Result: results[0]}
	return nil
}

func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.
	// Instantiate WASI, which implements host functions needed for TinyGo to
	// implement `panic`.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Instantiate the guest Wasm into the same runtime. It exports the `add`
	// function, implemented in WebAssembly.
	var err error
	domainA, err = r.Instantiate(ctx, domainAWASM)
	if err != nil {
		log.Panicf("failed to instantiate module domain A: %v", err)
	}

	// Create a new WebAssembly Runtime.
	rb := wazero.NewRuntime(ctx)
	defer rb.Close(ctx) // This closes everything this Runtime created.
	wasi_snapshot_preview1.MustInstantiate(ctx, rb)
	domainB, err = rb.Instantiate(ctx, domainBWASM)
	if err != nil {
		log.Panicf("failed to instantiate module domain B: %v", err)
	}

	rpcServer := rpc.NewServer()

	rpcServer.RegisterCodec(json.NewCodec(), "application/json")
	rpcServer.RegisterCodec(json.NewCodec(), "application/json;charset=UTF-8")

	tokenA := new(TokenADomain)
	tokenB := new(TokenBDomain)

	rpcServer.RegisterService(tokenA, "TokenA")
	rpcServer.RegisterService(tokenB, "TokenB")

	router := mux.NewRouter()
	router.Handle("/", rpcServer)
	http.ListenAndServe(":1337", router)
}
