// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

// Package webhook implements a DRANet "Bring Your Own DRANet Provider" (BYODP)
// Profile Provider webhook server. It resolves the IP address for an
// InfiniBand (IPoIB) device from the local HyperV KVP store, reusing the
// ibaddrparser logic, and returns it to DRANet as a NetworkConfig.
//
// The JSON wire types below mirror DRANet's public webhook contract
// (sigs.k8s.io/dranet/pkg/cloudprovider/webhook and .../pkg/apis). They are
// re-declared locally, rather than importing DRANet, to keep this module small
// and free of the Kubernetes dependency tree. The JSON field tags MUST stay in
// sync with DRANet's types.
package webhook

// HTTP endpoint paths exposed by a webhook provider, matching DRANet's
// webhook.Path* constants.
const (
	PathHealth               = "/health"
	PathGetDeviceAttributes  = "/GetDeviceAttributes"
	PathGetDeviceConfig      = "/GetDeviceConfig"
	PathGetProfileConfig     = "/GetProfileConfig"
	PathReleaseProfileConfig = "/ReleaseProfileConfig"
)

// Capabilities represents the functionality supported by the webhook server.
// Mirrors DRANet's webhook.Capabilities.
type Capabilities struct {
	CloudProvider   bool `json:"cloudProvider"`
	ProfileProvider bool `json:"profileProvider"`
}

// DeviceIdentifiers contains the locally discovered hardware identifiers that
// DRANet passes to the webhook. Mirrors DRANet's cloudprovider.DeviceIdentifiers.
type DeviceIdentifiers struct {
	MAC        string `json:"mac_address,omitempty"`
	PCIAddress string `json:"pci_address,omitempty"`
	Name       string `json:"name"`
}

// ProfileRequest is the payload DRANet POSTs to the profile endpoints.
// Mirrors DRANet's webhook.ProfileRequest.
type ProfileRequest struct {
	Device   DeviceIdentifiers `json:"device"`
	ClaimUID string            `json:"claim_uid"`
	Config   *NetworkConfig    `json:"config,omitempty"`
}

// NetworkConfig is the subset of DRANet's apis.NetworkConfig that this webhook
// consumes (Profile) and produces (Interface.Addresses). Additional fields from
// the incoming request are ignored on decode.
type NetworkConfig struct {
	// Profile references a pre-configured set of parameters resolved by the
	// provider plugin. Used here to optionally gate which requests this
	// webhook answers.
	Profile string `json:"profile,omitempty"`
	// Interface holds the resolved interface configuration.
	Interface InterfaceConfig `json:"interface"`
}

// InterfaceConfig mirrors the relevant fields of DRANet's apis.InterfaceConfig.
type InterfaceConfig struct {
	// Name is the desired logical name of the interface inside the Pod.
	Name string `json:"name,omitempty"`
	// Addresses is a list of IP addresses in CIDR format assigned to the
	// interface.
	Addresses []string `json:"addresses,omitempty"`
}
