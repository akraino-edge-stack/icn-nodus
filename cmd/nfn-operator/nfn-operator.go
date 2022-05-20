package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/auth"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/kube"
	notif "github.com/akraino-edge-stack/icn-nodus/internal/pkg/nfnNotify"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/openshift"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/ovn"
	"github.com/akraino-edge-stack/icn-nodus/pkg/apis"
	"github.com/akraino-edge-stack/icn-nodus/pkg/controller"

	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var log = logf.Log.WithName("nfn-operator")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func main() {

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.New())

	printVersion()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	kubeClientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "Error building Kuberenetes clientset")
	}

	var clustercli kube.Interface
	isOpenshift := false

	// Checks if eiteher k8s or OpenShift cluster is in use and creates proper client for it
	if kube.CheckIfKubernetesCluster(kubeClientset) {
		clustercli, err = kube.GetKubeClient()
		if err != nil {
			log.Error(err, "error getting kube client")
		}
	} else {
		isOpenshift = true
		clustercli, err = openshift.GetOpenShiftClient()
		if err != nil {
			log.Error(err, "error getting kube client: %v")
		}
	}

	namespace := os.Getenv(auth.NamespaceEnv)
	if err := auth.PrepareOVNSecrets(namespace, clustercli); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Create an OVN Controller
	_, err = ovn.NewOvnController(nil, isOpenshift)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}
	//Initialize all the controllers that are supported here

	// Start GRPC Notification Server
	go notif.SetupNotifServer(cfg)

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}
	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}

}
