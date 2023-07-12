FROM golang:1.20 as BUILD
COPY . /src
WORKDIR /src
RUN go build -o /syncd /src/main.go

FROM golang:1.20 AS gRPC
RUN apt-get update && \
    apt-get install -y protobuf-compiler=3.12.4-1 && \
    rm -rf /var/lib/apt/lists/*
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28; \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
WORKDIR /src
ENTRYPOINT ["protoc"]

FROM cgr.dev/chainguard/static:latest-glibc
COPY --from=BUILD /syncd /syncd