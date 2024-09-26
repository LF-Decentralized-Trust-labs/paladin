# Stage 1: Builder
FROM ubuntu:24.04 AS builder

# Set environment variables
ENV LANG=C.UTF-8
ENV PATH=$PATH:/usr/local/go/bin:/opt/gradle/bin:/usr/local/bin/protoc/bin:/usr/local/bin

# Install build dependencies
RUN apt-get update && apt-get install -y \
    curl \
    unzip \
    git \
    openjdk-21-jdk \
    build-essential \
    gcc \
    g++ \
    gcc-multilib \
    libc6-dev \
    musl-dev \
    pkg-config \
    libgomp1 \
    && apt-get clean

# Install Node.js v18 and npm
RUN curl -fsSL https://deb.nodesource.com/setup_18.x | bash - && \
    apt-get install -y nodejs && \
    npm install -g npm@latest

# Install Protoc
RUN PROTO_VERSION=28.2 && \
    curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v$PROTO_VERSION/protoc-$PROTO_VERSION-linux-x86_64.zip && \
    unzip protoc-$PROTO_VERSION-linux-x86_64.zip -d /usr/local/bin/protoc && \
    rm protoc-$PROTO_VERSION-linux-x86_64.zip

# Install Go
RUN GO_VERSION=1.22.7 && \
    curl -LO https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && \
    rm go${GO_VERSION}.linux-amd64.tar.gz

# Install Gradle
RUN GRADLE_VERSION=8.4 && \
    curl -LO https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip && \
    unzip gradle-${GRADLE_VERSION}-bin.zip -d /opt && \
    ln -s /opt/gradle-${GRADLE_VERSION}/bin/gradle /usr/bin/gradle && \
    rm gradle-${GRADLE_VERSION}-bin.zip

# Install Wasmer (which includes libwasmer.so)
RUN curl https://get.wasmer.io -sSfL | sh

# Add Wasmer to the system path
ENV PATH="/root/.wasmer/bin:${PATH}"

# Set the working directory
WORKDIR /app

# Copy project files
COPY . .

# Set Go CGO environment variables
ENV CGO_ENABLED=1
ENV GOARCH=amd64
ENV CC=gcc

# Assemble executables/artifacts
RUN ./gradlew --no-daemon assemble

# Stage 2: Runtime
FROM ubuntu:24.04 AS runtime

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    libgomp1 \
    openjdk-21-jdk \
    && apt-get clean

# Set environment variables
ENV LANG=C.UTF-8
ENV LD_LIBRARY_PATH=/app/libs:/app/.wasmer/lib:$LD_LIBRARY_PATH

# Set the working directory
WORKDIR /app

# Copy Wasmer shared libraries to the runtime container
COPY --from=builder /root/.wasmer/lib/libwasmer.so /app/.wasmer/lib/libwasmer.so

# Copy the build artifacts from the builder stage
COPY --from=builder /app/build /app

# Define the entry point for running the application
ENTRYPOINT [                         \
    "java",                          \
    "-Djna.library.path=/app/libs",  \
    "-jar",                          \
    "/app/libs/paladin.jar"          \
]

