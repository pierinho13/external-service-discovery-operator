package discovery

import (
	"context"
	"testing"

	"github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
)

func TestStaticProviderCanonicalizesSortsAndDeduplicates(t *testing.T) {
	resource := &v1alpha1.DiscoveredService{Spec: v1alpha1.DiscoveredServiceSpec{Discovery: v1alpha1.DiscoveryProvider{Static: &v1alpha1.StaticDiscovery{Addresses: []string{"10.0.0.2", "10.0.0.1", "10.0.0.2"}}}}}
	got, err := (StaticProvider{}).Discover(context.Background(), resource)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Endpoints) != 2 || got.Endpoints[0].Address != "10.0.0.1" || got.Endpoints[1].Address != "10.0.0.2" {
		t.Fatalf("unexpected endpoints: %#v", got)
	}
}

func TestStaticProviderRejectsNonIPv4(t *testing.T) {
	resource := &v1alpha1.DiscoveredService{Spec: v1alpha1.DiscoveredServiceSpec{Discovery: v1alpha1.DiscoveryProvider{Static: &v1alpha1.StaticDiscovery{Addresses: []string{"2001:db8::1"}}}}}
	if _, err := (StaticProvider{}).Discover(context.Background(), resource); err == nil {
		t.Fatal("expected IPv6 to be rejected")
	}
}
