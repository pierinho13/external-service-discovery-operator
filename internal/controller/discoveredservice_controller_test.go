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
)

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
