.PHONY: run
run: domains
	go run main.go

domains: domains/domainC/domainC.wasm

domains/domainC/domainC.wasm: domains/domainC/main.mjs domains/domainC/main.wit
	cd domains/domainC && npx jco componentize main.mjs --wit main.wit --world-name example --out domainC.wasm
