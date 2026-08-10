package envtest_test

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
	"github.com/pierinho13/external-service-discovery-operator/internal/controller"
	"github.com/pierinho13/external-service-discovery-operator/internal/discovery"
)

type mutableDNSResolver struct {
	addresses []net.IPAddr
	err       error
}

func (r *mutableDNSResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	if r.err != nil {
		return nil, r.err
	}
	return append([]net.IPAddr(nil), r.addresses...), nil
}

func (r *mutableDNSResolver) setAddresses(addresses ...string) {
	r.err = nil
	r.addresses = make([]net.IPAddr, 0, len(addresses))
	for _, address := range addresses {
		r.addresses = append(r.addresses, net.IPAddr{IP: net.ParseIP(address)})
	}
}

//nolint:gocyclo // One expensive envtest control plane is shared by focused integration subtests.
func TestDiscoveredServiceIntegration(t *testing.T) {
	testEnvironment := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")}, ErrorIfCRDPathMissing: true}
	config, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	must(t, corev1.AddToScheme(scheme))
	must(t, discoveryv1.AddToScheme(scheme))
	must(t, discoveryv1alpha1.AddToScheme(scheme))
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	must(t, err)
	ctx := context.Background()
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "discovery-integration-"}}
	must(t, k8sClient.Create(ctx, namespace))

	t.Run("CRD validation rejects invalid discovery and ports", func(t *testing.T) {
		invalidDiscovery := validResource("invalid-discovery", namespace.Name)
		invalidDiscovery.Spec.Discovery.Static = nil
		if err := k8sClient.Create(ctx, invalidDiscovery); err == nil || !apierrors.IsInvalid(err) {
			t.Fatalf("expected invalid discovery rejection, got %v", err)
		}

		invalidPort := validResource("invalid-port", namespace.Name)
		invalidPort.Spec.Ports[0].Port = 0
		if err := k8sClient.Create(ctx, invalidPort); err == nil || !apierrors.IsInvalid(err) {
			t.Fatalf("expected invalid port rejection, got %v", err)
		}

		invalidTarget := validResource("invalid-target", namespace.Name)
		invalid := int32(65536)
		invalidTarget.Spec.Ports[0].TargetPort = &invalid
		if err := k8sClient.Create(ctx, invalidTarget); err == nil || !apierrors.IsInvalid(err) {
			t.Fatalf("expected invalid targetPort rejection, got %v", err)
		}

		emptyDNSName := validResource("empty-dns-name", namespace.Name)
		emptyDNSName.Spec.Discovery = discoveryv1alpha1.DiscoveryProvider{
			DNS: &discoveryv1alpha1.DNSDiscovery{Names: []string{""}},
		}
		if err := k8sClient.Create(ctx, emptyDNSName); err == nil || !apierrors.IsInvalid(err) {
			t.Fatalf("expected empty DNS name rejection, got %v", err)
		}

		duplicatePorts := validResource("duplicate-ports", namespace.Name)
		duplicatePorts.Spec.Ports = []discoveryv1alpha1.DiscoveredServicePort{
			{Name: "http", Port: 80},
			{Name: "http", Port: 8080},
		}
		if err := k8sClient.Create(ctx, duplicatePorts); err == nil || !apierrors.IsInvalid(err) {
			t.Fatalf("expected duplicate port name rejection, got %v", err)
		}
	})

	t.Run("CRD accepts DNS and enforces exactly one provider", func(t *testing.T) {
		dns := validResource("valid-dns", namespace.Name)
		dns.Spec.Discovery = discoveryv1alpha1.DiscoveryProvider{DNS: &discoveryv1alpha1.DNSDiscovery{Names: []string{"tomcat.internal.example.com"}}}
		must(t, k8sClient.Create(ctx, dns))

		both := validResource("both-providers", namespace.Name)
		both.Spec.Discovery.DNS = &discoveryv1alpha1.DNSDiscovery{Names: []string{"tomcat.internal.example.com"}}
		if err := k8sClient.Create(ctx, both); err == nil || !apierrors.IsInvalid(err) {
			t.Fatalf("expected static plus DNS rejection, got %v", err)
		}

		neither := validResource("no-provider", namespace.Name)
		neither.Spec.Discovery = discoveryv1alpha1.DiscoveryProvider{}
		if err := k8sClient.Create(ctx, neither); err == nil || !apierrors.IsInvalid(err) {
			t.Fatalf("expected missing provider rejection, got %v", err)
		}
	})

	t.Run("real API reconciliation persists children ownership status and targetPort", func(t *testing.T) {
		resource := validResource("tomcat-erp", namespace.Name)
		target := int32(8080)
		resource.Spec.Ports[0].Port = 80
		resource.Spec.Ports[0].TargetPort = &target
		must(t, k8sClient.Create(ctx, resource))
		reconciler := &controller.DiscoveredServiceReconciler{Client: k8sClient, Scheme: scheme, Provider: discovery.Resolver{Static: discovery.StaticProvider{}}}
		request := ctrl.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}}
		_, err := reconciler.Reconcile(ctx, request)
		must(t, err)

		created := &discoveryv1alpha1.DiscoveredService{}
		must(t, k8sClient.Get(ctx, request.NamespacedName, created))
		if created.UID == "" || created.Status.EndpointCount != 2 || created.Status.ServiceName != created.Name {
			t.Fatalf("unexpected persisted status: %#v", created.Status)
		}
		ready := findCondition(created.Status.Conditions, "Ready")
		if ready == nil || ready.Status != metav1.ConditionTrue || ready.ObservedGeneration != created.Generation {
			t.Fatalf("unexpected Ready condition: %#v", ready)
		}

		service := &corev1.Service{}
		must(t, k8sClient.Get(ctx, request.NamespacedName, service))
		if service.Spec.Selector != nil || service.Spec.Ports[0].Port != 80 || service.Spec.Ports[0].TargetPort.IntVal != 8080 {
			t.Fatalf("unexpected Service: %#v", service.Spec)
		}
		assertControlled(t, service.OwnerReferences, created.UID)

		slice := &discoveryv1.EndpointSlice{}
		must(t, k8sClient.Get(ctx, request.NamespacedName, slice))
		if len(slice.Endpoints) != 2 || *slice.Ports[0].Port != 8080 {
			t.Fatalf("unexpected EndpointSlice: %#v", slice)
		}
		assertControlled(t, slice.OwnerReferences, created.UID)
	})

	t.Run("DNS lifecycle updates addresses and fails closed", func(t *testing.T) {
		testDNSLifecycle(t, ctx, k8sClient, scheme, namespace.Name)
	})

	t.Run("preexisting Service is never adopted", func(t *testing.T) {
		preexisting := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "conflict", Namespace: namespace.Name}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "manual", Port: 9000}}}}
		must(t, k8sClient.Create(ctx, preexisting))
		resource := validResource("conflict", namespace.Name)
		must(t, k8sClient.Create(ctx, resource))
		reconciler := &controller.DiscoveredServiceReconciler{Client: k8sClient, Scheme: scheme, Provider: discovery.Resolver{Static: discovery.StaticProvider{}}}
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}})
		if err == nil {
			t.Fatal("expected resource conflict")
		}
		got := &corev1.Service{}
		must(t, k8sClient.Get(ctx, types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}, got))
		if len(got.OwnerReferences) != 0 || got.Spec.Ports[0].Port != 9000 {
			t.Fatalf("preexisting Service was changed: %#v", got)
		}
		latest := &discoveryv1alpha1.DiscoveredService{}
		must(t, k8sClient.Get(ctx, types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}, latest))
		condition := findCondition(latest.Status.Conditions, "Ready")
		if condition == nil || condition.Reason != "ResourceConflict" || condition.Status != metav1.ConditionFalse {
			t.Fatalf("unexpected conflict status: %#v", condition)
		}
	})
}

func testDNSLifecycle(t *testing.T, ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, namespace string) {
	t.Helper()
	resolver := &mutableDNSResolver{}
	resolver.setAddresses("10.140.0.11", "10.140.0.12")
	refreshInterval := 43 * time.Second
	resource := validResource("tomcat-dns-lifecycle", namespace)
	resource.Spec.Discovery = discoveryv1alpha1.DiscoveryProvider{
		DNS: &discoveryv1alpha1.DNSDiscovery{Names: []string{"tomcat.internal.example.com"}},
	}
	must(t, k8sClient.Create(ctx, resource))
	reconciler := &controller.DiscoveredServiceReconciler{
		Client: k8sClient,
		Scheme: scheme,
		Provider: discovery.Resolver{DNS: discovery.DNSProvider{
			Resolver: resolver, RefreshInterval: refreshInterval,
		}},
	}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}}
	result, err := reconciler.Reconcile(ctx, request)
	must(t, err)
	if result.RequeueAfter != refreshInterval {
		t.Fatalf("expected refresh %s, got %s", refreshInterval, result.RequeueAfter)
	}
	service := &corev1.Service{}
	must(t, k8sClient.Get(ctx, request.NamespacedName, service))
	servicePorts := append([]corev1.ServicePort(nil), service.Spec.Ports...)
	slice := &discoveryv1.EndpointSlice{}
	must(t, k8sClient.Get(ctx, request.NamespacedName, slice))
	assertEndpointAddresses(t, slice, "10.140.0.11", "10.140.0.12")
	assertEndpointsReady(t, slice)
	assertReadyStatus(t, k8sClient, request.NamespacedName, metav1.ConditionTrue, "Reconciled", 2)

	resolver.setAddresses("10.140.0.11", "10.140.0.57")
	result, err = reconciler.Reconcile(ctx, request)
	must(t, err)
	if result.RequeueAfter != refreshInterval {
		t.Fatalf("expected refresh %s, got %s", refreshInterval, result.RequeueAfter)
	}
	must(t, k8sClient.Get(ctx, request.NamespacedName, slice))
	assertEndpointAddresses(t, slice, "10.140.0.11", "10.140.0.57")
	assertReadyStatus(t, k8sClient, request.NamespacedName, metav1.ConditionTrue, "Reconciled", 2)

	resolver.err = errors.New("temporary DNS failure")
	if _, err := reconciler.Reconcile(ctx, request); err == nil {
		t.Fatal("expected DNS reconciliation failure")
	}
	must(t, k8sClient.Get(ctx, request.NamespacedName, slice))
	assertEndpointAddresses(t, slice, "10.140.0.11", "10.140.0.57")
	must(t, k8sClient.Get(ctx, request.NamespacedName, service))
	if !reflect.DeepEqual(service.Spec.Ports, servicePorts) {
		t.Fatalf("Service changed after DNS failure: %#v", service.Spec.Ports)
	}
	assertReadyStatus(t, k8sClient, request.NamespacedName, metav1.ConditionFalse, "DiscoveryFailed", 2)
}

func validResource(name, namespace string) *discoveryv1alpha1.DiscoveredService {
	return &discoveryv1alpha1.DiscoveredService{TypeMeta: metav1.TypeMeta{APIVersion: discoveryv1alpha1.GroupVersion.String(), Kind: "DiscoveredService"}, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Spec: discoveryv1alpha1.DiscoveredServiceSpec{Discovery: discoveryv1alpha1.DiscoveryProvider{Static: &discoveryv1alpha1.StaticDiscovery{Addresses: []string{"10.0.0.1", "10.0.0.2"}}}, Ports: []discoveryv1alpha1.DiscoveredServicePort{{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP}}}}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
func assertControlled(t *testing.T, references []metav1.OwnerReference, uid types.UID) {
	t.Helper()
	if len(references) != 1 || references[0].UID != uid || references[0].Controller == nil || !*references[0].Controller {
		t.Fatalf("unexpected ownerReferences: %#v", references)
	}
}

func assertEndpointAddresses(t *testing.T, slice *discoveryv1.EndpointSlice, want ...string) {
	t.Helper()
	got := make([]string, 0, len(slice.Endpoints))
	for _, endpoint := range slice.Endpoints {
		got = append(got, endpoint.Addresses...)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected EndpointSlice addresses: got %v, want %v", got, want)
	}
}

func assertEndpointsReady(t *testing.T, slice *discoveryv1.EndpointSlice) {
	t.Helper()
	for _, endpoint := range slice.Endpoints {
		if endpoint.Conditions.Ready == nil || !*endpoint.Conditions.Ready {
			t.Fatalf("endpoint is not ready: %#v", endpoint)
		}
	}
}

func assertReadyStatus(t *testing.T, k8sClient client.Client, key types.NamespacedName, status metav1.ConditionStatus, reason string, count int32) {
	t.Helper()
	resource := &discoveryv1alpha1.DiscoveredService{}
	must(t, k8sClient.Get(context.Background(), key, resource))
	condition := findCondition(resource.Status.Conditions, "Ready")
	if condition == nil || condition.Status != status || condition.Reason != reason || resource.Status.EndpointCount != count {
		t.Fatalf("unexpected status: %#v", resource.Status)
	}
}
