FROM golang:1.22

WORKDIR /usr/src/app

COPY go.mod go.sum /usr/src/app/
COPY tools/besu_bootstrap/ /usr/src/app/tools/besu_bootstrap
RUN go build -o /usr/local/bin/besu_bootstrap ./tools/besu_bootstrap

ENTRYPOINT [ "/usr/local/bin/besu_bootstrap" ]
