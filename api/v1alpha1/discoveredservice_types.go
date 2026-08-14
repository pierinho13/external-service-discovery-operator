package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type HealthCheckType string

const (
	HealthCheckTypeHTTP  HealthCheckType = "HTTP"
	HealthCheckTypeHTTPS HealthCheckType = "HTTPS"
	HealthCheckTypeTCP   HealthCheckType = "TCP"
)

// HealthCheck configures active readiness checks for every discovered address.
// +kubebuilder:validation:XValidation:rule="self.type == 'TCP' ? !has(self.path) && !has(self.host) && !has(self.expectedStatuses) : true",message="path, host and expectedStatuses are only valid for HTTP and HTTPS health checks"
type HealthCheck struct {
	// +kubebuilder:validation:Enum=HTTP;HTTPS;TCP
	Type HealthCheckType `json:"type"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +optional
	// +kubebuilder:validation:Pattern=`^/`
	Path string `json:"path,omitempty"`
	// Host is the optional HTTP Host header and HTTPS TLS server name.
	// +optional
	Host string `json:"host,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Minimum=100
	// +kubebuilder:validation:items:Maximum=599
	ExpectedStatuses []int32 `json:"expectedStatuses,omitempty"`
	// +optional
	// +kubebuilder:default="10s"
	Interval metav1.Duration `json:"interval,omitempty"`
	// +optional
	// +kubebuilder:default="3s"
	Timeout metav1.Duration `json:"timeout,omitempty"`
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	SuccessThreshold int32 `json:"successThreshold,omitempty"`
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
}

// StaticDiscovery contains fixed IPv4 addresses. Other providers will be added later.
type StaticDiscovery struct {
	// Addresses are IPv4 addresses of external workloads.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Format=ipv4
	Addresses []string `json:"addresses"`
}

// DNSDiscovery contains hostnames resolved to external workload IPv4 addresses.
type DNSDiscovery struct {
	// Names are hostnames resolved using the operator's system DNS resolver.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	Names []string `json:"names"`
}

// DiscoveryProvider selects the source used to discover endpoints.
// +kubebuilder:validation:XValidation:rule="has(self.static) != has(self.dns)",message="exactly one of static or dns must be configured"
type DiscoveryProvider struct {
	// Static supplies fixed endpoints.
	// +optional
	Static *StaticDiscovery `json:"static,omitempty"`

	// DNS resolves hostnames to IPv4 endpoints.
	// +optional
	DNS *DNSDiscovery `json:"dns,omitempty"`
}

// DiscoveredServicePort is a port projected onto the Service and EndpointSlice.
type DiscoveredServicePort struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=15
	// +kubebuilder:validation:Pattern=`^[a-z]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// TargetPort is the port exposed by the external workload. It defaults
	// logically to Port when omitted.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	TargetPort *int32 `json:"targetPort,omitempty"`
	// +kubebuilder:validation:Enum=TCP;UDP;SCTP
	// +kubebuilder:default=TCP
	Protocol corev1.Protocol `json:"protocol,omitempty"`
}

// DiscoveredServiceSpec defines the desired discovery and networking projection.
type DiscoveredServiceSpec struct {
	Discovery DiscoveryProvider `json:"discovery"`
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=name
	Ports []DiscoveredServicePort `json:"ports"`
	// +optional
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`
}

// EndpointHealthStatus contains persisted readiness for one address.
type EndpointHealthStatus struct {
	Address              string      `json:"address"`
	Healthy              bool        `json:"healthy"`
	ConsecutiveSuccesses int32       `json:"consecutiveSuccesses,omitempty"`
	ConsecutiveFailures  int32       `json:"consecutiveFailures,omitempty"`
	Reason               string      `json:"reason,omitempty"`
	Message              string      `json:"message,omitempty"`
	LastCheckedAt        metav1.Time `json:"lastCheckedAt,omitempty"`
}

// DiscoveredServiceStatus describes the last completed reconciliation.
type DiscoveredServiceStatus struct {
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	EndpointCount      int32  `json:"endpointCount,omitempty"`
	ReadyEndpointCount int32  `json:"readyEndpointCount,omitempty"`
	ServiceName        string `json:"serviceName,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=address
	EndpointHealth []EndpointHealthStatus `json:"endpointHealth,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=dsvc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Endpoints",type="integer",JSONPath=".status.endpointCount"
// +kubebuilder:printcolumn:name="Healthy",type="integer",JSONPath=".status.readyEndpointCount"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type DiscoveredService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiscoveredServiceSpec   `json:"spec,omitempty"`
	Status            DiscoveredServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DiscoveredServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiscoveredService `json:"items"`
}
