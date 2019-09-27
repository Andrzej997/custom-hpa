package autoscaler

import (
	"custom-hpa/clients"
	"custom-hpa/metrics"
	"custom-hpa/model"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"log"
	"time"
)

type DefinitionChanges struct {
	definitionsToAdd    []model.AutoscalingDefinition
	definitionsToRemove []model.AutoscalingDefinition
}

type DefinitionChannel struct {
	definition                     model.AutoscalingDefinition
	metricChannels                 []MetricChannels
	mainAutoscaleEvaluationChannel chan AutoscaleEvaluation
	clearMetricBufferChannel       chan model.AutoscalingDefinitionMetric
}

type MetricChannels struct {
	metric                        model.AutoscalingDefinitionMetric
	testResultsChannel            chan metrics.TestResult
	scrapeInterval                chan bool
	testInterval                  chan bool
	autoscaleEvaluation           chan AutoscaleEvaluation
	closeEvaluationProcessChannel chan bool
	closeRewriteChannel           chan bool
	clearChannel                  chan bool
}

func MainAutoscalingLoop(client *clients.Client, extensionsClient *kubernetes.Clientset) {
	var newDefinitions []model.AutoscalingDefinition
	var channels []DefinitionChannel
	for {
		oldDefinitions := newDefinitions
		def, err := client.AutoscalerDefinitions("default").List(meta_v1.ListOptions{})
		if err != nil {
			log.Printf("Error %s", err.Error())
			continue
		}
		if def == nil {
			continue
		}
		newDefinitions = def.Items
		changes := detectDefinitionChanges(newDefinitions, oldDefinitions)

		if changes.definitionsToAdd != nil {
			addedChannels := addDefinitions(changes.definitionsToAdd, extensionsClient)
			channels = append(channels, addedChannels...)
		}
		if changes.definitionsToRemove != nil {
			removedChannels := removeDefinitions(changes.definitionsToRemove, channels)
			channels = removeAllMatchingItemsFromArray(channels, removedChannels)
		}
		time.Sleep(time.Duration(60.0))
	}
}

func addDefinitions(definitions []model.AutoscalingDefinition, client *kubernetes.Clientset) []DefinitionChannel {
	var result []DefinitionChannel
	for _, definition := range definitions {
		if &definition == nil {
			continue
		}
		log.Printf("---------------------------------")
		log.Printf("Checking %s", definition.Spec.ScaleTarget.MatchLabel)

		if definition.Spec.Metrics == nil && len(definition.Spec.Metrics) <= 0 {
			log.Printf("No metrics found in definition")
		}
		var channel = DefinitionChannel{
			definition:                     definition,
			metricChannels:                 make([]MetricChannels, len(definition.Spec.Metrics)),
			mainAutoscaleEvaluationChannel: make(chan AutoscaleEvaluation),
			clearMetricBufferChannel:       make(chan model.AutoscalingDefinitionMetric),
		}
		for i, metric := range definition.Spec.Metrics {
			scrapeResultChannel, err := metrics.MakeScrape(metric)
			if err != nil {
				log.Printf("Scrape error: %s", err.Error())
				continue
			}
			testResultsChannel, err := metrics.MakeTest(metric, scrapeResultChannel)
			if err != nil {
				log.Printf("Test error: %s", err.Error())
				continue
			}

			autoscaleEvaluationResult := EvaluateAutoscaling(testResultsChannel, metric)
			channel.metricChannels[i] = MetricChannels{
				metric:                        metric,
				testResultsChannel:            testResultsChannel.TestResultsChannel,
				scrapeInterval:                testResultsChannel.ScrapeInterval,
				testInterval:                  testResultsChannel.TestInterval,
				autoscaleEvaluation:           autoscaleEvaluationResult.AutoscaleEvaluation,
				closeEvaluationProcessChannel: autoscaleEvaluationResult.CloseEvaluationProcessChannel,
				closeRewriteChannel:           rewriteToMainChannel(autoscaleEvaluationResult, channel.mainAutoscaleEvaluationChannel),
				clearChannel:                  autoscaleEvaluationResult.ClearBufferChannel,
			}
		}
		result = append(result, channel)
		rewriteToConcreteClearBufferChannel(channel.clearMetricBufferChannel, channel.metricChannels)
		StartAutoscaleProcess(channel.mainAutoscaleEvaluationChannel, client, definition, channel.clearMetricBufferChannel)
	}
	return result
}

func rewriteToConcreteClearBufferChannel(clearMetricBufferChannel chan model.AutoscalingDefinitionMetric, metricChannels []MetricChannels) {
	go func() {
		for {
			select {
			case metric := <-clearMetricBufferChannel:
				for _, mc := range metricChannels {
					if &mc.metric == &metric {
						mc.clearChannel <- true
					}
				}
			}
		}
	}()
}

func rewriteToMainChannel(autoscaleEvaluationResult AutoscaleEvaluationResult, mainAutoscaleEvaluationChannel chan AutoscaleEvaluation) chan bool {
	var closeChannel = make(chan bool)
	go func() {
		for {
			select {
			case <-closeChannel:
				return
			case value := <-autoscaleEvaluationResult.AutoscaleEvaluation:
				mainAutoscaleEvaluationChannel <- value
			}
		}
	}()
	return closeChannel
}

func removeDefinitions(definitions []model.AutoscalingDefinition, channels []DefinitionChannel) []DefinitionChannel {
	var result []DefinitionChannel
	for _, def := range definitions {
		log.Printf("Removing definition for: %s", def.Spec.ScaleTarget.MatchLabel)
		var channel *DefinitionChannel
		for _, ch := range channels {
			if &def == &ch.definition {
				channel = &ch
			}
		}
		if channel != nil {
			for _, mc := range channel.metricChannels {
				mc.scrapeInterval <- true
				mc.testInterval <- true
				mc.closeEvaluationProcessChannel <- true
				mc.closeRewriteChannel <- true
				close(mc.autoscaleEvaluation)
				close(mc.testResultsChannel)
			}
			result = append(result, *channel)
		}
	}
	return result
}

func detectDefinitionChanges(newDefinitions []model.AutoscalingDefinition, oldDefinitions []model.AutoscalingDefinition) DefinitionChanges {
	if oldDefinitions == nil || len(oldDefinitions) <= 0 {
		return DefinitionChanges{
			definitionsToAdd:    newDefinitions,
			definitionsToRemove: nil,
		}
	}
	if newDefinitions == nil || len(newDefinitions) <= 0 {
		return DefinitionChanges{
			definitionsToAdd:    nil,
			definitionsToRemove: oldDefinitions,
		}
	}
	var definitionsToRemove []model.AutoscalingDefinition
	for _, old := range oldDefinitions {
		var found = false
		for _, n := range newDefinitions {
			if old.Name == n.Name {
				found = true
			}
		}
		if !found {
			definitionsToRemove = append(definitionsToRemove, old)
		}
	}
	var definitionsToAdd []model.AutoscalingDefinition
	for _, n := range newDefinitions {
		var found = false
		for _, old := range oldDefinitions {
			if n.Name == old.Name {
				found = true
			}
		}
		if !found {
			definitionsToAdd = append(definitionsToAdd, n)
		}
	}
	return DefinitionChanges{
		definitionsToAdd:    definitionsToAdd,
		definitionsToRemove: definitionsToRemove,
	}
}

func removeAllMatchingItemsFromArray(array []DefinitionChannel, itemsToDelete []DefinitionChannel) []DefinitionChannel {
	if itemsToDelete == nil || len(itemsToDelete) <= 0 {
		return array
	}
	var result []DefinitionChannel
	for _, item := range array {
		var toDelete = false
		for _, itemsToDelete := range array {
			if &item == &itemsToDelete {
				toDelete = true
				break
			}
		}
		if !toDelete {
			result = append(result, item)
		}
	}
	return result
}
