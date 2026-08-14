package health

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
)

func TestNetworkCheckerHTTP(t *testing.T) {
	tests := []struct {
		name, path string
		status     int
		expected   []int32
		healthy    bool
		reason     string
	}{
		{name: "default 200", path: "/estasbien", status: http.StatusOK, healthy: true, reason: "ProbeSucceeded"},
		{name: "unexpected status", path: "/failing", status: http.StatusServiceUnavailable, healthy: false, reason: "UnexpectedStatus"},
		{name: "configured status", path: "/accepted", status: http.StatusNoContent, expected: []int32{http.StatusNoContent}, healthy: true, reason: "ProbeSucceeded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != test.path {
					t.Fatalf("got path %q, want %q", request.URL.Path, test.path)
				}
				response.WriteHeader(test.status)
			}))
			defer server.Close()
			address, port := serverAddress(t, server.URL)
			result := (NetworkChecker{}).Check(context.Background(), address, &discoveryv1alpha1.HealthCheck{Type: discoveryv1alpha1.HealthCheckTypeHTTP, Port: port, Path: test.path, ExpectedStatuses: test.expected, Timeout: duration(time.Second)})
			if result.Healthy != test.healthy || result.Reason != test.reason {
				t.Fatalf("unexpected result: %#v", result)
			}
		})
	}
}

func TestNetworkCheckerTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	port := int32(listener.Addr().(*net.TCPAddr).Port)
	result := (NetworkChecker{}).Check(context.Background(), "127.0.0.1", &discoveryv1alpha1.HealthCheck{Type: discoveryv1alpha1.HealthCheckTypeTCP, Port: port, Timeout: duration(time.Second)})
	if !result.Healthy || result.Reason != "ProbeSucceeded" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestNetworkCheckerHTTPSValidatesCertificates(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	address, port := serverAddress(t, server.URL)
	result := (NetworkChecker{}).Check(context.Background(), address, &discoveryv1alpha1.HealthCheck{Type: discoveryv1alpha1.HealthCheckTypeHTTPS, Port: port, Timeout: duration(time.Second)})
	if result.Healthy || result.Reason != "ProbeFailed" {
		t.Fatalf("an untrusted HTTPS certificate must fail: %#v", result)
	}
}

func TestHealthCheckDefaults(t *testing.T) {
	config := &discoveryv1alpha1.HealthCheck{}
	if Interval(config) != 10*time.Second || Timeout(config) != 3*time.Second || SuccessThreshold(config) != 1 || FailureThreshold(config) != 3 {
		t.Fatalf("unexpected defaults")
	}
	if statuses := ExpectedStatuses(config); len(statuses) != 1 || statuses[0] != http.StatusOK {
		t.Fatalf("unexpected statuses: %v", statuses)
	}
}

func serverAddress(t *testing.T, rawURL string) (string, int32) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	host, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	return host, int32(port)
}

func duration(value time.Duration) metav1.Duration {
	return metav1.Duration{Duration: value}
}
