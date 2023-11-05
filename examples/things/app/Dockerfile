FROM golang:1.20 as BUILD
COPY go.mod go.sum /src/
WORKDIR /src
RUN go mod download
COPY . /src
RUN CGO_ENABLED=0 go build -o /things ./examples/things/app

FROM scratch
COPY --from=BUILD /things /things
ENTRYPOINT ["/things"]
