package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getsops/sops/v3/decrypt"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type ServiceSyncer interface {
	GetConfig() Config
	GetRunningServices() (map[string]string, error)
	CreateService(service string, serviceConfig Service) (string, error)
	RestartService(service string) error
	StopService(service string) error

	RunPre(cmd string) error
	RunPost(cmd string) error
}

// Relative to hostGitopsDir
var CONFIG_FILE_PATH = "%s/config.yml"

// Relative to service dir
var SERVICE_MANIFEST_FILE = "%s/manifest.yml"

// Relative to service dir
var SERVICE_MANIFEST_SOPS_FILE = "%s/manifest.sops.yml"

type Manifest struct {
	Container map[string][]string `yaml:"Container"`
	Unit      map[string][]string `yaml:"Unit"`
	Service   map[string][]string `yaml:"Service"`
}

type PrePostScript struct {
	Script string `yaml:"script"`
}

type Service struct {
	Hash string
}

type Config struct {
	Pre  *PrePostScript `yaml:"pre"`
	Post *PrePostScript `yaml:"post"`

	Networks map[string]map[string][]string `yaml:"networks"`
	Volumes  map[string]map[string][]string `yaml:"volumes"`
	Services map[string]Service             `yaml:"services"`
}

func ReadConfigFile(path string) Config {
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

func ReadFile(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func RunCommand(pwd string, env []string, panicOnErr bool, cmdRaw string, args ...string) (string, error) {
	log.WithFields(log.Fields{
		"pwd":  pwd,
		"cmd":  cmdRaw,
		"args": args,
	}).Info("running command")
	cmd := exec.Command(cmdRaw, args...)
	cmd.Env = env
	if pwd != "" {
		cmd.Dir = pwd
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		log.WithFields(log.Fields{
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

func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func BuildFields(kvs map[string][]string, templateKV map[string]string) string {
	fields := ""

	keys := make([]string, 0, len(kvs))

	for k := range kvs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, field := range keys {
		values := kvs[field]
		for i := range values {
			value := values[i]
			for k, v := range templateKV {
				value = strings.ReplaceAll(value, fmt.Sprintf("${%s}", k), v)
			}
			fields += fmt.Sprintf("%s=%s\n", field, value)
		}
	}
	return fields
}

func ReadManifest(serviceDir string) (Manifest, error) {
	config := Manifest{
		make(map[string][]string),
		make(map[string][]string),
		make(map[string][]string),
	}

	manifest, err := os.ReadFile(fmt.Sprintf(SERVICE_MANIFEST_FILE, serviceDir))
	if err != nil {
		return config, err
	}
	var manifestSops *[]byte = nil
	sopsFile := fmt.Sprintf(SERVICE_MANIFEST_SOPS_FILE, serviceDir)
	if PathExists(sopsFile) {
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
		sopsConfig := Manifest{
			make(map[string][]string),
			make(map[string][]string),
			make(map[string][]string),
		}
		if err := yaml.Unmarshal(*manifestSops, &sopsConfig); err != nil {
			return config, err
		}
		for k, v := range sopsConfig.Container {
			if values, ok := config.Container[k]; ok {
				config.Container[k] = append(values, v...)
			} else {
				config.Container[k] = v
			}
		}
	}

	return config, nil
}

// This hashes the content of a directory. It doesn't currently support traversing nested directories.
func HashDir(dir string) (string, error) {
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

			content, err := ReadFile(path)
			if err != nil {
				return err
			}

			_, err = sha.Write([]byte(content))
			return err
		})

	return hex.EncodeToString(sha.Sum(nil)), err
}
