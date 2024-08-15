FROM golang:1.22

WORKDIR /usr/src/app

ENV CGO_ENABLED 1

COPY go.mod go.sum /usr/src/app/
COPY . /usr/src/app
RUN go build -o /usr/local/bin/testbed ./testbed

ENTRYPOINT [ "/usr/local/bin/testbed" ]
