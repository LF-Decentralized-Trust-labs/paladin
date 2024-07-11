package main

import (
	"fmt"
)

//export assemble
func assemble(x, y uint32) uint32 {
	fmt.Printf("A")
	return x + y
}

// main is required for the `wasi` target, even if it isn't used.
// See https://wazero.io/languages/tinygo/#why-do-i-have-to-define-main
func main() {
	fmt.Printf("main")
}
