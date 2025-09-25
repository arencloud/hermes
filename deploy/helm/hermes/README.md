# Hermes Helm Chart

Deploy Hermes (S3 storage manager) to Kubernetes, OKD, or OpenShift in rootless mode.

## Features
- Rootless Deployment (non-root securityContext, no privileged permissions)
- SQLite persistence via PVC, or PostgreSQL by setting `db.driver=postgres` and `db.dsn`
- Kubernetes Ingress or OpenShift Route support
- Configurable probes, resources, and environment

## Prerequisites
- A container image for Hermes available in your registry (set `image.repository` and `image.tag`)
- Kubernetes 1.22+ (or OKD/OpenShift 4.x)
- Helm 3.8+

## Quickstart (Kubernetes / OKD)

```bash
# Set the image reference
export IMG=ghcr.io/arencloud/hermes:latest

helm upgrade --install hermes deploy/helm/hermes \
  --set image.repository=${IMG%%:*} \
  --set image.tag=${IMG##*:} \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=hermes.local \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix
```

Then add an entry in your `/etc/hosts` pointing to your ingress controller IP or use a real DNS record.

## Quickstart (OpenShift / OKD Route)

```bash
# Login to your OpenShift cluster
# oc login ...

# Create a project/namespace
oc new-project hermes || true

# Install via Helm with an OpenShift Route
helm upgrade --install hermes deploy/helm/hermes -n hermes \
  --set image.repository=${IMG%%:*} \
  --set image.tag=${IMG##*:} \
  --set openshift.route.enabled=true \
  --set openshift.route.host=hermes.apps.example.openshift.com

# Get the route
oc -n hermes get route hermes
```

If you do not specify a `host`, OpenShift can assign one automatically depending on your cluster’s configuration.

## Persistence
- By default, the chart provisions a PersistentVolumeClaim and mounts it at `/data`.
- Hermes stores SQLite at `/data/hermes.db` (configurable by `db.sqlitePath`).
- To use PostgreSQL instead, set `db.driver=postgres` and `db.dsn` and you can disable persistence with `persistence.enabled=false`.

## Environment
- Default environment is provided via a ConfigMap using values:
  - `HTTP_PORT` (from `http.containerPort`)
  - `DB_DRIVER`, `DB_PATH`/`DATABASE_URL`
  - `STATIC_DIR`
- Extra environment variables can be set via `extraEnv` and `extraEnvFrom`.

## Security / Rootless
- The chart sets `runAsNonRoot: true` and avoids pinning a specific UID, which is compatible with OpenShift’s random UID strategy.
- `allowPrivilegeEscalation` is disabled and `readOnlyRootFilesystem` is false by default (Hermes writes the SQLite DB to `/data`).

## Values Overview
See `values.yaml` for full options. Common flags:

```yaml
image:
  repository: ghcr.io/arencloud/hermes
  tag: latest

service:
  type: ClusterIP
  port: 8080

http:
  containerPort: 8080

persistence:
  enabled: true
  size: 1Gi
  mountPath: /data

db:
  driver: sqlite
  sqlitePath: /data/hermes.db
  dsn: ""

upload:
  # Max upload size enforced by the app (0 = unlimited)
  maxBodyBytes: 0

ingress:
  enabled: false
  annotations: {}
  # For NGINX ingress controller you may need:
  #   nginx.ingress.kubernetes.io/proxy-body-size: "0"     # unlimited
  #   nginx.ingress.kubernetes.io/proxy-request-buffering: "off"  # stream uploads

openshift:
  route:
    enabled: false
    annotations: {}
    # The default OpenShift HAProxy router does not have a per-route body size limit.
    # If you use a third-party ingress controller on OpenShift (e.g., NGINX), configure its annotations above.
```

## Large file uploads (Kubernetes/OKD/OpenShift)
- At the cluster edge, ensure your ingress/controller allows large bodies and streaming:
  - NGINX Ingress: set `ingress.annotations`:
    - `nginx.ingress.kubernetes.io/proxy-body-size: "0"` (or a concrete size like `200m`)
    - `nginx.ingress.kubernetes.io/proxy-request-buffering: "off"`
- Probes under load: if you observe pod restarts during long uploads, relax probe timings:
  - Increase `livenessProbe.timeoutSeconds` (e.g., 5) and `livenessProbe.failureThreshold` (e.g., 6).
  - Increase `readinessProbe.timeoutSeconds` and `readinessProbe.failureThreshold` similarly.
  - Optionally enable a `startupProbe` to prevent early liveness checks during cold start or heavy IO.
- Review resource limits: heavy uploads can be CPU-throttled if `resources.limits.cpu` is too low; consider raising CPU and memory limits/requests.
- In the app, you can cap uploads with `upload.maxBodyBytes` (maps to env `MAX_UPLOAD_SIZE_BYTES`).
- Hermes streams multipart file parts directly to S3 (no full in-memory buffering).

## Uninstall
```bash
helm uninstall hermes -n hermes
```
