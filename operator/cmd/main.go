package main

import (
	"flag"
	"os"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	xposv1alpha1 "github.com/ametow/xpos/operator/api/v1alpha1"
	"github.com/ametow/xpos/operator/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(xposv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(gatewayv1a2.Install(scheme))
}

func main() {
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8443",
		"The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for the controller manager. "+
			"Required when running more than one operator replica.")

	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "xpos-operator.xpos-io.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	tunnelCfg := controller.TunnelReconcilerConfig{
		GatewayName:      getenv("XPOS_GATEWAY_NAME", "xpos-gateway"),
		GatewayNamespace: os.Getenv("XPOS_GATEWAY_NAMESPACE"),
		RelayHTTPPort:    int32(getenvInt("XPOS_RELAY_HTTP_PORT", 8080)),
		TCPPortMin:       int32(getenvInt("XPOS_TCP_PORT_MIN", 30000)),
		TCPPortMax:       int32(getenvInt("XPOS_TCP_PORT_MAX", 30099)),
	}

	allocator := &controller.TCPPortAllocator{
		Client: mgr.GetClient(),
		Min:    tunnelCfg.TCPPortMin,
		Max:    tunnelCfg.TCPPortMax,
	}

	if err = (&controller.TunnelReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Config:    tunnelCfg,
		Allocator: allocator,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up controller", "controller", "Tunnel")
		os.Exit(1)
	}
	if err = (&controller.AgentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up controller", "controller", "Agent")
		os.Exit(1)
	}
	if err = (&controller.RelayPodReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: controller.RelayPodReconcilerConfig{
			PodLabelKey:   getenv("XPOS_RELAY_POD_LABEL_KEY", "app.kubernetes.io/name"),
			PodLabelValue: getenv("XPOS_RELAY_POD_LABEL_VALUE", "xpos-relay"),
			HTTPPort:      int32(getenvInt("XPOS_RELAY_HTTP_PORT", 8080)),
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up controller", "controller", "RelayPod")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
