package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func generateContainerFile(manifest Manifest, service string, hash string, templateKV map[string]string) string {
	containerFields := buildFields(manifest.Container, templateKV)
	unitFields := buildFields(manifest.Unit, templateKV)
	serviceFields := buildFields(manifest.Service, templateKV)

	file := fmt.Sprintf(`
[Install]
WantedBy=default.target

[Container]
%s
Label=gitops-service=%s
Label=gitops-hash=%s

[Unit]
%s

[Service]
%s
`, containerFields, service, hash, unitFields, serviceFields)

	return file
}

func generateNetworkFile(kvs map[string][]string, networkName string) string {
	networkFields := buildFields(kvs, make(map[string]string))

	sha := sha256.New()
	_, _ = sha.Write([]byte(networkFields))
	hash := hex.EncodeToString(sha.Sum(nil))

	file := fmt.Sprintf(`
[Network]
%s
Label=gitops-network=%s
Label=gitops-hash=%s
`, networkFields, networkName, hash)

	return file
}

func generateVolumeFile(kvs map[string][]string, volumeName string) string {
	volumeFields := buildFields(kvs, make(map[string]string))

	sha := sha256.New()
	_, _ = sha.Write([]byte(volumeFields))
	hash := hex.EncodeToString(sha.Sum(nil))

	file := fmt.Sprintf(`
[Volume]
%s
Label=gitops-volume=%s
Label=gitops-hash=%s
`, volumeFields, volumeName, hash)

	return file
}
