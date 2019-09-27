package metrics

import (
	"custom-hpa/model"
	"custom-hpa/util"
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
	LowerBoundTestPassed bool
	UpperBoundTestPassed bool
	MetricName           string
	Value                float64
}

// public functions
func MakeTest(metric model.AutoscalingDefinitionMetric, scrapeResultChannel ScrapeResultChannel) (TestResultsChannel, error) {
	err := validateRequiredMetricFields(metric)
	if err != nil {
		return TestResultsChannel{}, err
	}
	fillEmptyMetricFields(&metric)
	testDuration, err := time.ParseDuration(metric.TestInterval)
	if err != nil {
		return TestResultsChannel{}, err
	}
	testResultsChannel, testInterval := testSingleMetric(metric, testDuration, scrapeResultChannel.scrapedMetricsChannel)
	return TestResultsChannel{
		TestResultsChannel: testResultsChannel,
		ScrapeInterval:     scrapeResultChannel.scrapeInterval,
		TestInterval:       testInterval,
	}, nil
}

func TestSingleValueBounds(metric model.AutoscalingDefinitionMetric, value float64) (bool, bool) {
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
	lowerBoundTest := value <= scaleDownValue
	upperBoundTest := value >= scaleUpValue
	return lowerBoundTest, upperBoundTest
}

// private functions
func testSingleMetric(metric model.AutoscalingDefinitionMetric, testDuration time.Duration, scrapedMetricsChannel chan []MetricValidateResult) (testResultsChannel chan TestResult, testInterval chan bool) {
	maxNumOfTests := metric.NumOfTests
	var testCounter = 0
	testResultsChannel = make(chan TestResult)
	testInterval = util.SetInterval(func() {
		select {
		case scrapes := <-scrapedMetricsChannel:
			lowerBoundTest, upperBoundTest, value := testScrapeList(scrapes, metric)
			var testResult = TestResult{
				LowerBoundTestPassed: lowerBoundTest,
				UpperBoundTestPassed: upperBoundTest,
				MetricName:           metric.Name,
				Value:                value,
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

func testScrapeList(scrapeList []MetricValidateResult, metric model.AutoscalingDefinitionMetric) (bool, bool, float64) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		log.Printf("No scrapes found.")
		return false, false, 0
	}
	scaleDownValue, err := strconv.ParseFloat(metric.ScaleDownValue, 64)
	if err != nil {
		log.Printf("Float conversion error - scaleUpValue: %s", err.Error())
		return false, false, 0
	}
	scaleUpValue, err := strconv.ParseFloat(metric.ScaleUpValue, 64)
	if err != nil {
		log.Printf("Float conversion error - scaleDownValue: %s", err.Error())
		return false, false, 0
	}
	switch strings.ToUpper(metric.Algorithm) {
	case "MEAN":
		return calculateMean(scrapeList, scaleDownValue, scaleUpValue)
	case "MEDIAN":
		return calculateMedian(scrapeList, scaleDownValue, scaleUpValue)
	case "TRIMMEDMEAN":
		return calculateRobustMean(scrapeList, scaleDownValue, scaleUpValue, metric.TrimmedPercentage)
	case "ARIMAX":
		return calculateRobustMean(scrapeList, scaleDownValue, scaleUpValue, metric.TrimmedPercentage)
	default:
		return testScrapeListDefault(scrapeList, metric.PercentageOfTestConditionFulfillment)
	}
}

func flattenScrapeList(scrapeList []MetricValidateResult) []MetricValidateResult {
	if scrapeList == nil || len(scrapeList) <= 0 {
		return nil
	}
	var result []MetricValidateResult
	for _, scrape := range scrapeList {
		for _, value := range scrape.Value {
			result = append(result, MetricValidateResult{
				LowerBoundPassed: scrape.LowerBoundPassed,
				UpperBoundPassed: scrape.UpperBoundPassed,
				MetricName:       scrape.MetricName,
				IsMetricValid:    scrape.IsMetricValid,
				Value:            []model2.Value{value}},
			)
		}
	}
	return result
}

func calculateRobustMean(scrapeList []MetricValidateResult, scaleDownValue float64, scaleUpValue float64, trimmedPercentage int) (bool, bool, float64) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		return false, false, 0
	}
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
		return calculateMean(flatScrapeList, scaleDownValue, scaleUpValue)
	}
	flatScrapeList = flatScrapeList[k:]
	flatScrapeList = flatScrapeList[:len(flatScrapeList)-k-1]
	return calculateMean(flatScrapeList, scaleDownValue, scaleUpValue)

}

func calculateMedian(scrapeList []MetricValidateResult, scaleDownValue float64, scaleUpValue float64) (bool, bool, float64) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		return false, false, 0
	}
	flatScrapeList := flattenScrapeList(scrapeList)
	sort.Slice(flatScrapeList, func(i, j int) bool {
		a, aok := flatScrapeList[i].Value[0].(*model2.Scalar)
		b, bok := flatScrapeList[j].Value[0].(*model2.Scalar)
		if aok && bok {
			return float64(a.Value) > float64(b.Value)
		}
		return false
	})
	var median float64
	if len(scrapeList)%2 == 1 {
		median = float64(flatScrapeList[len(flatScrapeList)/2].Value[0].(*model2.Scalar).Value)
	} else {
		a1 := float64(flatScrapeList[len(flatScrapeList)/2-1].Value[0].(*model2.Scalar).Value)
		a2 := float64(flatScrapeList[len(flatScrapeList)/2].Value[0].(*model2.Scalar).Value)
		median = (a1 + a2) / 2.0
	}
	lowerBoundTest := median <= scaleDownValue
	upperBoundTest := median >= scaleUpValue
	return lowerBoundTest, upperBoundTest, median
}

func calculateMean(scrapeList []MetricValidateResult, scaleDownValue float64, scaleUpValue float64) (bool, bool, float64) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		return false, false, 0
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
	mean := sum / float64(len(scrapeList))
	lowerBoundTest := mean <= scaleDownValue
	upperBoundTest := mean >= scaleUpValue
	return lowerBoundTest, upperBoundTest, mean
}

func testScrapeListDefault(scrapeList []MetricValidateResult, percentageOfTestConditionFulfillment int) (bool, bool, float64) {
	if scrapeList == nil || len(scrapeList) <= 0 {
		log.Printf("No scrapes found.")
		return false, false, 0
	}
	var lowerBoundsPassed, upperBoundsPassed = 0, 0
	var lowerBoundsFailed, upperBoundsFailed = 0, 0
	for _, scrape := range scrapeList {
		if !scrape.IsMetricValid {
			lowerBoundsFailed++
			upperBoundsFailed++
			continue
		}
		if scrape.LowerBoundPassed {
			lowerBoundsPassed++
			upperBoundsFailed++
		} else if scrape.UpperBoundPassed {
			upperBoundsPassed++
			lowerBoundsFailed++
		} else {
			lowerBoundsFailed++
			upperBoundsFailed++
		}
	}
	var lowerBoundTest = lowerBoundsPassed/(lowerBoundsPassed+lowerBoundsFailed) >= percentageOfTestConditionFulfillment
	var upperBoundTest = upperBoundsPassed/(upperBoundsPassed+upperBoundsFailed) >= percentageOfTestConditionFulfillment
	return lowerBoundTest, upperBoundTest, 0
}
