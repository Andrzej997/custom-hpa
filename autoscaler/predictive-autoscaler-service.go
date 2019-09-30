package autoscaler

import (
	"container/ring"
	"custom-hpa/metrics"
	"custom-hpa/model"
	model2 "github.com/prometheus/common/model"
	"log"
	"math"
	"strconv"
	"time"
)

func EvaluateAutoscalingPredictive(
	resultChannel metrics.TestResultsChannel,
	metric model.AutoscalingDefinitionMetric) AutoscaleEvaluationResult {

	autoscaleEvaluationChannel := make(chan AutoscaleEvaluation)
	closeEvaluationProcessChannel := make(chan bool)
	clearBufferChannel := make(chan bool)
	go func() {
		var resultBuffer *ring.Ring = nil
		ad := metric.AutoregresionDegree
		mad := metric.MovingAverageDegree
		if metric.NumOfTests > ad {
			resultBuffer = ring.New(metric.NumOfTests)
		} else {
			resultBuffer = ring.New(ad)
		}
		var predictionBuffer *ring.Ring = nil
		if metric.NumOfTests > mad {
			predictionBuffer = ring.New(metric.NumOfTests)
		} else {
			predictionBuffer = ring.New(mad)
		}
		var requiredPositiveTests = int(math.Round(float64(metric.NumOfTests+1) / 2.0))

		for {
			select {
			case testResult := <-resultChannel.TestResultsChannel:
				resultBuffer.Value = testResult
				resultBuffer = resultBuffer.Next()
				predictionBuffer = calculatePredictedMetricValue(metric, resultBuffer, predictionBuffer)
				ae := checkBufferPredictive(resultBuffer, predictionBuffer, requiredPositiveTests, metric.NumOfTests)
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

func calculatePredictedMetricValue(metric model.AutoscalingDefinitionMetric, resultBuffer *ring.Ring, predictionBuffer *ring.Ring) *ring.Ring {
	exogenousRegressorMaxValue, err := strconv.ParseFloat(metric.ExogenousRegressorMaxValue, 64)
	if err != nil {
		log.Printf("Float conversion error - autregressionCoefficients: %s", err.Error())
		panic(err)
	}
	inputMetric, err := metrics.ScrapeMetric(metric, metric.ExogenousRegressorQuery)
	if err != nil || !inputMetric.IsMetricValid {
		scalar := model2.Scalar{
			Value:     model2.SampleValue(exogenousRegressorMaxValue),
			Timestamp: 0,
		}
		inputMetric = struct {
			LowerBoundPassed bool
			UpperBoundPassed bool
			MetricName       string
			IsMetricValid    bool
			Value            []model2.Value
		}{LowerBoundPassed: false, UpperBoundPassed: false, MetricName: metric.Name, IsMetricValid: true, Value: []model2.Value{&scalar}}
	}
	resultBufferPtr := resultBuffer
	ad := metric.AutoregresionDegree
	if bufferFulfillmentDegree(resultBufferPtr) < ad {
		return predictionBuffer
	}
	mad := metric.MovingAverageDegree
	if bufferFulfillmentDegree(resultBufferPtr) < mad {
		return predictionBuffer
	}

	var predictedValue = 0.0 // predicted value variable

	// AR
	for i := 0; i < ad; i++ {
		autregressionCoefficientString := metric.AutoregressionCoefficients[i]
		autregressionCoefficient, err := strconv.ParseFloat(autregressionCoefficientString, 64)
		if err != nil {
			log.Printf("Float conversion error - autregressionCoefficients: %s", err.Error())
			continue
		}
		resultBufferPtr = resultBufferPtr.Prev()
		if resultBufferPtr.Value != nil {
			testResult := resultBufferPtr.Value.(metrics.TestResult)
			predictedValue += autregressionCoefficient * testResult.Value
		}
	}

	resultBufferPtr = resultBuffer
	predictionBufferPtr := predictionBuffer

	// MA
	for j := 0; j < mad; j++ {
		movingAverageCoeffitientString := metric.MovingAverageCoefficients[j]
		movingAverageCoeffitient, err := strconv.ParseFloat(movingAverageCoeffitientString, 64)
		if err != nil {
			log.Printf("Float conversion error - movingAverageCoeffitients: %s", err.Error())
			continue
		}
		resultBufferPtr = resultBufferPtr.Prev()
		predictionBufferPtr = predictionBufferPtr.Prev()
		if resultBufferPtr.Value != nil {
			testResult := resultBufferPtr.Value.(metrics.TestResult)
			predictionValueResult := predictionBufferPtr.Value
			predictionValue := 0.0
			if predictionValueResult != nil {
				predictionValue = predictionValueResult.(metrics.TestResult).Value
			}
			noiseValue := testResult.Value - predictionValue
			predictedValue = movingAverageCoeffitient * noiseValue
		}
	}

	// Exogenous variable
	inputValueSum := 0.0
	for _, value := range inputMetric.Value {
		switch value.Type() {
		case model2.ValScalar:
			var value, ok = value.(*model2.Scalar)
			if ok {
				inputValueSum += float64(value.Value)
			}
		}
	}
	if inputValueSum > exogenousRegressorMaxValue {
		inputValueSum = exogenousRegressorMaxValue
	}
	exogenousRegressorCoefficient, err := strconv.ParseFloat(metric.ExogenousRegressorCoefficient, 64)
	if err != nil {
		return predictionBuffer
	}
	predictedValue += exogenousRegressorCoefficient * inputValueSum

	// result validation
	lower, upper := metrics.TestSingleValueBounds(metric, predictedValue)
	predictionBuffer.Value = metrics.TestResult{
		LowerBoundTestPassed: lower,
		UpperBoundTestPassed: upper,
		MetricName:           metric.Name,
		Value:                predictedValue,
	}
	return predictionBuffer.Next()
}

func checkBufferPredictive(buffer *ring.Ring, predictionBuffer *ring.Ring, requiredPositiveTests int, numOfTests int) AutoscaleEvaluation {
	if !isBufferFilled(buffer) {
		return AutoscaleEvaluation{
			ScaleDown: false,
			ScaleUp:   false,
		}
	}
	var scaleUpCounter = 0
	var scaleDownCounter = 0
	var bufferPtr = buffer

	if predictionBuffer.Prev().Value == nil {
		return checkBuffer(buffer, requiredPositiveTests)
	}
	predictedTestResult := predictionBuffer.Prev().Value.(metrics.TestResult)
	for i := 0; i < numOfTests-1; i++ {
		bufferPtr = bufferPtr.Prev()
		testResult := bufferPtr.Value.(metrics.TestResult)
		if &testResult != nil {
			if testResult.LowerBoundTestPassed {
				scaleDownCounter++
			}
			if testResult.UpperBoundTestPassed {
				scaleUpCounter++
			}
		}
	}
	if predictedTestResult.LowerBoundTestPassed {
		scaleDownCounter++
	}
	if predictedTestResult.UpperBoundTestPassed {
		scaleUpCounter++
	}

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
