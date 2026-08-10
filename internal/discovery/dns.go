package discovery

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"time"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
)

// DNSResolver is the narrow part of net.Resolver used by DNSProvider.
type DNSResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type DNSProvider struct {
	Resolver        DNSResolver
	RefreshInterval time.Duration
}

func (p DNSProvider) Discover(ctx context.Context, resource *discoveryv1alpha1.DiscoveredService) (Result, error) {
	if resource.Spec.Discovery.DNS == nil {
		return Result{}, fmt.Errorf("DNS discovery configuration is required")
	}
	if p.Resolver == nil {
		return Result{}, fmt.Errorf("DNS resolver is not configured")
	}
	seen := make(map[string]struct{})
	for _, name := range resource.Spec.Discovery.DNS.Names {
		addresses, err := p.Resolver.LookupIPAddr(ctx, name)
		if err != nil {
			return Result{}, fmt.Errorf("resolve DNS name %q: %w", name, err)
		}
		usable := 0
		for _, candidate := range addresses {
			address, ok := netip.AddrFromSlice(candidate.IP)
			if !ok {
				continue
			}
			address = address.Unmap()
			if !address.Is4() {
				continue
			}
			canonical := address.String()
			seen[canonical] = struct{}{}
			usable++
		}
		if usable == 0 {
			return Result{}, fmt.Errorf("DNS name %q returned no usable IPv4 addresses", name)
		}
	}
	endpoints := make([]Endpoint, 0, len(seen))
	for address := range seen {
		endpoints = append(endpoints, Endpoint{Address: address})
	}
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].Address < endpoints[j].Address })
	return Result{Endpoints: endpoints, RequeueAfter: p.RefreshInterval}, nil
}
