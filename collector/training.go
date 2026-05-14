package collector

import (
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

func (c *trainingCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}
	now := time.Now()

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	readiness, err := garminClient.TrainingReadiness(now)
	if err != nil {
		c.logger.Debug("training readiness unavailable", "err", err)
	} else {
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

	metrics, err := garminClient.MaxMetrics(now.AddDate(0, 0, -30), now)
	if err != nil {
		c.logger.Debug("max metrics unavailable", "err", err)
	} else {
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

	preds, err := garminClient.RacePredictions()
	if err != nil {
		c.logger.Debug("race predictions unavailable", "err", err)
	} else if preds != nil {
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

	endurance, err := garminClient.EnduranceScore(now.AddDate(0, 0, -7), now)
	if err != nil {
		c.logger.Debug("endurance score unavailable", "err", err)
	} else {
		var entries []garminconnect.EnduranceScoreEntry
		if json.Unmarshal(endurance, &entries) == nil {
			for i := len(entries) - 1; i >= 0; i-- {
				if entries[i].Score > 0 {
					g(c.enduranceScore, entries[i].Score)
					break
				}
			}
		}
	}

	hill, err := garminClient.HillScore(now.AddDate(0, 0, -7), now)
	if err != nil {
		c.logger.Debug("hill score unavailable", "err", err)
	} else {
		var entries []garminconnect.HillScoreEntry
		if json.Unmarshal(hill, &entries) == nil {
			for i := len(entries) - 1; i >= 0; i-- {
				if entries[i].HillScore > 0 {
					g(c.hillScore, entries[i].HillScore)
					break
				}
			}
		}
	}

	return nil
}
