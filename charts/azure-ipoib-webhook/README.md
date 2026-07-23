# azure-ipoib-webhook Helm chart

Deploys the azure-ipoib IPoIB [DRANet](https://github.com/kubernetes-sigs/dranet)
BYODP Profile Provider webhook as a standalone DaemonSet, node-local alongside
the DRANet DaemonSet. It optionally installs the `azure-ipoib-ipam-cni` CNI
plugin binary into the host CNI bin directory.

## Install

```bash
helm install azure-ipoib-webhook ./charts/azure-ipoib-webhook -n kube-system
```

Point the DRANet DaemonSet at the webhook socket:

```
--profile-provider=webhook
--webhook-url=unix:///var/run/dranet/webhook.sock
```

## Installing the CNI binary

CNI binary installation is **disabled by default**. When enabled, an init
container copies the `azure-ipoib-ipam-cni` plugin binary into the default CNI
plugin location (`/opt/cni/bin`) on every node:

```bash
helm install azure-ipoib-webhook ./charts/azure-ipoib-webhook -n kube-system \
  --set installCNIBinary.enabled=true
```

## Values

| Key | Default | Description |
| --- | --- | --- |
| `webhook.image.repository` | `ghcr.io/azure/azure-ipoib-ipam-cni-webhook` | Webhook image repository. |
| `webhook.image.tag` | `latest` | Webhook image tag. |
| `webhook.image.pullPolicy` | `IfNotPresent` | Webhook image pull policy. |
| `webhook.bindAddress` | `unix:///var/run/dranet/webhook.sock` | Address the webhook listens on. |
| `webhook.kvpPath` | `/var/lib/hyperv/.kvp_pool_0` | HyperV KVP pool file (mounted read-only). |
| `webhook.profile` | `""` | Optional DRANet profile gate. Empty accepts all profiles. |
| `webhook.resources` | requests `10m` CPU / `16Mi` mem | Webhook container resources. |
| `installCNIBinary.enabled` | `false` | Install the CNI plugin binary into the host CNI bin directory. |
| `installCNIBinary.image.repository` | `ghcr.io/azure/azure-ipoib-ipam-cni` | CNI installer image repository. |
| `installCNIBinary.image.tag` | `latest` | CNI installer image tag. |
| `installCNIBinary.image.pullPolicy` | `IfNotPresent` | CNI installer image pull policy. |
| `installCNIBinary.cniBinDir` | `/opt/cni/bin` | Host CNI plugin binary directory. |
| `socketDir` | `/var/run/dranet` | Host directory shared with DRANet for the webhook socket. |
| `hostNetwork` | `true` | Run pods on the host network. |
| `tolerations` | schedule on all nodes | Pod tolerations. |
| `nodeSelector` | `{}` | Pod node selector. |
| `affinity` | `{}` | Pod affinity. |
| `imagePullSecrets` | `[]` | Image pull secrets. |
| `podAnnotations` | `{}` | Extra pod annotations. |
