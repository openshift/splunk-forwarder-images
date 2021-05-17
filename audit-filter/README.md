# audit-log-exporter

Enables policy-driven event filtering and deduplication of OpenShift audit logs for OSD

## Enabling in stage

Forwarding audit logs to `openshift_managed_audit_stage` can be enabled by adding a policy file to the `osd-monitored-logs-local` configmap

```bash
oc set data cm/osd-monitored-logs-local --from-file=policy.yaml
```

An example [policy file](policy.yaml) is included in the repo

The [startup script](../containers/forwarder/run.sh) in the universal forwarder image polls this file periodically for changes and enables or disables the input accordingly

## Policy filtering

The policy file uses the [Kubernetes audit policy](https://kubernetes.io/docs/tasks/debug-application-cluster/audit/) format, extended slightly to allow wildcard suffixes for namespaces and user groups

Rules are checked in order, and checking stops after the first matching rule. A `level` of `None` drops the request

The example policy file included in the repo was generated from 4.7 and 4.8 audit log analysis

## Default filtering

Events for which __no policy rules__ match are filtered by the following conditions:

* User events (ie. non-system and non-serviceaccount) are __forwarded__
* Read-only system events (`get`/`list`/`watch` etc) are __dropped__
* Control plane leader lease locks and renewals are __dropped__
* `openshift-*` service account write events that occur within the same namespace are __dropped__
* Requests that had a response code of `409`, `422` or `404` are __dropped__
* All other events are __forwarded__

## Deduplication

The process by which `update` events that include the full object in the `requestBody` are deduplicated is:

1. The `metadata.resourceVersion` and `metadata.generation` fields are removed (resourceVersion remains present at `objectRef.resourceVersion`)
  * A number of [status and data fields](main.go#L38) are also removed, if present, to work around issues with certain resources
2. The event is then converted to a `patch` request by generating a three-way JSON patch based on the previous cached `update` request, if one exists
3. The event is discarded if a patch was generated but its body is empty

Events converted from `update` to `patch` (but _not_ discarded) are annotated indicating the conversion. Metrics for discarded events are recorded in `events_processed_total{decision="drop",reason="no-op write"}`

Because deduplication depends on the presence of the `requestBody`, it will only work when `WriteRequestBody` or `AllRequestBody` audit profiles are used, and will not work for resources that are only audited at the `Metadata` level (such as secrets)

Deduplication can be disabled with `--dedupe=false`

## Testing locally

Fetch the audit logs from a master node with `WriteRequestBody` logging enabled:

```bash
POD=$(oc get pod  -n openshift-kube-apiserver -l apiserver=true -o name | head -n 1)
oc exec -n openshift-kube-apiserver $POD -- bash -c 'cat /var/log/kube-apiserver/audit*.log | gzip -f -9' | gzip -d > audit.log
```

Basic usage:
```bash
./audit-log-exporter --input audit.log --policy policy.yaml > filtered.log
```

By default `audit-log-exporter` will try to follow input files until they are rotated, use the `--follow=false` flag to instead exit at EOF

Metrics can be written to standard error at exit with `--print-metrics`, intended for testing against large audit files:

```bash
./audit-log-exporter --input audit.log --policy policy.yaml --follow=false --print-metrics >/dev/null
[...]
splunkforwarder_audit_filter_events_processed_total{decision="drop",reason="leader lease"} 33833.0
splunkforwarder_audit_filter_events_processed_total{decision="drop",reason="no-op write"} 29555.0
splunkforwarder_audit_filter_events_processed_total{decision="drop",reason="policy rule #1"} 9365.0
splunkforwarder_audit_filter_events_processed_total{decision="drop",reason="policy rule #2"} 77110.0
splunkforwarder_audit_filter_events_processed_total{decision="drop",reason="policy rule #5"} 183.0
splunkforwarder_audit_filter_events_processed_total{decision="drop",reason="response code 409"} 1391.0
splunkforwarder_audit_filter_events_processed_total{decision="drop",reason="response code 422"} 23.0
splunkforwarder_audit_filter_events_processed_total{decision="forward",reason="policy rule #10"} 381.0
splunkforwarder_audit_filter_events_processed_total{decision="forward",reason="policy rule #3"} 180.0
splunkforwarder_audit_filter_events_processed_total{decision="forward",reason="policy rule #4"} 512.0
```

To show only the events that would normally be dropped, use `--invert`

## TODO

* Unit tests
* Better policy
* Move `policy.yaml` and `inputs.conf` into `splunk-forwarder-operator`
* Configurable deduplication (eg, dedupe or merge event when CSVs are copied installed to all namespaces)
* Configurable general field removal/anonymization
* Second-level dedupe on heavy-forwarders
