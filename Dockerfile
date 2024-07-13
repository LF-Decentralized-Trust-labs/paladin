FROM golang:1.22-bullseye AS builder

ARG BUILD_VERSION
ENV BUILD_VERSION=${BUILD_VERSION}
ADD --chown=1001:0 ./runtime /runtime
ADD --chown=1001:0 ./blockchain-transaction-manager /blockchain-transaction-manager
ADD --chown=1001:0 ./transaction-manager /transaction-manager
ADD --chown=1001:0 ./Makefile.common /Makefile.common

WORKDIR /runtime
RUN mkdir /.cache \
    && chgrp -R 0 /.cache \
    && chmod -R g+rwX /.cache
<<<<<<< HEAD
# A PSQL DB must be setup already for running all the tests.
ENV CREATE_TEST_DB=0
=======
>>>>>>> d664889020b1294dc3305d0940e7f3baff5bbce2
USER 1001
RUN make

FROM debian:buster-slim
WORKDIR /runtime
RUN chgrp -R 0 /runtime \
    && chmod -R g+rwX /runtime
RUN apt update -y \
    && apt install -y curl jq \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder --chown=1001:0 /runtime/paladin /usr/bin/paladin
USER 1001

ENTRYPOINT [ "/usr/bin/paladin" ]
