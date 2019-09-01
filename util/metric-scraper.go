package util

import (
	"context"
	"custom-hpa/model"
	"errors"
	"fmt"
	promApi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	model2 "github.com/prometheus/common/model"
	"log"
	"strconv"
	"strings"
	"time"
)

type ValueString struct {
	*model2.String
}

func (c *ValueString) validateBounds(scaleDownValue string, scaleUpValue string, scaleValueType string) (MetricTestResult, error) {
	if strings.ToUpper(scaleValueType) != "STRING" && strings.ToUpper(scaleUpValue) != "BOOLEAN" {
		return MetricTestResult{}, errors.New(fmt.Sprintf("String metric cannot be cast to type %s", scaleValueType))
	}
	isBoolean := strings.ToUpper(scaleUpValue) == "BOOLEAN"
	result := MetricTestResult{}
	if strings.ToUpper(scaleDownValue) == strings.ToUpper(c.Value) {
		result.lowerBoundPassed = true
	} else if isBoolean && strings.ToUpper(scaleDownValue) == "FALSE" {
		result.lowerBoundPassed = true
	}
	if strings.ToUpper(scaleUpValue) == strings.ToUpper(c.Value) {
		result.upperBoundPassed = true
	} else if isBoolean && strings.ToUpper(scaleUpValue) == "TRUE" {
		result.upperBoundPassed = true
	}
	return result, nil
}

type ValueScalar struct {
	*model2.Scalar
}

func (c *ValueScalar) validateBounds(scaleDownValue string, scaleUpValue string, scaleValueType string) (MetricTestResult, error) {
	switch strings.ToUpper(scaleValueType) {
	case "INTEGER":
		return c.validateIntegerBounds(scaleDownValue, scaleUpValue)
	case "DOUBLE":
		return c.validateDoubleBounds(scaleDownValue, scaleUpValue)
	case "BOOLEAN":
		return c.validateBooleanBounds()
	case "TIME":
		return c.validateTimeBounds(scaleDownValue, scaleUpValue)
	}
	return MetricTestResult{}, errors.New("Unsupported scaleValueType: " + scaleUpValue)
}

func (c *ValueScalar) validateIntegerBounds(scaleDownValue string, scaleUpValue string) (MetricTestResult, error) {
	downValue, err := strconv.ParseInt(scaleDownValue, 10, 64)
	if err != nil {
		return MetricTestResult{}, err
	}
	upValue, err := strconv.ParseInt(scaleUpValue, 10, 64)
	if err != nil {
		return MetricTestResult{}, err
	}
	value := float64(c.Value)
	intValue := int64(value)
	result := MetricTestResult{}
	if intValue <= downValue {
		result.lowerBoundPassed = true
	}
	if intValue >= upValue {
		result.upperBoundPassed = true
	}
	return result, nil
}

func (c *ValueScalar) validateDoubleBounds(scaleDownValue string, scaleUpValue string) (MetricTestResult, error) {
	downValue, err := strconv.ParseFloat(scaleDownValue, 64)
	if err != nil {
		return MetricTestResult{}, err
	}
	upValue, err := strconv.ParseFloat(scaleUpValue, 64)
	if err != nil {
		return MetricTestResult{}, err
	}
	value := float64(c.Value)
	result := MetricTestResult{}
	if value <= downValue {
		result.lowerBoundPassed = true
	}
	if value >= upValue {
		result.upperBoundPassed = true
	}
	return result, nil
}

func (c *ValueScalar) validateBooleanBounds() (MetricTestResult, error) {
	value := float64(c.Value)
	result := MetricTestResult{
		lowerBoundPassed: false,
		upperBoundPassed: false,
	}
	if value > 0 {
		result.upperBoundPassed = true
	} else {
		result.lowerBoundPassed = true
	}
	return result, nil
}

func (c *ValueScalar) validateTimeBounds(scaleDownValue string, scaleUpValue string) (MetricTestResult, error) {
	value := float64(c.Value)
	refTime := time.Unix(int64(value), 0)
	downTime, err := time.Parse(time.RFC3339, scaleDownValue)
	if err != nil {
		return MetricTestResult{}, err
	}
	upTime, err := time.Parse(time.RFC3339, scaleUpValue)
	if err != nil {
		return MetricTestResult{}, err
	}

	result := MetricTestResult{}
	if refTime.After(upTime) {
		result.upperBoundPassed = true
	}
	if refTime.Before(downTime) {
		result.lowerBoundPassed = true
	}
	return result, nil
}

type MetricTestResult struct {
	lowerBoundPassed bool
	upperBoundPassed bool
	metricName       string
	isMetricValid    bool
	value            []model2.Value
}

func scrapeMetric(metric model.AutoscalingDefinitionMetric) (MetricTestResult, error) {
	var result MetricTestResult
	value, err := readMetric(metric)
	if err != nil {
		log.Printf("Error: %s", err.Error())
		result = MetricTestResult{isMetricValid: false, metricName: metric.Name}
		return result, err
	}
	result, err = validateMetricBounds(value, metric)
	if err != nil {
		log.Printf("Error: %s", err.Error())
		result = MetricTestResult{isMetricValid: false, metricName: metric.Name}
		return result, err
	}
	result.isMetricValid = true
	return result, nil
}

func readMetric(metric model.AutoscalingDefinitionMetric) (model2.Value, error) {
	if len(metric.PrometheusPath) <= 0 || len(metric.PrometheusQuery) <= 0 {
		return nil, errors.New("prometheus query or path should not be null")
	}
	return readPrometheusMetrics(metric.PrometheusPath, metric.PrometheusQuery)
}

func validateMetricBounds(value model2.Value, metric model.AutoscalingDefinitionMetric) (MetricTestResult, error) {
	var result = MetricTestResult{}
	var err error
	switch value.Type() {
	case model2.ValString:
		res, ok := value.(*model2.String)
		if ok {
			var vs = &ValueString{res}
			result, err = vs.validateBounds(metric.ScaleDownValue, metric.ScaleUpValue, metric.ScaleValueType)
			result.value = append(result.value, value)
		} else {
			err = errors.New("cannot cast metric type to string")
		}
	case model2.ValScalar:
		res, ok := value.(*model2.Scalar)
		if ok {
			var vs = &ValueScalar{res}
			result, err = vs.validateBounds(metric.ScaleDownValue, metric.ScaleUpValue, metric.ScaleValueType)
			result.value = append(result.value, value)
		} else {
			err = errors.New("cannot cast metric type to scalar")
		}
	case model2.ValVector:
		res, ok := value.(model2.Vector)
		result.upperBoundPassed = true
		result.lowerBoundPassed = true
		result.isMetricValid = true
		if ok {
			if res.Len() <= 0 {
				err = errors.New("metrics vector is empty")
			} else {
				for i := 0; i < res.Len(); i++ {
					sample := res[i]
					scalar := model2.Scalar{
						Value:     sample.Value,
						Timestamp: sample.Timestamp,
					}
					var vs = &ValueScalar{&scalar}
					partialResult, err := vs.validateBounds(metric.ScaleDownValue, metric.ScaleUpValue, metric.ScaleValueType)
					if err != nil {
						return MetricTestResult{}, err
					}
					result.upperBoundPassed = result.upperBoundPassed && partialResult.upperBoundPassed
					result.lowerBoundPassed = result.lowerBoundPassed && partialResult.lowerBoundPassed
					result.isMetricValid = result.isMetricValid && partialResult.isMetricValid
					result.value = append(result.value, &scalar)
				}
			}
		} else {
			err = errors.New("cannot cast metric type to vector")
		}
	case model2.ValMatrix:
		res, ok := value.(model2.Matrix)
		result.upperBoundPassed = true
		result.lowerBoundPassed = true
		result.isMetricValid = true
		if ok && res.Len() > 0 {
			for i := 0; i < res.Len(); i++ {
				for j := 0; j < len(res[i].Values); j++ {
					sample := res[i].Values[j]
					scalar := model2.Scalar{
						Value:     sample.Value,
						Timestamp: sample.Timestamp,
					}
					var vs = &ValueScalar{&scalar}
					partialResult, err := vs.validateBounds(metric.ScaleDownValue, metric.ScaleUpValue, metric.ScaleValueType)
					if err != nil {
						return MetricTestResult{}, err
					}
					result.upperBoundPassed = result.upperBoundPassed && partialResult.upperBoundPassed
					result.lowerBoundPassed = result.lowerBoundPassed && partialResult.lowerBoundPassed
					result.isMetricValid = result.isMetricValid && partialResult.isMetricValid
					result.value = append(result.value, &scalar)
				}
			}
		} else {
			err = errors.New("cannot cast metric type to matrix")
		}
		err = errors.New("matrix type metrics are not supported yet")
	case model2.ValNone:
		err = errors.New("cannot recognize metric type")
	default:
		err = errors.New("cannot recognize metric type")
	}
	if &result != nil {
		result.metricName = metric.Name
	}
	return result, err
}

func readPrometheusMetrics(address string, query string) (model2.Value, error) {
	client := client(address)
	api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	value, warnings, err := api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	return value, nil
}

func client(address string) promApi.Client {
	client, err := promApi.NewClient(promApi.Config{Address: address})
	if err != nil {
		panic(err)
	}
	return client
}
