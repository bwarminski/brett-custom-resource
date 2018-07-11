package main

import (
	"flag"
	"k8s.io/sample-controller/pkg/signals"

	"k8s.io/client-go/tools/clientcmd"
	"github.com/golang/glog"
	argoinformers "github.com/argoproj/argo/pkg/client/informers/externalversions"

	argoclientset "github.com/argoproj/argo/pkg/client/clientset/versioned"
	clientset "github.com/bwarminski/brett-custom-resource/deployment/pkg/client/clientset/versioned"
	informers "github.com/bwarminski/brett-custom-resource/deployment/pkg/client/informers/externalversions"
	"time"
	"os"
)

// Most of this is taken from https://github.com/kubernetes/sample-controller/blob/master/main.go

var (
	masterURL  string
	kubeconfig string
	threadiness int
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. ")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.IntVar(&threadiness, "threads", 2, "The number of worker processes to start up (min 1)")
}

func main() {
	flag.Parse()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
	}

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	deploymentClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientset: %s", err.Error())
	}

	argoClient, err := argoclientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf( "Error building argo clientset: %s", err.Error())
	}

	deploymentInformerFactory := informers.NewSharedInformerFactory(deploymentClient, time.Second*30)
	argoInformerFactory := argoinformers.NewSharedInformerFactory(argoClient, time.Second*30)

	deploymentInformer := deploymentInformerFactory.Bwarminski().V1().BrettDeployments()
	workflowInformer := argoInformerFactory.Argoproj().V1alpha1().Workflows()

	controller := NewController(deploymentClient, argoClient, deploymentInformer, workflowInformer)

	go argoInformerFactory.Start(stopCh)
	go deploymentInformerFactory.Start(stopCh)

	if err = controller.Run(threadiness, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
}


