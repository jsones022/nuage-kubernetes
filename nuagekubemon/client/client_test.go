package client

import (
	"flag"
	"fmt"
	"github.com/nuagenetworks/openshift-integration/nuagekubemon/api"
	"github.com/nuagenetworks/openshift-integration/nuagekubemon/config"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"
)

var kubemonConfig *config.NuageKubeMonConfig

func TestMain(m *testing.M) {
	kubemonConfig = &config.NuageKubeMonConfig{}
	addArgs(kubemonConfig, flag.CommandLine)
	flag.CommandLine.Parse(os.Args[1:])
	fmt.Println("TestMain")
	os.Exit(m.Run())
}

func addArgs(myConfig *config.NuageKubeMonConfig, flagSet *flag.FlagSet) {
	flagSet.StringVar(&myConfig.OsClusterAdmin, "osusername",
		"system:admin", "User name of the cluster administrator")
	flagSet.StringVar(&myConfig.KubeConfigFile, "kubeconfig",
		"", "kubeconfig File for Openshift User")
	flagSet.StringVar(&myConfig.OsMasterConfigFile, "osmasterconfig",
		"", "Path to master-config.yaml for the cluster master")
	flagSet.StringVar(&myConfig.NuageVsdApiUrl, "nuagevsdurl",
		"", "Nuage VSD URL")
	flagSet.StringVar(&myConfig.NuageVspVersion, "nuagevspversion",
		"", "Nuage VSP Version")
	flagSet.StringVar(&myConfig.LicenseFile, "license_file",
		"", "VSD License File Path")
}

func TestGetNamespaces(t *testing.T) {
	// Check if we have `oc`.  If it's not present, this test isn't runnable,
	// so skip it.
	_, err := exec.Command("oc", "whoami").CombinedOutput()
	if err != nil {
		t.Skip("Not on target system.  Cannot run this test")
	}
	myClient := NewNuageOsClient(kubemonConfig)
	// Get the names of all existing projects.
	output, err := exec.Command("bash", "-c",
		"oc get projects|awk '!/^NAME/ { print $1 }'").CombinedOutput()
	if err != nil {
		t.Fatalf("output: %v\nerror: %v\n", string(output), err)
	}
	output = []byte(strings.Trim(string(output), "\n \t"))
	cliProjectNames := strings.Split(string(output), "\n")
	// Get the names from GetNamespaces()
	goProjectEvents, err := myClient.GetNamespaces()
	if err != nil {
		t.Fatalf("output: %v\nerror: %v\n", string(output), err)
	}
	// If the lengths are different, the lists must differ, so we don't have to
	// do any comparison.
	if len(cliProjectNames) != len(*goProjectEvents) {
		t.Fatalf("Mismatched project list! cli returned '%v', GetNamespaces()"+
			" returned '%v'.", len(cliProjectNames), len(*goProjectEvents))
	}
	// In order to sort the list of names, pull out the names from the
	// NamespaceEvents.
	goProjectNames := make([]string, len(*goProjectEvents))
	for i, event := range *goProjectEvents {
		goProjectNames[i] = event.Name
	}
	// Sort the lists to make compares simpler
	sort.StringSlice(cliProjectNames).Sort()
	sort.StringSlice(goProjectNames).Sort()
	// The lists should be identical
	for i := range cliProjectNames {
		if cliProjectNames[i] != goProjectNames[i] {
			t.Fatalf("Mismatch after item %v. cli: '%v', GetNamespaces(): '%v'",
				i, cliProjectNames[i], goProjectNames[i])
		}
	}
}

func TestAddDelProject(t *testing.T) {
	// Check if we have `oc`.  If it's not present, this test isn't runnable,
	// so skip it.
	_, err := exec.Command("oc", "whoami").CombinedOutput()
	if err != nil {
		t.Skip("Not on target system.  Cannot run this test")
	}
	myClient := NewNuageOsClient(kubemonConfig)
	nsChannel := make(chan *api.NamespaceEvent)
	stop := make(chan bool)
	go myClient.WatchNamespaces(nsChannel, stop)
	projectName := "test-project"
	// Create project
	output, err := exec.Command("oc", "new-project", projectName).CombinedOutput()
	if err != nil {
		t.Fatalf("output: %v\nerror: %v\n", string(output), err)
	}
	var event *api.NamespaceEvent
	event = <-nsChannel
	// Verify that an added event was processed for that namespace
	switch {
	case event.Name != projectName:
		t.Fatalf("Name mismatch! Expected '%v', got '%v'", projectName,
			event.Name)
	case event.Type != api.Added:
		t.Fatal("Type mismatch! Expected Added, got Deleted")
	}
	// Delete project
	output, err = exec.Command("oc", "delete", "project", projectName).CombinedOutput()
	if err != nil {
		t.Fatalf("output: %v\nerror: %v\n", string(output), err)
	}
	event = <-nsChannel
	// Verify that a deleted event was processed for that namespace
	switch {
	case event.Name != projectName:
		t.Fatalf("Name mismatch! Expected '%v', got '%v'", projectName,
			event.Name)
	case event.Type != api.Deleted:
		t.Fatal("Type mismatch! Expected Deleted, got Added")
	}
}

type projectEvent struct {
	name string
	add  bool
}

func (self projectEvent) equals(other *projectEvent) bool {
	if self.name != other.name || self.add != other.add {
		return false
	}
	return true
}

func (self projectEvent) String() string {
	if self.add {
		return fmt.Sprintf("<Add: %s>", self.name)
	} else {
		return fmt.Sprintf("<Del: %s>", self.name)
	}
}

func TestAddDelManyStatic(t *testing.T) {
	// Check if we have `oc`.  If it's not present, this test isn't runnable,
	// so skip it.
	_, err := exec.Command("oc", "whoami").CombinedOutput()
	if err != nil {
		t.Skip("Not on target system.  Cannot run this test")
	}
	events := []projectEvent{
		{"test1", true},
		{"test2", true},
		{"test3", true},
		{"test2", false},
		{"test4", true},
		{"test1", false},
		{"test3", false},
		{"test4", false},
		{"test5", true},
		{"test5", false},
	}
	myClient := NewNuageOsClient(kubemonConfig)
	nsChannel := make(chan *api.NamespaceEvent, len(events))
	stop := make(chan bool)
	go myClient.WatchNamespaces(nsChannel, stop)
	for _, event := range events {
		if event.add {
			output, err := exec.Command("oc", "new-project", event.name).CombinedOutput()
			if err != nil {
				t.Fatalf("output: %v\nerror: %v\n", string(output), err)
			}
		} else {
			output, err := exec.Command("oc", "delete", "project", event.name).CombinedOutput()
			if err != nil {
				t.Fatalf("output: %v\nerror: %v\n", string(output), err)
			}
		}
	}
	for i := 0; i < len(events); i++ {
		select {
		case nsEvent := <-nsChannel:
			projEvent := projectEvent{nsEvent.Name, (nsEvent.Type == api.Added)}
			exists := false
			for _, event := range events {
				if exists = event.equals(&projEvent); exists {
					break
				}
			}
			if !exists {
				t.Fatalf("Unexpected event %s\n", projEvent)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("Timeout! Not enough events were triggered.")
		}
	}
}

func TestAddDelManyDynamic(t *testing.T) {
	// Check if we have `oc`.  If it's not present, this test isn't runnable,
	// so skip it.
	_, err := exec.Command("oc", "whoami").CombinedOutput()
	if err != nil {
		t.Skip("Not on target system.  Cannot run this test")
	}
	rand.Seed(time.Now().UnixNano())
	events := make([]projectEvent, 20)
	size := 0
	for i := 0; i <= 9; i++ {
		name := "test" + string(int('0')+i)
		events[i] = projectEvent{name, true}
		size++
	}
	for i := 0; i < 19; i++ {
		if !events[i].add {
			continue
		}
		name := events[i].name
		// Offset the delete at least 1 item in the future, max at the end of
		// the list.
		j := rand.Intn(size-i) + i + 1
		// Shift later items by 1
		for k := size; k > j; k-- {
			events[k] = events[k-1]
		}
		events[j] = projectEvent{name, false}
		size++
	}
	myClient := NewNuageOsClient(kubemonConfig)
	nsChannel := make(chan *api.NamespaceEvent, len(events))
	stop := make(chan bool)
	go myClient.WatchNamespaces(nsChannel, stop)
	for _, event := range events {
		if event.add {
			output, err := exec.Command("oc", "new-project", event.name).CombinedOutput()
			if err != nil {
				t.Fatalf("output: %v\nerror: %v\n", string(output), err)
			}
		} else {
			output, err := exec.Command("oc", "delete", "project", event.name).CombinedOutput()
			if err != nil {
				t.Fatalf("output: %v\nerror: %v\n", string(output), err)
			}
		}
	}
	for i := 0; i < len(events); i++ {
		// Read the events from we generated by creating/deleting projects.  If
		// we get blocked for too long, assume an event wasn't triggered.
		// TODO: Find a way to better detect when events are dropped.
		select {
		case nsEvent := <-nsChannel:
			projEvent := projectEvent{nsEvent.Name, (nsEvent.Type == api.Added)}
			exists := false
			for _, event := range events {
				if exists = event.equals(&projEvent); exists {
					break
				}
			}
			if !exists {
				t.Fatalf("Unexpected event %s\n", projEvent)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("Timeout! Not enough events were triggered.")
		}
	}
}