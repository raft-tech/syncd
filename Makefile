.PHONY: build
build: generate
	CGO_ENABLED=0 go build -o syncd

.PHONY: clean
clean:
	[ ! -f syncd ] || rm syncd
	[ ! -f pkg/server/server.crt ] || rm pkg/server/server.crt
	[ ! -f pkg/server/server.key ] || rm pkg/server/server.key
	go clean -cache -testcache -x

.PHONY: generate
generate: internal/api/syncd.pb.go internal/api/syncd_grpc.pb.go

.PHONY: test-all
test-all: test test-postgres

.PHONY: test
test: generate
	go vet ./...
	go test ./... -short -timeout 15s

.PHONY: test-postgres
test-postgres: postgres-start test-postgres-do postgres-stop

.PHONY: test-postgres-do
test-postgres-do: export SYNCD_POSTGRESQL_CONN=postgres://postgres:syncd@127.0.0.1:5432/postgres
test-postgres-do:
	go clean -testcache
	go test ./pkg/graph/postgres/... -timeout 20s || (docker stop syncd-postgres && exit 1)
	docker stop syncd-postgres


internal/api/syncd.pb.go internal/api/syncd_grpc.pb.go: api/syncd.proto
	docker run --rm -v $$(pwd):/src syncd-grpc:latest \
	--experimental_allow_proto3_optional \
	--go_out=. --go_opt=module=github.com/raft-tech/syncd --go_opt=paths=import \
	--go-grpc_out=. --go-grpc_opt=module=github.com/raft-tech/syncd --go-grpc_opt=paths=import \
	api/syncd.proto

.PHONY: tls-test-cert
tls-test-cert:
	openssl req -x509 -newkey rsa:1024 -keyout server.key -out server.crt -days 30 -nodes \
		-subj "/C=US/CN=syncd" \
		-addext "subjectAltName=DNS:syncd"

# PostgreSQL Instances

.PHONY: postgres-start
postgres-start:
	[ -z $$(docker container ls -q --filter name=syncd-postgres) ] && docker run --rm -d --name syncd-postgres -e POSTGRES_PASSWORD=syncd -p 127.0.0.1:5432:5432/tcp postgres:15.4
	docker run --rm -e POSTGRES_PASSWORD=syncd --net=host --entrypoint=/bin/bash postgres:15.4 -c 'for i in $$(seq 1 7); do if pg_isready -h 127.0.0.1 -U postgres -d syncd; then break; else [ $$i -eq 7 ] && exit 1 || sleep 1; fi done'

.PHONY: postgres-stop
postgres-stop:
	docker container ls -q --filter name=syncd-postgres
	[ -z $$(docker container ls -q --filter name=syncd-postgres) ] || docker stop syncd-postgres
	[ -z $$(docker container ls -aq --filter name=syncd-postgres) ] || docker container rm syncd-postgres