package collector

import (
	"context"
	"log/slog"

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
	readinessSleep    metric.Int64ObservableGauge
	readinessRecovery metric.Int64ObservableGauge
	readinessHRVAvg   metric.Int64ObservableGauge
	vo2maxGeneric     metric.Float64ObservableGauge
	vo2maxCycling     metric.Float64ObservableGauge
	fitnessAge        metric.Int64ObservableGauge
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
	if c.readinessSleep, err = intGauge("garmin.training.readiness_sleep_score", "Sleep component of training readiness score.", "{score}"); err != nil {
		return err
	}
	if c.readinessRecovery, err = intGauge("garmin.training.readiness_recovery_minutes", "Recovery time in minutes.", "min"); err != nil {
		return err
	}
	if c.readinessHRVAvg, err = intGauge("garmin.training.readiness_hrv_weekly_avg_ms", "HRV weekly average used in readiness score.", "ms"); err != nil {
		return err
	}
	if c.vo2maxGeneric, err = floatGauge("garmin.training.vo2max_generic", "VO2 Max for running/generic activities.", "mL/min/kg"); err != nil {
		return err
	}
	if c.vo2maxCycling, err = floatGauge("garmin.training.vo2max_cycling", "VO2 Max for cycling activities.", "mL/min/kg"); err != nil {
		return err
	}
	if c.fitnessAge, err = intGauge("garmin.training.fitness_age_years", "Estimated fitness age in years.", "a"); err != nil {
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
		c.readinessScore, c.readinessSleep, c.readinessRecovery, c.readinessHRVAvg,
		c.vo2maxGeneric, c.vo2maxCycling, c.fitnessAge,
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
			if r.SleepScore > 0 {
				o.ObserveInt64(c.readinessSleep, int64(r.SleepScore))
			}
			if r.RecoveryTime > 0 {
				o.ObserveInt64(c.readinessRecovery, int64(r.RecoveryTime))
			}
			if r.HrvWeeklyAverage > 0 {
				o.ObserveInt64(c.readinessHRVAvg, int64(r.HrvWeeklyAverage))
			}
			break
		}
	}

	for i := len(t.Max) - 1; i >= 0; i-- {
		m := t.Max[i]
		if m.Generic != nil && m.Generic.VO2MaxValue > 0 {
			o.ObserveFloat64(c.vo2maxGeneric, m.Generic.VO2MaxValue)
			if m.Generic.FitnessAge > 0 {
				o.ObserveInt64(c.fitnessAge, int64(m.Generic.FitnessAge))
			}
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
