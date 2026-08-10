package discovery

import (
	"context"
	"fmt"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
)

// Resolver selects the concrete provider described by a DiscoveredService.
// New provider branches can be added here without changing the controller.
type Resolver struct {
	Static Provider
	DNS    Provider
}

func (r Resolver) Discover(ctx context.Context, resource *discoveryv1alpha1.DiscoveredService) (Result, error) {
	if resource.Spec.Discovery.Static != nil {
		if r.Static == nil {
			return Result{}, fmt.Errorf("static discovery provider is not configured")
		}
		return r.Static.Discover(ctx, resource)
	}
	if resource.Spec.Discovery.DNS != nil {
		if r.DNS == nil {
			return Result{}, fmt.Errorf("DNS discovery provider is not configured")
		}
		return r.DNS.Discover(ctx, resource)
	}
	return Result{}, fmt.Errorf("no supported discovery provider is configured")
}
