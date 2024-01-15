// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package main

import (
	"fmt"
	"os"
)

func main() {
	content, err := os.ReadFile("azure-ipoib-ipam-cni")
	if err != nil {
		fmt.Printf("failed to find file: %s", err.Error())
		os.Exit(1)
	}
	if err := os.WriteFile("/opt/cni/bin/azure-ipoib-ipam-cni", content, 0755); err != nil {
		fmt.Printf("failed to write file: %s", err.Error())
		os.Exit(1)
	}
}
