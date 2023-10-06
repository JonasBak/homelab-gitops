package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"go.mozilla.org/sops/v3/decrypt"
	"gopkg.in/yaml.v3"
)

func readConfigFile(path string) Config {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read yaml failed: %v", err)
	}

	config := Config{}
	if err := yaml.Unmarshal(b, &config); err != nil {
		log.Fatalf("unmarshal yaml failed: %v", err)
	}

	return config
}

func readFile(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func runCommand(pwd string, panicOnErr bool, cmdRaw string, args ...string) (string, error) {
	log.WithFields(logrus.Fields{
		"pwd":  pwd,
		"cmd":  cmdRaw,
		"args": args,
	}).Info("running command")
	cmd := exec.Command(cmdRaw, args...)
	cmd.Env = environ
	if pwd != "" {
		cmd.Dir = pwd
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		log.WithFields(logrus.Fields{
			"stdout": stdout.String(),
			"stderr": stderr.String(),
			"cmd":    cmdRaw,
			"args":   args,
		}).Warn(stderr.String())
		if panicOnErr {
			log.Fatal(err)
		} else {
			return "", err
		}
	}
	return stdout.String(), nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Returns a map of running service name -> service hash
func parseRunningServices() (map[string]string, error) {
	services := make(map[string]string)

	type container struct {
		Labels map[string]string
	}

	output, err := runCommand("", false, "podman", "ps", "--format", "json")
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

// This hashes the content of a directory. It doesn't currently support traversing nested directories.
func hashDir(dir string) (string, error) {
	sha := sha256.New()

	err := filepath.Walk(dir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				_, err = sha.Write([]byte(path))
				return err
			}

			_, err = sha.Write([]byte(path))
			if err != nil {
				return err
			}

			content, err := readFile(path)
			if err != nil {
				return err
			}

			_, err = sha.Write([]byte(content))
			return err
		})

	return hex.EncodeToString(sha.Sum(nil)), err
}

func generateContainerFile(manifest map[string][]string, service string, hash string, templateKV map[string]string) string {
	file := ""
	for field, values := range manifest {
		for i := range values {
			value := values[i]
			for k, v := range templateKV {
				value = strings.ReplaceAll(value, fmt.Sprintf("${%s}", k), v)
			}
			file += fmt.Sprintf("%s=%s\n", field, value)
		}
	}

	file = fmt.Sprintf(`
[Install]
WantedBy=default.target

[Container]
%s
Label=gitops-service=%s
Label=gitops-hash=%s
`, file, service, hash)

	return file
}

func readManifest(serviceDir string) (map[string][]string, error) {
	config := make(map[string][]string)

	manifest, err := os.ReadFile(fmt.Sprintf(SERVICE_MANIFEST_FILE, serviceDir))
	if err != nil {
		return config, err
	}
	var manifestSops *[]byte = nil
	sopsFile := fmt.Sprintf(SERVICE_MANIFEST_SOPS_FILE, serviceDir)
	if pathExists(sopsFile) {
		s, err := decrypt.File(sopsFile, "yaml")
		if err != nil {
			return config, err
		}
		manifestSops = &s
	}

	if err := yaml.Unmarshal(manifest, &config); err != nil {
		return config, err
	}
	if manifestSops != nil {
		sopsConfig := make(map[string][]string)
		if err := yaml.Unmarshal(*manifestSops, &sopsConfig); err != nil {
			return config, err
		}
		for k, v := range sopsConfig {
			if values, ok := config[k]; ok {
				config[k] = append(values, v...)
			} else {
				config[k] = v
			}
		}
	}

	return config, nil
}