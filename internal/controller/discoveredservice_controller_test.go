package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestReconcileCreatesAndUpdatesNetworkingResources(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	must(t, corev1.AddToScheme(scheme))
	must(t, discoveryv1.AddToScheme(scheme))
	must(t, discoveryv1alpha1.AddToScheme(scheme))
	resource := &discoveryv1alpha1.DiscoveredService{
		TypeMeta:   metav1.TypeMeta{APIVersion: discoveryv1alpha1.GroupVersion.String(), Kind: "DiscoveredService"},
		ObjectMeta: metav1.ObjectMeta{Name: "tomcat-erp", Namespace: "default", UID: types.UID("test-uid"), Generation: 1},
		Spec:       discoveryv1alpha1.DiscoveredServiceSpec{Discovery: discoveryv1alpha1.DiscoveryProvider{Static: &discoveryv1alpha1.StaticDiscovery{Addresses: []string{"10.140.0.11", "10.140.0.12"}}}, Ports: []discoveryv1alpha1.DiscoveredServicePort{{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP}}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(resource).WithObjects(resource).Build()
	reconciler := &DiscoveredServiceReconciler{Client: client, Scheme: scheme, Provider: provider.StaticProvider{}}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}}
	mustReconcile(t, reconciler, ctx, request)

	service := &corev1.Service{}
	must(t, client.Get(ctx, request.NamespacedName, service))
	if service.Spec.Selector != nil {
		t.Fatalf("Service must be selectorless: %#v", service.Spec.Selector)
	}
	if service.Spec.Type != corev1.ServiceTypeClusterIP || len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != 8080 {
		t.Fatalf("unexpected Service: %#v", service.Spec)
	}
	assertOwner(t, service.OwnerReferences, resource.UID)

	slice := &discoveryv1.EndpointSlice{}
	must(t, client.Get(ctx, request.NamespacedName, slice))
	if slice.Labels[discoveryv1.LabelServiceName] != resource.Name || slice.Labels[discoveryv1.LabelManagedBy] != ManagedBy {
		t.Fatalf("unexpected labels: %#v", slice.Labels)
	}
	if len(slice.Endpoints) != 2 || slice.Endpoints[0].Addresses[0] != "10.140.0.11" || slice.Endpoints[0].Conditions.Ready == nil || !*slice.Endpoints[0].Conditions.Ready {
		t.Fatalf("unexpected endpoints: %#v", slice.Endpoints)
	}
	assertOwner(t, slice.OwnerReferences, resource.UID)

	current := &discoveryv1alpha1.DiscoveredService{}
	must(t, client.Get(ctx, request.NamespacedName, current))
	condition := findReady(current.Status.Conditions)
	if current.Status.EndpointCount != 2 || current.Status.ServiceName != resource.Name || condition == nil || condition.Status != metav1.ConditionTrue {
		t.Fatalf("unexpected status: %#v", current.Status)
	}

	current.Spec.Discovery.Static.Addresses = []string{"10.140.0.11", "10.140.0.13"}
	targetPort := int32(9443)
	current.Spec.Ports = []discoveryv1alpha1.DiscoveredServicePort{{Name: "https", Port: 8443, TargetPort: &targetPort, Protocol: corev1.ProtocolTCP}}
	must(t, client.Update(ctx, current))
	mustReconcile(t, reconciler, ctx, request)
	must(t, client.Get(ctx, request.NamespacedName, service))
	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Name != "https" || service.Spec.Ports[0].Port != 8443 || service.Spec.Ports[0].TargetPort.IntVal != 9443 {
		t.Fatalf("Service ports not updated: %#v", service.Spec.Ports)
	}
	must(t, client.Get(ctx, request.NamespacedName, slice))
	if len(slice.Endpoints) != 2 || slice.Endpoints[1].Addresses[0] != "10.140.0.13" || *slice.Ports[0].Name != "https" || *slice.Ports[0].Port != 9443 {
		t.Fatalf("EndpointSlice not updated: %#v", slice)
	}

	// Child watches enqueue the owner; a deleted child is recreated by the same idempotent reconciliation.
	must(t, client.Delete(ctx, service))
	mustReconcile(t, reconciler, ctx, request)
	must(t, client.Get(ctx, request.NamespacedName, service))

	assertEndpointSliceRecreationAndDrift(t, ctx, client, reconciler, request, resource, service, slice)
}

func assertEndpointSliceRecreationAndDrift(t *testing.T, ctx context.Context, client client.Client, reconciler *DiscoveredServiceReconciler, request ctrl.Request, resource *discoveryv1alpha1.DiscoveredService, service *corev1.Service, slice *discoveryv1.EndpointSlice) {
	t.Helper()
	// EndpointSlices are also recreated with their complete desired state.
	must(t, client.Delete(ctx, slice))
	mustReconcile(t, reconciler, ctx, request)
	must(t, client.Get(ctx, request.NamespacedName, slice))
	if slice.Labels[discoveryv1.LabelServiceName] != resource.Name || slice.Labels[discoveryv1.LabelManagedBy] != ManagedBy {
		t.Fatalf("unexpected recreated EndpointSlice labels: %#v", slice.Labels)
	}
	assertOwner(t, slice.OwnerReferences, resource.UID)
	if len(slice.Ports) != 1 || *slice.Ports[0].Name != "https" || *slice.Ports[0].Port != 9443 || len(slice.Endpoints) != 2 {
		t.Fatalf("unexpected recreated EndpointSlice: %#v", slice)
	}

	// Drift in correctly owned children is restored to the declared and discovered state.
	must(t, client.Get(ctx, request.NamespacedName, service))
	service.Spec.Ports[0].Port = 9999
	must(t, client.Update(ctx, service))
	driftPort := int32(7777)
	slice.Ports[0].Port = &driftPort
	slice.Endpoints = []discoveryv1.Endpoint{{Addresses: []string{"192.0.2.1"}}}
	must(t, client.Update(ctx, slice))
	mustReconcile(t, reconciler, ctx, request)
	must(t, client.Get(ctx, request.NamespacedName, service))
	must(t, client.Get(ctx, request.NamespacedName, slice))
	if service.Spec.Ports[0].Port != 8443 || *slice.Ports[0].Port != 9443 {
		t.Fatalf("managed port drift was not restored: Service=%#v EndpointSlice=%#v", service.Spec.Ports, slice.Ports)
	}
	if len(slice.Endpoints) != 2 || slice.Endpoints[0].Addresses[0] != "10.140.0.11" || slice.Endpoints[1].Addresses[0] != "10.140.0.13" {
		t.Fatalf("managed endpoint drift was not restored: %#v", slice.Endpoints)
	}
}

func TestReconcileDoesNotAdoptExistingService(t *testing.T) {
	resource, scheme := testResourceAndScheme(t, "service-conflict")
	existing := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: resource.Name, Namespace: resource.Namespace}}
	testClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(resource).WithObjects(resource, existing).Build()
	reconciler := &DiscoveredServiceReconciler{Client: testClient, Scheme: scheme, Provider: provider.StaticProvider{}}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}})
	var conflict *ResourceConflictError
	if !errors.As(err, &conflict) || conflict.Kind != "Service" {
		t.Fatalf("expected Service conflict, got %v", err)
	}
	got := &corev1.Service{}
	must(t, testClient.Get(context.Background(), types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}, got))
	if len(got.OwnerReferences) != 0 {
		t.Fatalf("existing Service was adopted: %#v", got.OwnerReferences)
	}
	assertReadyReason(t, testClient, resource, "ResourceConflict")
}

func TestReconcileDoesNotAdoptExistingEndpointSlice(t *testing.T) {
	resource, scheme := testResourceAndScheme(t, "slice-conflict")
	controlled := true
	owner := metav1.OwnerReference{APIVersion: discoveryv1alpha1.GroupVersion.String(), Kind: "DiscoveredService", Name: resource.Name, UID: resource.UID, Controller: &controlled}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: resource.Name, Namespace: resource.Namespace, OwnerReferences: []metav1.OwnerReference{owner}}}
	existing := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: resource.Name, Namespace: resource.Namespace}, AddressType: discoveryv1.AddressTypeIPv4}
	testClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(resource).WithObjects(resource, service, existing).Build()
	reconciler := &DiscoveredServiceReconciler{Client: testClient, Scheme: scheme, Provider: provider.StaticProvider{}}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}})
	var conflict *ResourceConflictError
	if !errors.As(err, &conflict) || conflict.Kind != "EndpointSlice" {
		t.Fatalf("expected EndpointSlice conflict, got %v", err)
	}
	got := &discoveryv1.EndpointSlice{}
	must(t, testClient.Get(context.Background(), types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}, got))
	if len(got.OwnerReferences) != 0 {
		t.Fatalf("existing EndpointSlice was adopted: %#v", got.OwnerReferences)
	}
	assertReadyReason(t, testClient, resource, "ResourceConflict")
}

type failingProvider struct{}

func (failingProvider) Discover(context.Context, *discoveryv1alpha1.DiscoveredService) (provider.Result, error) {
	return provider.Result{}, errors.New("provider unavailable")
}

type scheduledProvider struct{ interval time.Duration }

func (p scheduledProvider) Discover(context.Context, *discoveryv1alpha1.DiscoveredService) (provider.Result, error) {
	return provider.Result{Endpoints: []provider.Endpoint{{Address: "10.0.0.1"}}, RequeueAfter: p.interval}, nil
}

func TestReconcileUsesProviderRefreshSchedule(t *testing.T) {
	resource, scheme := testResourceAndScheme(t, "scheduled")
	testClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(resource).WithObjects(resource).Build()
	interval := 45 * time.Second
	reconciler := &DiscoveredServiceReconciler{Client: testClient, Scheme: scheme, Provider: scheduledProvider{interval: interval}}
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}})
	must(t, err)
	if result.RequeueAfter != interval {
		t.Fatalf("expected requeue after %s, got %s", interval, result.RequeueAfter)
	}
}

func TestHealthChecksPersistThresholdsAndEndpointReadiness(t *testing.T) {
	resource, scheme := testResourceAndScheme(t, "health-checked")
	resource.Spec.Discovery.Static.Addresses = []string{"10.0.0.1", "10.0.0.2"}
	resource.Spec.HealthCheck = &discoveryv1alpha1.HealthCheck{Type: discoveryv1alpha1.HealthCheckTypeHTTP, Port: 8080, Path: "/estasbien", FailureThreshold: 2, SuccessThreshold: 1, Interval: metav1.Duration{Duration: 7 * time.Second}}
	checker := &sequenceChecker{results: map[string][]healthcheck.Result{
		"10.0.0.1": {{Healthy: true, Reason: "ProbeSucceeded"}, {Healthy: true, Reason: "ProbeSucceeded"}, {Healthy: true, Reason: "ProbeSucceeded"}, {Healthy: true, Reason: "ProbeSucceeded"}},
		"10.0.0.2": {{Healthy: true, Reason: "ProbeSucceeded"}, {Reason: "ProbeFailed"}, {Reason: "ProbeFailed"}, {Healthy: true, Reason: "ProbeSucceeded"}},
	}}
	testClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(resource).WithObjects(resource).Build()
	reconciler := &DiscoveredServiceReconciler{Client: testClient, Scheme: scheme, Provider: provider.StaticProvider{}, Checker: checker}
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
	if len(slice.Endpoints) != 2 || slice.Endpoints[0].Conditions.Ready == nil || !*slice.Endpoints[0].Conditions.Ready || slice.Endpoints[1].Conditions.Ready == nil || *slice.Endpoints[1].Conditions.Ready {
		t.Fatalf("unexpected endpoint readiness: %#v", slice.Endpoints)
	}
	latest := &discoveryv1alpha1.DiscoveredService{}
	must(t, testClient.Get(context.Background(), request.NamespacedName, latest))
	if latest.Status.EndpointCount != 2 || latest.Status.ReadyEndpointCount != 1 || len(latest.Status.EndpointHealth) != 2 || latest.Status.EndpointHealth[1].ConsecutiveFailures != 2 {
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

func TestDiscoveryFailurePreservesLastMaterializedEndpointCount(t *testing.T) {
	resource, scheme := testResourceAndScheme(t, "discovery-failure")
	resource.Status.EndpointCount = 2
	resource.Status.ServiceName = resource.Name
	testClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(resource).WithObjects(resource).Build()
	reconciler := &DiscoveredServiceReconciler{Client: testClient, Scheme: scheme, Provider: failingProvider{}}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}})
	if err == nil {
		t.Fatal("expected discovery error")
	}
	latest := &discoveryv1alpha1.DiscoveredService{}
	must(t, testClient.Get(context.Background(), types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}, latest))
	if latest.Status.EndpointCount != 2 {
		t.Fatalf("endpointCount changed after failed discovery: %d", latest.Status.EndpointCount)
	}
	condition := findReady(latest.Status.Conditions)
	if condition == nil || condition.Reason != "DiscoveryFailed" || condition.Status != metav1.ConditionFalse {
		t.Fatalf("unexpected failure condition: %#v", condition)
	}
}

func testResourceAndScheme(t *testing.T, name string) (*discoveryv1alpha1.DiscoveredService, *runtime.Scheme) {
	t.Helper()
	scheme := runtime.NewScheme()
	must(t, corev1.AddToScheme(scheme))
	must(t, discoveryv1.AddToScheme(scheme))
	must(t, discoveryv1alpha1.AddToScheme(scheme))
	resource := &discoveryv1alpha1.DiscoveredService{TypeMeta: metav1.TypeMeta{APIVersion: discoveryv1alpha1.GroupVersion.String(), Kind: "DiscoveredService"}, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name + "-uid")}, Spec: discoveryv1alpha1.DiscoveredServiceSpec{Discovery: discoveryv1alpha1.DiscoveryProvider{Static: &discoveryv1alpha1.StaticDiscovery{Addresses: []string{"10.0.0.1"}}}, Ports: []discoveryv1alpha1.DiscoveredServicePort{{Name: "http", Port: 80}}}}
	return resource, scheme
}

func assertReadyReason(t *testing.T, c client.Client, resource *discoveryv1alpha1.DiscoveredService, reason string) {
	t.Helper()
	latest := &discoveryv1alpha1.DiscoveredService{}
	must(t, c.Get(context.Background(), types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}, latest))
	condition := findReady(latest.Status.Conditions)
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != reason {
		t.Fatalf("unexpected Ready condition: %#v", condition)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func mustReconcile(t *testing.T, r *DiscoveredServiceReconciler, ctx context.Context, req ctrl.Request) {
	t.Helper()
	_, err := r.Reconcile(ctx, req)
	must(t, err)
}
func assertOwner(t *testing.T, refs []metav1.OwnerReference, uid types.UID) {
	t.Helper()
	if len(refs) != 1 || refs[0].UID != uid || refs[0].Controller == nil || !*refs[0].Controller {
		t.Fatalf("unexpected ownerReferences: %#v", refs)
	}
}
func findReady(conditions []metav1.Condition) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == readyCondition {
			return &conditions[i]
		}
	}
	return nil
}
