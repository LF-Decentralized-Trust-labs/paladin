# Dependency versions (some used by builder and runtime)
ARG JAVA_VERSION=21.0.4+7
ARG NODE_VERSION=20.17.0
ARG PROTO_VERSION=28.2
ARG GO_VERSION=1.22.7
ARG GO_MIGRATE_VERSION=4.18.1
ARG GRADLE_VERSION=8.4
ARG WASMER_VERSION=4.3.7

# Additional JVM selection options
ARG JVM_TYPE=hotspot
ARG JVM_HEAP=normal

# Stage 1: Builder
FROM ubuntu:24.04 AS builder   

ARG TARGETOS
ARG TARGETARCH
ARG JAVA_VERSION
ARG JVM_TYPE
ARG JVM_HEAP
ARG NODE_VERSION
ARG PROTO_VERSION
ARG GO_VERSION
ARG GRADLE_VERSION
ARG WASMER_VERSION

# Set environment variables
ENV LANG=C.UTF-8

# Install build dependencies
RUN apt-get update && apt-get install -y \
    curl \
    unzip \
    git \
    build-essential \
    gcc \
    g++ \
    libc6-dev \
    pkg-config \
    libgomp1 \
    xz-utils \
    && apt-get clean

# Install JDK
RUN JAVA_ARCH=$( if [ "$TARGETARCH" = "arm64" ]; then echo -n "aarch64"; else echo -n "x64"; fi ) && \
    curl -sLo - https://api.adoptium.net/v3/binary/version/jdk-${JAVA_VERSION}/${TARGETOS}/${JAVA_ARCH}/jdk/${JVM_TYPE}/${JVM_HEAP}/eclipse | \
    tar -C /usr/local -xzf - && \
    ln -s /usr/local/jdk-* /usr/local/java

# Install Node.js v18 and npm
RUN NODE_ARCH=$( if [ "$TARGETARCH" = "arm64" ]; then echo -n "arm64"; else echo -n "x64"; fi ) && \
    curl -sLo - https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-${TARGETOS}-${NODE_ARCH}.tar.xz | \
    xz -cd - | tar -C /usr/local -xf - && \
    ln -s /usr/local/node-* /usr/local/node

# Install Protoc
RUN PROTO_ARCH=$( if [ "$TARGETARCH" = "arm64" ]; then echo -n "aarch_64"; else echo -n "x86_64"; fi ) && \
    curl -sLo protoc-$PROTO_VERSION-${TARGETOS}-${PROTO_ARCH}.zip \
      https://github.com/protocolbuffers/protobuf/releases/download/v$PROTO_VERSION/protoc-$PROTO_VERSION-${TARGETOS}-${PROTO_ARCH}.zip && \
    unzip protoc-$PROTO_VERSION-${TARGETOS}-${PROTO_ARCH}.zip -d /usr/local/protoc && \
    rm protoc-$PROTO_VERSION-${TARGETOS}-${PROTO_ARCH}.zip

# Install Go
RUN GO_ARCH=$( if [ "$TARGETARCH" = "arm64" ]; then echo -n "arm64"; else echo -n "amd64"; fi  ) && \
    curl -sLo - https://go.dev/dl/go${GO_VERSION}.${TARGETOS}-${GO_ARCH}.tar.gz | \
    tar -C /usr/local -xzf -

# Install Gradle
RUN curl -sLo gradle-${GRADLE_VERSION}-bin.zip https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip && \
    unzip gradle-${GRADLE_VERSION}-bin.zip -d /usr/local && \
    rm gradle-${GRADLE_VERSION}-bin.zip && \
    ln -s /usr/local/gradle-* /usr/local/gradle

# Install Wasmer (which includes libwasmer.so)
RUN WASMER_ARCH=$( if [ "$TARGETARCH" = "arm64" ]; then echo -n "aarch64"; else echo -n "amd64"; fi  ) && \
    mkdir -p /usr/local/wasmer && \
    curl -sLo - https://github.com/wasmerio/wasmer/releases/download/v${WASMER_VERSION}/wasmer-${TARGETOS}-${WASMER_ARCH}.tar.gz | \
    tar -C /usr/local/wasmer -zxf -

# Add all the tools we installed to the path
ENV PATH=$PATH:/usr/local/bin
ENV PATH=$PATH:/usr/local/go/bin
ENV PATH=$PATH:/root/go/bin
ENV PATH=$PATH:/usr/local/node/bin
ENV PATH=$PATH:/usr/local/java/bin
ENV PATH=$PATH:/usr/local/gradle/bin
ENV PATH=$PATH:/usr/local/protoc/bin
ENV PATH=$PATH:/usr/local/wasmer/bin

# Set the working directory
WORKDIR /app

# Copy project files (check .dockerignore for details of what goes up)
COPY . .

# Set Go CGO environment variables
ENV CGO_ENABLED=1
ENV CC=gcc

# Assemble executables/artifacts
RUN gradle --no-daemon --parallel assemble

# Stage 2: Runtime
FROM ubuntu:24.04 AS runtime

ARG TARGETOS
ARG TARGETARCH
ARG JAVA_VERSION
ARG JVM_TYPE
ARG JVM_HEAP
ARG GO_MIGRATE_VERSION

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    libgomp1 \
    curl \
    && apt-get clean

# Set environment variables
ENV LANG=C.UTF-8
ENV LD_LIBRARY_PATH=/app/libs:/usr/local/wasmer/lib:$LD_LIBRARY_PATH

# Set the working directory
WORKDIR /app

# Install JRE
RUN JAVA_ARCH=$( if [ "$TARGETARCH" = "arm64" ]; then echo -n "aarch64"; else echo -n "x64"; fi ) && \
    curl -sLo - https://api.adoptium.net/v3/binary/version/jdk-${JAVA_VERSION}/${TARGETOS}/${JAVA_ARCH}/jre/${JVM_TYPE}/${JVM_HEAP}/eclipse | \
    tar -C /usr/local -xzf - && \
    ln -s /usr/local/jdk-* /usr/local/java

# Install DB migration tool
RUN GO_MIRGATE_ARCH=$( if [ "$TARGETARCH" = "arm64" ]; then echo -n "arm64"; else echo -n "amd64"; fi ) && \
    curl -sLo - https://github.com/golang-migrate/migrate/releases/download/v$GO_MIGRATE_VERSION/migrate.${TARGETOS}-${GO_MIRGATE_ARCH}.tar.gz | \
    tar -C /usr/local/bin -xzf - migrate

# Copy Wasmer shared libraries to the runtime container
COPY --from=builder /usr/local/wasmer/lib/libwasmer.so /usr/local/wasmer/lib/libwasmer.so

# Copy the build artifacts from the builder stage
COPY --from=builder /app/build /app

# Copy the db migration files
COPY --from=builder /app/core/go/db /app/db

# Add tools we installed to the path
ENV PATH=$PATH:/usr/local/java/bin

# Define the entry point for running the application
ENTRYPOINT [                         \
    "java",                          \
    "-Djna.library.path=/app/libs",  \
    "-jar",                          \
    "/app/libs/paladin.jar"          \
]
 