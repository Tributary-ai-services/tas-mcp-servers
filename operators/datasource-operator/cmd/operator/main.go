package main

import (
	"flag"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	datasourcev1 "github.com/Tributary-ai-services/datasource-operator/api/v1"
	"github.com/Tributary-ai-services/datasource-operator/pkg/bridge"
	"github.com/Tributary-ai-services/datasource-operator/pkg/controllers"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(datasourcev1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8088", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8089", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for HA.")
	flag.Parse()

	// Build zap logger
	zapCfg := zap.NewProductionConfig()
	zapCfg.EncoderConfig.TimeKey = "ts"
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := zapCfg.Build()
	if err != nil {
		panic("failed to create logger: " + err.Error())
	}
	defer logger.Sync()

	// Set controller-runtime logger
	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(false)))

	logger.Info("Starting datasource-operator",
		zap.String("metrics", metricsAddr),
		zap.String("probes", probeAddr),
	)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "datasource-operator.tas.ai",
	})
	if err != nil {
		logger.Fatal("Unable to create manager", zap.Error(err))
		os.Exit(1)
	}

	// Create bridge client
	bridgeClient := bridge.NewClient()

	// Setup reconciler
	reconciler := &controllers.DatasourceConnectionReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		BridgeClient: bridgeClient,
		Logger:       logger,
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		logger.Fatal("Unable to create controller", zap.Error(err))
		os.Exit(1)
	}

	// Health probes
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Fatal("Unable to set up health check", zap.Error(err))
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Fatal("Unable to set up ready check", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Fatal("Manager exited with error", zap.Error(err))
		os.Exit(1)
	}
}
