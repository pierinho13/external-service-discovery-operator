package discovery

import (
	"context"
	"time"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
)

type Endpoint struct{ Address string }

// Result is the normalized discovery output and its refresh policy. A zero
// RequeueAfter means Kubernetes events alone trigger reconciliation.
type Result struct {
	Endpoints    []Endpoint
	RequeueAfter time.Duration
}

type Provider interface {
	Discover(context.Context, *discoveryv1alpha1.DiscoveredService) (Result, error)
}
