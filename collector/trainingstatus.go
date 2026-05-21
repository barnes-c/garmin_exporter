package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("trainingstatus", defaultEnabled, newTrainingStatusCollector)
}

type trainingStatusCollector struct {
	status          *prometheus.Desc
	weeklyLoad      *prometheus.Desc
	acwrPercent     *prometheus.Desc
	acwrRatio       *prometheus.Desc
	aerobicLowLoad  *prometheus.Desc
	aerobicHighLoad *prometheus.Desc
	anaerobicLoad   *prometheus.Desc
	logger          *slog.Logger
}

func newTrainingStatusCollector(logger *slog.Logger) (Collector, error) {
	const sub = "trainingstatus"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &trainingStatusCollector{
		// TrainingStatus int codes: 0=unknown, 1=not_enough_data, 2=peaking, 3=productive,
		// 4=maintaining, 5=recovery, 6=unproductive, 7=strained, 8=over_reaching, 9=detraining
		status:          d("status", "Training status code from primary device (see Garmin Connect API docs)."),
		weeklyLoad:      d("weekly_training_load", "Weekly training load from the primary device."),
		acwrPercent:     d("acwr_percent", "Acute:chronic workload ratio as a percentage."),
		acwrRatio:       d("acwr_ratio", "Daily acute:chronic workload ratio."),
		aerobicLowLoad:  d("aerobic_low_monthly_load", "Monthly aerobic low-intensity training load."),
		aerobicHighLoad: d("aerobic_high_monthly_load", "Monthly aerobic high-intensity training load."),
		anaerobicLoad:   d("anaerobic_monthly_load", "Monthly anaerobic training load."),
		logger:          logger,
	}, nil
}

func (c *trainingStatusCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	resp, err := garminClient.TrainingStatus(time.Now())
	if err != nil {
		return err
	}
	if resp == nil {
		return ErrNoData
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	if resp.MostRecentTrainingStatus != nil {
		for _, entry := range resp.MostRecentTrainingStatus.LatestTrainingStatusData {
			if !entry.PrimaryTrainingDevice {
				continue
			}
			g(c.status, float64(entry.TrainingStatus))
			if entry.WeeklyTrainingLoad != nil {
				g(c.weeklyLoad, *entry.WeeklyTrainingLoad)
			}
			if entry.AcuteTrainingLoad != nil {
				g(c.acwrPercent, float64(entry.AcuteTrainingLoad.ACWRPercent))
				g(c.acwrRatio, entry.AcuteTrainingLoad.DailyAcuteChronicWorkloadRatio)
			}
			break
		}
	}

	if resp.MostRecentTrainingLoadBalance != nil {
		for _, entry := range resp.MostRecentTrainingLoadBalance.MetricsMap {
			if !entry.PrimaryTrainingDevice {
				continue
			}
			g(c.aerobicLowLoad, float64(entry.MonthlyLoadAerobicLow))
			g(c.aerobicHighLoad, float64(entry.MonthlyLoadAerobicHigh))
			g(c.anaerobicLoad, float64(entry.MonthlyLoadAnaerobic))
			break
		}
	}

	return nil
}
