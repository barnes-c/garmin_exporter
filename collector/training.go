package collector

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/barnes-c/go-garminconnect/garminconnect"
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("training", defaultEnabled, newTrainingCollector)
}

type trainingCollector struct {
	readinessScore    *prometheus.Desc
	readinessSleep    *prometheus.Desc
	readinessRecovery *prometheus.Desc
	readinessHRVAvg   *prometheus.Desc
	vo2maxGeneric     *prometheus.Desc
	vo2maxCycling     *prometheus.Desc
	fitnessAge        *prometheus.Desc
	race5KSeconds     *prometheus.Desc
	race10KSeconds    *prometheus.Desc
	raceHalfSeconds   *prometheus.Desc
	raceMarathon      *prometheus.Desc
	enduranceScore    *prometheus.Desc
	hillScore         *prometheus.Desc
	logger            *slog.Logger
}

func newTrainingCollector(logger *slog.Logger) (Collector, error) {
	const sub = "training"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &trainingCollector{
		readinessScore:    d("readiness_score", "Training readiness score (0-100)."),
		readinessSleep:    d("readiness_sleep_score", "Sleep component of training readiness score."),
		readinessRecovery: d("readiness_recovery_minutes", "Recovery time in minutes."),
		readinessHRVAvg:   d("readiness_hrv_weekly_avg_ms", "HRV weekly average used in readiness score."),
		vo2maxGeneric:     d("vo2max_generic", "VO2 Max for running/generic activities."),
		vo2maxCycling:     d("vo2max_cycling", "VO2 Max for cycling activities."),
		fitnessAge:        d("fitness_age_years", "Estimated fitness age in years."),
		race5KSeconds:     d("race_prediction_5k_seconds", "Predicted 5K finish time in seconds."),
		race10KSeconds:    d("race_prediction_10k_seconds", "Predicted 10K finish time in seconds."),
		raceHalfSeconds:   d("race_prediction_half_marathon_seconds", "Predicted half marathon finish time in seconds."),
		raceMarathon:      d("race_prediction_marathon_seconds", "Predicted marathon finish time in seconds."),
		enduranceScore:    d("endurance_score", "Overall endurance score."),
		hillScore:         d("hill_score", "Hill climbing score."),
		logger:            logger,
	}, nil
}

func (c *trainingCollector) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}
	now := time.Now()
	c.collectReadiness(ctx, client, now, ch)
	c.collectMaxMetrics(ctx, client, now, ch)
	c.collectRacePredictions(ctx, client, ch)
	c.collectEnduranceScore(ctx, client, now, ch)
	c.collectHillScore(ctx, client, now, ch)
	return nil
}

func (c *trainingCollector) collectReadiness(ctx context.Context, client *garminconnect.Client, now time.Time, ch chan<- prometheus.Metric) {
	readiness, err := client.TrainingReadiness(ctx, now)
	if err != nil {
		c.logger.Debug("training readiness unavailable", "err", err)
		return
	}
	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}
	for _, r := range readiness {
		if r.Score > 0 {
			g(c.readinessScore, float64(r.Score))
			if r.SleepScore > 0 {
				g(c.readinessSleep, float64(r.SleepScore))
			}
			if r.RecoveryTime > 0 {
				g(c.readinessRecovery, float64(r.RecoveryTime))
			}
			if r.HrvWeeklyAverage > 0 {
				g(c.readinessHRVAvg, float64(r.HrvWeeklyAverage))
			}
			break
		}
	}
}

func (c *trainingCollector) collectMaxMetrics(ctx context.Context, client *garminconnect.Client, now time.Time, ch chan<- prometheus.Metric) {
	metrics, err := client.MaxMetrics(ctx, now.AddDate(0, 0, -30), now)
	if err != nil {
		c.logger.Debug("max metrics unavailable", "err", err)
		return
	}
	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}
	for i := len(metrics) - 1; i >= 0; i-- {
		m := metrics[i]
		if m.Generic != nil && m.Generic.VO2MaxValue > 0 {
			g(c.vo2maxGeneric, m.Generic.VO2MaxValue)
			if m.Generic.FitnessAge > 0 {
				g(c.fitnessAge, float64(m.Generic.FitnessAge))
			}
			break
		}
	}
	for i := len(metrics) - 1; i >= 0; i-- {
		m := metrics[i]
		if m.Cycling != nil && m.Cycling.VO2MaxValue > 0 {
			g(c.vo2maxCycling, m.Cycling.VO2MaxValue)
			break
		}
	}
}

func (c *trainingCollector) collectRacePredictions(ctx context.Context, client *garminconnect.Client, ch chan<- prometheus.Metric) {
	preds, err := client.RacePredictions(ctx)
	if err != nil {
		c.logger.Debug("race predictions unavailable", "err", err)
		return
	}
	if preds == nil {
		return
	}
	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}
	if preds.Time5K > 0 {
		g(c.race5KSeconds, float64(preds.Time5K))
	}
	if preds.Time10K > 0 {
		g(c.race10KSeconds, float64(preds.Time10K))
	}
	if preds.TimeHalfMarathon > 0 {
		g(c.raceHalfSeconds, float64(preds.TimeHalfMarathon))
	}
	if preds.TimeMarathon > 0 {
		g(c.raceMarathon, float64(preds.TimeMarathon))
	}
}

func (c *trainingCollector) collectEnduranceScore(ctx context.Context, client *garminconnect.Client, now time.Time, ch chan<- prometheus.Metric) {
	endurance, err := client.EnduranceScore(ctx, now.AddDate(0, 0, -7), now)
	if err != nil {
		c.logger.Debug("endurance score unavailable", "err", err)
		return
	}
	var entries []garminconnect.EnduranceScoreEntry
	if json.Unmarshal(endurance, &entries) != nil {
		return
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Score > 0 {
			ch <- prometheus.MustNewConstMetric(c.enduranceScore, prometheus.GaugeValue, entries[i].Score)
			break
		}
	}
}

func (c *trainingCollector) collectHillScore(ctx context.Context, client *garminconnect.Client, now time.Time, ch chan<- prometheus.Metric) {
	hill, err := client.HillScore(ctx, now.AddDate(0, 0, -7), now)
	if err != nil {
		c.logger.Debug("hill score unavailable", "err", err)
		return
	}
	var entries []garminconnect.HillScoreEntry
	if json.Unmarshal(hill, &entries) != nil {
		return
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].HillScore > 0 {
			ch <- prometheus.MustNewConstMetric(c.hillScore, prometheus.GaugeValue, entries[i].HillScore)
			break
		}
	}
}
