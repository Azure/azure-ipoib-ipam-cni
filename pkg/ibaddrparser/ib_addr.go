// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package ibaddrparser

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/Azure/hyperkv"
)

const (
	IPoIBDataKey = "IPoIB_Data"
)

func GetIBAddr(rawContent []byte, macAddr string) (*net.IPNet, error) {
	log.Print("GetIBAddr", macAddr)
	list := hyperkv.Parse(rawContent)
	var ipoibData string
	for _, item := range list {
		if strings.EqualFold(item.Key, IPoIBDataKey) {
			ipoibData = item.Value
		}
	}
	if ipoibData == "" {
		return nil, fmt.Errorf("IPoIB_Data not found in config file")
	}

	configMap, err := ParseIBAddrConfig([]byte(ipoibData))
	if err != nil {
		return nil, err
	}
	if configMap == nil {
		return nil, fmt.Errorf("invalid config map")
	}
	contractedMacAddr := ConvertHardwareAddr(macAddr)
	if ipAddr, ok := configMap[strings.ToLower(contractedMacAddr)]; ok {
		return ipAddr, nil
	} else {
		content, err := json.Marshal(configMap)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get ip address %s,contracted %s, got %s", strings.ToLower(macAddr), ConvertHardwareAddr(macAddr), (content))
	}
}
func ConvertHardwareAddr(s string) string {
	elements := strings.Split(s, ":")
	length := len(elements)
	if length < 20 {
		return s
	}
	return strings.Join(append(elements[length-8:length-5], elements[length-3:]...), ":")
}
