package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/JonasBak/homelab-gitops/utils"
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

	hostname := os.Getenv("HOSTNAME")

	hostGitopsDir := fmt.Sprintf("%s/gitops/%s", gitopsRepoDir, hostname)

	return head, hostGitopsDir
}

type SyncError struct {
	err error

	// TODO should there be "non-fatal" errors?
	nonFatal        bool
	servicesErrored []string
}

func (err *SyncError) Error() string {
	return err.err.Error()
}

// Start/restart configured services
func servicesUp(syncer utils.ServiceSyncer) *SyncError {
	config := syncer.GetConfig()

	if config.Pre != nil {
		log.Info("running pre script")
		err := syncer.RunPre(config.Pre.Script)
		if err != nil {
			return &SyncError{err: fmt.Errorf("pre script failed: %s", err.Error())}
		}
	}

	runningServices, err := syncer.GetRunningServices()
	if err != nil {
		return &SyncError{err: fmt.Errorf("failed to get running containers: %s", err.Error())}
	}

	log.Info("creating services")
	for service := range config.Services {
		log := log.WithField("service", service)

		hash, err := syncer.CreateService(service, config.Services[service])
		if err != nil {
			// TODO Should a bad manifest cause the rollout to stop, or should the other services be started?
			return &SyncError{err: fmt.Errorf("failed to create service: %s", err.Error()), servicesErrored: []string{service}}
		}
		s := config.Services[service]
		s.Hash = hash
		config.Services[service] = s

		oldHash := runningServices[service]
		log = log.WithField("oldHash", oldHash).WithField("newHash", hash)

		log.Info("service created")
		if oldHash != hash {
			log.Info("service changed")
		}
	}

	updatedServices := getUpdatedServices(config, runningServices)
	restartAttempts := map[string]int{}

	log.Info("starting services")
	for len(updatedServices) > 0 {
		sort.SliceStable(updatedServices, func(i, j int) bool {
			return restartAttempts[updatedServices[i]] < restartAttempts[updatedServices[j]]
		})
		service := updatedServices[0]

		// Could increase this to attempt to start each service more than once
		if restartAttempts[service] > 0 {
			return &SyncError{err: fmt.Errorf("some services failed to start"), servicesErrored: updatedServices}
		}
		restartAttempts[service] = restartAttempts[service] + 1

		newHash := config.Services[service].Hash
		oldHash := runningServices[service]
		log := log.WithField("service", service).WithField("oldHash", oldHash).WithField("newHash", newHash)
		log.Info("restarting service")
		err := syncer.RestartService(service)
		if err != nil {
			log.WithField("error", err.Error()).Errorf("failed to start service")
		}
		// Wait a bit for the container to start before reading updated services
		time.Sleep(3 * time.Second)
		// starting one service might have automatically started dependencies, so we need to get an updated list
		runningServices, _ = syncer.GetRunningServices()
		updatedServices = getUpdatedServices(config, runningServices)
	}

	// Wait a bit to see if containers still run
	time.Sleep(4 * time.Second)
	runningServices, _ = syncer.GetRunningServices()
	if servicesNotUpdated := getUpdatedServices(config, runningServices); len(servicesNotUpdated) != 0 {
		return &SyncError{err: fmt.Errorf("some services didn't start properly"), servicesErrored: servicesNotUpdated}
	}

	if config.Post != nil {
		log.Info("running post script")
		err := syncer.RunPost(config.Post.Script)
		if err != nil {
			return &SyncError{err: fmt.Errorf("post script failed: %s", err.Error())}
		}
	}

	log.Info("services up ok")
	return nil
}

// Stop orphaned services
func orphansDown(syncer utils.ServiceSyncer) *SyncError {
	config := syncer.GetConfig()

	runningServices, err := syncer.GetRunningServices()
	if err != nil {
		log.WithField("error", err.Error()).Fatal("failed to get running containers")
	}

	serviceFailed := []string{}

	orphanedServices := getOrphanedServices(config, runningServices)

	for _, service := range orphanedServices {
		err := syncer.StopService(service)
		if err != nil {
			log.WithField("error", err.Error()).Errorf("failed to stop orphaned service")
			serviceFailed = append(serviceFailed, service)
			continue
		}
		log.WithField("service", service).Info("stopped orphaned service")
	}

	if len(serviceFailed) > 0 {
		return &SyncError{err: fmt.Errorf("failed to stop some orphaned services"), servicesErrored: serviceFailed}
	}

	log.Info("orphaned services cleaned up")

	return nil
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
