package metrics

import (
	"custom-hpa/model"
	"errors"
	"fmt"
	model2 "github.com/prometheus/common/model"
	"strconv"
	"strings"
	"time"
)

type MetricValidateResult struct {
	LowerBoundPassed bool
	UpperBoundPassed bool
	MetricName       string
	IsMetricValid    bool
	Value            []model2.Value
}

type MetricValidateResultMap struct {
	Name        string
	ScrapedList []MetricValidateResult
}

// public functions
func ValidateMetricBounds(value model2.Value, metric model.AutoscalingDefinitionMetric) (MetricValidateResult, error) {
	var result = MetricValidateResult{}
	var err error
	switch value.Type() {
	case model2.ValString:
		res, ok := value.(*model2.String)
		if ok {
			var vs = &ValueString{res}
			result, err = vs.validateBounds(metric.ScaleDownValue, metric.ScaleUpValue, metric.ScaleValueType)
			if &result != nil {
				result.Value = append(result.Value, value)
			}
		} else {
			err = errors.New("cannot cast metric type to string")
		}
	case model2.ValScalar:
		res, ok := value.(*model2.Scalar)
		if ok {
			var vs = &ValueScalar{res}
			result, err = vs.validateBounds(metric.ScaleDownValue, metric.ScaleUpValue, metric.ScaleValueType)
			if &result != nil {
				result.Value = append(result.Value, value)
			}
		} else {
			err = errors.New("cannot cast metric type to scalar")
		}
	case model2.ValVector:
		res, ok := value.(model2.Vector)
		result.UpperBoundPassed = true
		result.LowerBoundPassed = true
		result.IsMetricValid = true
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
						return MetricValidateResult{}, err
					}
					result.UpperBoundPassed = result.UpperBoundPassed && partialResult.UpperBoundPassed
					result.LowerBoundPassed = result.LowerBoundPassed && partialResult.LowerBoundPassed
					result.IsMetricValid = result.IsMetricValid && partialResult.IsMetricValid
					result.Value = append(result.Value, &scalar)
				}
			}
		} else {
			err = errors.New("cannot cast metric type to vector")
		}
	case model2.ValMatrix:
		res, ok := value.(model2.Matrix)
		result.UpperBoundPassed = true
		result.LowerBoundPassed = true
		result.IsMetricValid = true
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
						return MetricValidateResult{}, err
					}
					result.UpperBoundPassed = result.UpperBoundPassed && partialResult.UpperBoundPassed
					result.LowerBoundPassed = result.LowerBoundPassed && partialResult.LowerBoundPassed
					result.IsMetricValid = result.IsMetricValid && partialResult.IsMetricValid
					result.Value = append(result.Value, &scalar)
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
		result.MetricName = metric.Name
	}
	return result, err
}

// private model and functions
type ValueString struct {
	*model2.String
}

type ValueScalar struct {
	*model2.Scalar
}

func (c *ValueString) validateBounds(scaleDownValue string, scaleUpValue string, scaleValueType string) (MetricValidateResult, error) {
	if strings.ToUpper(scaleValueType) != "STRING" && strings.ToUpper(scaleUpValue) != "BOOLEAN" {
		return MetricValidateResult{}, errors.New(fmt.Sprintf("String metric cannot be cast to type %s", scaleValueType))
	}
	isBoolean := strings.ToUpper(scaleUpValue) == "BOOLEAN"
	result := MetricValidateResult{}
	if strings.ToUpper(scaleDownValue) == strings.ToUpper(c.Value) {
		result.LowerBoundPassed = true
	} else if isBoolean && strings.ToUpper(scaleDownValue) == "FALSE" {
		result.LowerBoundPassed = true
	}
	if strings.ToUpper(scaleUpValue) == strings.ToUpper(c.Value) {
		result.UpperBoundPassed = true
	} else if isBoolean && strings.ToUpper(scaleUpValue) == "TRUE" {
		result.UpperBoundPassed = true
	}
	return result, nil
}

func (c *ValueScalar) validateBounds(scaleDownValue string, scaleUpValue string, scaleValueType string) (MetricValidateResult, error) {
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
	return MetricValidateResult{}, errors.New("Unsupported scaleValueType: " + scaleUpValue)
}

func (c *ValueScalar) validateIntegerBounds(scaleDownValue string, scaleUpValue string) (MetricValidateResult, error) {
	downValue, err := strconv.ParseInt(scaleDownValue, 10, 64)
	if err != nil {
		return MetricValidateResult{}, err
	}
	upValue, err := strconv.ParseInt(scaleUpValue, 10, 64)
	if err != nil {
		return MetricValidateResult{}, err
	}
	value := float64(c.Value)
	intValue := int64(value)
	result := MetricValidateResult{}
	if intValue <= downValue {
		result.LowerBoundPassed = true
	}
	if intValue >= upValue {
		result.UpperBoundPassed = true
	}
	return result, nil
}

func (c *ValueScalar) validateDoubleBounds(scaleDownValue string, scaleUpValue string) (MetricValidateResult, error) {
	downValue, err := strconv.ParseFloat(scaleDownValue, 64)
	if err != nil {
		return MetricValidateResult{}, err
	}
	upValue, err := strconv.ParseFloat(scaleUpValue, 64)
	if err != nil {
		return MetricValidateResult{}, err
	}
	value := float64(c.Value)
	result := MetricValidateResult{}
	if value <= downValue {
		result.LowerBoundPassed = true
	}
	if value >= upValue {
		result.UpperBoundPassed = true
	}
	return result, nil
}

func (c *ValueScalar) validateBooleanBounds() (MetricValidateResult, error) {
	value := float64(c.Value)
	result := MetricValidateResult{
		LowerBoundPassed: false,
		UpperBoundPassed: false,
	}
	if value > 0 {
		result.UpperBoundPassed = true
	} else {
		result.LowerBoundPassed = true
	}
	return result, nil
}

func (c *ValueScalar) validateTimeBounds(scaleDownValue string, scaleUpValue string) (MetricValidateResult, error) {
	value := float64(c.Value)
	refTime := time.Unix(int64(value), 0)
	downTime, err := time.Parse(time.RFC3339, scaleDownValue)
	if err != nil {
		return MetricValidateResult{}, err
	}
	upTime, err := time.Parse(time.RFC3339, scaleUpValue)
	if err != nil {
		return MetricValidateResult{}, err
	}

	result := MetricValidateResult{}
	if refTime.After(upTime) {
		result.UpperBoundPassed = true
	}
	if refTime.Before(downTime) {
		result.LowerBoundPassed = true
	}
	return result, nil
}

func validateRequiredMetricFields(metric model.AutoscalingDefinitionMetric) (err error) {
	if &metric == nil {
		err = errors.New("metric cannot be null")
		return
	}
	if len(metric.Name) <= 0 || len(metric.MetricType) <= 0 || len(metric.ScaleDownValue) <= 0 || len(metric.ScaleUpValue) <= 0 || len(metric.ScaleValueType) <= 0 {
		err = errors.New("one of required fields not found: name, metricType, scaleDownValue, scaleUpValue, scaleValueType")
		return
	}
	return nil
}

func fillEmptyMetricFields(metric *model.AutoscalingDefinitionMetric) {
	if metric.NumOfTests <= 0 {
		metric.NumOfTests = 1
	}
	if len(metric.Algorithm) <= 0 {
		metric.Algorithm = "default"
	}
	if metric.TrimmedPercentage < 0 {
		metric.TrimmedPercentage = 0
	}
	if metric.TrimmedPercentage > 100 {
		metric.TrimmedPercentage = 100
	}
	if metric.PercentageOfTestConditionFulfillment < 0 {
		metric.PercentageOfTestConditionFulfillment = 0
	}
	if metric.PercentageOfTestConditionFulfillment > 100 {
		metric.PercentageOfTestConditionFulfillment = 100
	}
	if _, err := time.ParseDuration(metric.ScrapeInterval); len(metric.ScrapeInterval) < 0 || err != nil {
		metric.ScrapeInterval = "1s"
	}
	if _, err := time.ParseDuration(metric.TestInterval); len(metric.TestInterval) < 0 || err != nil {
		metric.TestInterval = "1m"
	}
	if metric.AutoregresionDegree < 0 {
		metric.AutoregresionDegree = 0
	}
	if metric.MovingAverageDegree < 0 {
		metric.MovingAverageDegree = 0
	}
	if metric.AutoregressionCoefficients == nil {
		metric.AutoregressionCoefficients = []string{}
	}
	if metric.MovingAverageCoefficients == nil {
		metric.MovingAverageCoefficients = []string{}
	}
	if _, err := strconv.ParseFloat(metric.ExogenousRegressorCoefficient, 64); err != nil {
		metric.ExogenousRegressorCoefficient = "0.0"
	}
	for i, coef := range metric.AutoregressionCoefficients {
		if _, err := strconv.ParseFloat(coef, 64); err != nil {
			metric.AutoregressionCoefficients[i] = "0.0"
		}
	}
	for i, coef := range metric.MovingAverageCoefficients {
		if _, err := strconv.ParseFloat(coef, 64); err != nil {
			metric.MovingAverageCoefficients[i] = "0.0"
		}
	}
	if len(metric.MovingAverageCoefficients) < metric.MovingAverageDegree {
		length := len(metric.MovingAverageCoefficients)
		for i := 0; i < metric.MovingAverageDegree-length; i++ {
			metric.MovingAverageCoefficients = append(metric.MovingAverageCoefficients, "0.0")
		}
	}
	if len(metric.AutoregressionCoefficients) < metric.AutoregresionDegree {
		length := len(metric.AutoregressionCoefficients)
		for i := 0; i < metric.AutoregresionDegree-length; i++ {
			metric.AutoregressionCoefficients = append(metric.AutoregressionCoefficients, "0.0")
		}
	}
}
