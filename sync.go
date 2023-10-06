package main

import (
	"fmt"
	"os"
	"strings"
)

// Relative to hostGitopsDir
var CONFIG_FILE_PATH = "%s/config.yml"
// Relative to home, with service name
var CONTAINER_UNIT_FILE_PATH = "%s/.config/containers/systemd/gitops-%s.container"
// With service name
var SERVICE_UNIT_NAME = "gitops-%s.service"
// Relative to service dir
var SERVICE_MANIFEST_FILE = "%s/manifest.yml"
// Relative to service dir
var SERVICE_MANIFEST_SOPS_FILE = "%s/manifest.sops.yml"

// Set up ssh env and load key in $SSH_KEY if provided
func setupSSHEnv() {
	sshAgent, _ := runCommand("", true, "ssh-agent")
	lines := strings.Split(sshAgent, "\n")
	sshAuthSock := strings.Split(lines[0], ";")[0]
	sshAgentPid := strings.Split(lines[1], ";")[0]
	environ = append(environ,
		sshAuthSock,
		sshAgentPid,
	)
	if sshKey := os.Getenv("SSH_KEY"); sshKey != "" {
		runCommand("", true, "ssh-add", sshKey)
	}
}

// Fetches and cleans up the repo, and checks that the newest commit is signed
func fetch(gitopsRepo string, gitopsRepoDir string) (string, string) {
	log.Info("setting up ssh env")
	setupSSHEnv()

	log.Info("syncing git repo")
	if !pathExists(gitopsRepoDir) {
		runCommand("", true, "git", "clone", gitopsRepo, gitopsRepoDir)
	}

	runCommand(gitopsRepoDir, true, "git", "fetch")

	runCommand(gitopsRepoDir, true, "git", "clean", "--force")

	commitToDeploy := "origin/HEAD"

	runCommand(gitopsRepoDir, true, "git", "reset", "--hard", commitToDeploy)

	head, _ := runCommand(gitopsRepoDir, true, "git", "rev-parse", "HEAD")

	runCommand(gitopsRepoDir, true, "git", "verify-commit", "-v", "HEAD")

	log.Infof("deploying %s", head)

	hostname := os.Getenv("HOSTNAME")

	hostGitopsDir := fmt.Sprintf("%s/gitops/%s", gitopsRepoDir, hostname)

	return head, hostGitopsDir
}

// Start/restart configured services
func servicesUp(hostGitopsDir string) {
	config := readConfigFile(fmt.Sprintf(CONFIG_FILE_PATH, hostGitopsDir))

	if config.Pre != nil {
		log.Info("running pre script")
		_, err := runCommand(hostGitopsDir, false, "bash", "-c", "--", config.Pre.Script)
		if err != nil {
			log.WithField("error", err.Error()).Fatal("pre script failed")
		}
	}

	runningServices, err := parseRunningServices()
	if err != nil {
		log.WithField("error", err.Error()).Fatal("failed to get running containers")
	}

	serviceOk := make(map[string]struct{})
	serviceFailed := make(map[string]struct{})

	log.Info("starting services")
	for service := range config.Services {
		log := log.WithField("service", service)

		log.Info("updating service")

		serviceFailed[service] = struct{}{}

		serviceDir := fmt.Sprintf("%s/%s", hostGitopsDir, service)

		hash, err := hashDir(serviceDir + "/")
		if err != nil {
			log.WithField("error", err.Error()).Error("failed to hash dir")
			continue
		}
		s := config.Services[service]
		s.Hash = hash
		config.Services[service] = s

		log = log.WithField("hash", hash)

		manifest, err := readManifest(serviceDir)
		if err != nil {
			log.WithField("error", err.Error()).Error("failed to read manifest for service")
			continue
		}

		templateValues := make(map[string]string)
		templateValues["HOST_DIR"] = hostGitopsDir
		templateValues["SERVICE_DIR"] = serviceDir
		templateValues["SERVICE"] = service
		templateValues["HASH"] = hash

		containerFile := generateContainerFile(manifest, service, hash, templateValues)
		err = os.WriteFile(fmt.Sprintf(CONTAINER_UNIT_FILE_PATH, os.Getenv("HOME"), service), []byte(containerFile), 0640)
		if err != nil {
			log.WithField("error", err.Error()).Errorf("failed to write container file")
			continue
		}

		_, err = runCommand(hostGitopsDir, false, "systemctl", "--user", "daemon-reload")
		if err != nil {
			log.WithField("error", err.Error()).Errorf("failed to reload service")
			continue
		}

		delete(serviceFailed, service)
		serviceOk[service] = struct{}{}

		log.Info("container file created")
	}

	for service := range serviceOk {
		newHash := config.Services[service].Hash
		oldHash := runningServices[service]
		log := log.WithField("service", service).WithField("oldHash", oldHash).WithField("newHash", newHash)
		log.Info("evaluating restart")
		if newHash != oldHash {
			log.Info("service changed, restarting")
			_, err := runCommand(hostGitopsDir, false, "systemctl", "--user", "restart", fmt.Sprintf(SERVICE_UNIT_NAME, service))
			if err != nil {
				log.WithField("error", err.Error()).Errorf("failed to start service")
				serviceFailed[service] = struct{}{}
			}
		}
	}

	if len(serviceFailed) > 0 {
		log.Fatal("failed to start some services")
	}

	if config.Post != nil {
		log.Info("running post script")
		_, err := runCommand(hostGitopsDir, false, "bash", "-c", "--", config.Post.Script)
		if err != nil {
			log.WithField("error", err.Error()).Fatal("post script failed")
		}
	}

	log.Info("services up ok")
}

// Stop orphaned services
func orphansDown(hostGitopsDir string) {
	config := readConfigFile(fmt.Sprintf(CONFIG_FILE_PATH, hostGitopsDir))

	runningServices, err := parseRunningServices()
	if err != nil {
		log.WithField("error", err.Error()).Fatal("failed to get running containers")
	}

	serviceStopped := make(map[string]struct{})
	serviceFailed := make(map[string]struct{})

	for service := range runningServices {
		if _, ok := config.Services[service]; !ok {
      // TODO remove old .container files
			_, err := runCommand(hostGitopsDir, false, "systemctl", "--user", "stop", fmt.Sprintf(SERVICE_UNIT_NAME, service))
			if err != nil {
				log.WithField("error", err.Error()).Errorf("failed to stop orphaned service")
				serviceFailed[service] = struct{}{}
				continue
			}
			log.WithField("service", service).Info("stopped orphaned service")
			serviceStopped[service] = struct{}{}
		}
	}

	if len(serviceFailed) > 0 {
		log.Fatal("failed to stop some orphaned services")
	}

	log.Info("orphaned services cleaned up")
}
