package util

import (
	"custom-hpa/model"
	"errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"log"
	"os"
)

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	log.Printf("Cannot find EnvVar %s, falling back to %s", key, fallback)
	return fallback
}

func getNamespaces(client *kubernetes.Clientset) (vpcs []string) {
	namespaces, _ := client.CoreV1().Namespaces().List(v1.ListOptions{})
	for _, ns := range namespaces.Items {
		vpcs = append(vpcs, ns.ObjectMeta.Name)
	}
	return
}

func getNamespace(name string, client *kubernetes.Clientset) (vpcs *corev1.Namespace, err error) {
	namespace, err := client.CoreV1().Namespaces().Get(name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return namespace, nil
}

func GetScale(client *kubernetes.Clientset, target model.AutoscalingDefinitionScaleTarget) (*v1beta1.Scale, error) {
	set := labels.Set{target.LabelName: target.MatchLabel}
	if target.TargetType == "deployment" {
		deployments, err := client.ExtensionsV1beta1().Deployments(target.MatchNamespace).List(v1.ListOptions{LabelSelector: set.AsSelector().String()})
		if err != nil {
			return nil, err
		}
		if deployments.Items == nil || len(deployments.Items) <= 0 {
			return nil, err
		}
		return client.ExtensionsV1beta1().Deployments(target.MatchNamespace).GetScale(deployments.Items[0].Name, v1.GetOptions{})
	} else if target.TargetType == "replicaset" {
		replicasets, err := client.ExtensionsV1beta1().ReplicaSets(target.MatchNamespace).List(v1.ListOptions{LabelSelector: set.AsSelector().String()})
		if err != nil {
			return nil, err
		}
		if replicasets.Items == nil || len(replicasets.Items) <= 0 {
			return nil, err
		}
		return client.ExtensionsV1beta1().ReplicaSets(target.MatchNamespace).GetScale(replicasets.Items[0].Name, v1.GetOptions{})
	}
	return nil, errors.New("not recognized target type")
}

func ScaleObject(client *kubernetes.Clientset, target model.AutoscalingDefinitionScaleTarget, scale *v1beta1.Scale) (*v1beta1.Scale, error) {
	set := labels.Set{target.LabelName: target.MatchLabel}
	if target.TargetType == "deployment" {
		deployments, err := client.ExtensionsV1beta1().Deployments(target.MatchNamespace).List(v1.ListOptions{LabelSelector: set.AsSelector().String()})
		if err != nil {
			return nil, err
		}
		if deployments.Items == nil || len(deployments.Items) <= 0 {
			return nil, err
		}
		return client.ExtensionsV1beta1().Deployments(target.MatchNamespace).UpdateScale(deployments.Items[0].Name, scale)
	} else if target.TargetType == "replicaset" {
		replicasets, err := client.ExtensionsV1beta1().ReplicaSets(target.MatchNamespace).List(v1.ListOptions{LabelSelector: set.AsSelector().String()})
		if err != nil {
			return nil, err
		}
		if replicasets.Items == nil || len(replicasets.Items) <= 0 {
			return nil, err
		}
		return client.ExtensionsV1beta1().ReplicaSets(target.MatchNamespace).UpdateScale(replicasets.Items[0].Name, scale)
	}
	return nil, errors.New("not recognized target type")
}
