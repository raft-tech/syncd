FROM golang:1.20 as BUILD
COPY . /src
WORKDIR /src
RUN CGO_ENABLED=0 go build -C examples/things/app -o /things .

FROM scratch
COPY --from=BUILD /things /things
USER 1000
ENTRYPOINT ["/things"]
