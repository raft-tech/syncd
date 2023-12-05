FROM golang:1.20@sha256:bdd178660145acd8edcc076bec4c92888988a6556de7b3363a30cdea87b7086b as BUILD
COPY go.mod go.sum /src/
WORKDIR /src
RUN go mod download
COPY . /src
RUN CGO_ENABLED=0 go build -C examples/things/app -o /things .

FROM scratch
COPY --from=BUILD /things /things
USER 1099
ENTRYPOINT ["/things"]
