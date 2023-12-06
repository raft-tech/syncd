FROM golang:1.20@sha256:bdd178660145acd8edcc076bec4c92888988a6556de7b3363a30cdea87b7086b as BUILD
COPY go.mod go.sum /src/
WORKDIR /src
RUN go mod download
COPY . /src
RUN CGO_ENABLED=0 go build -o /syncd /src/main.go

FROM golang:1.20@sha256:bdd178660145acd8edcc076bec4c92888988a6556de7b3363a30cdea87b7086b AS gRPC
ARG PROTOC_VERSION=23.4
RUN apt-get update && apt-get install --no-install-recommends -y \
    unzip=6.0-26+deb11u1 \
    && rm -rf /var/lib/apt/lists/*
RUN wget -O /tmp/protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip \
    && unzip -d /usr/local /tmp/protoc.zip bin/protoc \
    && rm -rf /tmp/protoc.zip \
    # protoc-gen-go@v1.28
    # protoc-gen-go-grpc@v1.2.0
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@32051b4f86e54c2142c7c05362c6e96ae3454a1c; \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@cdee119ee21e61eef7093a41ba148fa83585e143
WORKDIR /src
ENTRYPOINT ["protoc"]

FROM scratch
COPY --from=BUILD /syncd /syncd
USER 1099
ENTRYPOINT ["/syncd"]