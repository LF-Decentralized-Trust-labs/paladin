GOFILES := $(shell find . -name '*.go' -print)

build: gable.so

gable.so: ${GOFILES}
	go build -o gable.so -buildmode=c-shared gable.go
