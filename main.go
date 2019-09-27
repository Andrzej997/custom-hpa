package main

import (
	"custom-hpa/autoscaler"
	"custom-hpa/clients"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
)

func main() {

	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(os.Getenv("USERPROFILE"), ".kube", "config")
		err = nil
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	Scheme := scheme.Scheme
	_ = clients.AddToScheme(Scheme)

	client, err := clients.NewForConfig(config, Scheme)
	if err != nil {
		panic(err.Error())
	}
	autoscaler.MainAutoscalingLoop(client, clientset)
}
