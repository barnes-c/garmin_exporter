FROM golang:1.26 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION
ARG COMMIT
ARG BRANCH
ARG DATE
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/prometheus/common/version.Version=${VERSION} \
    -X github.com/prometheus/common/version.Revision=${COMMIT} \
    -X github.com/prometheus/common/version.Branch=${BRANCH} \
    -X github.com/prometheus/common/version.BuildDate=${DATE}" \
    -o garmin_exporter .

FROM gcr.io/distroless/static-debian13:nonroot
LABEL maintainer="Christopher Barnes <github@barnes.biz>"
LABEL org.opencontainers.image.authors="Christopher Barnes"
LABEL org.opencontainers.image.vendor="Christopher Barnes"
LABEL org.opencontainers.image.title="garmin_exporter"
LABEL org.opencontainers.image.description="Exporter for Garmin Connect health and training metrics"
LABEL org.opencontainers.image.source="https://github.com/barnes-c/garmin_exporter"
LABEL org.opencontainers.image.url="https://github.com/barnes-c/garmin_exporter"
LABEL org.opencontainers.image.documentation="https://github.com/barnes-c/garmin_exporter"
LABEL org.opencontainers.image.licenses="Apache License 2.0"
LABEL io.prometheus.image.variant="distroless"

COPY --from=builder /src/garmin_exporter /bin/garmin_exporter
COPY LICENSE /

EXPOSE      10043
ENTRYPOINT  [ "/bin/garmin_exporter" ]
