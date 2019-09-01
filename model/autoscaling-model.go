package model

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type AutoscalingDefinition struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               AutoscalingDefinitionSpec   `json:"spec"`
	Status             AutoscalingDefinitionStatus `json:"status,omitempty"`
}

type AutoscalingDefinitionSpec struct {
	ScaleTarget                AutoscalingDefinitionScaleTarget `json:"scaleTarget"`
	MinReplicas                int                              `json:"minReplicas,omitempty"`
	MaxReplicas                int                              `json:"maxReplicas,omitempty"`
	IntervalBetweenAutoscaling string                           `json:"intervalBetweenAutoscaling,omitempty"`
	ScalingStep                int                              `json:"scalingStep,omitempty"`
	Metrics                    []AutoscalingDefinitionMetric    `json:"metrics"`
}

type AutoscalingDefinitionStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

type AutoscalingDefinitionScaleTarget struct {
	MatchNamespace string `json:"matchNamespace,omitempty"`
	LabelName      string `json:"labelName"`
	MatchLabel     string `json:"matchLabel"`
	TargetType     string `json:"targetType,omitempty"`
}

type AutoscalingDefinitionMetric struct {
	Name                                 string `json:"name"`
	MetricType                           string `json:"metricType"`
	PrometheusPath                       string `json:"prometheusPath"`
	PrometheusQuery                      string `json:"prometheusQuery"`
	ScaleDownValue                       string `json:"scaleDownValue"`
	ScaleUpValue                         string `json:"scaleUpValue"`
	ScaleValueType                       string `json:"scaleValueType"`
	NumOfTests                           int    `json:"numOfTests"`
	Algorithm                            string `json:"algorithm"`
	TrimmedPercentage                    int    `json:"trimmedPercentage"`
	PercentageOfTestConditionFulfillment int    `json:"percentageOfTestConditionFulfillment"`
	ScrapeInterval                       string `json:"scrapeInterval"`
	TestInterval                         string `json:"testInterval"`
}

type AutoscalingDefinitionList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []AutoscalingDefinition `json:"items"`
}

func (in *AutoscalingDefinitionList) DeepCopyObject() runtime.Object {
	out := AutoscalingDefinitionList{}
	in.DeepCopyInto(&out)

	return &out
}

func (in *AutoscalingDefinition) DeepCopyObject() runtime.Object {
	out := AutoscalingDefinition{}
	in.DeepCopyInto(&out)

	return &out
}

func (in *AutoscalingDefinitionList) DeepCopyInto(out *AutoscalingDefinitionList) {
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]AutoscalingDefinition, len(in.Items))
		for i, item := range in.Items {
			out.Items[i] = AutoscalingDefinition{}
			item.DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *AutoscalingDefinition) DeepCopyInto(out *AutoscalingDefinition) {
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = in.ObjectMeta
	out.Spec = AutoscalingDefinitionSpec{}
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = AutoscalingDefinitionStatus{}
	in.Status.DeepCopyInto(&out.Status)
}

func (in *AutoscalingDefinitionSpec) DeepCopyInto(out *AutoscalingDefinitionSpec) {
	out.MinReplicas = in.MinReplicas
	out.MaxReplicas = in.MinReplicas
	out.IntervalBetweenAutoscaling = in.IntervalBetweenAutoscaling
	out.ScalingStep = in.ScalingStep
	out.ScaleTarget = AutoscalingDefinitionScaleTarget{}
	in.ScaleTarget.DeepCopyInto(&out.ScaleTarget)
	if in.Metrics != nil {
		out.Metrics = make([]AutoscalingDefinitionMetric, len(in.Metrics))
		for i, met := range in.Metrics {
			out.Metrics[i] = AutoscalingDefinitionMetric{}
			met.DeepCopyInto(&out.Metrics[i])
		}
	}
}

func (in *AutoscalingDefinitionStatus) DeepCopyInto(out *AutoscalingDefinitionStatus) {
	out.State = in.State
	out.Message = in.Message
}

func (in *AutoscalingDefinitionScaleTarget) DeepCopyInto(out *AutoscalingDefinitionScaleTarget) {
	out.MatchNamespace = in.MatchNamespace
	out.MatchLabel = in.MatchLabel
	out.LabelName = in.LabelName
	out.TargetType = in.TargetType
}

func (in *AutoscalingDefinitionMetric) DeepCopyInto(out *AutoscalingDefinitionMetric) {
	out.Name = in.Name
	out.MetricType = in.MetricType
	out.PrometheusPath = in.PrometheusPath
	out.PrometheusQuery = in.PrometheusQuery
	out.ScaleDownValue = in.ScaleDownValue
	out.ScaleUpValue = in.ScaleUpValue
	out.ScaleValueType = in.ScaleValueType
	out.NumOfTests = in.NumOfTests
	out.PercentageOfTestConditionFulfillment = in.PercentageOfTestConditionFulfillment
	out.ScrapeInterval = in.ScrapeInterval
	out.TestInterval = in.TestInterval
}
