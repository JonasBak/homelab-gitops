package quadlet_syncer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/JonasBak/homelab_gitops/utils"
	log "github.com/sirupsen/logrus"
)

// Relative to home, with service name
var CONTAINER_UNIT_FILE_PATH = "%s/.config/containers/systemd/gitops-%s.container"

// With service name
var SERVICE_UNIT_NAME = "gitops-%s.service"

type QuadletSyncer struct {
	HostGitopsDir string
	Environ       []string
}

var _ utils.ServiceSyncer = &QuadletSyncer{}

func (s *QuadletSyncer) GetConfig() utils.Config {
	return utils.ReadConfigFile(fmt.Sprintf(utils.CONFIG_FILE_PATH, s.HostGitopsDir))
}

func (s *QuadletSyncer) GetRunningServices() (map[string]string, error) {
	return parseRunningServices()
}

func (s *QuadletSyncer) CreateService(service string, serviceConfig utils.Service) (string, error) {
	return writeContainerFile(s.HostGitopsDir, service, serviceConfig)
}

func (s *QuadletSyncer) RestartService(service string) error {
	return restartService(service)
}

func (s *QuadletSyncer) StopService(service string) error {
	return stopService(service)
}

func (s *QuadletSyncer) RunPre(cmd string) error {
	_, err := utils.RunCommand(s.HostGitopsDir, []string{}, false, "bash", "-c", "--", cmd)
	return err
}

func (s *QuadletSyncer) RunPost(cmd string) error {
	return s.RunPre(cmd)
}

// Returns a map of running service name -> service hash
func parseRunningServices() (map[string]string, error) {
	services := make(map[string]string)

	type container struct {
		Labels map[string]string
	}

	output, err := utils.RunCommand("", []string{}, false, "podman", "ps", "--format", "json")
	if err != nil {
		return nil, err
	}

	containers := []container{}

	json.Unmarshal([]byte(output), &containers)

	for i := range containers {
		service := containers[i].Labels["gitops-service"]
		hash := containers[i].Labels["gitops-hash"]

		if service != "" {
			services[service] = hash
		}
	}

	return services, nil
}

func writeContainerFile(hostGitopsDir string, name string, config utils.Service) (string, error) {
	log := log.WithField("service", name)

	log.Info("updating service")

	serviceDir := fmt.Sprintf("%s/%s", hostGitopsDir, name)

	hash, err := utils.HashDir(serviceDir + "/")
	if err != nil {
		return "", err
	}

	log = log.WithField("hash", hash)

	manifest, err := utils.ReadManifest(serviceDir)
	if err != nil {
		return "", err
	}

	templateValues := make(map[string]string)
	templateValues["HOST_DIR"] = hostGitopsDir
	templateValues["SERVICE_DIR"] = serviceDir
	templateValues["SERVICE"] = name
	templateValues["HASH"] = hash

	containerFile := generateContainerFile(manifest, name, hash, templateValues)
	err = os.WriteFile(fmt.Sprintf(CONTAINER_UNIT_FILE_PATH, os.Getenv("HOME"), name), []byte(containerFile), 0640)
	if err != nil {
		return "", err
	}

	_, err = utils.RunCommand(hostGitopsDir, []string{}, false, "systemctl", "--user", "daemon-reload")
	if err != nil {
		return "", err
	}

	return hash, nil
}

func restartService(service string) error {
	_, err := utils.RunCommand("", []string{}, false, "systemctl", "--user", "restart", fmt.Sprintf(SERVICE_UNIT_NAME, service))
	return err
}

func stopService(service string) error {
	_, err := utils.RunCommand("", []string{}, false, "systemctl", "--user", "stop", fmt.Sprintf(SERVICE_UNIT_NAME, service))
	if err != nil {
		return err
	}
	_ = os.Remove(fmt.Sprintf(CONTAINER_UNIT_FILE_PATH, os.Getenv("HOME"), service))
	return nil
}

func generateContainerFile(manifest utils.Manifest, service string, hash string, templateKV map[string]string) string {
	containerFields := utils.BuildFields(manifest.Container, templateKV)
	unitFields := utils.BuildFields(manifest.Unit, templateKV)
	serviceFields := utils.BuildFields(manifest.Service, templateKV)

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
	networkFields := utils.BuildFields(kvs, make(map[string]string))

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
	volumeFields := utils.BuildFields(kvs, make(map[string]string))

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
