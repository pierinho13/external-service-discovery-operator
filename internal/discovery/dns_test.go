package discovery

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
)

type fakeDNSResolver struct {
	addresses map[string][]net.IPAddr
	errors    map[string]error
}

func (r fakeDNSResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if err := r.errors[host]; err != nil {
		return nil, err
	}
	return r.addresses[host], nil
}

func TestDNSProviderDiscovery(t *testing.T) {
	tests := []struct {
		name      string
		hostnames []string
		addresses map[string][]net.IPAddr
		want      []string
	}{
		{name: "one hostname one address", hostnames: []string{"one.internal"}, addresses: records("one.internal", "10.0.0.1"), want: []string{"10.0.0.1"}},
		{name: "multiple hostnames", hostnames: []string{"one.internal", "two.internal"}, addresses: mergeRecords(records("one.internal", "10.0.0.2"), records("two.internal", "10.0.0.1")), want: []string{"10.0.0.1", "10.0.0.2"}},
		{name: "one hostname multiple A records", hostnames: []string{"many.internal"}, addresses: records("many.internal", "10.0.0.3", "10.0.0.1", "10.0.0.2"), want: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}},
		{name: "duplicates deduplicated and IPv6 ignored", hostnames: []string{"one.internal", "two.internal"}, addresses: mergeRecords(records("one.internal", "10.0.0.2", "2001:db8::1", "10.0.0.1"), records("two.internal", "10.0.0.1")), want: []string{"10.0.0.1", "10.0.0.2"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			refresh := 37 * time.Second
			provider := DNSProvider{Resolver: fakeDNSResolver{addresses: test.addresses}, RefreshInterval: refresh}
			result, err := provider.Discover(context.Background(), dnsResource(test.hostnames...))
			if err != nil {
				t.Fatal(err)
			}
			if result.RequeueAfter != refresh {
				t.Fatalf("expected refresh %s, got %s", refresh, result.RequeueAfter)
			}
			if len(result.Endpoints) != len(test.want) {
				t.Fatalf("got %#v, want %#v", result.Endpoints, test.want)
			}
			for i := range test.want {
				if result.Endpoints[i].Address != test.want[i] {
					t.Fatalf("got %#v, want %#v", result.Endpoints, test.want)
				}
			}
		})
	}
}

func TestDNSProviderFailsClosedOnResolverError(t *testing.T) {
	want := errors.New("lookup failed")
	provider := DNSProvider{Resolver: fakeDNSResolver{addresses: records("one.internal", "10.0.0.1"), errors: map[string]error{"two.internal": want}}}
	result, err := provider.Discover(context.Background(), dnsResource("one.internal", "two.internal"))
	if !errors.Is(err, want) || len(result.Endpoints) != 0 {
		t.Fatalf("expected fail-closed resolver error, got result=%#v err=%v", result, err)
	}
}

func TestDNSProviderRejectsNoUsableIPv4(t *testing.T) {
	provider := DNSProvider{Resolver: fakeDNSResolver{addresses: records("ipv6.internal", "2001:db8::1")}}
	_, err := provider.Discover(context.Background(), dnsResource("ipv6.internal"))
	if err == nil || !strings.Contains(err.Error(), "no usable IPv4") {
		t.Fatalf("expected useful IPv4 error, got %v", err)
	}
}

func TestDNSProviderRejectsMissingConfiguration(t *testing.T) {
	_, err := (DNSProvider{Resolver: fakeDNSResolver{}}).Discover(context.Background(), &discoveryv1alpha1.DiscoveredService{})
	if err == nil || !strings.Contains(err.Error(), "configuration is required") {
		t.Fatalf("expected missing configuration error, got %v", err)
	}
}

func dnsResource(names ...string) *discoveryv1alpha1.DiscoveredService {
	return &discoveryv1alpha1.DiscoveredService{Spec: discoveryv1alpha1.DiscoveredServiceSpec{Discovery: discoveryv1alpha1.DiscoveryProvider{DNS: &discoveryv1alpha1.DNSDiscovery{Names: names}}}}
}

func records(host string, addresses ...string) map[string][]net.IPAddr {
	result := make([]net.IPAddr, 0, len(addresses))
	for _, address := range addresses {
		result = append(result, net.IPAddr{IP: net.ParseIP(address)})
	}
	return map[string][]net.IPAddr{host: result}
}
func mergeRecords(records ...map[string][]net.IPAddr) map[string][]net.IPAddr {
	result := map[string][]net.IPAddr{}
	for _, group := range records {
		for host, addresses := range group {
			result[host] = addresses
		}
	}
	return result
}
