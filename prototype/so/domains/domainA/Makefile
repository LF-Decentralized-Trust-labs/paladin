.PHONY: all
all: clean build 

.PHONY: build
build: domain_a.so

domain_a.so: main.go
	go build -buildmode=plugin -o domain_a.so main.go

clean:
	rm -f domain_a.so
