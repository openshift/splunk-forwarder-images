FROM quay.io/redhat-services-prod/openshift/boilerplate:image-v7.0.0 AS builder
COPY . /go/src/github.com/openshift/splunk-forwarder-images
WORKDIR /go/src/github.com/openshift/splunk-forwarder-images
RUN env CGO_ENABLED=1 GOOS=linux go build -o runner ./

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.10-1179.1741863533

COPY --from=builder /go/src/github.com/openshift/splunk-forwarder-images/runner /
COPY .splunk-version /
COPY .splunk-version-hash /
RUN export VERSION=$(cat /.splunk-version) && export VERSION_HASH=$(cat /.splunk-version-hash) && \
    microdnf install -y libsemanage shadow-utils findutils procps wget && \
    microdnf clean all && \
    wget -O /tmp/splunkforwarder.rpm "https://download.splunk.com/products/universalforwarder/releases/${VERSION}/linux/splunkforwarder-${VERSION}-${VERSION_HASH}.x86_64.rpm" && \
    rpm -ivh /tmp/splunkforwarder.rpm && \
    rm -f /tmp/splunkforwarder.rpm && \
    mkdir -p /host && chown splunkfwd:splunkfwd /host && \
    mkdir -p /opt/splunkforwarder/var/{lib,run,spool}/splunk && \
    mkdir -p /opt/splunkforwarder/var/lib/splunk/{authDb,hashDb} && \
    mkdir -p /opt/splunkforwarder/var/run/splunk/appserver/{i18n,modules/static/css} && \
    mkdir -p /opt/splunkforwarder/var/run/splunk/{upload,search_telemetry} && \
    mkdir -p /opt/splunkforwarder/var/spool/dirmoncache && \
    mkdir -p /opt/splunkforwarder/etc/apps/osd_monitored_logs/{local,metadata} && \
    mkdir -p /opt/splunkforwarder/etc/apps/splunkauth/{default,local,metadata} && \
    chown -R splunkfwd:splunkfwd /opt/splunkforwarder && \
    chgrp splunkfwd /runner && \
    chmod +x /runner && \
    sed -i 's/\.maxBackupIndex=5/\.maxBackupIndex=1/g' /opt/splunkforwarder/etc/log.cfg && \
    sed -i 's/\.maxFileSize=25000000/\.maxFileSize=250000/g' /opt/splunkforwarder/etc/log.cfg

ENV SPLUNK_HOME=/opt/splunkforwarder
USER splunkfwd
CMD [ "/runner" ]
