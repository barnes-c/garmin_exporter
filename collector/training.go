package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("training", newTrainingCollector)
}

type trainingCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	readinessScore    metric.Int64ObservableGauge
	readinessRecovery metric.Int64ObservableGauge
	readinessFactor   metric.Int64ObservableGauge
	vo2maxGeneric     metric.Float64ObservableGauge
	vo2maxCycling     metric.Float64ObservableGauge
	race5KSeconds     metric.Int64ObservableGauge
	race10KSeconds    metric.Int64ObservableGauge
	raceHalfSeconds   metric.Int64ObservableGauge
	raceMarathon      metric.Int64ObservableGauge
	enduranceScore    metric.Float64ObservableGauge
	hillScore         metric.Float64ObservableGauge
}

func newTrainingCollector(log *slog.Logger) (Collector, error) {
	return &trainingCollector{log: log}, nil
}

func (c *trainingCollector) Name() string { return "training" }

func (c *trainingCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	intGauge := func(name, desc, unit string) (metric.Int64ObservableGauge, error) {
		return meter.Int64ObservableGauge(name,
			metric.WithDescription(desc), metric.WithUnit(unit))
	}
	floatGauge := func(name, desc, unit string) (metric.Float64ObservableGauge, error) {
		return meter.Float64ObservableGauge(name,
			metric.WithDescription(desc), metric.WithUnit(unit))
	}

	var err error
	if c.readinessScore, err = intGauge("garmin.training.readiness_score", "Training readiness score (0-100).", "{score}"); err != nil {
		return err
	}
	if c.readinessRecovery, err = intGauge("garmin.training.readiness_recovery_seconds", "Recovery time in seconds.", "s"); err != nil {
		return err
	}
	if c.readinessFactor, err = intGauge("garmin.training.readiness_factor_percent", "Contribution of each factor to training readiness, in percent.", "%"); err != nil {
		return err
	}
	if c.vo2maxGeneric, err = floatGauge("garmin.training.vo2max_generic", "VO2 Max for running/generic activities.", ""); err != nil {
		return err
	}
	if c.vo2maxCycling, err = floatGauge("garmin.training.vo2max_cycling", "VO2 Max for cycling activities.", ""); err != nil {
		return err
	}
	if c.race5KSeconds, err = intGauge("garmin.training.race_prediction_5k_seconds", "Predicted 5K finish time in seconds.", "s"); err != nil {
		return err
	}
	if c.race10KSeconds, err = intGauge("garmin.training.race_prediction_10k_seconds", "Predicted 10K finish time in seconds.", "s"); err != nil {
		return err
	}
	if c.raceHalfSeconds, err = intGauge("garmin.training.race_prediction_half_marathon_seconds", "Predicted half marathon finish time in seconds.", "s"); err != nil {
		return err
	}
	if c.raceMarathon, err = intGauge("garmin.training.race_prediction_marathon_seconds", "Predicted marathon finish time in seconds.", "s"); err != nil {
		return err
	}
	if c.enduranceScore, err = floatGauge("garmin.training.endurance_score", "Overall endurance score.", "{score}"); err != nil {
		return err
	}
	if c.hillScore, err = floatGauge("garmin.training.hill_score", "Hill climbing score.", "{score}"); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.readinessScore, c.readinessRecovery, c.readinessFactor,
		c.vo2maxGeneric, c.vo2maxCycling,
		c.race5KSeconds, c.race10KSeconds, c.raceHalfSeconds, c.raceMarathon,
		c.enduranceScore, c.hillScore,
	)
	return err
}

func (c *trainingCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Training == nil {
		return nil
	}
	t := snap.Training

	for _, r := range t.Readiness {
		if r.Score > 0 {
			o.ObserveInt64(c.readinessScore, int64(r.Score))
			if r.RecoveryTime > 0 {
				o.ObserveInt64(c.readinessRecovery, int64(r.RecoveryTime)*60) // Garmin reports minutes
			}
			for factor, pct := range map[string]int{
				"sleep_score":    r.SleepScoreFactorPercent,
				"sleep_history":  r.SleepHistoryScore,
				"recovery_time":  r.RecoveryTimeFactorPercent,
				"acwr":           r.ACWRFactorPercent,
				"hrv":            r.HRVFactorPercent,
				"stress_history": r.StressHistoryFactorPercent,
			} {
				o.ObserveInt64(c.readinessFactor, int64(pct), metric.WithAttributes(attribute.String("factor", factor)))
			}
			break
		}
	}

	for i := len(t.Max) - 1; i >= 0; i-- {
		m := t.Max[i]
		if m.Generic != nil && m.Generic.VO2MaxValue > 0 {
			o.ObserveFloat64(c.vo2maxGeneric, m.Generic.VO2MaxValue)
			break
		}
	}
	for i := len(t.Max) - 1; i >= 0; i-- {
		m := t.Max[i]
		if m.Cycling != nil && m.Cycling.VO2MaxValue > 0 {
			o.ObserveFloat64(c.vo2maxCycling, m.Cycling.VO2MaxValue)
			break
		}
	}

	if r := t.Race; r != nil {
		if r.Time5K > 0 {
			o.ObserveInt64(c.race5KSeconds, int64(r.Time5K))
		}
		if r.Time10K > 0 {
			o.ObserveInt64(c.race10KSeconds, int64(r.Time10K))
		}
		if r.TimeHalfMarathon > 0 {
			o.ObserveInt64(c.raceHalfSeconds, int64(r.TimeHalfMarathon))
		}
		if r.TimeMarathon > 0 {
			o.ObserveInt64(c.raceMarathon, int64(r.TimeMarathon))
		}
	}

	for i := len(t.Endurance) - 1; i >= 0; i-- {
		if t.Endurance[i].Score > 0 {
			o.ObserveFloat64(c.enduranceScore, t.Endurance[i].Score)
			break
		}
	}
	for i := len(t.Hill) - 1; i >= 0; i-- {
		if t.Hill[i].HillScore > 0 {
			o.ObserveFloat64(c.hillScore, t.Hill[i].HillScore)
			break
		}
	}
	return nil
}
