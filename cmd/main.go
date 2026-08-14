package main

import (
	"flag"
	"net"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	discoveryv1alpha1 "github.com/pierinho13/external-service-discovery-operator/api/v1alpha1"
	"github.com/pierinho13/external-service-discovery-operator/internal/controller"
	"github.com/pierinho13/external-service-discovery-operator/internal/discovery"
)

var scheme = runtime.NewScheme()
var setupLog = ctrl.Log.WithName("setup")

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(discoveryv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr, probeAddr string
	var leaderElect bool
	var discoveryRefreshInterval time.Duration
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "Metrics bind address; 0 disables metrics.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Health probe bind address.")
	flag.BoolVar(&leaderElect, "leader-elect", false, "Enable leader election.")
	flag.DurationVar(&discoveryRefreshInterval, "discovery-refresh-interval", time.Minute, "Refresh interval for dynamic discovery providers such as DNS.")
	// Production logging defaults to info. The Helm chart maps log.level to
	// controller-runtime's --zap-log-level flag when debug output is needed.
	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: metricsAddr}, HealthProbeBindAddress: probeAddr, LeaderElection: leaderElect, LeaderElectionID: "external-service-discovery-operator.k8sready.com"})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}
	resolver := discovery.Resolver{Static: discovery.StaticProvider{}, DNS: discovery.DNSProvider{Resolver: net.DefaultResolver, RefreshInterval: discoveryRefreshInterval}}
	if err := (&controller.DiscoveredServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Provider: resolver}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to add health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to add ready check")
		os.Exit(1)
	}
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "manager exited")
		os.Exit(1)
	}
}
