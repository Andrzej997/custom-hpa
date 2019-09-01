package util

import (
	"container/ring"
	"custom-hpa/model"
	"k8s.io/client-go/kubernetes"
	"log"
	"math"
	"time"
)

type AutoscaleEvaluationResult struct {
	AutoscaleEvaluation           chan AutoscaleEvaluation
	CloseEvaluationProcessChannel chan bool
	ClearBufferChannel            chan bool
}

type AutoscaleEvaluation struct {
	scaleDown bool
	scaleUp   bool
	metric    model.AutoscalingDefinitionMetric
}

func EvaluateAutoscaling(
	resultChannel TestResultsChannel,
	metric model.AutoscalingDefinitionMetric) AutoscaleEvaluationResult {

	autoscaleEvaluationChannel := make(chan AutoscaleEvaluation)
	closeEvaluationProcessChannel := make(chan bool)
	clearBufferChannel := make(chan bool)
	go func() {
		var resultBuffer = ring.New(metric.NumOfTests)
		var requiredPositiveTests = int(math.Round(float64(metric.NumOfTests+1) / 2.0))
		for {
			select {
			case testResult := <-resultChannel.TestResultsChannel:
				resultBuffer.Value = testResult
				resultBuffer = resultBuffer.Next()
				ae := checkBuffer(resultBuffer, requiredPositiveTests)
				ae.metric = metric
				autoscaleEvaluationChannel <- ae
				resultBuffer.Value = nil
			case <-closeEvaluationProcessChannel:
				return
			case <-clearBufferChannel:
				clearBuffer(resultBuffer)
			}
		}
	}()
	return AutoscaleEvaluationResult{
		AutoscaleEvaluation:           autoscaleEvaluationChannel,
		CloseEvaluationProcessChannel: closeEvaluationProcessChannel,
		ClearBufferChannel:            clearBufferChannel,
	}
}

func fillDefinitionsDefaultValues(definition *model.AutoscalingDefinition) {
	if definition.Spec.MinReplicas <= 0 {
		definition.Spec.MinReplicas = 1
	}
	if definition.Spec.MaxReplicas <= 0 {
		definition.Spec.MaxReplicas = 1
	}
	if definition.Spec.ScalingStep <= 0 {
		definition.Spec.ScalingStep = 1
	}
	if _, err := time.ParseDuration(definition.Spec.IntervalBetweenAutoscaling); len(definition.Spec.IntervalBetweenAutoscaling) <= 0 || err != nil {
		definition.Spec.IntervalBetweenAutoscaling = "2m"
	}
	if len(definition.Spec.ScaleTarget.MatchNamespace) <= 0 {
		definition.Spec.ScaleTarget.MatchNamespace = "default"
	}
	if len(definition.Spec.ScaleTarget.TargetType) <= 0 {
		definition.Spec.ScaleTarget.TargetType = "deployment"
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
				deploymentScale, err := GetScale(client, definition.Spec.ScaleTarget)
				if autoscalingBlocked {
					log.Printf("Autoscaling temporary blocked by intervalBetweenAutoscaling")
				} else if err != nil {
					log.Printf("Autoscaling error: %s", err.Error())
				} else {
					if ae.scaleUp && int(deploymentScale.Spec.Replicas) < definition.Spec.MaxReplicas {
						log.Printf("Scaling up %s based on metric: %s", definition.Spec.ScaleTarget.MatchLabel, ae.metric.Name)
						deploymentScale.Spec.Replicas = int32(math.Min(float64(definition.Spec.MaxReplicas), float64(deploymentScale.Spec.Replicas+int32(definition.Spec.ScalingStep))))
						deploymentScale, err = ScaleObject(client, definition.Spec.ScaleTarget, deploymentScale)
						if err != nil {
							log.Printf("Autoscaling error: %s", err.Error())
						}
						clearMetricBufferChannel <- ae.metric
						autoscalingBlocked = true
						SetTimeout(func() {
							autoscalingBlocked = false
						}, intervalBetweenAutoscaling)
					} else if ae.scaleDown && int(deploymentScale.Spec.Replicas) > definition.Spec.MinReplicas {
						log.Printf("Scaling down %s based on metric: %s", definition.Spec.ScaleTarget.MatchLabel, ae.metric.Name)
						deploymentScale.Spec.Replicas = int32(math.Max(float64(definition.Spec.MinReplicas), float64(deploymentScale.Spec.Replicas-int32(definition.Spec.ScalingStep))))
						deploymentScale, err = ScaleObject(client, definition.Spec.ScaleTarget, deploymentScale)
						if err != nil {
							log.Printf("Autoscaling error: %s", err.Error())
						}
						autoscalingBlocked = true
						clearMetricBufferChannel <- ae.metric
						SetTimeout(func() {
							autoscalingBlocked = false
						}, intervalBetweenAutoscaling)
					} else if ae.scaleUp && int(deploymentScale.Spec.Replicas) >= definition.Spec.MaxReplicas {
						log.Printf("Reached maximum replicas, can't scale up anymore. Metric: %s", ae.metric.Name)
					} else if ae.scaleDown && int(deploymentScale.Spec.Replicas) <= definition.Spec.MinReplicas {
						log.Printf("Reached minimum replicas, can't scale down anymore. Metric: %s", ae.metric.Name)
					} else {
						log.Printf("Verified metric: %s, no need to scale", ae.metric.Name)
					}
				}
			}
		}
	}()
}

func checkBuffer(buffer *ring.Ring, requiredPositiveTests int) AutoscaleEvaluation {
	if !isBufferFilled(buffer) {
		return AutoscaleEvaluation{
			scaleDown: false,
			scaleUp:   false,
		}
	}
	var scaleUpCounter = 0
	var scaleDownCounter = 0
	buffer.Do(func(item interface{}) {
		var testResult = item.(TestResult)
		if &testResult != nil {
			if testResult.lowerBoundTestPassed {
				scaleDownCounter++
			}
			if testResult.upperBoundTestPassed {
				scaleUpCounter++
			}
		}
	})
	var result = AutoscaleEvaluation{
		scaleDown: false,
		scaleUp:   false,
	}
	if scaleDownCounter >= requiredPositiveTests {
		result.scaleDown = true
	}
	if scaleUpCounter >= requiredPositiveTests {
		result.scaleUp = true
	}
	return result
}

func isBufferFilled(buffer *ring.Ring) bool {
	var isFilled = true
	buffer.Do(func(value interface{}) {
		if value == nil {
			isFilled = false
			return
		}
	})
	return isFilled
}

func clearBuffer(buffer *ring.Ring) {
	b := buffer
	if b != nil {
		for i := 0; i < b.Len(); i++ {
			b.Value = nil
			b = b.Next()
		}
	}
}
