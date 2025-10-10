# Build the manager binary
FROM golang:1.24-bookworm AS builder

WORKDIR /workspace

# We use Dockerfile.dockerignore to make this efficient
COPY . .

# Build
# TODO: Investigate if there is a need for cross-compilation, and if so we need to
#       wait to implement until the domain specific signer plugin interface 
#       is enabled, such that Zeto can remove non-cross-compilable ZKP libs
#       from the core build.
RUN cd operator; go build -a -o manager cmd/main.go

# TODO: Use distroless as minimal base image to package the manager binary
#       Depends on debugging/reconciling dependencies of operator
# FROM gcr.io/distroless/static:nonroot

FROM ubuntu:24.04
WORKDIR /
COPY --from=builder --chown=65532:65532 /workspace/operator/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
