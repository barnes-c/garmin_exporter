package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("body", defaultEnabled, newBodyCollector)
}

type bodyCollector struct {
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

func newBodyCollector(logger *slog.Logger) (Collector, error) {
	const sub = "body"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &bodyCollector{
		weightGrams:  d("weight_grams", "Body weight in grams."),
		bmi:          d("bmi", "Body mass index."),
		bodyFat:      d("fat_percent", "Body fat percentage."),
		bodyWater:    d("water_percent", "Body water percentage."),
		boneMass:     d("bone_mass_grams", "Bone mass in grams."),
		muscleMass:   d("muscle_mass_grams", "Muscle mass in grams."),
		visceralFat:  d("visceral_fat", "Visceral fat rating."),
		metabolicAge: d("metabolic_age_years", "Metabolic age in years."),
		logger:       logger,
	}, nil
}

func (c *bodyCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	resp, err := garminClient.DailyWeighIns(time.Now())
	if err != nil {
		return err
	}
	if len(resp.DateWeightList) == 0 {
		return ErrNoData
	}

	w := resp.DateWeightList[0]
	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	if w.Weight > 0 {
		g(c.weightGrams, w.Weight)
	}
	if w.Bmi > 0 {
		g(c.bmi, w.Bmi)
	}
	if w.BodyFat > 0 {
		g(c.bodyFat, w.BodyFat)
	}
	if w.BodyWater > 0 {
		g(c.bodyWater, w.BodyWater)
	}
	if w.BoneMass > 0 {
		g(c.boneMass, w.BoneMass)
	}
	if w.MuscleMass > 0 {
		g(c.muscleMass, w.MuscleMass)
	}
	if w.VisceralFat > 0 {
		g(c.visceralFat, w.VisceralFat)
	}
	if w.MetabolicAge > 0 {
		g(c.metabolicAge, w.MetabolicAge)
	}
	return nil
}
