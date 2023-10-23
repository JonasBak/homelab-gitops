package main

import "testing"

func TestGetOrphanedServices(t *testing.T) {
	config := Config{
		Services: map[string]Service{
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
