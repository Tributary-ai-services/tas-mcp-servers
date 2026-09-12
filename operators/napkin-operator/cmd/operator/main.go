package main

import (
	"flag"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	napkinv1 "github.com/Tributary-ai-services/napkin-operator/api/v1"
	"github.com/Tributary-ai-services/napkin-operator/pkg/controllers"
	minioclient "github.com/Tributary-ai-services/napkin-operator/pkg/minio"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(napkinv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var napkinURL string
	var minioEndpoint string
	var minioAccessKey string
	var minioSecretKey string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8088", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8089", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.StringVar(&napkinURL, "napkin-url", getEnv("NAPKIN_API_BASE_URL", "https://api.napkin.ai"), "Napkin AI API base URL")
	flag.StringVar(&minioEndpoint, "minio-endpoint", getEnv("MINIO_ENDPOINT", "minio-shared.tas-shared.svc.cluster.local:9000"), "MinIO endpoint")
	flag.StringVar(&minioAccessKey, "minio-access-key", getEnv("MINIO_ACCESS_KEY", "minioadmin"), "MinIO access key")
	flag.StringVar(&minioSecretKey, "minio-secret-key", getEnv("MINIO_SECRET_KEY", "minioadmin123"), "MinIO secret key")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("Starting Napkin Visual Operator",
		"version", "v1.0.0",
		"metrics-addr", metricsAddr,
		"probe-addr", probeAddr,
		"leader-election", enableLeaderElection,
		"napkin-url", napkinURL,
		"minio-endpoint", minioEndpoint,
	)

	// Initialize MinIO client
	mc, err := minioclient.NewClient(minioEndpoint, minioAccessKey, minioSecretKey, false)
	if err != nil {
		setupLog.Error(err, "Failed to create MinIO client")
		os.Exit(1)
	}

	// Set public URL for external-facing download links
	if publicURL := getEnv("MINIO_PUBLIC_URL", ""); publicURL != "" {
		mc.SetPublicURL(publicURL)
		setupLog.Info("MinIO public URL configured", "url", publicURL)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress:        probeAddr,
		LeaderElection:                enableLeaderElection,
		LeaderElectionID:              "napkin-operator-leader-election",
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.NapkinVisualReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		NapkinURL:   napkinURL,
		MinioClient: mc,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "NapkinVisual")
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
