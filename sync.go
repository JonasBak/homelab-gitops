package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JonasBak/homelab_gitops/utils"
)

// Set up ssh env and load key in $SSH_KEY if provided
func setupSSHEnv() {
	sshAgent, _ := utils.RunCommand("", environ, true, "ssh-agent")
	lines := strings.Split(sshAgent, "\n")
	sshAuthSock := strings.Split(lines[0], ";")[0]
	sshAgentPid := strings.Split(lines[1], ";")[0]
	environ = append(environ,
		sshAuthSock,
		sshAgentPid,
	)
	if sshKey := os.Getenv("SSH_KEY"); sshKey != "" {
		utils.RunCommand("", environ, true, "ssh-add", sshKey)
	}
}

// Fetches and cleans up the repo, and checks that the newest commit is signed
func fetch(gitopsRepo string, gitopsRepoDir string) (string, string) {
	log.Info("setting up ssh env")
	setupSSHEnv()

	log.Info("syncing git repo")
	if !utils.PathExists(gitopsRepoDir) {
		utils.RunCommand("", environ, true, "git", "clone", gitopsRepo, gitopsRepoDir)
	}

	utils.RunCommand(gitopsRepoDir, environ, true, "git", "fetch")

	utils.RunCommand(gitopsRepoDir, environ, true, "git", "clean", "--force")

	commitToDeploy := "origin/HEAD"

	utils.RunCommand(gitopsRepoDir, environ, true, "git", "reset", "--hard", commitToDeploy)

	head, _ := utils.RunCommand(gitopsRepoDir, environ, true, "git", "rev-parse", "HEAD")

	utils.RunCommand(gitopsRepoDir, environ, true, "git", "verify-commit", "-v", "HEAD")

	log.Infof("deploying %s", head)

	hostname := os.Getenv("HOSTNAME")

	hostGitopsDir := fmt.Sprintf("%s/gitops/%s", gitopsRepoDir, hostname)

	return head, hostGitopsDir
}

// Start/restart configured services
func servicesUp(syncer utils.ServiceSyncer) error {
	config := syncer.GetConfig()

	if config.Pre != nil {
		log.Info("running pre script")
		err := syncer.RunPre(config.Pre.Script)
		if err != nil {
			log.WithField("error", err.Error()).Error("pre script failed")
			return fmt.Errorf("pre script failed")
		}
	}

	runningServices, err := syncer.GetRunningServices()
	if err != nil {
		log.WithField("error", err.Error()).Fatal("failed to get running containers")
	}

	log.Info("starting services")
	for service := range config.Services {
		log := log.WithField("service", service)

		hash, err := syncer.CreateService(service, config.Services[service])
		if err != nil {
			log.WithField("error", err.Error()).Error("failed to write container file")
			continue
		}
		s := config.Services[service]
		s.Hash = hash
		config.Services[service] = s

		oldHash := runningServices[service]
		log = log.WithField("oldHash", oldHash).WithField("newHash", hash)

		log.Info("container file created")
		if oldHash != hash {
			log.Info("service changed")
		}
	}

	tries := len(config.Services)
	updatedServices := getUpdatedServices(config, runningServices)

	for tries >= 0 && len(updatedServices) > 0 {
		service := updatedServices[0]
		newHash := config.Services[service].Hash
		oldHash := runningServices[service]
		log := log.WithField("service", service).WithField("oldHash", oldHash).WithField("newHash", newHash)
		log.Info("restarting service")
		err := syncer.RestartService(service)
		if err != nil {
			log.WithField("error", err.Error()).Errorf("failed to start service")
			// Remove the hash to indicate that the service failed
			s := config.Services[service]
			s.Hash = ""
			config.Services[service] = s
		}
		// Wait a bit for the container to start before reading updated services
		time.Sleep(3 * time.Second)
		// starting one service might have automatically started dependencies, so we need to get an updated list
		runningServices, _ = syncer.GetRunningServices()
		updatedServices = getUpdatedServices(config, runningServices)
		tries -= 1
	}

	if tries < 0 {
		log.Error("used too many attempts to start services")
		return fmt.Errorf("used too many attempts to start services")
	}

	for _, v := range config.Services {
		if v.Hash == "" {
			log.Error("failed to start one or more services")
			return fmt.Errorf("failed to start one or more services")
		}
	}

	if config.Post != nil {
		log.Info("running post script")
		err := syncer.RunPost(config.Post.Script)
		if err != nil {
			log.WithField("error", err.Error()).Error("post script failed")
			return fmt.Errorf("post script failed")
		}
	}

	log.Info("services up ok")
	return nil
}

// Stop orphaned services
func orphansDown(syncer utils.ServiceSyncer) {
	config := syncer.GetConfig()

	runningServices, err := syncer.GetRunningServices()
	if err != nil {
		log.WithField("error", err.Error()).Fatal("failed to get running containers")
	}

	serviceFailed := make(map[string]struct{})

	orphanedServices := getOrphanedServices(config, runningServices)

	for _, service := range orphanedServices {
		err := syncer.StopService(service)
		if err != nil {
			log.WithField("error", err.Error()).Errorf("failed to stop orphaned service")
			serviceFailed[service] = struct{}{}
			continue
		}
		log.WithField("service", service).Info("stopped orphaned service")
	}

	if len(serviceFailed) > 0 {
		log.Error("failed to stop some orphaned services")
		return
	}

	log.Info("orphaned services cleaned up")
}

// Stop orphaned services
func allDown(syncer utils.ServiceSyncer) {
	runningServices, err := syncer.GetRunningServices()
	if err != nil {
		log.WithField("error", err.Error()).Fatal("failed to get running containers")
	}

	serviceFailed := make(map[string]struct{})

	for service := range runningServices {
		err := syncer.StopService(service)
		if err != nil {
			log.WithField("error", err.Error()).Errorf("failed to stop service")
			serviceFailed[service] = struct{}{}
			continue
		}
		log.WithField("service", service).Info("stopped service")
	}

	if len(serviceFailed) > 0 {
		log.Error("failed to stop some services")
		return
	}

	log.Info("services stopped")
}
