package util

import (
	"custom-hpa/model"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"time"
)

type AutoscalerDefinitionV1Interface interface {
	AutoscalerDefinitions(namespace string) AutoscalerDefinitionInterface
}

type Client struct {
	restClient rest.Interface
}

type AutoscalerDefinitionInterface interface {
	List(opts meta_v1.ListOptions) (*model.AutoscalingDefinitionList, error)
	Get(name string, options meta_v1.GetOptions) (*model.AutoscalingDefinition, error)
	Watch(opts meta_v1.ListOptions) (watch.Interface, error)
	WatchAutoscalingDefinitions() cache.Store
}

type AutoscalerDefinitionClient struct {
	restClient rest.Interface
	ns         string
}

var SchemeGroupVersion = schema.GroupVersion{Group: "scaling.com", Version: "v1"}

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion, &model.AutoscalingDefinition{}, &model.AutoscalingDefinitionList{})
	meta_v1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

func NewForConfig(c *rest.Config, scheme *runtime.Scheme) (*Client, error) {
	config := *c
	config.ContentConfig.GroupVersion = &SchemeGroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme)
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	restClient, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &Client{restClient: restClient}, nil
}

func (c *Client) AutoscalerDefinitions(namespace string) AutoscalerDefinitionInterface {
	return &AutoscalerDefinitionClient{
		restClient: c.restClient,
		ns:         namespace,
	}
}

func (c *AutoscalerDefinitionClient) Get(name string, options meta_v1.GetOptions) (*model.AutoscalingDefinition, error) {
	result := model.AutoscalingDefinition{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource("autoscalingdefinitions").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(&result)

	return &result, err
}

func (c *AutoscalerDefinitionClient) List(opts meta_v1.ListOptions) (*model.AutoscalingDefinitionList, error) {
	result := model.AutoscalingDefinitionList{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource("autoscalingdefinitions").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(&result)
	return &result, err
}

func (c *AutoscalerDefinitionClient) Watch(opts meta_v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.restClient.
		Get().
		Namespace(c.ns).
		Resource("autoscalingdefinitions").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

func (c *AutoscalerDefinitionClient) WatchAutoscalingDefinitions() cache.Store {
	projectStore, projectController := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(lo meta_v1.ListOptions) (result runtime.Object, err error) {
				return c.List(lo)
			},
			WatchFunc: func(lo meta_v1.ListOptions) (watch.Interface, error) {
				return c.Watch(lo)
			},
		},
		&model.AutoscalingDefinition{},
		1*time.Minute,
		cache.ResourceEventHandlerFuncs{},
	)
	go projectController.Run(wait.NeverStop)
	return projectStore
}
