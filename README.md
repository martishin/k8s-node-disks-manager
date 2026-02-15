# node-disks-manager

`node-disks-manager` demonstrates an operator + node-agent pattern for discovering node-local disks and coordinating their allocation.

The `node-disks-operator` reconciles `NodeDisk` resources and applies a label-driven policy. The `node-disks-agent` runs on every node, discovers local disk files, and updates node-observed state. The flow looks like this:

* when a `NodeDisk` is labeled with `localtest.example.com/auto-reserve=true`, it sets `spec.desired.state=Reserved` and `spec.desired.owner=auto`
* when the label is removed, auto-owned reservations are reverted to `Available`.

`NodeDisk` is a cluster-scoped resource representing one disk on one node, with explicit ownership boundaries:

* `node-disks-agent` writes `status.node` (phase, capacity, heartbeat, message)
* `node-disks-operator` writes `status.controller` (validation and reconciliation state).

For local testing, disks are simulated as `*.img` files under `/var/lib/node-disks-manager/disks`.

## NodeDisk Model

`NodeDisk` is the contract between the operator and the node agent:

- `spec` describes identity and desired intent:
  - `nodeName`, `diskID`, `path`
  - `desired.state` (`Available` or `Reserved`)
  - `desired.owner` (for example `auto`)
- `status.node` is written by the agent:
  - `phase`, `capacityBytes`, `lastSeenTime`, `message`
- `status.controller` is written by the operator:
  - `accepted`, `observedGeneration`, `conditions`

## Prerequisites

- Go 1.25+
- `docker`
- `kubectl`
- `helm`
- `minikube`

## Quickstart

### 1) Start a local multi-node cluster

```bash
make minikube-up
```

### 2) Build and load images into minikube

```bash
make images-load
```

This target builds each image once locally, then loads it into the selected minikube profile.

### 3) Install the chart

```bash
make helm-install
kubectl -n node-disks-system get pods -o wide
```

At this point, the operator, agent DaemonSet, and seeder DaemonSet should be running.

### 4) Verify discovery

```bash
kubectl get crd nodedisks.localtest.example.com
kubectl get nodedisks -o wide
```

You should see one `NodeDisk` per seeded disk file on each worker node.

## Validate Reservation Flow

### 1) Pick a target disk

```bash
kubectl get nodedisks -o wide
```

Choose one resource name from the output, for example `node-disk-node-disks-dev-m02-disk1`.

### 2) Reserve the disk through policy

```bash
ND_NAME=node-disk-<node-name>-<disk-id>
kubectl label nodedisk "$ND_NAME" localtest.example.com/auto-reserve=true --overwrite
kubectl get nodedisk "$ND_NAME" -o jsonpath='{.spec.desired.state} {.spec.desired.owner} {.status.node.phase}{"\n"}'
```

This shows controller intent (`spec.desired`) and agent-observed state (`status.node.phase`) in one place.

### 3) Confirm node-local side effect

```bash
# derive node and disk directly from the selected NodeDisk
NODE_NAME=$(kubectl get nodedisk "$ND_NAME" -o jsonpath='{.spec.nodeName}')
DISK_ID=$(kubectl get nodedisk "$ND_NAME" -o jsonpath='{.spec.diskID}')
echo "node=$NODE_NAME disk=$DISK_ID"

SEEDER_POD=$(kubectl -n node-disks-system get pod -l app.kubernetes.io/name=node-disks-seeder --field-selector spec.nodeName="$NODE_NAME" -o jsonpath='{.items[0].metadata.name}')
kubectl -n node-disks-system exec "$SEEDER_POD" -- ls -1 /host-node-disks-manager/disks | grep -E "^${DISK_ID}\\.img(\\.reserved)?$"
```

You should see both `<disk-id>.img` and `<disk-id>.img.reserved`.

### 4) Verify directly on the node over SSH

```bash
minikube -p node-disks-dev ssh -n "$NODE_NAME"
ls -la /var/lib/node-disks-manager/disks
cat /var/lib/node-disks-manager/disks/${DISK_ID}.img.reserved
```

### 5) Release the disk and verify it was freed

```bash
kubectl label nodedisk "$ND_NAME" localtest.example.com/auto-reserve-
kubectl get nodedisk "$ND_NAME" -o jsonpath='{.spec.desired.state} {.spec.desired.owner} {.status.node.phase}{"\n"}'

# should show only the base image file for this disk after release
kubectl -n node-disks-system exec "$SEEDER_POD" -- ls -1 /host-node-disks-manager/disks | grep -E "^${DISK_ID}\\.img(\\.reserved)?$"

# optional: verify the reserved marker is gone on the node
minikube -p node-disks-dev ssh -n "$NODE_NAME" -- test ! -f /var/lib/node-disks-manager/disks/${DISK_ID}.img.reserved && echo "reserved marker removed"
```

## Cleanup

```bash
make helm-uninstall
make minikube-delete
```

## Local Development

Use these commands when you change Go types, CRD markers, controller logic, or chart wiring.

### 1) List available tasks

```bash
make help
```

### 2) Install local code-generation tooling

```bash
make tools
```

This installs `controller-gen` into `./bin`.

### 3) Regenerate API artifacts after API/marker changes

```bash
make codegen
```

This runs:

- `make generate` to refresh Go deep-copy code.
- `make manifests` to regenerate CRDs in `charts/node-disks-manager/crds/`.

### 4) Run unit tests

```bash
make test
```

### 5) Optional formatting and module cleanup

```bash
make fmt
make tidy
```

### Monitoring (Optional)

```bash
kubectl -n node-disks-system port-forward deploy/node-disks-operator 8080:8080
# run in another terminal
curl -s localhost:8080/metrics | head
```

```bash
AGENT_POD=$(kubectl -n node-disks-system get pod -l app.kubernetes.io/name=node-disks-agent -o jsonpath='{.items[0].metadata.name}')
kubectl -n node-disks-system port-forward pod/$AGENT_POD 2112:2112
# run in another terminal
curl -s localhost:2112/metrics | grep node_disks_agent_
```

## Notes

- Helm is the single deployment path in this repository (`charts/node-disks-manager`).
- `make manifests` clears and regenerates CRDs in `charts/node-disks-manager/crds/`.
- CRDs are not removed automatically by Helm uninstall.
