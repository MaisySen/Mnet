package control

import (
	"path/filepath"
	"testing"
)

func TestMultiesDeviceListListenerConfiguration(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []string{"", "off"} {
		srv, err := NewMultiesDeviceListServer(store, MultiesDeviceListConfig{Enabled: enabled})
		if err != nil || srv != nil {
			t.Fatal("the listener must not exist without opt-in")
		}
	}
	const origin = "https://devices.mnet.test"
	for name, config := range map[string]MultiesDeviceListConfig{
		"production":        {Enabled: "on", Deployment: "production", WorkspaceID: "org_fixture", Origin: origin, Listen: "127.0.0.1:0"},
		"missing workspace": {Enabled: "on", Deployment: "staging", Origin: origin, Listen: "127.0.0.1:0"},
		"public listener":   {Enabled: "on", Deployment: "staging", WorkspaceID: "org_fixture", Origin: origin, Listen: "0.0.0.0:8443"},
		"insecure origin":   {Enabled: "on", Deployment: "staging", WorkspaceID: "org_fixture", Origin: "http://devices.mnet.test", Listen: "127.0.0.1:0"},
		"missing cert":      {Enabled: "on", Deployment: "staging", WorkspaceID: "org_fixture", Origin: origin, Listen: "127.0.0.1:0"},
	} {
		t.Run(name, func(t *testing.T) {
			srv, err := NewMultiesDeviceListServer(store, config)
			if err == nil || srv != nil {
				t.Fatal("invalid configuration must fail closed")
			}
		})
	}
}
