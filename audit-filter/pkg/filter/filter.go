package filter

import (
	"fmt"
	"log"
	"strings"
	_ "unsafe"

	"github.com/hashicorp/golang-lru"
	jsonp "k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit/policy"
	authz "k8s.io/apiserver/pkg/authorization/authorizer"

	. "github.com/openshift/splunk-forwarder-images/audit-filter/pkg/event"
	. "github.com/openshift/splunk-forwarder-images/audit-filter/pkg/metrics"
	"github.com/openshift/splunk-forwarder-images/audit-filter/pkg/reader"
)

func FilterEvent(e *Event, p *audit.Policy, dedupe bool) bool {

	reason := ""

	// check the policy first
	if rulenum := MatchesPolicy(e, p); rulenum > 0 {

		// record the matching rule number for metrics
		reason = fmt.Sprintf("policy rule #%d", rulenum)

		// policy says drop
		if e.Level == audit.LevelNone {
			return RecordDrop(e, reason)
		}

	} else {

		// rules for events that aren't covered by the policy

		// always keep user events
		if e.User.Username != "" && !(strings.HasPrefix(e.User.Username, "system:")) {
			return RecordForward(e, "user event")
		}

		// drop read-only system events
		if e.Attributes.IsReadOnly() {
			return RecordDrop(e, "system read")
		}

		// drop leader locks
		if _, exists := e.Object.GetAnnotations()["control-plane.alpha.kubernetes.io/leader"]; exists {
			return RecordDrop(e, "leader lease")
		}

		// for metadata-level updates (that can't be deduped), drop authorized intra-namespace system activity
		if e.RequestObject == nil && e.Verb == "update" && strings.Contains(e.User.Username, e.ObjectRef.Namespace) &&
			e.ResponseStatus.Code == 200 {
			return RecordDrop(e, "system update")
		}

		// downgrade RequestResponse level to Request (except for create)
		if e.Verb != "create" && e.Level.GreaterOrEqual(audit.LevelRequestResponse) {
			e.Level = audit.LevelRequest
			e.ResponseObject = nil
		}

	}

	// drop 404s and temporary errors
	if e.ResponseStatus != nil &&
		e.ResponseStatus.Code == 404 || // often operators trying to delete non-existent resources
		e.ResponseStatus.Code == 409 || // update conflicts (resource version too old)
		e.ResponseStatus.Code == 422 { // server busy
		return RecordDrop(e, fmt.Sprintf("response code %d", e.ResponseStatus.Code))
	}

	// deduplicate patches and updates
	if dedupe == true && (e.Verb == "update" || e.Verb == "patch") &&
		e.RequestObject != nil && e.ResponseStatus.Code == 200 &&
		(IsDuplicate(e) || IsEmptyPatch(e)) {
		return RecordDrop(e, "no-op write")
	}

	return RecordForward(e, reason)

}

var filterPolicy *audit.Policy

func LoadPolicy(path string, watch bool) *audit.Policy {
	var err error
	if filterPolicy == nil {
		filterPolicy, err = policy.LoadPolicyFromFile(path)
		if err != nil {
			log.Println("couldn't load policy:", err.Error())
			filterPolicy = DefaultPolicy()
		}
		if watch {
			go reader.WatchPolicyPath(path, filterPolicy)
		}
	}
	return filterPolicy
}

// default policy to use if no valid policy is provided
func DefaultPolicy() *audit.Policy {
	return &audit.Policy{
		Rules: []audit.PolicyRule{
			audit.PolicyRule{
				Level: audit.LevelNone,
				Resources: []audit.GroupResources{
					audit.GroupResources{Group: "authentication.k8s.io", Resources: []string{"tokenreviews"}},
					audit.GroupResources{Group: "authorization.k8s.io", Resources: []string{"subjectaccessreviews"}},
					audit.GroupResources{Group: "coordination.k8s.io", Resources: []string{"leases"}},
				},
			},
		},
	}
}

// checks an event against a policy, returns the matching rule number or 0 or there was no match
func MatchesPolicy(e *Event, p *audit.Policy) int {
	for i, r := range p.Rules {
		if MatchesRule(e, &r) {
			if audit.Level(r.Level).Less(e.Level) {
				e.Level = audit.Level(r.Level)
				if e.Level.Less(audit.LevelRequestResponse) {
					e.ResponseObject = nil
				}
				if r.Level.Less(audit.LevelRequest) {
					e.RequestObject = nil
				}
			}
			return i + 1
		}
	}
	return 0
}

//go:linkname ruleMatches k8s.io/apiserver/pkg/audit/policy.ruleMatches
func ruleMatches(*audit.PolicyRule, authz.Attributes) bool

func matchesWildcard(needle string, haystacks ...string) *string {
	for _, haystack := range haystacks {
		if strings.HasSuffix(needle, "*") && strings.HasPrefix(haystack, needle[:len(needle)-2]) {
			return &haystack
		}
	}
	return nil
}

func MatchesRule(e *Event, r *audit.PolicyRule) bool {
	for i, group := range r.UserGroups {
		if match := matchesWildcard(group, e.User.Groups...); match != nil {
			new := r.DeepCopy()
			new.UserGroups[i] = *match
			r = new
			break
		}
	}
	if e.ObjectRef != nil {
		for i, namespace := range r.Namespaces {
			if match := matchesWildcard(namespace, e.ObjectRef.Namespace); match != nil {
				new := r.DeepCopy()
				new.Namespaces[i] = *match
				r = new
				break
			}
		}
	}
	return ruleMatches(r, e.Attributes)
}

// reduces an update to a patch, returns true on success
func ReduceToPatch(e *Event) bool {
	if now, then, ok := GetPreviousRequest(e); ok {
		patch, err := jsonp.CreateThreeWayJSONMergePatch(then, now, then)
		if err != nil {
			return false
		}
		e.Verb, e.RequestObject.Raw = "patch", patch
		return true
	}
	return false
}

func IsEmptyPatch(e *Event) bool {
	if e.Level.GreaterOrEqual(audit.LevelRequest) && (e.Verb == "patch" || e.Verb == "update") {
		if e.RequestObject == nil || e.RequestBody() == nil {
			return true
		} else {
			raw := string(e.RequestBody())
			return raw == "{}" || raw == "null" || raw == ""
		}
	} else if e.Level.GreaterOrEqual(audit.LevelMetadata) {
		return false
	}
	return true
}

func IsDuplicate(e *Event) bool {
	return ReduceToPatch(e) && IsEmptyPatch(e)
}

var Cache *lru.Cache

func GetPreviousRequest(e *Event) ([]byte, []byte, bool) {
	if e.RequestObject == nil {
		return nil, nil, false
	}
	now := e.RequestBody()
	if Cache == nil {
		var err error
		Cache, err = lru.New(1000)
		if err != nil {
			log.Println("warning: can't init event cache: ", err)
			return now, nil, false
		}
	}
	SetCachedObjectsCount(Cache.Len())
	key := strings.Split(e.RequestURI, "?")[0]
	defer Cache.Add(key, now)
	if val, ok := Cache.Get(key); ok {
		if then, ok := val.([]byte); ok {
			return now, then, ok
		}
	}
	return now, nil, false
}
