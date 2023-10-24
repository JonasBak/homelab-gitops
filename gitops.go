package main

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

var l = logrus.New()
var log = l.WithFields(logrus.Fields{})

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

var environ = append(os.Environ())

func main() {
	if len(os.Args) == 1 {
		log.Fatalf("Expected command")
	}

	cmd := os.Args[1]

	switch cmd {
	case "sync":
		gitopsRepo := os.Args[2]
		gitopsRepoDir := os.Args[3]

		_, dir := fetch(gitopsRepo, gitopsRepoDir)
		servicesUp(dir)
		orphansDown(dir)
		break
	case "up":
		path, err := filepath.Abs(os.Args[2])
		if err != nil {
			log.Fatal(err.Error())
		}
		servicesUp(path)
		break
	case "clean":
		path, err := filepath.Abs(os.Args[2])
		if err != nil {
			log.Fatal(err.Error())
		}
		orphansDown(path)
		break
	case "down":
		allDown()
		break
	default:
		log.Fatalf("Unknown command '%s'", cmd)
	}
}
