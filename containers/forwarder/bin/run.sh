#!/usr/bin/env bash

cd /opt/splunkforwarder

SPLUNK_ARGS="--answer-yes"
SPLUNK_APP=osd_monitored_logs

if [[ ${SPLUNK_ACCEPT_LICENSE} == "yes" ]]; then
    SPLUNK_ARGS="${SPLUNK_ARGS} --accept-license"
fi

. bin/setSplunkEnv

export SPLUNK_PASS=$(openssl rand -base64 32 | tr -dc [:alnum:]) APP_PATH=etc/apps/${SPLUNK_APP}

printf "[user_info]\nUSERNAME=admin\nPASSWORD=%s\n" ${SPLUNK_PASS} > etc/system/local/user-seed.conf

# temporary hack until sfo can set up these two files
if [ -s ${APP_PATH}/local/inputs.conf ]; then
    cat > ${APP_PATH}/default/inputs.conf <<-EOF
[script://./bin/audit-log-exporter]
disabled = true
interval = 0
source = kube-apiserver.log
sourcetype = _json
index = openshift_managed_audit_stage
EOF
    cat > ${APP_PATH}/default/props.conf <<-EOF
[_json]
TRUNCATE = 10000000
INDEXED_EXTRACTIONS = json
KV_MODE = none
LINE_BREAKER = ([\r\n]+)
NO_BINARY_CHECK = true
DATETIME_CONFIG =
TIMESTAMP_FIELDS = stageTimestamp
TZ = GMT
disabled = false
EOF
    egrep _meta ${APP_PATH}/local/inputs.conf | head -n 1 >> ${APP_PATH}/default/inputs.conf
fi

./bin/splunk start ${SPLUNK_ARGS}

LAST_INPUTS_CHECKSUM="$(cksum ${APP_PATH}/local/inputs.conf)"

# The above command still forks to the background even with --nodaemon so
# we do the tried and true while true sleep
SPLUNK_PID_FILE="/opt/splunkforwarder/var/run/splunk/splunkd.pid"
while test -s $SPLUNK_PID_FILE; do

    sleep 5

    ps -p $(tr -dc [0-9][:space:] <${SPLUNK_PID_FILE}) > /dev/null || exit 1

    # Check for presence of an audit forwarding policy file
    if [ -s ${APP_PATH}/local/policy.yaml ]; then
        if grep -sq "disabled = true" ${APP_PATH}/default/inputs.conf; then
            sed -i "s/disabled = true/disabled = false/" ${APP_PATH}/default/inputs.conf
            splunk reload exec -auth "admin:${SPLUNK_PASS}" -app ${SPLUNK_APP}
        fi
    else
        if grep -sq "disabled = false" ${APP_PATH}/default/inputs.conf; then
            sed -i "s/disabled = false/disabled = true/" ${APP_PATH}/default/inputs.conf
            splunk reload exec -auth "admin:${SPLUNK_PASS}" -app ${SPLUNK_APP}
        fi
    fi

    INPUTS_CHECKSUM="$(cksum ${APP_PATH}/local/inputs.conf)"

    # If the contents of the inputs.conf file has changed, reload monitor and exec inputs
    if [ "$INPUTS_CHECKSUM" != "$LAST_INPUTS_CHECKSUM" ]; then
        splunk reload monitor -auth "admin:${SPLUNK_PASS}" -app ${SPLUNK_APP}
        splunk reload exec -auth "admin:${SPLUNK_PASS}" -app ${SPLUNK_APP}
    fi

    LAST_INPUTS_CHECKSUM="$INPUTS_CHECKSUM"

    # Clean up old logs
    find var/log -type f -iname '*.log.*' -exec rm {} +

done
