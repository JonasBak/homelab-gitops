package main

import (
	"github.com/JonasBak/homelab_gitops/utils"
	"sort"
	"testing"
)

func assert(t *testing.T, v bool, reason string) {
	if !v {
		t.Errorf(`Error: %s`, reason)
	}
}

func assertEq[K comparable](t *testing.T, got, want K, reason string) {
	if got != want {
		t.Errorf(`Error: %s.
    Got:
    %+v
    Expected:
    %+v`, reason, got, want)
	}
}

func TestGetOrphanedServices(t *testing.T) {
	config := utils.Config{
		Services: map[string]utils.Service{
			"service-a": {Hash: "a"},
			"service-b": {Hash: "a"},
			"service-c": {Hash: "a"},
		},
	}

	runningServices := map[string]string{
		// Up to date
		"service-a": "a",
		// Out of date
		"service-b": "b",
		// Orphaned
		"service-d": "b",
	}

	orphanedServices := getOrphanedServices(config, runningServices)

	if len(orphanedServices) != 1 || orphanedServices[0] != "service-d" {
		t.Errorf("expected one orphaned service: 'service-d', got: '%v'", orphanedServices)
	}
}

func TestGetUpdatedServices(t *testing.T) {
	config := utils.Config{
		Services: map[string]utils.Service{
			// This is up to date
			"service-a": {Hash: "a"},
			// This is a new service
			"service-b": {Hash: "b"},
			// This existing service failed to create container file, has no hash
			"service-c": {Hash: ""},
			// This is updated
			"service-d": {Hash: "d"},
			// This new service failed to create container file, has no hash
			"service-e": {Hash: ""},
		},
	}

	runningServices := map[string]string{
		"service-a": "a",
		"service-c": "c",
		"service-d": "1",
	}

	updatedServices := getUpdatedServices(config, runningServices)
	sort.Strings(updatedServices)

	assertEq(t, len(updatedServices), 2, "expected two updated services")
	assertEq(t, updatedServices[0], "service-b", "expected service-b to be updated")
	assertEq(t, updatedServices[1], "service-d", "expected service-d to be updated")
}
