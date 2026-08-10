package main

import (
	"fmt"
	"io"
	"os"
	"slices"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type rbacDocument struct {
	Kind  string              `json:"kind"`
	Rules []rbacv1.PolicyRule `json:"rules"`
}

type requiredPermission struct {
	group    string
	resource string
	verbs    []string
}

var requiredPermissions = []requiredPermission{
	{group: "", resource: "events", verbs: []string{"create", "patch"}},
	{group: "", resource: "services", verbs: []string{"create", "get", "list", "patch", "update", "watch"}},
	{
		group: "discovery.k8s.io", resource: "endpointslices",
		verbs: []string{"create", "get", "list", "patch", "update", "watch"},
	},
	{group: "discovery.k8sready.com", resource: "discoveredservices", verbs: []string{"get", "list", "watch"}},
	{group: "discovery.k8sready.com", resource: "discoveredservices/status", verbs: []string{"get", "patch", "update"}},
	{
		group: "coordination.k8s.io", resource: "leases",
		verbs: []string{"create", "get", "list", "patch", "update", "watch"},
	},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: verify-rbac <rendered-yaml> [rendered-yaml...]")
		os.Exit(2)
	}

	rules := make([]rbacv1.PolicyRule, 0, len(requiredPermissions))
	for _, path := range os.Args[1:] {
		file, err := os.Open(path)
		if err != nil {
			fatalf("open %s: %v", path, err)
		}
		rules = append(rules, readRules(file)...)
		if err := file.Close(); err != nil {
			fatalf("close %s: %v", path, err)
		}
	}

	for _, rule := range rules {
		if slices.Contains(rule.Resources, "*") || slices.Contains(rule.Verbs, "*") {
			fatalf("RBAC contains a wildcard resource or verb: %#v", rule)
		}
	}
	for _, permission := range requiredPermissions {
		if !hasPermission(rules, permission) {
			fatalf(
				"missing RBAC permission: group=%q resource=%q verbs=%v",
				permission.group, permission.resource, permission.verbs,
			)
		}
	}

	fmt.Println("Required operator and leader-election RBAC permissions are present.")
}

func readRules(reader io.Reader) []rbacv1.PolicyRule {
	decoder := yaml.NewYAMLOrJSONDecoder(reader, 4096)
	rules := make([]rbacv1.PolicyRule, 0)
	for {
		document := rbacDocument{}
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				return rules
			}
			fatalf("decode RBAC YAML: %v", err)
		}
		if document.Kind == "Role" || document.Kind == "ClusterRole" {
			rules = append(rules, document.Rules...)
		}
	}
}

func hasPermission(rules []rbacv1.PolicyRule, permission requiredPermission) bool {
	for _, rule := range rules {
		if !slices.Contains(rule.APIGroups, permission.group) || !slices.Contains(rule.Resources, permission.resource) {
			continue
		}
		if every(permission.verbs, func(verb string) bool { return slices.Contains(rule.Verbs, verb) }) {
			return true
		}
	}
	return false
}

func every[T any](values []T, predicate func(T) bool) bool {
	for _, value := range values {
		if !predicate(value) {
			return false
		}
	}
	return true
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
