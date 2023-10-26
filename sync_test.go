package main

import (
	"fmt"
	"sort"
	"testing"

	"github.com/JonasBak/homelab-gitops/utils"
)

type testSyncer struct {
	config             utils.Config
	getRunningServices func() map[string]string
	createService      func(service string) (string, error)
	restartService     func(service string) error
	stopService        func(service string) error
}

func (s *testSyncer) GetConfig() utils.Config {
	return s.config
}
func (s *testSyncer) GetRunningServices() (map[string]string, error) {
	return s.getRunningServices(), nil
}
func (s *testSyncer) CreateService(service string, serviceConfig utils.Service) (string, error) {
	return s.createService(service)
}
func (s *testSyncer) RestartService(service string) error {
	return s.restartService(service)
}
func (s *testSyncer) StopService(service string) error {
	return s.stopService(service)
}
func (s *testSyncer) RunPre(cmd string) error {
	return nil
}
func (s *testSyncer) RunPost(cmd string) error {
	return nil
}

var _ utils.ServiceSyncer = &testSyncer{}

func TestServicesUp(t *testing.T) {
	expectToStart := map[string]int{
		"service-a": 0,
		"service-c": 0,
	}
	runningServices := map[string]string{
		"service-b": "service-b",
		"service-c": "123",
	}
	config := utils.Config{
		Services: map[string]utils.Service{
			// New service
			"service-a": {},
			// Up to date service
			"service-b": {},
			// Updated service
			"service-c": {},
		},
	}
	syncer := testSyncer{
		config: config,
		getRunningServices: func() map[string]string {
			return runningServices
		},
		createService: func(service string) (string, error) {
			if service == "service-d" {
				return "", fmt.Errorf("service-d failed")
			}
			return service, nil
		},
		restartService: func(service string) error {
			runningServices[service] = service
			if _, ok := expectToStart[service]; !ok {
				t.Fatalf("service '%s' wasn't expected to be (re)started", service)
			}
			expectToStart[service] = expectToStart[service] + 1
			return nil
		},
	}

	err := servicesUp(&syncer)

	if err != nil {
		t.Fatalf("servicesUp should have exited without error, but got: %s", err.Error())
	}

	for service, count := range expectToStart {
		if count != 1 {
			t.Fatalf("service '%s' was expected to be started 1 time, was started %d times", service, count)
		}
	}
}

func TestServicesUpFailesToStart(t *testing.T) {
	attempts := map[string]int{
		"service-a": 0,
		"service-b": 0,
		"service-c": 0,
		"service-d": 0,
		"service-e": 0,
	}
	runningServices := map[string]string{}
	config := utils.Config{
		Services: map[string]utils.Service{
			// These are ok
			"service-a": {},
			"service-b": {},
			// These two fails
			"service-c": {},
			"service-d": {},
			// This is ok
			"service-e": {},
		},
	}
	syncer := testSyncer{
		config: config,
		getRunningServices: func() map[string]string {
			return runningServices
		},
		createService: func(service string) (string, error) {
			return service, nil
		},
		restartService: func(service string) error {
			attempts[service] = attempts[service] + 1
			if service == "service-c" || service == "service-d" {
				return fmt.Errorf("service failed")
			}
			runningServices[service] = service
			return nil
		},
	}

	err := servicesUp(&syncer)

	if err == nil {
		t.Fatal("service-d failing should have made servicesUp return error")
	}

	for service, attempts := range attempts {
		if attempts != 1 {
			t.Fatalf("service '%s' should have been attempted started 1 time, was %d", service, attempts)
		}
	}

	assertEq(t, runningServices["service-a"], "service-a", "service-a should have been started")
	assertEq(t, runningServices["service-b"], "service-b", "service-b should have been started")
	assertEq(t, runningServices["service-e"], "service-e", "service-e should have been started")
	assertEq(t, runningServices["service-c"], "", "service-c should not have been started")
	assertEq(t, runningServices["service-d"], "", "service-d should not have been started")

	sort.Strings(err.servicesErrored)
	assertEq(t, err.servicesErrored[0], "service-c", "service-c should be reported as failed")
	assertEq(t, err.servicesErrored[1], "service-d", "service-d should be reported as failed")
}

func TestOrphansDown(t *testing.T) {
	expectToStop := map[string]int{
		"service-c": 0,
		"service-d": 0,
	}
	runningServices := map[string]string{
		"service-a": "service-a",
		"service-b": "service-b",
		"service-c": "service-c",
		"service-d": "service-d",
	}
	config := utils.Config{
		Services: map[string]utils.Service{
			"service-a": {},
			"service-b": {},
		},
	}
	syncer := testSyncer{
		config: config,
		getRunningServices: func() map[string]string {
			return runningServices
		},
		stopService: func(service string) error {
			delete(runningServices, service)
			if _, ok := expectToStop[service]; !ok {
				t.Fatalf("service '%s' wasn't expected to be stopped", service)
			}
			expectToStop[service] = expectToStop[service] + 1
			return nil
		},
	}

	orphansDown(&syncer)

	for service, count := range expectToStop {
		if count != 1 {
			t.Fatalf("service '%s' was expected to be started 1 time, was started %d times", service, count)
		}
	}
}
