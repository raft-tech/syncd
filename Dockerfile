FROM golang:1.20 as BUILD
COPY . /src
WORKDIR /src
RUN go build -o /syncd /src/main.go

FROM golang:1.20 AS gRPC
ARG PROTOC_VERSION=23.4
RUN apt-get update && apt-get install -y \
    unzip=6.0-26+deb11u1 \
    && rm -rf /var/lib/apt/lists/*
RUN wget -O /tmp/protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip \
    && unzip -d /usr/local /tmp/protoc.zip bin/protoc \
    && rm -rf /tmp/protoc.zip
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28; \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
WORKDIR /src
ENTRYPOINT ["protoc"]

FROM cgr.dev/chainguard/static:latest-glibc
COPY --from=BUILD /syncd /syncd