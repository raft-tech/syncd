
.PHONY: generate
generate: pkg/api/syncd.pb.go pkg/api/syncd_grpc.pb.go

pkg/api/syncd.pb.go pkg/api/syncd_grpc.pb.go: api/syncd.proto
	docker run --rm -v $$(pwd):/src syncd-grpc:latest \
	--experimental_allow_proto3_optional \
	--go_opt=module=github.com/raft-tech/syncd --go_opt=paths=import \
	--go-grpc_opt=module=github.com/raft-tech/syncd --go-grpc_opt=paths=import \
	api/syncd.proto