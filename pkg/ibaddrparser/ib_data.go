// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package ibaddrparser

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// parsed from kvp_pool_0 , sample `NUMPAIRS:1|00155D33FF0B:172.16.1.2`

func ParseIBAddrConfig(content []byte) (map[string]*net.IPNet, error) {
	segments := strings.Split(string(content), "|")
	if len(segments) != 2 {
		return nil, fmt.Errorf("invalid segments")
	}
	summarysegments := strings.Split(segments[0], ":")
	if len(summarysegments) != 2 {
		return nil, fmt.Errorf("invalid summary segment")
	}
	numPairs, err := strconv.Atoi(summarysegments[1])
	if err != nil {
		return nil, err
	}
	if numPairs != len(segments)-1 {
		return nil, fmt.Errorf("number of pairs does not match")
	}

	ipoibConfig := make(map[string]*net.IPNet)

	for _, item := range segments[1:] {
		itemStrs := strings.Split(item, ":")
		if len(itemStrs) != 2 {
			return nil, fmt.Errorf("invalid item segment")
		}
		macAddr := itemStrs[0][0:2] + ":" + itemStrs[0][2:4] + ":" + itemStrs[0][4:6] + ":" + itemStrs[0][6:8] + ":" + itemStrs[0][8:10] + ":" + itemStrs[0][10:12]
		hardwareAddr, err := net.ParseMAC(macAddr)
		if err != nil {
			return nil, err
		}
		ipAddr := net.ParseIP(itemStrs[1])
		if ipAddr == nil {
			return nil, fmt.Errorf("invalid ip address")
		}
		ipoibConfig[strings.ToLower(hardwareAddr.String())] = &net.IPNet{IP: ipAddr, Mask: net.CIDRMask(16, 32)}
	}
	return ipoibConfig, nil
}
