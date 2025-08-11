FROM golang:1.24-bookworm

WORKDIR /usr/src/app

COPY go.mod go.sum /usr/src/app/
COPY pkg /usr/src/app/pkg
COPY besu_bootstrap/ /usr/src/app/besu_bootstrap
RUN go build -o /usr/local/bin/besu_bootstrap ./besu_bootstrap

ENTRYPOINT [ "/usr/local/bin/besu_bootstrap" ]
