// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package ibaddrparser

import (
	"net"
	"reflect"
	"testing"

	"github.com/Azure/hyperkv"
)

func TestParseIPOIBConfig(t *testing.T) {
	type args struct {
		content []byte
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]*net.IPNet
		wantErr bool
	}{
		{
			name: "valid",
			args: args{
				content: []byte("NUMPAIRS:1|00155D33FF0B:172.16.1.2"),
			},
			want: map[string]*net.IPNet{
				"00:15:5d:33:ff:0b": {IP: net.IPv4(172, 16, 1, 2), Mask: net.CIDRMask(16, 32)},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hyperkv.Parse(tt.args.content)
			got, err := ParseIBAddrConfig(tt.args.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseIPOIBConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseIPOIBConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
