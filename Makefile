
SYNCD_VERSION := $(shell cat VERSION)
SYNCD_IMAGE := rafttech/syncd:v$(SYNCD_VERSION)

.PHONY: build
build: generate
	CGO_ENABLED=0 go build -o syncd

.PHONY: clean
clean: cluster-clean docker-clean
	[ ! -f syncd ] || rm syncd
	[ ! -d cmd/build ] || rm -rf cmd/build
	[ ! -f server.crt ] || rm server.crt
	[ ! -f server.key ] || rm server.key
	[ ! -f things ] || rm things
	go clean -cache -testcache

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


server.crt server.key:
	openssl req -x509 -newkey rsa:2048 -keyout server.key -out server.crt -days 30 -nodes \
		-subj "/C=US/CN=syncd" \
		-extensions "subjectAltName=DNS:syncd"

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


# Docker
.PHONY: docker
docker:
	docker build -t $(SYNCD_IMAGE) .

.PHONY: docker-clean
docker-clean:
	[ -z "$$(docker images $(SYNCD_IMAGE) --quiet)" ] || docker image rm $(SYNCD_IMAGE) && docker image prune --force


# Things (Data Generator at examples/things/)

THINGS_VERSION := $(shell cat VERSION)
THINGS_IMAGE := rafttech/things:v$(SYNCD_VERSION)

.PHONY: things
things:
	CGO_ENABLED=false go build -C examples/things/app -o ../../../things .

.PHONY: docker-things
docker-things:
	docker build -t $(THINGS_IMAGE) -f examples/things/app/Dockerfile .

.PHONY: docker-clean-things
docker-clean-things:
	[ -z "$$(docker images $(THINGS_IMAGE) --quiet)" ] || docker image rm $(THINGS_IMAGE) && docker image prune --force


# KIND demo

.PHONY: demo
demo: cluster cluster-load demo-server demo-client

define CLUSTER_CONFIG
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 8080
    protocol: TCP
endef

.PHONY: cluster
export CLUSTER_CONFIG
cluster:
	@[ -n "$$(kind get clusters |grep syncd)" ] && echo "syncd cluster already exists" || echo "$$CLUSTER_CONFIG" | kind create cluster --name syncd --config=-
	@[ "$$(kubectl config current-context)" == "kind-syncd" ] && echo "proper context already set" || kubectl config set-context kind-syncd
	@kubectl get ingressclass/nginx > /dev/null && echo "nginx ingress already installed" || kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
	@kubectl -n ingress-nginx wait deploy/ingress-nginx-controller --for=condition=Available=True
	@kubectl -n ingress-nginx wait pod -l app.kubernetes.io/name=ingress-nginx -l app.kubernetes.io/component=controller --for=condition=Ready=True
	@[ -n "$$(helm repo list |grep syncd-prometheus)" ] || helm repo add syncd-prometheus https://prometheus-community.github.io/helm-charts
	@helm repo update syncd-prometheus
	@helm upgrade --install syncd-monitoring syncd-prometheus/kube-prometheus-stack --version=52.0.1 --create-namespace --namespace monitoring \
		--set 'grafana.enabled=true' \
		--set 'grafana.ingress.enabled=true' \
		--set 'grafana.ingress.class=nginx' \
		--set 'grafana.ingress.hosts={localhost}' \
		--set 'grafana.ingress.path=/grafana' \
		--set 'grafana.grafana\.ini.server.security.cookie_secure=false' \
		--set 'grafana.grafana\.ini.server.domain=localhost' \
		--set 'grafana.grafana\.ini.server.protocol=http' \
		--set 'grafana.grafana\.ini.server.root_url=%(protocol)s://%(domain)s:%(http_port)s/grafana/' \
		--set 'grafana.grafana\.ini.server.serve_from_sub_path=true'

.PHONY: cluster-clean
cluster-clean: cluster-clean
	[ -z "$$(kind get clusters |grep syncd)" ] && echo "syncd cluster not found" || kind delete cluster --name syncd
	[ -z "$$(helm repo list |grep syncd-prometheus)" ] || helm repo remove syncd-prometheus

.PHONY: cluster-load
cluster-load: docker docker-things
	kind load docker-image $(SYNCD_IMAGE) $(THINGS_IMAGE) --name syncd
	
.PHONY: demo-server
demo-server: server.crt server.key
	helm upgrade --install things examples/things/charts/things --namespace alpha --create-namespace
	kubectl -n alpha get secret/things-syncd -o name > /dev/null || kubectl -n alpha create secret generic things-syncd \
		--from-literal=HOST=things-postgresql \
		--from-literal=USERNAME=things \
		--from-literal=PASSWORD=$$(kubectl -n alpha get secret/things-postgresql -o template --template '{{ index .data "password"  }}' |base64 -d) \
		--from-literal=DATABASE=things
	helm upgrade --install alpha charts/syncd --namespace alpha \
		--set auth.preSharedKey.enabled=true \
		--set auth.preSharedKey.value=mysecret \
		--set graph.source.postgres.connection.fromPreExistingSecret=true \
		--set graph.source.postgres.connection.preExistingSecret.name=things-syncd \
		--set server.enabled=true \
		--set server.tls.enabled=true \
		--set server.tls.values.certificate="$$(cat server.crt)" \
		--set server.tls.values.privateKey="$$(cat server.key)"

.PHONY: demo-server-clean
demo-server-clean:
	-helm -n alpha uninstall alpha things
	-kubectl delete ns/alpha

.PHONY: demo-client
demo-client: server.crt
	helm upgrade --install things examples/things/charts/things --namespace bravo --create-namespace
	kubectl -n bravo get secret/things-syncd -o name > /dev/null || kubectl -n bravo create secret generic things-syncd \
		--from-literal=HOST=things-postgresql \
		--from-literal=USERNAME=things \
		--from-literal=PASSWORD=$$(kubectl -n bravo get secret/things-postgresql -o template --template '{{ index .data "password"  }}' |base64 -d) \
		--from-literal=DATABASE=things
	helm upgrade --install bravo charts/syncd --namespace bravo \
		--set auth.preSharedKey.enabled=true \
		--set auth.preSharedKey.value=mysecret \
		--set graph.source.postgres.connection.fromPreExistingSecret=true \
		--set graph.source.postgres.connection.preExistingSecret.name=things-syncd \
		--set client.enabled=true \
		--set client.tls.enabled=true \
		--set client.tls.trustedCAs.values.syncd\\.crt="$$(cat server.crt)" \
		--set client.peers.alpha.address=alpha-syncd-server.alpha.svc:443 \
		--set client.peers.alpha.authority=syncd \
		--set "client.peers.alpha.pull[0]=things" \
		--set "client.peers.alpha.push.things.filters[0].key=owner" \
		--set "client.peers.alpha.push.things.filters[0].operator=Equals" \
		--set "client.peers.alpha.push.things.filters[0].value=bravo"

.PHONY: demo-client-clean
demo-client-clean:
	-helm -n bravo uninstall bravo things
	-kubectl delete ns/bravo