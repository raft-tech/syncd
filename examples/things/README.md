## Environment Set Up

### Create a KIND cluster for local Kubernetes
```shell
cat <<EOF | kind create cluster --config=-
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
    hostPort: 80
    protocol: TCP
EOF
```

#### Add NGINX Ingress Controller
```shell
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
```

### Add Kube Prometheus Stack
```shell
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm upgrade --install --create-namespace --namespace monitoring kube-prometheus prometheus-community/kube-prometheus-stack \
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
```

### Install Things

```shell
helm upgrade --install --create-namespace --namespace things things charts/things
```

### Create a Secret for syncd
```shell
cat << EOF |kubectl -n things apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: things-syncd
  namespace: things
stringData:
  SYNCD_POSTGRESQL_CONN: postgres://syncd:$(kubectl -n things get secret/things-postgresql -o template --template '{{ index .data "postgres-password"  }}' |base64 -d)@things-postgresql:5432/syncd
EOF
```