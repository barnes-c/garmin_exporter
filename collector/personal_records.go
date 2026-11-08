package collector

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("personal_records", newPersonalRecordsCollector)
}

type personalRecordsCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	value metric.Float64ObservableGauge
}

func newPersonalRecordsCollector(log *slog.Logger) (Collector, error) {
	return &personalRecordsCollector{log: log}, nil
}

func (c *personalRecordsCollector) Name() string { return "personal_records" }

func (c *personalRecordsCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.value, err = meter.Float64ObservableGauge(
		"garmin.personal_records.value",
		metric.WithDescription("Personal record value in raw Garmin units (varies by pr_type)."),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.value)
	return err
}

func (c *personalRecordsCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil {
		return nil
	}
	keys := make(map[int64]string, len(snap.PersonalRecordTypes))
	for _, t := range snap.PersonalRecordTypes {
		keys[int64(t.ID)] = t.Key
	}
	for _, r := range snap.PersonalRecords {
		if r.Value == 0 {
			continue
		}
		o.ObserveFloat64(c.value, r.Value, metric.WithAttributes(
			attribute.String("pr_type", prType(r.TypeID, r.PrTypeLabelKey, keys)),
		))
	}
	return nil
}

// prType names a record by Garmin's type key ("pr.label.5k.run" -> "5k.run").
// Records rarely carry the key themselves, so it comes from the type list,
// falling back to the numeric type ID.
func prType(id int64, labelKey string, keys map[int64]string) string {
	key := keys[id]
	if key == "" {
		key = labelKey
	}
	if key == "" {
		return "type_" + strconv.FormatInt(id, 10)
	}
	return strings.TrimPrefix(key, "pr.label.")
}
