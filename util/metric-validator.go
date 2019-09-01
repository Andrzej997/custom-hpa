package util

import (
	"custom-hpa/model"
	"errors"
	model2 "github.com/prometheus/common/model"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type TestResultsChannel struct {
	TestResultsChannel chan TestResult
	ScrapeInterval     chan bool
	TestInterval       chan bool
}

type TestResult struct {
	lowerBoundTestPassed bool
	upperBoundTestPassed bool
	metricName           string
}

type MetricTestResultMap struct {
	name        string
	scrapedList []MetricTestResult
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
}

func MakeTest(metric model.AutoscalingDefinitionMetric) (TestResultsChannel, error) {
	err := validateRequiredMetricFields(metric)
	if err != nil {
		return TestResultsChannel{}, err
	}
	fillEmptyMetricFields(&metric)
	scrapeDuration, err := time.ParseDuration(metric.ScrapeInterval)
	if err != nil {
		return TestResultsChannel{}, err
	}
	testDuration, err := time.ParseDuration(metric.TestInterval)
	if err != nil {
		return TestResultsChannel{}, err
	}
	scrapedMetricsChannel, scrapeInterval := scrapeMetrics(metric, testDuration, scrapeDuration)
	testResultsChannel, testInterval := testSingleMetric(metric, testDuration, scrapedMetricsChannel)
	return TestResultsChannel{
		TestResultsChannel: testResultsChannel,
		ScrapeInterval:     scrapeInterval,
		TestInterval:       testInterval,
	}, nil
}

func scrapeMetrics(metric model.AutoscalingDefinitionMetric, testDuration time.Duration, scrapeDuration time.Duration) (scrapedMetricsChannel chan []MetricTestResult, scrapeInterval chan bool) {
	maxNumOfScrapes := int64(int64(testDuration) / int64(scrapeDuration))
	var scrapesCounter int64 = 0
	scrapedMetrics := MetricTestResultMap{
		name:        metric.Name,
		scrapedList: make([]MetricTestResult, 0),
	}
	scrapedMetricsChannel = make(chan []MetricTestResult)

	scrapeInterval = SetInterval(func() {
		result, err := scrapeMetric(metric)
		if err != nil || !result.isMetricValid {
			return
		}
		scrapesCounter++
		scrapedMetrics.scrapedList = append(scrapedMetrics.scrapedList, result)
		if scrapesCounter >= maxNumOfScrapes {
			scrapesCounter = 0
			scrapedMetricsChannel <- scrapedMetrics.scrapedList
			scrapedMetrics.scrapedList = nil
		}
	}, scrapeDuration, false)

	return
}

func calculateRobustMean(scrapeList []MetricTestResult, scaleDownValue float64, scaleUpValue float64, trimmedPercentage int) (bool, bool) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		return false, false
	}
	flatScrapeList := flattenScrapeList(scrapeList)
	sort.Slice(flatScrapeList, func(i, j int) bool {
		a, aok := flatScrapeList[i].value[0].(*model2.Scalar)
		b, bok := flatScrapeList[j].value[0].(*model2.Scalar)
		if aok && bok {
			return float64(a.Value) > float64(b.Value)
		}
		return false
	})

	percent := float64(trimmedPercentage)
	n := len(flatScrapeList)
	k := int(math.Round(float64(n) * (percent / 100.0) / 2.0))
	if k == 0 {
		return calculateMean(flatScrapeList, scaleDownValue, scaleUpValue)
	}
	flatScrapeList = flatScrapeList[k:]
	flatScrapeList = flatScrapeList[:len(flatScrapeList)-k-1]
	return calculateMean(flatScrapeList, scaleDownValue, scaleUpValue)

}

func flattenScrapeList(scrapeList []MetricTestResult) []MetricTestResult {
	if scrapeList == nil || len(scrapeList) <= 0 {
		return nil
	}
	var result []MetricTestResult
	for _, scrape := range scrapeList {
		for _, value := range scrape.value {
			result = append(result, MetricTestResult{
				lowerBoundPassed: scrape.lowerBoundPassed,
				upperBoundPassed: scrape.upperBoundPassed,
				metricName:       scrape.metricName,
				isMetricValid:    scrape.isMetricValid,
				value:            []model2.Value{value}},
			)
		}
	}
	return result
}

func calculateMedian(scrapeList []MetricTestResult, scaleDownValue float64, scaleUpValue float64) (bool, bool) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		return false, false
	}
	flatScrapeList := flattenScrapeList(scrapeList)
	sort.Slice(flatScrapeList, func(i, j int) bool {
		a, aok := flatScrapeList[i].value[0].(*model2.Scalar)
		b, bok := flatScrapeList[j].value[0].(*model2.Scalar)
		if aok && bok {
			return float64(a.Value) > float64(b.Value)
		}
		return false
	})
	var median float64
	if len(scrapeList)%2 == 1 {
		median = float64(flatScrapeList[len(flatScrapeList)/2].value[0].(*model2.Scalar).Value)
	} else {
		a1 := float64(flatScrapeList[len(flatScrapeList)/2-1].value[0].(*model2.Scalar).Value)
		a2 := float64(flatScrapeList[len(flatScrapeList)/2].value[0].(*model2.Scalar).Value)
		median = (a1 + a2) / 2.0
	}
	lowerBoundTest := median <= scaleDownValue
	upperBoundTest := median >= scaleUpValue
	return lowerBoundTest, upperBoundTest
}

func calculateMean(scrapeList []MetricTestResult, scaleDownValue float64, scaleUpValue float64) (bool, bool) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		return false, false
	}
	var sum = 0.0
	for _, scrape := range scrapeList {
		switch scrape.value[0].Type() {
		case model2.ValScalar:
			var value, ok = scrape.value[0].(*model2.Scalar)
			if ok {
				sum += float64(value.Value)
			}
		}
	}
	mean := sum / float64(len(scrapeList))
	lowerBoundTest := mean <= scaleDownValue
	upperBoundTest := mean >= scaleUpValue
	return lowerBoundTest, upperBoundTest
}

func testSingleMetric(metric model.AutoscalingDefinitionMetric, testDuration time.Duration, scrapedMetricsChannel chan []MetricTestResult) (testResultsChannel chan TestResult, testInterval chan bool) {
	maxNumOfTests := metric.NumOfTests
	var testCounter = 0
	testResultsChannel = make(chan TestResult)
	testInterval = SetInterval(func() {
		select {
		case scrapes := <-scrapedMetricsChannel:
			lowerBoundTest, upperBoundTest := testScrapeList(scrapes, metric)
			var testResult = TestResult{
				lowerBoundTestPassed: lowerBoundTest,
				upperBoundTestPassed: upperBoundTest,
				metricName:           metric.Name,
			}
			testResultsChannel <- testResult
		}
		testCounter++
		if testCounter >= maxNumOfTests {
			testCounter = 0
		}
	}, testDuration, false)
	return
}

func testScrapeList(scrapeList []MetricTestResult, metric model.AutoscalingDefinitionMetric) (bool, bool) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		log.Printf("No scrapes found.")
		return false, false
	}
	scaleDownValue, err := strconv.ParseFloat(metric.ScaleDownValue, 64)
	if err != nil {
		log.Printf("Float conversion error - scaleUpValue: %s", err.Error())
		return false, false
	}
	scaleUpValue, err := strconv.ParseFloat(metric.ScaleUpValue, 64)
	if err != nil {
		log.Printf("Float conversion error - scaleDownValue: %s", err.Error())
		return false, false
	}
	switch strings.ToUpper(metric.Algorithm) {
	case "MEAN":
		return calculateMean(scrapeList, scaleDownValue, scaleUpValue)
	case "MEDIAN":
		return calculateMedian(scrapeList, scaleDownValue, scaleUpValue)
	case "TRIMMEDMEAN":
		return calculateRobustMean(scrapeList, scaleDownValue, scaleUpValue, metric.TrimmedPercentage)
	default:
		return testScrapeListDefault(scrapeList, metric.PercentageOfTestConditionFulfillment)
	}
}

func testScrapeListDefault(scrapeList []MetricTestResult, percentageOfTestConditionFulfillment int) (bool, bool) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		log.Printf("No scrapes found.")
		return false, false
	}
	var lowerBoundsPassed, upperBoundsPassed = 0, 0
	var lowerBoundsFailed, upperBoundsFailed = 0, 0
	for _, scrape := range scrapeList {
		if !scrape.isMetricValid {
			lowerBoundsFailed++
			upperBoundsFailed++
			continue
		}
		if scrape.lowerBoundPassed {
			lowerBoundsPassed++
			upperBoundsFailed++
		} else if scrape.upperBoundPassed {
			upperBoundsPassed++
			lowerBoundsFailed++
		} else {
			lowerBoundsFailed++
			upperBoundsFailed++
		}
	}
	var lowerBoundTest = int(lowerBoundsPassed/(lowerBoundsPassed+lowerBoundsFailed)) >= percentageOfTestConditionFulfillment
	var upperBoundTest = int(upperBoundsPassed/(upperBoundsPassed+upperBoundsFailed)) >= percentageOfTestConditionFulfillment
	return lowerBoundTest, upperBoundTest
}
