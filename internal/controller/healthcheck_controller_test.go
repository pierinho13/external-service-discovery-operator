package controller

import (
	"context"
	"testing"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
	provider "github.com/pierinho13/external-service-discovery-operator/internal/discovery"
	healthcheck "github.com/pierinho13/external-service-discovery-operator/internal/health"
)

type sequenceChecker struct {
	results map[string][]healthcheck.Result
}

func (c *sequenceChecker) Check(_ context.Context, address string, _ *discoveryv1alpha1.HealthCheck) healthcheck.Result {
	values := c.results[address]
	if len(values) == 0 {
		return healthcheck.Result{Reason: "ProbeFailed", Message: "no result configured"}
	}
	result := values[0]
	c.results[address] = values[1:]
	return result
}

func TestHealthChecksPersistThresholdsAndEndpointReadiness(t *testing.T) {
	resource, scheme := testResourceAndScheme(t, "health-checked")
	resource.Spec.Discovery.Static.Addresses = []string{"10.0.0.1", "10.0.0.2"}
	resource.Spec.HealthCheck = &discoveryv1alpha1.HealthCheck{
		Type:             discoveryv1alpha1.HealthCheckTypeHTTP,
		Port:             8080,
		Path:             "/healthz",
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Interval:         metav1.Duration{Duration: 7 * time.Second},
	}
	checker := &sequenceChecker{results: map[string][]healthcheck.Result{
		"10.0.0.1": {
			{Healthy: true, Reason: "ProbeSucceeded"},
			{Healthy: true, Reason: "ProbeSucceeded"},
			{Healthy: true, Reason: "ProbeSucceeded"},
			{Healthy: true, Reason: "ProbeSucceeded"},
		},
		"10.0.0.2": {
			{Healthy: true, Reason: "ProbeSucceeded"},
			{Reason: "ProbeFailed"},
			{Reason: "ProbeFailed"},
			{Healthy: true, Reason: "ProbeSucceeded"},
		},
	}}
	testClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(resource).WithObjects(resource).Build()
	reconciler := &DiscoveredServiceReconciler{
		Client:   testClient,
		Scheme:   scheme,
		Provider: provider.StaticProvider{},
		Checker:  checker,
	}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}}

	for attempt := 1; attempt <= 3; attempt++ {
		result, err := reconciler.Reconcile(context.Background(), request)
		must(t, err)
		if result.RequeueAfter != 7*time.Second {
			t.Fatalf("unexpected health refresh: %s", result.RequeueAfter)
		}
	}

	slice := &discoveryv1.EndpointSlice{}
	must(t, testClient.Get(context.Background(), request.NamespacedName, slice))
	if len(slice.Endpoints) != 2 ||
		slice.Endpoints[0].Conditions.Ready == nil || !*slice.Endpoints[0].Conditions.Ready ||
		slice.Endpoints[1].Conditions.Ready == nil || *slice.Endpoints[1].Conditions.Ready {
		t.Fatalf("unexpected endpoint readiness: %#v", slice.Endpoints)
	}

	latest := &discoveryv1alpha1.DiscoveredService{}
	must(t, testClient.Get(context.Background(), request.NamespacedName, latest))
	if latest.Status.EndpointCount != 2 ||
		latest.Status.ReadyEndpointCount != 1 ||
		len(latest.Status.EndpointHealth) != 2 ||
		latest.Status.EndpointHealth[1].ConsecutiveFailures != 2 {
		t.Fatalf("unexpected health status: %#v", latest.Status)
	}
	condition := findReady(latest.Status.Conditions)
	if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != "PartiallyHealthy" {
		t.Fatalf("unexpected condition: %#v", condition)
	}

	// A successful probe makes the endpoint ready again after successThreshold.
	_, err := reconciler.Reconcile(context.Background(), request)
	must(t, err)
	must(t, testClient.Get(context.Background(), request.NamespacedName, slice))
	if slice.Endpoints[1].Conditions.Ready == nil || !*slice.Endpoints[1].Conditions.Ready {
		t.Fatalf("recovered endpoint did not become ready: %#v", slice.Endpoints[1])
	}
}
