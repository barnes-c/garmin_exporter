package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("trainingstatus", newTrainingStatusCollector)
}

type trainingStatusCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	// TrainingStatus int codes: 0=unknown, 1=not_enough_data, 2=peaking, 3=productive,
	// 4=maintaining, 5=recovery, 6=unproductive, 7=strained, 8=over_reaching, 9=detraining
	status          metric.Int64ObservableGauge
	weeklyLoad      metric.Float64ObservableGauge
	acwrPercent     metric.Int64ObservableGauge
	acwrRatio       metric.Float64ObservableGauge
	aerobicLowLoad  metric.Float64ObservableGauge
	aerobicHighLoad metric.Float64ObservableGauge
	anaerobicLoad   metric.Float64ObservableGauge
}

func newTrainingStatusCollector(log *slog.Logger) (Collector, error) {
	return &trainingStatusCollector{log: log}, nil
}

func (c *trainingStatusCollector) Name() string { return "trainingstatus" }

func (c *trainingStatusCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	if c.status, err = meter.Int64ObservableGauge(
		"garmin.trainingstatus.status",
		metric.WithDescription("Training status code from primary device (see Garmin Connect API docs)."),
	); err != nil {
		return err
	}
	if c.weeklyLoad, err = meter.Float64ObservableGauge(
		"garmin.trainingstatus.weekly_training_load",
		metric.WithDescription("Weekly training load from the primary device."),
	); err != nil {
		return err
	}
	if c.acwrPercent, err = meter.Int64ObservableGauge(
		"garmin.trainingstatus.acwr_percent",
		metric.WithDescription("Acute:chronic workload ratio as a percentage."),
		metric.WithUnit("%"),
	); err != nil {
		return err
	}
	if c.acwrRatio, err = meter.Float64ObservableGauge(
		"garmin.trainingstatus.acwr_ratio",
		metric.WithDescription("Daily acute:chronic workload ratio."),
		metric.WithUnit("{ratio}"),
	); err != nil {
		return err
	}
	if c.aerobicLowLoad, err = meter.Float64ObservableGauge(
		"garmin.trainingstatus.aerobic_low_monthly_load",
		metric.WithDescription("Monthly aerobic low-intensity training load."),
	); err != nil {
		return err
	}
	if c.aerobicHighLoad, err = meter.Float64ObservableGauge(
		"garmin.trainingstatus.aerobic_high_monthly_load",
		metric.WithDescription("Monthly aerobic high-intensity training load."),
	); err != nil {
		return err
	}
	if c.anaerobicLoad, err = meter.Float64ObservableGauge(
		"garmin.trainingstatus.anaerobic_monthly_load",
		metric.WithDescription("Monthly anaerobic training load."),
	); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.status, c.weeklyLoad, c.acwrPercent, c.acwrRatio,
		c.aerobicLowLoad, c.aerobicHighLoad, c.anaerobicLoad,
	)
	return err
}

func (c *trainingStatusCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.TrainingStatus == nil {
		return nil
	}
	resp := snap.TrainingStatus

	if resp.MostRecentTrainingStatus != nil {
		for _, entry := range resp.MostRecentTrainingStatus.LatestTrainingStatusData {
			if !entry.PrimaryTrainingDevice {
				continue
			}
			o.ObserveInt64(c.status, int64(entry.TrainingStatus))
			if entry.WeeklyTrainingLoad != nil {
				o.ObserveFloat64(c.weeklyLoad, *entry.WeeklyTrainingLoad)
			}
			if entry.AcuteTrainingLoad != nil {
				o.ObserveInt64(c.acwrPercent, int64(entry.AcuteTrainingLoad.ACWRPercent))
				o.ObserveFloat64(c.acwrRatio, entry.AcuteTrainingLoad.DailyAcuteChronicWorkloadRatio)
			}
			break
		}
	}

	if resp.MostRecentTrainingLoadBalance != nil {
		for _, entry := range resp.MostRecentTrainingLoadBalance.MetricsMap {
			if !entry.PrimaryTrainingDevice {
				continue
			}
			o.ObserveFloat64(c.aerobicLowLoad, entry.MonthlyLoadAerobicLow)
			o.ObserveFloat64(c.aerobicHighLoad, entry.MonthlyLoadAerobicHigh)
			o.ObserveFloat64(c.anaerobicLoad, entry.MonthlyLoadAnaerobic)
			break
		}
	}
	return nil
}
