package autoscaler

import (
	"container/ring"
	"custom-hpa/metrics"
	"custom-hpa/model"
	"math"
	"time"
)

func EvaluateAutoscalingReactive(
	resultChannel metrics.TestResultsChannel,
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
				ae.Metric = metric
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

func checkBuffer(buffer *ring.Ring, requiredPositiveTests int) AutoscaleEvaluation {
	if !isBufferFilled(buffer) {
		return AutoscaleEvaluation{
			ScaleDown: false,
			ScaleUp:   false,
		}
	}
	var scaleUpCounter = 0
	var scaleDownCounter = 0
	buffer.Do(func(item interface{}) {
		var testResult = item.(metrics.TestResult)
		if &testResult != nil {
			if testResult.LowerBoundTestPassed {
				scaleDownCounter++
			}
			if testResult.UpperBoundTestPassed {
				scaleUpCounter++
			}
		}
	})
	var result = AutoscaleEvaluation{
		ScaleDown: false,
		ScaleUp:   false,
	}
	if scaleDownCounter >= requiredPositiveTests {
		result.ScaleDown = true
	}
	if scaleUpCounter >= requiredPositiveTests {
		result.ScaleUp = true
	}
	return result
}
