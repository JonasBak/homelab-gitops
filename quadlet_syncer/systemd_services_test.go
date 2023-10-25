package quadlet_syncer

import (
	"testing"

	"github.com/JonasBak/homelab_gitops/utils"
)

func assert(t *testing.T, v bool, reason string) {
	if !v {
		t.Errorf(`Error: %s`, reason)
	}
}

func assertEq[K comparable](t *testing.T, got, want K, reason string) {
	if got != want {
		t.Errorf(`Error: %s.
    Got:
    %+v
    Expected:
    %+v`, reason, got, want)
	}
}

func TestGenerateContainerFile(t *testing.T) {
	manifest := utils.Manifest{
		Container: map[string][]string{
			"Image": {
				"docker.io/library/caddy:2.5.2-alpine",
			},
			"Label": {
				"TestLabel1=${TEST}",
				"TestLabel2=1${TEST}2",
			},
			"Volume": {
				"${DATA_DIR}/caddy:/data:z",
				"${CONFIG_DIR}/Caddyfile:/etc/caddy/Caddyfile:z",
			},
		},
		Unit: map[string][]string{
			"Requires": {
				"prometheus.service",
			},
		},
		Service: map[string][]string{
			"ExecStartPre": {
				"/usr/bin/mkdir -p ${DATA_DIR}/caddy",
			},
		},
	}

	templateKV := map[string]string{
		"TEST":       "abc",
		"DATA_DIR":   "/mnt",
		"CONFIG_DIR": "/config",
	}

	manifestFile := generateContainerFile(manifest, "test-service", "test-hash", templateKV)

	assertEq(t, manifestFile, `
[Install]
WantedBy=default.target

[Container]
Image=docker.io/library/caddy:2.5.2-alpine
Label=TestLabel1=abc
Label=TestLabel2=1abc2
Volume=/mnt/caddy:/data:z
Volume=/config/Caddyfile:/etc/caddy/Caddyfile:z

Label=gitops-service=test-service
Label=gitops-hash=test-hash

[Unit]
Requires=prometheus.service


[Service]
ExecStartPre=/usr/bin/mkdir -p /mnt/caddy

`, "generated container file doesn't match expected output")

}

func TestGenerateNetworkFile(t *testing.T) {
	manifest := map[string][]string{
		"Subnet": {"172.16.0.0/24"},
	}

	manifestFile := generateNetworkFile(manifest, "test-network")

	assertEq(t, manifestFile, `
[Network]
Subnet=172.16.0.0/24

Label=gitops-network=test-network
Label=gitops-hash=13d3693b192cd09e7320b7dbeea8d87c5df4b3cbcc3beec060bc931d327ef179
`, "generated network file doesn't match expected output")

}

func TestGenerateVolumeFile(t *testing.T) {
	manifest := map[string][]string{
		"User":  {"root"},
		"Group": {"root"},
	}

	manifestFile := generateVolumeFile(manifest, "test-volume")

	assertEq(t, manifestFile, `
[Volume]
Group=root
User=root

Label=gitops-volume=test-volume
Label=gitops-hash=8dca47a2e34d260922c4a6bd07890238091c260d3abb6dae29c4e675a4e8687c
`, "generated network file doesn't match expected output")

}
