package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
	provider "github.com/pierinho13/external-service-discovery-operator/internal/discovery"
)

const ManagedBy = "external-service-discovery-operator.k8sready.com"
const readyCondition = "Ready"

// ResourceConflictError indicates that a deterministic child name is already
// occupied by a resource not controlled by the current DiscoveredService.
type ResourceConflictError struct {
	Kind      string
	Namespace string
	Name      string
}

func (e *ResourceConflictError) Error() string {
	return fmt.Sprintf("%s %s/%s already exists and is not controlled by this DiscoveredService", e.Kind, e.Namespace, e.Name)
}

type DiscoveredServiceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Provider provider.Provider
}

// +kubebuilder:rbac:groups=discovery.k8sready.com,resources=discoveredservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=discovery.k8sready.com,resources=discoveredservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch;create;update;patch

func (r *DiscoveredServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	resource := &discoveryv1alpha1.DiscoveredService{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	discoveryResult, err := r.Provider.Discover(ctx, resource)
	if err != nil {
		return r.fail(ctx, resource, "DiscoveryFailed", err)
	}
	if err = r.reconcileService(ctx, resource); err != nil {
		return r.fail(ctx, resource, failureReason(err), err)
	}
	if err = r.reconcileEndpointSlice(ctx, resource, discoveryResult.Endpoints); err != nil {
		return r.fail(ctx, resource, failureReason(err), err)
	}
	if err := r.updateStatus(ctx, resource, metav1.ConditionTrue, "Reconciled", "Service and EndpointSlice are reconciled", int32Ptr(int32(len(discoveryResult.Endpoints)))); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: discoveryResult.RequeueAfter}, nil
}

func (r *DiscoveredServiceReconciler) fail(ctx context.Context, resource *discoveryv1alpha1.DiscoveredService, reason string, reconcileErr error) (ctrl.Result, error) {
	statusErr := r.updateStatus(ctx, resource, metav1.ConditionFalse, reason, reconcileErr.Error(), nil)
	if statusErr != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile failed: %v; update status: %w", reconcileErr, statusErr)
	}
	return ctrl.Result{}, reconcileErr
}

func failureReason(err error) string {
	var conflict *ResourceConflictError
	if errors.As(err, &conflict) {
		return "ResourceConflict"
	}
	return "ReconcileFailed"
}

func (r *DiscoveredServiceReconciler) reconcileService(ctx context.Context, owner *discoveryv1alpha1.DiscoveredService) error {
	service := &corev1.Service{}
	key := types.NamespacedName{Name: owner.Name, Namespace: owner.Namespace}
	err := r.Get(ctx, key, service)
	if apierrors.IsNotFound(err) {
		service = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: owner.Name, Namespace: owner.Namespace}}
		if err := r.setServiceDesired(owner, service); err != nil {
			return err
		}
		if err := r.Create(ctx, service); apierrors.IsAlreadyExists(err) {
			return r.reconcileService(ctx, owner)
		} else {
			return err
		}
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(service, owner) {
		return &ResourceConflictError{Kind: "Service", Namespace: owner.Namespace, Name: owner.Name}
	}
	if err := r.setServiceDesired(owner, service); err != nil {
		return err
	}
	return r.Update(ctx, service)
}

func (r *DiscoveredServiceReconciler) setServiceDesired(owner *discoveryv1alpha1.DiscoveredService, service *corev1.Service) error {
	if err := controllerutil.SetControllerReference(owner, service, r.Scheme); err != nil {
		return err
	}
	service.Spec.Selector = nil
	service.Spec.Type = corev1.ServiceTypeClusterIP
	service.Spec.Ports = servicePorts(owner.Spec.Ports)
	return nil
}

func (r *DiscoveredServiceReconciler) reconcileEndpointSlice(ctx context.Context, owner *discoveryv1alpha1.DiscoveredService, found []provider.Endpoint) error {
	slice := &discoveryv1.EndpointSlice{}
	key := types.NamespacedName{Name: owner.Name, Namespace: owner.Namespace}
	err := r.Get(ctx, key, slice)
	if apierrors.IsNotFound(err) {
		slice = &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: owner.Name, Namespace: owner.Namespace}}
		if err := r.setEndpointSliceDesired(owner, slice, found); err != nil {
			return err
		}
		if err := r.Create(ctx, slice); apierrors.IsAlreadyExists(err) {
			return r.reconcileEndpointSlice(ctx, owner, found)
		} else {
			return err
		}
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(slice, owner) {
		return &ResourceConflictError{Kind: "EndpointSlice", Namespace: owner.Namespace, Name: owner.Name}
	}
	if err := r.setEndpointSliceDesired(owner, slice, found); err != nil {
		return err
	}
	return r.Update(ctx, slice)
}

func (r *DiscoveredServiceReconciler) setEndpointSliceDesired(owner *discoveryv1alpha1.DiscoveredService, slice *discoveryv1.EndpointSlice, found []provider.Endpoint) error {
	if err := controllerutil.SetControllerReference(owner, slice, r.Scheme); err != nil {
		return err
	}
	if slice.Labels == nil {
		slice.Labels = map[string]string{}
	}
	slice.Labels[discoveryv1.LabelServiceName] = owner.Name
	slice.Labels[discoveryv1.LabelManagedBy] = ManagedBy
	slice.AddressType = discoveryv1.AddressTypeIPv4
	slice.Ports = endpointSlicePorts(owner.Spec.Ports)
	ready := true
	slice.Endpoints = make([]discoveryv1.Endpoint, 0, len(found))
	for _, endpoint := range found {
		slice.Endpoints = append(slice.Endpoints, discoveryv1.Endpoint{Addresses: []string{endpoint.Address}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}})
	}
	return nil
}

func servicePorts(ports []discoveryv1alpha1.DiscoveredServicePort) []corev1.ServicePort {
	result := make([]corev1.ServicePort, 0, len(ports))
	for _, port := range ports {
		result = append(result, corev1.ServicePort{Name: port.Name, Port: port.Port, TargetPort: intstr.FromInt32(endpointPort(port)), Protocol: protocolOrTCP(port.Protocol)})
	}
	return result
}

func endpointSlicePorts(ports []discoveryv1alpha1.DiscoveredServicePort) []discoveryv1.EndpointPort {
	result := make([]discoveryv1.EndpointPort, 0, len(ports))
	for _, port := range ports {
		name, number, protocol := port.Name, endpointPort(port), protocolOrTCP(port.Protocol)
		result = append(result, discoveryv1.EndpointPort{Name: &name, Port: &number, Protocol: &protocol})
	}
	return result
}

func endpointPort(port discoveryv1alpha1.DiscoveredServicePort) int32 {
	if port.TargetPort != nil {
		return *port.TargetPort
	}
	return port.Port
}

func protocolOrTCP(protocol corev1.Protocol) corev1.Protocol {
	if protocol == "" {
		return corev1.ProtocolTCP
	}
	return protocol
}

func (r *DiscoveredServiceReconciler) updateStatus(ctx context.Context, resource *discoveryv1alpha1.DiscoveredService, conditionStatus metav1.ConditionStatus, reason, message string, count *int32) error {
	latest := &discoveryv1alpha1.DiscoveredService{}
	if err := r.Get(ctx, types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace}, latest); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	latest.Status.ObservedGeneration = latest.Generation
	if count != nil {
		latest.Status.EndpointCount = *count
	}
	latest.Status.ServiceName = latest.Name
	apiMeta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{Type: readyCondition, Status: conditionStatus, Reason: reason, Message: message, ObservedGeneration: latest.Generation})
	return r.Status().Update(ctx, latest)
}

func int32Ptr(value int32) *int32 { return &value }

func (r *DiscoveredServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&discoveryv1alpha1.DiscoveredService{}).Owns(&corev1.Service{}).Owns(&discoveryv1.EndpointSlice{}).Complete(r)
}
