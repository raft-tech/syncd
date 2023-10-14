
.PHONY: generate
generate: pkg/api/syncd.pb.go pkg/api/syncd_grpc.pb.go


.PHONY: test-all
test-all: test test-postgres

.PHONY: test
test:
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


pkg/api/syncd.pb.go pkg/api/syncd_grpc.pb.go: api/syncd.proto
	docker run --rm -v $$(pwd):/src syncd-grpc:latest \
	--experimental_allow_proto3_optional \
	--go_out=. --go_opt=module=github.com/raft-tech/syncd --go_opt=paths=import \
	--go-grpc_out=. --go-grpc_opt=module=github.com/raft-tech/syncd --go-grpc_opt=paths=import \
	api/syncd.proto


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