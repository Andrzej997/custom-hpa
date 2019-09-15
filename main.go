package main

import (
	"custom-hpa/util"
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
	_ = util.AddToScheme(Scheme)

	client, err := util.NewForConfig(config, Scheme)
	if err != nil {
		panic(err.Error())
	}
	util.MainAutoscalingLoop(client, clientset)
}
