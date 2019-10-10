package metrics

import (
	"context"
	"custom-hpa/model"
	"custom-hpa/util"
	"errors"
	"fmt"
	promApi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	model2 "github.com/prometheus/common/model"
	"log"
	"time"
)

type ScrapeResultChannel struct {
	scrapedMetricsChannel chan []MetricValidateResult
	scrapeInterval        chan bool
}

func MakeScrape(metric model.AutoscalingDefinitionMetric) (ScrapeResultChannel, error) {
	err := validateRequiredMetricFields(metric)
	if err != nil {
		return ScrapeResultChannel{}, err
	}
	fillEmptyMetricFields(&metric)
	scrapeDuration, err := time.ParseDuration(metric.ScrapeInterval)
	if err != nil {
		return ScrapeResultChannel{}, err
	}
	testDuration, err := time.ParseDuration(metric.TestInterval)
	if err != nil {
		return ScrapeResultChannel{}, err
	}
	scrapedMetricsChannel, scrapeInterval := ScrapeMetrics(metric, testDuration, scrapeDuration)
	return ScrapeResultChannel{
		scrapedMetricsChannel: scrapedMetricsChannel,
		scrapeInterval:        scrapeInterval,
	}, nil
}

func ScrapeMetrics(metric model.AutoscalingDefinitionMetric, testDuration time.Duration, scrapeDuration time.Duration) (scrapedMetricsChannel chan []MetricValidateResult, scrapeInterval chan bool) {
	maxNumOfScrapes := int64(testDuration) / int64(scrapeDuration)
	var scrapesCounter int64 = 0
	scrapedMetrics := MetricValidateResultMap{
		Name:        metric.Name,
		ScrapedList: make([]MetricValidateResult, 0),
	}
	scrapedMetricsChannel = make(chan []MetricValidateResult)

	scrapeInterval = util.SetInterval(func() {
		result, err := ScrapeMetric(metric)
		if err == nil && result.IsMetricValid {
			scrapedMetrics.ScrapedList = append(scrapedMetrics.ScrapedList, result)
		}
		scrapesCounter++
		if scrapesCounter >= maxNumOfScrapes {
			scrapesCounter = 0
			scrapedMetricsChannel <- scrapedMetrics.ScrapedList
			scrapedMetrics.ScrapedList = nil
		}
	}, scrapeDuration, false)

	return
}

func ScrapeMetric(metric model.AutoscalingDefinitionMetric) (MetricValidateResult, error) {
	var result MetricValidateResult
	value, err := ReadMetric(metric.PrometheusPath, metric.PrometheusQuery)
	if err != nil {
		log.Printf("Error: %s", err.Error())
		result = MetricValidateResult{IsMetricValid: false, MetricName: metric.Name}
		return result, err
	}
	result, err = ValidateMetricBounds(value, metric)
	if err != nil {
		log.Printf("Error: %s", err.Error())
		result = MetricValidateResult{IsMetricValid: false, MetricName: metric.Name}
		return result, err
	}
	result.IsMetricValid = true
	return result, nil
}

func ReadMetric(PrometheusPath string, PrometheusQuery string) (model2.Value, error) {
	if len(PrometheusPath) <= 0 || len(PrometheusQuery) <= 0 {
		return nil, errors.New("prometheus query or path should not be null")
	}
	return readPrometheusMetrics(PrometheusPath, PrometheusQuery)
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
