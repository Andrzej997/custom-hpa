package autoscaler

import (
	"custom-hpa/clients"
	"custom-hpa/metrics"
	"custom-hpa/model"
	"custom-hpa/util"
	"k8s.io/client-go/kubernetes"
	"log"
	"math"
	"strings"
	"time"
)

type AutoscaleEvaluationResult struct {
	AutoscaleEvaluation           chan AutoscaleEvaluation
	CloseEvaluationProcessChannel chan bool
	ClearBufferChannel            chan bool
}

type AutoscaleEvaluation struct {
	ScaleDown bool
	ScaleUp   bool
	Metric    model.AutoscalingDefinitionMetric
}

func EvaluateAutoscaling(resultChannel metrics.TestResultsChannel,
	metric model.AutoscalingDefinitionMetric) AutoscaleEvaluationResult {
	switch strings.ToUpper(metric.Algorithm) {
	case "ARIMAX":
		return EvaluateAutoscalingPredictive(resultChannel, metric)
	default:
		return EvaluateAutoscalingReactive(resultChannel, metric)
	}
}

func StartAutoscaleProcess(autoscaleEvaluationChannel chan AutoscaleEvaluation, client *kubernetes.Clientset,
	definition model.AutoscalingDefinition, clearMetricBufferChannel chan model.AutoscalingDefinitionMetric) {
	fillDefinitionsDefaultValues(&definition)
	intervalBetweenAutoscaling, e := time.ParseDuration(definition.Spec.IntervalBetweenAutoscaling)
	if e != nil {
		log.Printf("intervalBetweenAutoscaling error: %s", e.Error())
		return
	}
	go func() {
		var autoscalingBlocked = false
		for {
			select {
			case ae := <-autoscaleEvaluationChannel:
				deploymentScale, err := clients.GetScale(client, definition.Spec.ScaleTarget)
				if autoscalingBlocked {
					log.Printf("Autoscaling temporary blocked by intervalBetweenAutoscaling")
				} else if err != nil {
					log.Printf("Autoscaling error: %s", err.Error())
				} else {
					if ae.ScaleUp && int(deploymentScale.Spec.Replicas) < definition.Spec.MaxReplicas {
						log.Printf("Scaling up %s based on metric: %s", definition.Spec.ScaleTarget.MatchLabel, ae.Metric.Name)
						deploymentScale.Spec.Replicas = int32(math.Min(float64(definition.Spec.MaxReplicas), float64(deploymentScale.Spec.Replicas+int32(definition.Spec.ScalingStep))))
						deploymentScale, err = clients.ScaleObject(client, definition.Spec.ScaleTarget, deploymentScale)
						if err != nil {
							log.Printf("Autoscaling error: %s", err.Error())
						}
						clearMetricBufferChannel <- ae.Metric
						autoscalingBlocked = true
						util.SetTimeout(func() {
							autoscalingBlocked = false
						}, intervalBetweenAutoscaling)
					} else if ae.ScaleDown && int(deploymentScale.Spec.Replicas) > definition.Spec.MinReplicas {
						log.Printf("Scaling down %s based on metric: %s", definition.Spec.ScaleTarget.MatchLabel, ae.Metric.Name)
						deploymentScale.Spec.Replicas = int32(math.Max(float64(definition.Spec.MinReplicas), float64(deploymentScale.Spec.Replicas-int32(definition.Spec.ScalingStep))))
						deploymentScale, err = clients.ScaleObject(client, definition.Spec.ScaleTarget, deploymentScale)
						if err != nil {
							log.Printf("Autoscaling error: %s", err.Error())
						}
						autoscalingBlocked = true
						clearMetricBufferChannel <- ae.Metric
						util.SetTimeout(func() {
							autoscalingBlocked = false
						}, intervalBetweenAutoscaling)
					} else if ae.ScaleUp && int(deploymentScale.Spec.Replicas) >= definition.Spec.MaxReplicas {
						log.Printf("Reached maximum replicas, can't scale up anymore. Metric: %s", ae.Metric.Name)
					} else if ae.ScaleDown && int(deploymentScale.Spec.Replicas) <= definition.Spec.MinReplicas {
						log.Printf("Reached minimum replicas, can't scale down anymore. Metric: %s", ae.Metric.Name)
					} else {
						log.Printf("Verified metric: %s, no need to scale", ae.Metric.Name)
					}
				}
			}
		}
	}()
}
