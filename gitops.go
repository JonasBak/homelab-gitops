package main

import (
	"os"
	"path/filepath"

	qs "github.com/JonasBak/homelab_gitops/quadlet_syncer"
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

		_, hostGitopsDir := fetch(gitopsRepo, gitopsDir)

		syncer.HostGitopsDir = hostGitopsDir

		servicesUp(&syncer)
		orphansDown(&syncer)
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
