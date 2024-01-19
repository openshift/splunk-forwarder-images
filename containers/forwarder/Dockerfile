FROM quay.io/app-sre/golangci-lint:latest AS builder
ADD . /go/src/github.com/openshift/splunk-forwarder-images
WORKDIR /go/src/github.com/openshift/splunk-forwarder-images
RUN env CGO_ENABLED=1 GOOS=linux go build -ldflags "-extldflags=-static -w -s" -o runner ./

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.9-1108

COPY --from=builder /go/src/github.com/openshift/splunk-forwarder-images/runner /
ADD containers/forwarder/bin/systemctl_wrapper.sh /bin/systemctl
ADD .splunk-version /
ADD .splunk-version-hash /
RUN export VERSION=$(cat /.splunk-version) && export VERSION_HASH=$(cat /.splunk-version-hash) && \
    microdnf install -y libsemanage shadow-utils findutils procps wget && \
    microdnf clean all && \
    wget -O /tmp/splunkforwarder.rpm "https://download.splunk.com/products/universalforwarder/releases/${VERSION}/linux/splunkforwarder-${VERSION}-${VERSION_HASH}-linux-2.6-x86_64.rpm" && \
    rpm -ivh /tmp/splunkforwarder.rpm && \
    rm -f /tmp/splunkforwarder.rpm && \
    mkdir -p /host && chown splunk:splunk /host && \
    mkdir -p /opt/splunkforwarder/var/{lib,run,spool}/splunk && \
    mkdir -p /opt/splunkforwarder/var/lib/splunk/{authDb,hashDb} && \
    mkdir -p /opt/splunkforwarder/var/run/splunk/appserver/{i18n,modules/static/css} && \
    mkdir -p /opt/splunkforwarder/var/run/splunk/{upload,search_telemetry} && \
    mkdir -p /opt/splunkforwarder/var/spool/dirmoncache && \
    mkdir -p /opt/splunkforwarder/etc/apps/osd_monitored_logs/{local,metadata} && \
    mkdir -p /opt/splunkforwarder/etc/apps/splunkauth/{default,local,metadata} && \
    chown -R splunk:splunk /opt/splunkforwarder && \
    chgrp splunk /runner && \
    chmod +x /runner /bin/systemctl && \
    sed -i 's/\.maxBackupIndex=5/\.maxBackupIndex=1/g' /opt/splunkforwarder/etc/log.cfg && \
    sed -i 's/\.maxFileSize=25000000/\.maxFileSize=250000/g' /opt/splunkforwarder/etc/log.cfg

ENV SPLUNK_HOME=/opt/splunkforwarder
USER splunk
CMD [ "/runner" ]
