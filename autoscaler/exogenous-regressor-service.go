package autoscaler

import (
	"custom-hpa/metrics"
	"custom-hpa/model"
	"custom-hpa/util"
	"errors"
	model2 "github.com/prometheus/common/model"
	"log"
	"math"
	"sort"
	"strconv"
	"time"
)

type ScrapedMetricItem struct {
	MetricName    string
	IsMetricValid bool
	Value         []model2.Value
}

type ScrapeResultMap struct {
	Name        string
	ScrapedList []ScrapedMetricItem
}

type ExogenousRegressorScrapeResult struct {
	Name    string
	Value   float64
	IsValid bool
}

type ExogenousRegressorResultChannel struct {
	exogenousRegressorResultChannel chan ExogenousRegressorScrapeResult
	scrapeInterval                  chan bool
}

func CollectExogenousMetrics(metric model.AutoscalingDefinitionMetric) (ExogenousRegressorResultChannel, error) {
	scrapeDuration, err := time.ParseDuration(metric.ScrapeInterval)
	if err != nil {
		return ExogenousRegressorResultChannel{}, err
	}
	testDuration, err := time.ParseDuration(metric.TestInterval)
	if err != nil {
		return ExogenousRegressorResultChannel{}, err
	}
	exogenousRegressorResultChannel, scrapeInterval := ScrapeExogenousMetrics(metric, testDuration, scrapeDuration)
	return ExogenousRegressorResultChannel{
		exogenousRegressorResultChannel: exogenousRegressorResultChannel,
		scrapeInterval:                  scrapeInterval,
	}, nil
}

func ScrapeExogenousMetrics(metric model.AutoscalingDefinitionMetric, testDuration time.Duration, scrapeDuration time.Duration) (exogenousRegressorResultChannel chan ExogenousRegressorScrapeResult, scrapeInterval chan bool) {
	maxNumOfScrapes := int64(testDuration) / int64(scrapeDuration)
	var scrapesCounter int64 = 0
	scrapedMetrics := ScrapeResultMap{
		Name:        metric.Name,
		ScrapedList: make([]ScrapedMetricItem, 0),
	}
	exogenousRegressorResultChannel = make(chan ExogenousRegressorScrapeResult, 2)

	scrapeInterval = util.SetInterval(func() {
		result, err := scrapeMetric(metric)
		scrapesCounter++
		if err != nil || !result.IsMetricValid {
			return
		}
		scrapedMetrics.ScrapedList = append(scrapedMetrics.ScrapedList, result)
		if scrapesCounter >= maxNumOfScrapes {
			scrapesCounter = 0
			if int((maxNumOfScrapes+1)/2) > len(scrapedMetrics.ScrapedList) {
				exogenousRegressorResultChannel <- ExogenousRegressorScrapeResult{
					Name:    metric.Name,
					Value:   0,
					IsValid: false,
				}
			}
			scrapeValuesRobustMean := calculateScrapeValuesRobustMean(scrapedMetrics.ScrapedList, metric)
			exogenousRegressorResultChannel <- ExogenousRegressorScrapeResult{
				Name:    metric.Name,
				Value:   scrapeValuesRobustMean,
				IsValid: true,
			}
			scrapedMetrics.ScrapedList = nil
		}
	}, scrapeDuration, false)

	return
}

func scrapeMetric(metric model.AutoscalingDefinitionMetric) (ScrapedMetricItem, error) {
	var result ScrapedMetricItem
	value, err := metrics.ReadMetric(metric.PrometheusPath, metric.ExogenousRegressorQuery)
	if err != nil {
		log.Printf("Error: %s", err.Error())
		result = ScrapedMetricItem{IsMetricValid: false, MetricName: metric.Name}
		return result, err
	}
	result, err = parseMetricValue(value, metric)
	if err != nil {
		log.Printf("Error: %s", err.Error())
		result = ScrapedMetricItem{IsMetricValid: false, MetricName: metric.Name}
		return result, err
	}
	result.IsMetricValid = true
	return result, nil
}

func parseMetricValue(value model2.Value, metric model.AutoscalingDefinitionMetric) (ScrapedMetricItem, error) {
	var result = ScrapedMetricItem{}
	var err error
	switch value.Type() {
	case model2.ValString:
		res, ok := value.(*model2.String)
		if ok {
			result.Value = append(result.Value, res)
		} else {
			err = errors.New("cannot cast metric type to string")
			result.IsMetricValid = false
		}
	case model2.ValScalar:
		res, ok := value.(*model2.Scalar)
		if ok {
			result.Value = append(result.Value, res)
		} else {
			err = errors.New("cannot cast metric type to scalar")
			result.IsMetricValid = false
		}
	case model2.ValVector:
		res, ok := value.(model2.Vector)
		result.IsMetricValid = true
		if ok {
			if res.Len() <= 0 {
				err = errors.New("metrics vector is empty")
				result.IsMetricValid = false
			} else {
				for i := 0; i < res.Len(); i++ {
					sample := res[i]
					scalar := model2.Scalar{
						Value:     sample.Value,
						Timestamp: sample.Timestamp,
					}
					result.Value = append(result.Value, &scalar)
				}
			}
		} else {
			err = errors.New("cannot cast metric type to vector")
			result.IsMetricValid = false
		}
	case model2.ValMatrix:
		res, ok := value.(model2.Matrix)
		result.IsMetricValid = true
		if ok && res.Len() > 0 {
			for i := 0; i < res.Len(); i++ {
				for j := 0; j < len(res[i].Values); j++ {
					sample := res[i].Values[j]
					scalar := model2.Scalar{
						Value:     sample.Value,
						Timestamp: sample.Timestamp,
					}
					result.Value = append(result.Value, &scalar)
				}
			}
		} else {
			err = errors.New("cannot cast metric type to matrix")
			result.IsMetricValid = false
		}
	case model2.ValNone:
		err = errors.New("cannot recognize metric type")
		result.IsMetricValid = false
	default:
		err = errors.New("cannot recognize metric type")
		result.IsMetricValid = false
	}
	if &result != nil {
		result.MetricName = metric.Name
	}
	return result, err
}

func calculateScrapeValuesRobustMean(scrapeList []ScrapedMetricItem, metric model.AutoscalingDefinitionMetric) float64 {
	if scrapeList == nil || len(scrapeList) <= 0 {
		log.Printf("No scrapes found.")
		return 0
	}
	result := calculateRobustMean(scrapeList, metric.TrimmedPercentage)

	exogenousRegressorMaxValue, err := strconv.ParseFloat(metric.ExogenousRegressorMaxValue, 64)
	if err != nil {
		log.Printf("Float conversion error - autregressionCoefficients: %s", err.Error())
		panic(err)
	}

	if result > exogenousRegressorMaxValue {
		result = exogenousRegressorMaxValue
	}
	return result
}

func calculateRobustMean(scrapeList []ScrapedMetricItem, trimmedPercentage int) float64 {
	flatScrapeList := flattenScrapeList(scrapeList)
	sort.Slice(flatScrapeList, func(i, j int) bool {
		a, aok := flatScrapeList[i].Value[0].(*model2.Scalar)
		b, bok := flatScrapeList[j].Value[0].(*model2.Scalar)
		if aok && bok {
			return float64(a.Value) > float64(b.Value)
		}
		return false
	})

	percent := float64(trimmedPercentage)
	n := len(flatScrapeList)
	k := int(math.Round(float64(n) * (percent / 100.0) / 2.0))
	if k == 0 {
		return calculateMean(flatScrapeList)
	}
	flatScrapeList = flatScrapeList[k:]
	flatScrapeList = flatScrapeList[:len(flatScrapeList)-k-1]
	return calculateMean(flatScrapeList)

}

func calculateMean(scrapeList []ScrapedMetricItem) float64 {
	if scrapeList == nil || len(scrapeList) <= 0 {
		return 0
	}
	var sum = 0.0
	for _, scrape := range scrapeList {
		switch scrape.Value[0].Type() {
		case model2.ValScalar:
			var value, ok = scrape.Value[0].(*model2.Scalar)
			if ok {
				sum += float64(value.Value)
			}
		}
	}
	return sum / float64(len(scrapeList))
}

func flattenScrapeList(scrapeList []ScrapedMetricItem) []ScrapedMetricItem {
	var result []ScrapedMetricItem
	for _, scrape := range scrapeList {
		for _, value := range scrape.Value {
			result = append(result, ScrapedMetricItem{
				MetricName:    scrape.MetricName,
				IsMetricValid: scrape.IsMetricValid,
				Value:         []model2.Value{value}},
			)
		}
	}
	return result
}
