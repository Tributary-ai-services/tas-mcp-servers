// Command operator runs the FederatedMCPServer controller: it watches
// FederatedMCPServer CRs and keeps a tas-mcp gateway's federation registry in
// sync with them (register on create/update, unregister on delete).
package main

import (
	"flag"
	"os"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	mcpv1 "github.com/Tributary-ai-services/federated-mcp-operator/api/v1"
	"github.com/Tributary-ai-services/federated-mcp-operator/pkg/controllers"
	"github.com/Tributary-ai-services/federated-mcp-operator/pkg/gateway"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(mcpv1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
		gatewayURL           string
		gatewayTimeout       time.Duration
		resyncInterval       time.Duration
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8088", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8089", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.StringVar(&gatewayURL, "gateway-url",
		getEnv("TAS_MCP_GATEWAY_URL", "http://prod-tas-mcp-http.tas-mcp-prod.svc.cluster.local:8082"),
		"Base URL of the tas-mcp federation gateway.")
	flag.DurationVar(&gatewayTimeout, "gateway-timeout", 10*time.Second, "HTTP timeout for gateway calls.")
	flag.DurationVar(&resyncInterval, "resync-interval", controllers.DefaultResyncInterval,
		"How often to re-check each server against the gateway to heal drift (e.g. after a gateway restart).")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("Starting FederatedMCPServer operator",
		"version", "v0.1.0",
		"metrics-addr", metricsAddr,
		"probe-addr", probeAddr,
		"leader-election", enableLeaderElection,
		"gateway-url", gatewayURL,
	)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                        scheme,
		Metrics:                       server.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress:        probeAddr,
		LeaderElection:                enableLeaderElection,
		LeaderElectionID:              "federated-mcp-operator-leader-election",
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.FederatedMCPServerReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		Gateway:        gateway.NewClient(gatewayURL, gatewayTimeout),
		ResyncInterval: resyncInterval,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "FederatedMCPServer")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
