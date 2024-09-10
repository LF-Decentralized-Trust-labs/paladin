FROM golang:1.22

WORKDIR /usr/src/app
RUN go env -w GOMODCACHE=/root/.cache/go-build

COPY go.mod go.sum /usr/src/app/

# Download dependencies separately from the build process to cache them
RUN --mount=type=cache,target=/root/.cache/go-build go mod download

COPY besu_bootstrap/ /usr/src/app/besu_bootstrap

RUN  --mount=type=cache,target=/root/.cache/go-build go build -o /usr/local/bin/besu_bootstrap ./besu_bootstrap

ENTRYPOINT [ "/usr/local/bin/besu_bootstrap" ]
