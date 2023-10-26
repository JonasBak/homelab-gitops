package main

import (
	"os"
	"path/filepath"

	qs "github.com/JonasBak/homelab-gitops/quadlet_syncer"
	"github.com/sirupsen/logrus"
)

var l = logrus.New()
var log = l.WithFields(logrus.Fields{})

var environ = append(os.Environ())

func main() {
	if len(os.Args) == 1 {
		log.Fatalf("Expected command")
	}

	cmd := os.Args[1]

	syncer := qs.QuadletSyncer{}

	switch cmd {
	case "sync":
		gitopsRepo := os.Args[2]
		gitopsDir := os.Args[3]

		ref, hostGitopsDir := fetch(gitopsRepo, gitopsDir)

		log.WithField("ref", ref).Info("running sync")

		syncer.HostGitopsDir = hostGitopsDir

		errUp := servicesUp(&syncer)
		if errUp != nil {
			log.WithField("error", errUp.Error()).WithField("services", errUp.servicesErrored).Error("failed to start services")
		}
		errDown := orphansDown(&syncer)
		if errDown != nil {
			log.WithField("error", errDown.Error()).WithField("services", errDown.servicesErrored).Error("failed to clean up services")
		}
		if errUp == nil && errDown == nil {
			log.WithField("ref", ref).Info("sync ok")
		} else {
			log.WithField("ref", ref).Fatal("sync failed")
		}
		break
	case "up":
		hostGitopsDir, err := filepath.Abs(os.Args[2])
		if err != nil {
			log.Fatal(err.Error())
		}

		syncer.HostGitopsDir = hostGitopsDir

		servicesUp(&syncer)
		break
	case "clean":
		hostGitopsDir, err := filepath.Abs(os.Args[2])
		if err != nil {
			log.Fatal(err.Error())
		}

		syncer.HostGitopsDir = hostGitopsDir

		orphansDown(&syncer)
		break
	case "down":
		allDown(&syncer)
		break
	default:
		log.Fatalf("Unknown command '%s'", cmd)
	}
}
