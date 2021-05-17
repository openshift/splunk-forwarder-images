package main

import (
	"container/heap"
	"log"
	"os"
	"runtime"
	"sync"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apiserver/pkg/apis/audit"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	auditpkg "k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/audit/event"

	. "github.com/openshift/splunk-forwarder-images/audit-filter/pkg/event"
	. "github.com/openshift/splunk-forwarder-images/audit-filter/pkg/filter"
	"github.com/openshift/splunk-forwarder-images/audit-filter/pkg/metrics"
	. "github.com/openshift/splunk-forwarder-images/audit-filter/pkg/reader"
)

var workers int
var filter bool
var follow bool
var dedupe bool
var invert bool
var printmetrics bool
var ignoreFields [][]string

func main() {

	inputfiles := []string{
		"/host/var/log/kube-apiserver/audit.log",
		"/host/var/log/openshift-apiserver/audit.log",
		"/host/var/log/oauth-apiserver/audit.log",
	}

	policyfile := "/opt/splunkforwarder/etc/apps/osd_monitored_logs/local/policy.yaml"

	// fields to ignore when deduplicating update requests
	// TODO: make configurable, allow for anonymizing fields without dedupe
	ignoreFields = [][]string{
		// strictly required for dedupe to work
		[]string{"metadata", "resourceVersion"},
		[]string{"metadata", "generation"},
		// workarounds and space-saving space
		[]string{"metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration"},
		[]string{"status", "relatedObjects"}, // cluster-operators reorder this slice on every update
		[]string{"status", "conditions"},
		[]string{"status", "lastSyncTimestamp"}, // from credentialsrequests
		[]string{"status", "components"}, // olm reorders this slice on every update
		[]string{"spec", "icon"},
		[]string{"data", "ca.crt"},
		[]string{"data", "ca-bundle.crt"},
		[]string{"data", "service-ca.crt"},
	}

	dedupe = true
	follow = true
	invert = false
	printmetrics = false
	workers = 1 + runtime.NumCPU()

	pflag.StringSliceVar(&inputfiles, "input", inputfiles, "audit log file(s) to monitor (can be repeated)")
	pflag.StringVar(&policyfile, "policy", policyfile, "path to filter policy file")
	pflag.BoolVar(&follow, "follow", follow, "follow and reopen files when rotated (tail -F)")
	pflag.BoolVar(&dedupe, "dedupe", dedupe, "deduplicate repeated update requests")
	pflag.BoolVar(&invert, "invert", invert, "only output dropped events (for testing)")
	pflag.IntVar(&workers, "workers", workers, "number of decode workers")
	pflag.BoolVar(&printmetrics, "print-metrics", printmetrics, "print metrics to stderr at exit")

	pflag.Parse()

	policy := LoadPolicy(policyfile, follow)

	lines := ReadFiles(follow, inputfiles...)
	decoded := make(chan Event)
	filtered := make(chan audit.Event)

	go Decode(lines, decoded)
	go Filter(decoded, policy, filtered)
	Encode(filtered)

	if printmetrics {
		metrics.Print()
	}

}

// worker group for decoding json input
func Decode(in <-chan Line, out chan Event) {
	defer close(out)
	gvk := (&auditv1.Event{}).TypeMeta.GroupVersionKind()
	wg := sync.WaitGroup{}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			codec := auditpkg.Codecs.LegacyCodec()
			for line := range in {
				ev := &audit.Event{}
				_, _, err := codec.Decode(line.Data, &gvk, ev)
				if err != nil {
					metrics.RecordError()
					log.Println("decode failed:", err)
				}
				obj := &unstructured.Unstructured{}
				if dedupe == true && ev.Verb == "update" && ev.RequestObject != nil {
					if err := obj.UnmarshalJSON(ev.RequestObject.Raw); err == nil {
						for i := range ignoreFields {
							unstructured.RemoveNestedField(obj.Object, ignoreFields[i]...)
						}
						ev.RequestObject.Raw = nil
					}
				}
				attr, err := event.NewAttributes(ev)
				if err != nil {
					metrics.RecordError()
					log.Println("attributes error:", err)
				}
				out <- Event{line.Index, *ev, attr, obj, metrics.ResourceLabels(ev), metrics.SubjectLabels(ev)}
				metrics.RecordParsed()
			}
		}()
	}
	wg.Wait()
}

// reorders and filters decoded events
func Filter(in chan Event, policy *audit.Policy, out chan audit.Event) {
	defer close(out)
	q := make(EventQueue, 0)
	heap.Init(&q)
	index := uint64(1)
	for event := range in {
		for event.Index > index {
			q.Add(event)
			event = (<-in)
		}
		for event.Index == index {
			if FilterEvent(&event, policy, dedupe) != invert {
				out <- event.Event
			}
			index += 1
			if len(q) > 0 {
				if event = q.Next(); event.Index != index {
					q.Add(event)
					break
				}
			}
		}
	}
}

// encodes filtered events
func Encode(in chan audit.Event) {
	codec := auditpkg.Codecs.LegacyCodec(auditv1.SchemeGroupVersion)
	for event := range in {
		codec.Encode(&event, os.Stdout)
	}
}
