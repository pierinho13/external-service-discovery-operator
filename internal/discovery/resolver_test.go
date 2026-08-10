package discovery

import (
	"context"
	"errors"
	"testing"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
)

type stubProvider struct {
	called bool
	err    error
}

func (p *stubProvider) Discover(context.Context, *discoveryv1alpha1.DiscoveredService) (Result, error) {
	p.called = true
	return Result{Endpoints: []Endpoint{{Address: "10.0.0.1"}}}, p.err
}

func TestResolverSelectsStaticProvider(t *testing.T) {
	static := &stubProvider{}
	resource := &discoveryv1alpha1.DiscoveredService{Spec: discoveryv1alpha1.DiscoveredServiceSpec{Discovery: discoveryv1alpha1.DiscoveryProvider{Static: &discoveryv1alpha1.StaticDiscovery{}}}}
	got, err := (Resolver{Static: static}).Discover(context.Background(), resource)
	if err != nil || !static.called || len(got.Endpoints) != 1 {
		t.Fatalf("static provider was not selected: endpoints=%v called=%v err=%v", got, static.called, err)
	}
}

func TestResolverSelectsDNSProvider(t *testing.T) {
	dns := &stubProvider{}
	resource := &discoveryv1alpha1.DiscoveredService{Spec: discoveryv1alpha1.DiscoveredServiceSpec{Discovery: discoveryv1alpha1.DiscoveryProvider{DNS: &discoveryv1alpha1.DNSDiscovery{Names: []string{"example.internal"}}}}}
	got, err := (Resolver{DNS: dns}).Discover(context.Background(), resource)
	if err != nil || !dns.called || len(got.Endpoints) != 1 {
		t.Fatalf("DNS provider was not selected: result=%v called=%v err=%v", got, dns.called, err)
	}
}

func TestResolverRejectsMissingProvider(t *testing.T) {
	if _, err := (Resolver{Static: &stubProvider{}}).Discover(context.Background(), &discoveryv1alpha1.DiscoveredService{}); err == nil {
		t.Fatal("expected missing provider error")
	}
}

func TestResolverPropagatesProviderError(t *testing.T) {
	want := errors.New("discovery unavailable")
	_, got := (Resolver{Static: &stubProvider{err: want}}).Discover(context.Background(), &discoveryv1alpha1.DiscoveredService{Spec: discoveryv1alpha1.DiscoveredServiceSpec{Discovery: discoveryv1alpha1.DiscoveryProvider{Static: &discoveryv1alpha1.StaticDiscovery{}}}})
	if !errors.Is(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
