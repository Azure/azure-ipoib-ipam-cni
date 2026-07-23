# azure-ipoib-ipam-cni

## Overview
azure-ipoib-ipam-cni is intended to be used as an IPAM CNI plugin for Kubernetes. It will retrieve an IP address from a HyperV kv pair and assign it to the IB NIC. It is designed to be used in conjunction with the [host-device](https://www.cni.dev/plugins/current/main/host-device/) and [network operator](https://github.com/Mellanox/network-operator).

## Requirements

It only works on [RDMA-capable instances](https://learn.microsoft.com/en-us/azure/virtual-machines/sizes-hpc#rdma-capable-instances)

## Usage

Install ofed drivers
```bash
# Apply required manifests
kubectl get namespace network-operator 2>/dev/null || kubectl create namespace network-operator

# Install node feature discovery
helm upgrade -i --wait \
  -n network-operator node-feature-discovery node-feature-discovery \
  --repo https://kubernetes-sigs.github.io/node-feature-discovery/charts \
  --set-json master.nodeSelector='{"kubernetes.azure.com/mode": "system"}' \
  --set-json worker.config.sources.pci.deviceClassWhitelist='["02","03","0200","0207"]' \
  --set-json worker.config.sources.pci.deviceLabelFields='["vendor"]'

# Install the network-operator
helm upgrade -i --wait \
  -n network-operator network-operator network-operator \
  --repo https://helm.ngc.nvidia.com/nvidia \
  --set deployCR=true \
  --set nfd.enabled=false \
  --set ofedDriver.deploy=true \
  --set secondaryNetwork.deploy=false \
  --set rdmaSharedDevicePlugin.deploy=true \
  --set sriovDevicePlugin.deploy=true \
  --set-json sriovDevicePlugin.resources='[{"name":"mlnxnics","linkTypes": ["infiniband"], "vendors":["15b3"]}]'
```
Download cni binaries and put it to /opt/cni/bin

Create HostDeviceNetwork with following content:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: mellanox.com/v1alpha1
kind: HostDeviceNetwork
metadata:
   name: hostdev-net
spec:
  networkNamespace: "default"
  resourceName: "mlnxnics"
  ipam: |
    {
      "type": "azure-ipoib-ipam-cni"
    }
EOF
```

And reference HostDeviceNetwork in manifest:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: nicworkspace
  name: nicworkspace
spec:
  progressDeadlineSeconds: 600
  replicas: 0
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: nicworkspace
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: hostdev-net
      creationTimestamp: null
      labels:
        app: nicworkspace
    spec:
      containers:
      - command:
        - sleep
        - infinity
        image: nvcr.io/nvidia/nvhpc:23.11-devel-cuda_multi-ubuntu22.04
        imagePullPolicy: IfNotPresent
        name: nvhpc
        resources:
          limits:
            nvidia.com/mlnxnics: "1"
          requests:
            nvidia.com/mlnxnics: "1"
      dnsPolicy: ClusterFirst
      restartPolicy: Always
```

ib0 interface should be created with IP address from HyperV kv pair.

```sh
root@nicworkspace-594fc84669-zlqn6:/# ip a
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host
       valid_lft forever preferred_lft forever
2: eth0@if271: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default
    link/ether e6:ba:a8:77:21:d2 brd ff:ff:ff:ff:ff:ff link-netnsid 0
    inet 10.244.0.24/24 brd 10.244.0.255 scope global eth0
       valid_lft forever preferred_lft forever
    inet6 fe80::e4ba:a8ff:fe77:21d2/64 scope link
       valid_lft forever preferred_lft forever
23: net1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 2044 qdisc mq state UP group default qlen 256
    link/infiniband 00:00:01:49:fe:80:00:00:00:00:00:00:00:15:5d:ff:fd:33:ff:0b brd 00:ff:ff:ff:ff:12:40:1b:80:01:00:00:00:00:00:00:ff:ff:ff:ff
    altname ibP257p0s0
    altname ibP257s55157
    inet 172.16.1.2/16 brd 172.16.255.255 scope global net1
       valid_lft forever preferred_lft forever
    inet6 fe80::215:5dff:fd33:ff0b/64 scope link
       valid_lft forever preferred_lft forever
```

## Usage with DRANet (Profile Provider webhook)

[DRANet](https://github.com/kubernetes-sigs/dranet) is a Kubernetes network
driver built on Dynamic Resource Allocation (DRA). It attaches devices to Pods
via NRI instead of the CNI `host-device` chain, so the CNI IPAM plugin above is
not invoked. To supply IPoIB addresses from the HyperV KVP store in a DRANet
cluster, this repository also ships a DRANet
[Bring Your Own DRANet Provider (BYODP)](https://dranet.sigs.k8s.io/docs/contributing/webhook-providers/)
**Profile Provider** webhook (`cmd/webhook`).

The webhook reuses the same MAC-to-IP resolution logic as the CNI plugin
(`pkg/ibaddrparser`). On `GetProfileConfig`, DRANet passes the device's MAC
address; the webhook reads `/var/lib/hyperv/.kvp_pool_0`, resolves the IPoIB
address, and returns it to DRANet as an interface address. Because the KVP store
is a read-only mapping, `ReleaseProfileConfig` is a no-op and the webhook is
naturally idempotent.

### Running the webhook

The webhook is meant to run node-local alongside the DRANet DaemonSet (as a
sidecar or a separate DaemonSet), with the HyperV KVP store mounted read-only
and, for the Unix-socket transport, a shared socket directory.

```
webhook \
  --bind-address=unix:///var/run/dranet/webhook.sock \
  --kvp-path=/var/lib/hyperv/.kvp_pool_0
```

Ready-to-apply manifests live in [`deploy/`](deploy):

* [`deploy/webhook-daemonset.yaml`](deploy/webhook-daemonset.yaml) runs the
  webhook as its own DaemonSet next to an existing DRANet DaemonSet. Both share
  the `/var/run/dranet` host directory so DRANet can dial the webhook's Unix
  socket.

  ```bash
  kubectl apply -f deploy/webhook-daemonset.yaml
  ```

* [`deploy/dranet-with-webhook-sidecar.yaml`](deploy/dranet-with-webhook-sidecar.yaml)
  is an example DRANet DaemonSet with the webhook running as a sidecar container
  in the same Pod, sharing the socket via an `emptyDir` volume and already
  wired up with the `--profile-provider`/`--webhook-url` flags below.

  ```bash
  kubectl apply -f deploy/dranet-with-webhook-sidecar.yaml
  ```

Both manifests use the `ghcr.io/azure/azure-ipoib-ipam-cni-webhook` image built
from [`Dockerfile.webhook`](Dockerfile.webhook); adjust the image tag to match
your build.

Flags:

* `--bind-address`: a TCP address (e.g. `:8080`) or a Unix socket path prefixed
  with `unix://` (e.g. `unix:///var/run/dranet/webhook.sock`). DRANet supports
  both transports via its `--webhook-url` flag.
* `--kvp-path`: path to the HyperV KVP pool file (default
  `/var/lib/hyperv/.kvp_pool_0`).
* `--profile`: when set, only answer requests whose DRANet
  `NetworkConfig.profile` matches this value; otherwise all profiles are
  accepted.

### Enabling the webhook in DRANet

Start the DRANet DaemonSet with the profile provider pointed at the webhook:

```
--profile-provider=webhook
--webhook-url=unix:///var/run/dranet/webhook.sock
```

The native cloud provider can remain auto-detected (or `--cloud-provider-hint=AZURE`)
so DRANet still publishes Azure topology attributes for scheduling while this
webhook supplies the IPoIB IPAM.
