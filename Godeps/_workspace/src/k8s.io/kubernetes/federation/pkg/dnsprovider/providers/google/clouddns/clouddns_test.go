/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clouddns

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"k8s.io/kubernetes/federation/pkg/dnsprovider"
	"k8s.io/kubernetes/federation/pkg/dnsprovider/providers/google/clouddns/internal/stubs"
	"k8s.io/kubernetes/federation/pkg/dnsprovider/rrstype"
)

func newTestInterface() (dnsprovider.Interface, error) {
	// Use this to test the real cloud service - insert appropriate project-id.  Default token source will be used.  See
	// https://github.com/golang/oauth2/blob/master/google/default.go for details.
	// i, err := dnsprovider.GetDnsProvider(ProviderName, strings.NewReader("\n[global]\nproject-id = federation0-cluster00"))
	return newFakeInterface() // Use this to stub out the entire cloud service
}

func newFakeInterface() (dnsprovider.Interface, error) {
	service := stubs.NewService()
	interface_ := newInterfaceWithStub("", service)
	zones := service.ManagedZones_
	// Add a fake zone to test against.
	zone := &stubs.ManagedZone{zones, "example.com", []stubs.ResourceRecordSet{}}
	call := zones.Create(interface_.project(), zone)
	_, err := call.Do()
	if err != nil {
		return nil, err
	}
	return interface_, nil
}

var interface_ dnsprovider.Interface

func TestMain(m *testing.M) {
	flag.Parse()
	var err error
	interface_, err = newTestInterface()
	if err != nil {
		fmt.Printf("Error creating interface: %v", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// firstZone returns the first zone for the configured dns provider account/project,
// or fails if it can't be found
func firstZone(t *testing.T) dnsprovider.Zone {
	t.Logf("Getting zones")
	z, supported := interface_.Zones()
	if supported {
		t.Logf("Got zones %v\n", z)
	} else {
		t.Fatalf("Zones interface not supported by interface %v", interface_)
	}
	zones, err := z.List()
	if err != nil {
		t.Fatalf("Failed to list zones: %v", err)
	} else {
		t.Logf("Got zone list: %v\n", zones)
	}
	if len(zones) < 1 {
		t.Fatalf("Zone listing returned %d, expected >= %d", len(zones), 1)
	} else {
		t.Logf("Got at least 1 zone in list:%v\n", zones[0])
	}
	return zones[0]
}

/* rrs returns the ResourceRecordSets interface for a given zone */
func rrs(t *testing.T, zone dnsprovider.Zone) (r dnsprovider.ResourceRecordSets) {
	rrsets, supported := zone.ResourceRecordSets()
	if !supported {
		t.Fatalf("ResourceRecordSets interface not supported by zone %v", zone)
		return r
	}
	return rrsets
}

func listRrsOrFail(t *testing.T, rrsets dnsprovider.ResourceRecordSets) []dnsprovider.ResourceRecordSet {
	rrset, err := rrsets.List()
	if err != nil {
		t.Fatalf("Failed to list recordsets: %v", err)
	} else {
		if len(rrset) < 0 {
			t.Fatalf("Record set length=%d, expected >=0", len(rrset))
		} else {
			t.Logf("Got %d recordsets: %v", len(rrset), rrset)
		}
	}
	return rrset
}

func getExampleRrs(zone dnsprovider.Zone) dnsprovider.ResourceRecordSet {
	rrsets, _ := zone.ResourceRecordSets()
	return rrsets.New("www11."+zone.Name(), []string{"10.10.10.10", "169.20.20.20"}, 180, rrstype.A)
}

func getInvalidRrs(zone dnsprovider.Zone) dnsprovider.ResourceRecordSet {
	rrsets, _ := zone.ResourceRecordSets()
	return rrsets.New("www12."+zone.Name(), []string{"rubbish", "rubbish"}, 180, rrstype.A)
}

func addRrsetOrFail(t *testing.T, rrsets dnsprovider.ResourceRecordSets, rrset dnsprovider.ResourceRecordSet) dnsprovider.ResourceRecordSet {
	result, err := rrsets.Add(rrset)
	if err != nil {
		t.Fatalf("Failed to add recordsets: %v", err)
	}
	return result
}

/* TestResourceRecordSetsList verifies that listing of zones succeeds */
func TestZonesList(t *testing.T) {
	firstZone(t)
}

/* TestResourceRecordSetsList verifies that listing of RRS's succeeds */
func TestResourceRecordSetsList(t *testing.T) {
	listRrsOrFail(t, rrs(t, firstZone(t)))
}

/* TestResourceRecordSetsAddSuccess verifies that addition of a valid RRS succeeds */
func TestResourceRecordSetsAddSuccess(t *testing.T) {
	zone := firstZone(t)
	sets := rrs(t, zone)
	set := addRrsetOrFail(t, sets, getExampleRrs(zone))
	defer sets.Remove(set)
	t.Logf("Successfully added resource record set: %v", set)
}

/* TestResourceRecordSetsAdditionVisible verifies that added RRS is visible after addition */
func TestResourceRecordSetsAdditionVisible(t *testing.T) {
	zone := firstZone(t)
	sets := rrs(t, zone)
	rrset := getExampleRrs(zone)
	set := addRrsetOrFail(t, sets, rrset)
	defer sets.Remove(set)
	t.Logf("Successfully added resource record set: %v", set)
	found := false
	for _, record := range listRrsOrFail(t, sets) {
		if record.Name() == rrset.Name() {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Failed to find added resource record set %s", rrset.Name())
	}
}

/* TestResourceRecordSetsAddDuplicateFail verifies that addition of a duplicate RRS fails */
func TestResourceRecordSetsAddDuplicateFail(t *testing.T) {
	zone := firstZone(t)
	sets := rrs(t, zone)
	rrset := getExampleRrs(zone)
	set := addRrsetOrFail(t, sets, rrset)
	defer sets.Remove(set)
	t.Logf("Successfully added resource record set: %v", set)
	// Try to add it again, and verify that the call fails.
	rrs, err := sets.Add(rrset)
	if err == nil {
		defer sets.Remove(rrs)
		t.Errorf("Should have failed to add duplicate resource record %v, but succeeded instead.", set)
	} else {
		t.Logf("Correctly failed to add duplicate resource record %v: %v", set, err)
	}
}

/* TestResourceRecordSetsRemove verifies that the removal of an existing RRS succeeds */
func TestResourceRecordSetsRemove(t *testing.T) {
	zone := firstZone(t)
	sets := rrs(t, zone)
	rrset := getExampleRrs(zone)
	set := addRrsetOrFail(t, sets, rrset)
	err := sets.Remove(set)
	if err != nil {
		// Try again to clean up.
		defer sets.Remove(rrset)
		t.Errorf("Failed to remove resource record set %v after adding", rrset)
	} else {
		t.Logf("Successfully removed resource set %v after adding", set)
	}
}

/* TestResourceRecordSetsRemoveGone verifies that a removed RRS no longer exists */
func TestResourceRecordSetsRemoveGone(t *testing.T) {
	zone := firstZone(t)
	sets := rrs(t, zone)
	rrset := getExampleRrs(zone)
	set := addRrsetOrFail(t, sets, rrset)
	err := sets.Remove(set)
	if err != nil {
		// Try again to clean up.
		defer sets.Remove(rrset)
		t.Errorf("Failed to remove resource record set %v after adding", rrset)
	} else {
		t.Logf("Successfully removed resource set %v after adding", set)
	}
	// Check that it's gone
	list := listRrsOrFail(t, sets)
	found := false
	for _, set := range list {
		if set.Name() == rrset.Name() {
			found = true
			break
		}
	}
	if found {
		t.Errorf("Deleted resource record set %v is still present", rrset)
	}
}