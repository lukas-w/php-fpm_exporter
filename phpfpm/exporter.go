// Copyright © 2018 Enrico Stahn <enrico.stahn@gmail.com>
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package phpfpm provides convenient access to PHP-FPM pool data
package phpfpm

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "phpfpm"
)

// Exporter configures and exposes PHP-FPM metrics to Prometheus.
type Exporter struct {
	mutex       sync.Mutex
	PoolManager PoolManager

	CountProcessState bool

	up                       *prometheus.Desc
	scrapeFailues            *prometheus.Desc
	startSince               *prometheus.Desc
	acceptedConnections      *prometheus.Desc
	listenQueue              *prometheus.Desc
	maxListenQueue           *prometheus.Desc
	listenQueueLength        *prometheus.Desc
	processes                *prometheus.Desc
	maxActiveProcesses       *prometheus.Desc
	maxChildrenReached       *prometheus.Desc
	slowRequests             *prometheus.Desc
	processRequests          *prometheus.Desc
	processLastRequestMemory *prometheus.Desc
	processLastRequestCPU    *prometheus.Desc
	contentLength            *prometheus.Desc
	processRequestDuration   *prometheus.Desc
	processState             *prometheus.Desc
}

// NewExporter creates a new Exporter for a PoolManager and configures the necessary metrics.
func NewExporter(pm PoolManager) *Exporter {
	return &Exporter{
		PoolManager: pm,

		CountProcessState: false,

		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Could PHP-FPM be reached?",
			[]string{"pool", "scrape_uri"},
			nil),

		scrapeFailues: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "scrape_failures_total"),
			"The number of failures scraping from PHP-FPM.",
			[]string{"pool", "scrape_uri"},
			nil),

		startSince: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "start_time_seconds"),
			"The number of seconds since FPM has started.",
			[]string{"pool", "scrape_uri"},
			nil),

		acceptedConnections: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "accepted_connections_total"),
			"The number of requests accepted by the pool.",
			[]string{"pool", "scrape_uri"},
			nil),

		listenQueue: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "listen_queue"),
			"The number of requests in the queue of pending connections.",
			[]string{"pool", "scrape_uri"},
			nil),

		maxListenQueue: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "max_listen_queue"),
			"The maximum number of requests in the queue of pending connections since FPM has started.",
			[]string{"pool", "scrape_uri"},
			nil),

		listenQueueLength: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "listen_queue_length"),
			"The size of the socket queue of pending connections.",
			[]string{"pool", "scrape_uri"},
			nil),

		processes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "processes"),
			"The number of processes.",
			[]string{"pool", "scrape_uri", "state"},
			nil),

		maxActiveProcesses: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "max_active_processes"),
			"The maximum number of active processes since FPM has started.",
			[]string{"pool", "scrape_uri"},
			nil),

		maxChildrenReached: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "max_children_reached_total"),
			"The number of times, the process limit has been reached, when pm tries to start more children (works only for pm 'dynamic' and 'ondemand').",
			[]string{"pool", "scrape_uri"},
			nil),

		slowRequests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "slow_requests_total"),
			"The number of requests that exceeded your 'request_slowlog_timeout' value.",
			[]string{"pool", "scrape_uri"},
			nil),

		processRequests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "process_requests_total"),
			"The number of requests the process has served.",
			[]string{"pool", "child", "scrape_uri"},
			nil),

		processLastRequestMemory: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "process_last_request_memory_bytes"),
			"The max amount of memory the last request consumed.",
			[]string{"pool", "child", "scrape_uri", "method", "uri", "script"},
			nil),

		processLastRequestCPU: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "process_last_request_cpu"),
			"The %cpu the last request consumed.",
			[]string{"pool", "child", "scrape_uri", "method", "uri", "script"},
			nil),

		contentLength: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "content_length"),
			"The content length of the last request (only with POST)",
			[]string{"pool", "child", "scrape_uri", "method", "uri", "script"},
			nil),

		processRequestDuration: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "process_request_duration_seconds"),
			"The duration in seconds of the requests.",
			[]string{"pool", "child", "scrape_uri", "method", "uri", "script"},
			nil),

		processState: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "process_state"),
			"The state of the process (Idle, Running, ...).",
			[]string{"pool", "child", "state", "scrape_uri"},
			nil),
	}
}

// Collect updates the Pools and sends the collected metrics to Prometheus
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if err := e.PoolManager.Update(); err != nil {
		log.Error(err)
	}

	for _, pool := range e.PoolManager.Pools {
		ch <- prometheus.MustNewConstMetric(e.scrapeFailues, prometheus.CounterValue, float64(pool.ScrapeFailures), pool.Name, pool.Address)

		if pool.ScrapeError != nil {
			ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0, pool.Name, pool.Address)
			log.Errorf("Error scraping PHP-FPM: %v", pool.ScrapeError)
			continue
		}

		active, idle := CountProcessState(pool.Processes)
		if !e.CountProcessState && (active != pool.ActiveProcesses || idle != pool.IdleProcesses) {
			log.Error("Inconsistent active and idle processes reported. Set `--phpfpm.fix-process-count` to have this calculated by php-fpm_exporter instead.")
		}

		if !e.CountProcessState {
			active = pool.ActiveProcesses
			idle = pool.IdleProcesses
		}

		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 1, pool.Name, pool.Address)
		ch <- prometheus.MustNewConstMetric(e.startSince, prometheus.GaugeValue, float64(pool.StartSince), pool.Name, pool.Address)
		ch <- prometheus.MustNewConstMetric(e.acceptedConnections, prometheus.CounterValue, float64(pool.AcceptedConnections), pool.Name, pool.Address)
		ch <- prometheus.MustNewConstMetric(e.listenQueue, prometheus.GaugeValue, float64(pool.ListenQueue), pool.Name, pool.Address)
		ch <- prometheus.MustNewConstMetric(e.maxListenQueue, prometheus.CounterValue, float64(pool.MaxListenQueue), pool.Name, pool.Address)
		ch <- prometheus.MustNewConstMetric(e.listenQueueLength, prometheus.GaugeValue, float64(pool.ListenQueueLength), pool.Name, pool.Address)
		ch <- prometheus.MustNewConstMetric(e.processes, prometheus.GaugeValue, float64(idle), pool.Name, pool.Address, "idle")
		ch <- prometheus.MustNewConstMetric(e.processes, prometheus.GaugeValue, float64(active), pool.Name, pool.Address, "active")
		ch <- prometheus.MustNewConstMetric(e.maxActiveProcesses, prometheus.GaugeValue, float64(pool.MaxActiveProcesses), pool.Name, pool.Address)
		ch <- prometheus.MustNewConstMetric(e.maxChildrenReached, prometheus.CounterValue, float64(pool.MaxChildrenReached), pool.Name, pool.Address)
		ch <- prometheus.MustNewConstMetric(e.slowRequests, prometheus.CounterValue, float64(pool.SlowRequests), pool.Name, pool.Address)

		for childNumber, process := range pool.Processes {
			childName := fmt.Sprintf("%d", childNumber)

			states := map[string]int{
				PoolProcessRequestIdle:           0,
				PoolProcessRequestRunning:        0,
				PoolProcessRequestFinishing:      0,
				PoolProcessRequestReadingHeaders: 0,
				PoolProcessRequestInfo:           0,
				PoolProcessRequestEnding:         0,
			}
			states[process.State]++

			for stateName, inState := range states {
				ch <- prometheus.MustNewConstMetric(e.processState, prometheus.GaugeValue, float64(inState), pool.Name, childName, stateName, pool.Address)
			}
			ch <- prometheus.MustNewConstMetric(e.processRequests, prometheus.CounterValue, float64(process.Requests), pool.Name, childName, pool.Address)
			ch <- prometheus.MustNewConstMetric(e.processLastRequestMemory, prometheus.GaugeValue, float64(process.LastRequestMemory), pool.Name, childName, pool.Address, process.RequestMethod, process.RequestURI, process.Script)
			ch <- prometheus.MustNewConstMetric(e.processLastRequestCPU, prometheus.GaugeValue, process.LastRequestCPU, pool.Name, childName, pool.Address, process.RequestMethod, process.RequestURI, process.Script)
			ch <- prometheus.MustNewConstMetric(e.processRequestDuration, prometheus.GaugeValue, float64(process.RequestDuration/1e6), pool.Name, childName, pool.Address, process.RequestMethod, process.RequestURI, process.Script)
			ch <- prometheus.MustNewConstMetric(e.contentLength, prometheus.GaugeValue, float64(process.ContentLength), pool.Name, childName, pool.Address, process.RequestMethod, process.RequestURI, process.Script)
		}
	}
}

// Describe exposes the metric description to Prometheus
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up
	ch <- e.startSince
	ch <- e.acceptedConnections
	ch <- e.listenQueue
	ch <- e.maxListenQueue
	ch <- e.listenQueueLength
	ch <- e.processes
	ch <- e.maxActiveProcesses
	ch <- e.maxChildrenReached
	ch <- e.slowRequests
	ch <- e.processState
	ch <- e.processRequests
	ch <- e.processLastRequestMemory
	ch <- e.processLastRequestCPU
	ch <- e.processRequestDuration
}
