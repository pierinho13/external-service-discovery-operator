package discovery

import (
	"context"
	"fmt"
	"net/netip"
	"sort"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
)

type StaticProvider struct{}

func (StaticProvider) Discover(_ context.Context, resource *discoveryv1alpha1.DiscoveredService) (Result, error) {
	if resource.Spec.Discovery.Static == nil {
		return Result{}, fmt.Errorf("static discovery configuration is required")
	}
	seen := make(map[string]struct{}, len(resource.Spec.Discovery.Static.Addresses))
	result := make([]Endpoint, 0, len(resource.Spec.Discovery.Static.Addresses))
	for _, raw := range resource.Spec.Discovery.Static.Addresses {
		address, err := netip.ParseAddr(raw)
		if err != nil || !address.Is4() {
			return Result{}, fmt.Errorf("static address %q is not a valid IPv4 address", raw)
		}
		canonical := address.String()
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		result = append(result, Endpoint{Address: canonical})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Address < result[j].Address })
	return Result{Endpoints: result}, nil
}
