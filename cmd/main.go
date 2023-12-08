// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	type100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/vishvananda/netlink"

	"github.com/Azure/azure-ipoib-ipam-cni/pkg/ibaddrparser"
)

const (
	IPAMPluginName = "azure-ipoib-ipam-cni"
	KVPStorePath   = "/var/lib/hyperv/.kvp_pool_0"
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString(IPAMPluginName))
}

func cmdAdd(args *skel.CmdArgs) error {
	config := &types.NetConf{}
	err := json.Unmarshal(args.StdinData, config)
	if err != nil {
		log.Print("failed to load netconf", err)
		return err
	}

	content, err := os.ReadFile(KVPStorePath)
	if err != nil {
		fmt.Printf("Error reading file %s: %v", KVPStorePath, err)
	}
	result := &type100.Result{CNIVersion: type100.ImplementedSpecVersion}
	err = ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		nic, err := netlink.LinkByName(args.IfName)
		if err != nil {
			log.Print("failed to get link by name", err)
			return err
		}
		ipAddr, err := ibaddrparser.GetIBAddr(content, nic.Attrs().HardwareAddr.String())
		if err != nil {
			return err
		}
		result.IPs = append(result.IPs, &type100.IPConfig{
			Address: *ipAddr,
		})
		return nil
	})
	if err != nil {
		log.Print("failed to get ip address", err)
		return err
	}
	// outputCmdArgs(args)
	return types.PrintResult(result, config.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	// get cni config
	config := &types.NetConf{}
	err := json.Unmarshal(args.StdinData, config)
	if err != nil {
		log.Print("failed to load netconf", err)
		return err
	}

	result := &type100.Result{CNIVersion: type100.ImplementedSpecVersion}
	return types.PrintResult(result, config.CNIVersion)
}

func cmdCheck(args *skel.CmdArgs) error {
	// get cni config
	config := &types.NetConf{}
	err := json.Unmarshal(args.StdinData, config)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(KVPStorePath)
	if err != nil {
		log.Printf("Error reading file %s: %v", KVPStorePath, err)
		return err
	}
	result := &type100.Result{CNIVersion: type100.ImplementedSpecVersion}
	if err = ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		nic, err := netlink.LinkByName(args.IfName)
		if err != nil {
			return err
		}
		ipAddr, err := ibaddrparser.GetIBAddr(content, nic.Attrs().HardwareAddr.String())
		if err != nil {
			return err
		}
		result.IPs = append(result.IPs, &type100.IPConfig{
			Address: *ipAddr,
		})
		return nil
	}); err != nil {
		return err
	}
	return types.PrintResult(result, config.CNIVersion)
}
