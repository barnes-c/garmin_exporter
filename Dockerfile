FROM golang:1.26@sha256:3aff6657219a4d9c14e27fb1d8976c49c29fddb70ba835014f477e1c70636647 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir /data

ARG VERSION
ARG COMMIT
ARG BRANCH
ARG DATE

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X github.com/prometheus/common/version.Version=${VERSION} \
    -X github.com/prometheus/common/version.Revision=${COMMIT} \
    -X github.com/prometheus/common/version.Branch=${BRANCH} \
    -X github.com/prometheus/common/version.BuildDate=${DATE}" \
    -o garmin_exporter .

FROM gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6

ARG COMMIT
ARG DATE
ARG VERSION

LABEL io.prometheus.image.variant="distroless"
LABEL org.opencontainers.image.authors="Christopher Barnes <github@barnes.biz>"
LABEL org.opencontainers.image.created=${DATE}
LABEL org.opencontainers.image.description="OTel-native Prometheus exporter for Garmin Connect"
LABEL org.opencontainers.image.documentation="https://github.com/barnes-c/garmin_exporter"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.revision=${COMMIT}
LABEL org.opencontainers.image.source="https://github.com/barnes-c/garmin_exporter"
LABEL org.opencontainers.image.title="Garmin Exporter"
LABEL org.opencontainers.image.url="https://github.com/barnes-c/garmin_exporter"
LABEL org.opencontainers.image.vendor="Christopher Barnes"
LABEL org.opencontainers.image.version=${VERSION}

COPY --from=builder /src/garmin_exporter /bin/garmin_exporter
COPY --from=builder --chown=65532:65532 /data /data
COPY LICENSE /

EXPOSE      10045
ENTRYPOINT  [ "/bin/garmin_exporter" ]
