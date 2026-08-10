package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
}

// DiscoveredServiceStatus describes the last completed reconciliation.
type DiscoveredServiceStatus struct {
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	EndpointCount      int32  `json:"endpointCount,omitempty"`
	ServiceName        string `json:"serviceName,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=dsvc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Endpoints",type="integer",JSONPath=".status.endpointCount"
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
