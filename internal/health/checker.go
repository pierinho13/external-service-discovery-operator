package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
)

const (
	defaultInterval = 10 * time.Second
	defaultTimeout  = 3 * time.Second
)

type Result struct {
	Healthy bool
	Reason  string
	Message string
}

type Checker interface {
	Check(context.Context, string, *discoveryv1alpha1.HealthCheck) Result
}

type NetworkChecker struct{}

func (NetworkChecker) Check(ctx context.Context, address string, config *discoveryv1alpha1.HealthCheck) Result {
	checkCtx, cancel := context.WithTimeout(ctx, Timeout(config))
	defer cancel()
	if config.Type == discoveryv1alpha1.HealthCheckTypeTCP {
		connection, err := (&net.Dialer{}).DialContext(checkCtx, "tcp", net.JoinHostPort(address, strconv.Itoa(int(config.Port))))
		if err != nil {
			return Result{Reason: "ProbeFailed", Message: err.Error()}
		}
		_ = connection.Close()
		return Result{Healthy: true, Reason: "ProbeSucceeded", Message: "TCP connection succeeded"}
	}
	return checkHTTP(checkCtx, address, config)
}

func checkHTTP(ctx context.Context, address string, config *discoveryv1alpha1.HealthCheck) Result {
	scheme := "http"
	if config.Type == discoveryv1alpha1.HealthCheckTypeHTTPS {
		scheme = "https"
	}
	host := config.Host
	if host == "" {
		host = address
	}
	path := config.Path
	if path == "" {
		path = "/"
	}
	target := url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.Itoa(int(config.Port))), Path: path}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(address, strconv.Itoa(int(config.Port))))
	}}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return Result{Reason: "ProbeFailed", Message: err.Error()}
	}
	if config.Host != "" {
		request.Host = config.Host
	}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return Result{Reason: "ProbeFailed", Message: err.Error()}
	}
	defer func() { _ = response.Body.Close() }()
	for _, expected := range ExpectedStatuses(config) {
		if response.StatusCode == int(expected) {
			return Result{Healthy: true, Reason: "ProbeSucceeded", Message: fmt.Sprintf("HTTP status %d", response.StatusCode)}
		}
	}
	return Result{Reason: "UnexpectedStatus", Message: fmt.Sprintf("unexpected HTTP status %d", response.StatusCode)}
}

func Interval(config *discoveryv1alpha1.HealthCheck) time.Duration {
	if config == nil || config.Interval.Duration <= 0 {
		return defaultInterval
	}
	return config.Interval.Duration
}

func Timeout(config *discoveryv1alpha1.HealthCheck) time.Duration {
	if config == nil || config.Timeout.Duration <= 0 {
		return defaultTimeout
	}
	return config.Timeout.Duration
}

func ExpectedStatuses(config *discoveryv1alpha1.HealthCheck) []int32 {
	if len(config.ExpectedStatuses) == 0 {
		return []int32{http.StatusOK}
	}
	return config.ExpectedStatuses
}

func SuccessThreshold(config *discoveryv1alpha1.HealthCheck) int32 {
	if config.SuccessThreshold <= 0 {
		return 1
	}
	return config.SuccessThreshold
}

func FailureThreshold(config *discoveryv1alpha1.HealthCheck) int32 {
	if config.FailureThreshold <= 0 {
		return 3
	}
	return config.FailureThreshold
}
