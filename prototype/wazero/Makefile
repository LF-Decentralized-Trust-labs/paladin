.PHONY: run
run: domains
	go run main.go

domains: domains/domainA/domainA.wasm domains/domainB/domainB.wasm

domains/domainA/domainA.wasm: domains/domainA/main.go
	# cd domains/domainA && GOOS=js GOARCH=wasm go build -o domainA.wasm main.go
	cd domains/domainA && tinygo build -o domainA.wasm -target=wasi main.go
	# cd domains/domainA && GOOS=js GOARCH=wasm tinygo build -o domainA.wasm -target=wasi main.go
	# cd domains/domainA && GOOS=wasip1 GOARCH=wasm go build -o domainA.wasm main.go


domains/domainB/domainB.wasm: domains/domainB/main.go
	cd domains/domainB && tinygo build -o domainB.wasm -target=wasi main.go
	# cd domains/domainB && GOOS=wasip1 GOARCH=wasm go build -o domainB.wasm main.go

