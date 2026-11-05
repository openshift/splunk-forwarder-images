FROM quay.io/redhat-services-prod/openshift/boilerplate:image-v8.4.4 AS builder
WORKDIR /build
COPY go.mod go.sum runner.go ./
RUN env CGO_ENABLED=1 GOOS=linux go build -o runner

# Keep package installation tools in a separate stage from the exported image.
FROM registry.access.redhat.com/hi/core-runtime:latest AS runtime-base

FROM registry.access.redhat.com/hi/core-runtime:latest-builder AS installer
USER 0
COPY --from=runtime-base / /runtime/

RUN dnf --installroot=/runtime --use-host-config --setopt=install_weak_deps=False install -y \
        bash coreutils-single libsemanage shadow-utils findutils procps-ng grep sed && \
    dnf --installroot=/runtime --use-host-config clean all

COPY --from=builder /build/runner /runtime/
COPY containers/forwarder/bin/systemctl_wrapper.sh /runtime/bin/systemctl
COPY .splunk-version /
COPY .splunk-version-hash /
RUN export VERSION=$(cat /.splunk-version) && export VERSION_HASH=$(cat /.splunk-version-hash) && \
    curl --fail --location --output /tmp/SplunkPGPKey.pub https://docs.splunk.com/images/6/6b/SplunkPGPKey.pub && \
    rpm --root /runtime --import /tmp/SplunkPGPKey.pub && \
    curl --fail --location --output /tmp/splunkforwarder.rpm "https://download.splunk.com/products/universalforwarder/releases/${VERSION}/linux/splunkforwarder-${VERSION}-${VERSION_HASH}.x86_64.rpm" && \
    rpm --root /runtime -ivh /tmp/splunkforwarder.rpm && \
    rm -f /tmp/splunkforwarder.rpm && \
    chroot /runtime /bin/bash -ec 'mkdir -p /host && chown splunkfwd:splunkfwd /host && \
    mkdir -p /opt/splunkforwarder/var/{lib,run,spool}/splunk && \
    mkdir -p /opt/splunkforwarder/var/lib/splunk/{authDb,hashDb} && \
    mkdir -p /opt/splunkforwarder/var/run/splunk/appserver/{i18n,modules/static/css} && \
    mkdir -p /opt/splunkforwarder/var/run/splunk/{upload,search_telemetry} && \
    mkdir -p /opt/splunkforwarder/var/spool/dirmoncache && \
    mkdir -p /opt/splunkforwarder/etc/apps/osd_monitored_logs/{local,metadata} && \
    mkdir -p /opt/splunkforwarder/etc/apps/splunkauth/{default,local,metadata} && \
    chown -R splunkfwd:splunkfwd /opt/splunkforwarder && \
    chgrp splunkfwd /runner && \
    chmod +x /runner /bin/systemctl && \
    sed -i "s/\.maxBackupIndex=5/\.maxBackupIndex=1/g" /opt/splunkforwarder/etc/log.cfg && \
    sed -i "s/\.maxFileSize=25000000/\.maxFileSize=250000/g" /opt/splunkforwarder/etc/log.cfg'

FROM runtime-base
COPY --from=installer /runtime/ /
COPY .splunk-version .splunk-version-hash /
# Clear any entrypoint inherited from the base so CMD starts the runner.
ENTRYPOINT []
ENV SPLUNK_HOME=/opt/splunkforwarder
USER splunkfwd
CMD [ "/runner" ]
