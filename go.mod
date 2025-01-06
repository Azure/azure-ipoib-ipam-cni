module github.com/Azure/azure-ipoib-ipam-cni

go 1.21.4
toolchain go1.23.4

require (
	github.com/Azure/hyperkv v0.0.1
	github.com/containernetworking/cni v1.2.3
	github.com/containernetworking/plugins v1.6.2
	github.com/vishvananda/netlink v1.3.0
)

require (
	github.com/vishvananda/netns v0.0.4 // indirect
	golang.org/x/sys v0.27.0 // indirect
)
