package event

import (
	"container/heap"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type Event struct {
	Index uint64
	audit.Event
	Attributes     authorizer.Attributes
	Object         *unstructured.Unstructured
	ResourceLabels prometheus.Labels
	SubjectLabels  prometheus.Labels
}

// returns the json-serialised requestObject
func (e *Event) RequestBody() []byte {
	if e.RequestObject != nil {
		if e.Object != nil && e.RequestObject.Raw == nil {
			e.RequestObject.Raw, _ = e.Object.MarshalJSON()
		}
		return e.RequestObject.Raw
	}
	return nil
}

// EventQueue holds Events and implements heap.Interface
type EventQueue []*Event

func (eq EventQueue) Len() int {
	return len(eq)
}

func (eq EventQueue) Less(i, j int) bool {
	return eq[i].Index < eq[j].Index
}

func (eq EventQueue) Swap(i, j int) {
	eq[i], eq[j] = eq[j], eq[i]
}

func (eq *EventQueue) Push(x interface{}) {
	*eq = append(*eq, x.(*Event))
}

func (eq *EventQueue) Pop() interface{} {
	old := *eq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*eq = old[0 : n-1]
	return item
}

func (eq *EventQueue) Add(e Event) {
	heap.Push(eq, &e)
}

func (eq *EventQueue) Next() Event {
	return *heap.Pop(eq).(*Event)
}
