package main

import (
	"github.com/JonasBak/homelab_gitops/utils"
)

func getOrphanedServices(config utils.Config, runningServices map[string]string) []string {
	orphanedServices := []string{}

	for service := range runningServices {
		if _, ok := config.Services[service]; !ok {
			orphanedServices = append(orphanedServices, service)
		}
	}

	return orphanedServices
}

func getUpdatedServices(config utils.Config, runningServices map[string]string) []string {
	updatedServices := []string{}

	for service := range config.Services {
		if newHash := runningServices[service]; config.Services[service].Hash != "" && config.Services[service].Hash != newHash {
			updatedServices = append(updatedServices, service)
		}
	}

	return updatedServices
}
