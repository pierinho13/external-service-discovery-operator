package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var GroupVersion = schema.GroupVersion{Group: "discovery.k8sready.com", Version: "v1alpha1"}
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
var AddToScheme = SchemeBuilder.AddToScheme

func Kind(kind string) schema.GroupKind { return GroupVersion.WithKind(kind).GroupKind() }
func Resource(resource string) schema.GroupResource {
	return GroupVersion.WithResource(resource).GroupResource()
}

func init() { SchemeBuilder.Register(&DiscoveredService{}, &DiscoveredServiceList{}) }

var _ = metav1.Condition{}
