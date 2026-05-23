package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("bodycomposition", defaultEnabled, newBodyCompositionCollector)
}

type bodyCompositionCollector struct {
	weightGrams  *prometheus.Desc
	bmi          *prometheus.Desc
	bodyFat      *prometheus.Desc
	bodyWater    *prometheus.Desc
	boneMass     *prometheus.Desc
	muscleMass   *prometheus.Desc
	visceralFat  *prometheus.Desc
	metabolicAge *prometheus.Desc
	logger       *slog.Logger
}

func newBodyCompositionCollector(logger *slog.Logger) (Collector, error) {
	const sub = "body_composition"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &bodyCompositionCollector{
		weightGrams:  d("weight_grams_avg", "30-day average body weight in grams."),
		bmi:          d("bmi_avg", "30-day average BMI."),
		bodyFat:      d("fat_percent_avg", "30-day average body fat percentage."),
		bodyWater:    d("water_percent_avg", "30-day average body water percentage."),
		boneMass:     d("bone_mass_grams_avg", "30-day average bone mass in grams."),
		muscleMass:   d("muscle_mass_grams_avg", "30-day average muscle mass in grams."),
		visceralFat:  d("visceral_fat_avg", "30-day average visceral fat rating."),
		metabolicAge: d("metabolic_age_years_avg", "30-day average metabolic age in years."),
		logger:       logger,
	}, nil
}

func (c *bodyCompositionCollector) Update(ch chan<- prometheus.Metric) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}

	now := time.Now()
	bc, err := client.BodyComposition(now.AddDate(0, 0, -30), now)
	if err != nil {
		return err
	}

	avg := bc.TotalAverage
	if avg.Weight == 0 && avg.Bmi == 0 {
		return ErrNoData
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	if avg.Weight > 0 {
		g(c.weightGrams, avg.Weight)
	}
	if avg.Bmi > 0 {
		g(c.bmi, avg.Bmi)
	}
	if avg.BodyFat > 0 {
		g(c.bodyFat, avg.BodyFat)
	}
	if avg.BodyWater > 0 {
		g(c.bodyWater, avg.BodyWater)
	}
	if avg.BoneMass > 0 {
		g(c.boneMass, avg.BoneMass)
	}
	if avg.MuscleMass > 0 {
		g(c.muscleMass, avg.MuscleMass)
	}
	if avg.VisceralFat > 0 {
		g(c.visceralFat, avg.VisceralFat)
	}
	if avg.MetabolicAge > 0 {
		g(c.metabolicAge, avg.MetabolicAge)
	}
	return nil
}
