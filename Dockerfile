FROM golang:1.20 as BUILD
COPY . /src
WORKDIR /src
RUN CGO_ENABLED=0 go build -o /syncd /src/main.go

FROM scratch
COPY --from=BUILD /syncd /syncd