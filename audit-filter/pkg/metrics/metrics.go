package metrics

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"

	. "github.com/openshift/splunk-forwarder-images/audit-filter/pkg/event"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
	"k8s.io/apiserver/pkg/apis/audit"
)

const namespace string = "splunkforwarder"
const subsystem string = "audit_filter"

var (
	registry *prometheus.Registry

	parsedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "events_total",
			Help:      "count of events parsed",
		})

	forwardedResourceCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "events_forwarded_resource",
			Help:      "count of accepted events by resource and verb",
		}, []string{"verb", "resource"})

	forwardedSubjectCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "events_forwarded_subject",
			Help:      "count of accepted events by subject and verb",
		}, []string{"verb", "subject"})

	droppedResourceCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "events_dropped_resource",
			Help:      "count of dropped events by resource and verb",
		}, []string{"verb", "resource"})

	droppedSubjectCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "events_dropped_subject",
			Help:      "count of dropped events by subject kind, verb and reason",
		}, []string{"verb", "subject"})

	processedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "events_processed_total",
			Help:      "count of processed events",
		}, []string{"decision", "reason"})

	errorCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "count of encoding or decoding errors",
		})

	cachedObjects = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cached_objects",
			Help:      "number of objects in cache",
		})
)

func init() {

	prometheus.MustRegister(parsedCounter, processedCounter,
		forwardedResourceCounter, forwardedSubjectCounter,
		droppedResourceCounter, droppedSubjectCounter,
		errorCounter, cachedObjects)

	http.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
		Registry: prometheus.DefaultRegisterer,
	}))

	go http.ListenAndServe(":9090", nil)

}

func Print() {
	mfs, _ := prometheus.DefaultGatherer.Gather()
	w := expfmt.NewEncoder(os.Stderr, expfmt.FmtOpenMetrics)
	for _, mf := range mfs {
		if err := w.Encode(mf); err != nil {
			log.Println(err)
		}
	}
}

func SubjectLabels(e *audit.Event) prometheus.Labels {
	s := "User"
	if strings.HasPrefix(e.User.Username, "system:serviceaccount:") {
		s = "ServiceAccount"
	} else if strings.HasPrefix(e.User.Username, "system:node:") {
		s = "Node"
	} else if strings.HasPrefix(e.User.Username, "system:anonymous") {
		s = "Anonymous"
	} else if strings.HasPrefix(e.User.Username, "system:") {
		s = "SystemUser"
	}
	return prometheus.Labels(map[string]string{
		"verb":    e.Verb,
		"subject": s,
	})
}

func ResourceLabels(e *audit.Event) prometheus.Labels {
	r := "" // non-resource request
	if e.ObjectRef != nil {
		r = e.ObjectRef.Resource
		if e.ObjectRef.Subresource != "" {
			r = r + "/" + e.ObjectRef.Subresource
		}
	}
	return prometheus.Labels(map[string]string{
		"verb":     e.Verb,
		"resource": r,
	})
}

func VerdictLabels(action, reason string) prometheus.Labels {
	return prometheus.Labels(map[string]string{
		"decision": action,
		"reason":   reason,
	})
}

func RecordDrop(e *Event, reason string) bool {
	droppedResourceCounter.With(e.ResourceLabels).Inc()
	droppedSubjectCounter.With(e.SubjectLabels).Inc()
	processedCounter.With(VerdictLabels("drop", reason)).Inc()
	return false
}

func RecordForward(e *Event, reason string) bool {
	forwardedResourceCounter.With(e.ResourceLabels).Inc()
	forwardedSubjectCounter.With(e.SubjectLabels).Inc()
	processedCounter.With(VerdictLabels("forward", reason)).Inc()
	return true
}

func RecordParsed() {
	parsedCounter.Inc()
}

func RecordError() {
	errorCounter.Inc()
}

func SetCachedObjectsCount(i int) {
	cachedObjects.Set(float64(i))
}
