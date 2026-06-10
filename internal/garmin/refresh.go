package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/barnes-c/go-garminconnect/garminconnect"

	"github.com/barnes-c/garmin_exporter/internal/scrape"
)

// RefreshConfig parameterizes NewRefresh.
type RefreshConfig struct {
	// ActivityLimit caps the number of recent activities fetched per tick.
	// Mirrors the previous --garmin.activity-limit flag.
	ActivityLimit int
}

// NewRefresh returns a scrape.RefreshFunc that calls every Garmin Connect
// endpoint backing a default-on collector and assembles the parsed results
// into a Snapshot.
//
// Failures are best-effort: a per-method transport or parse error logs at
// debug and leaves the corresponding snapshot field nil. The whole refresh
// fails only when every attempted call failed — that's the signal worth
// firing readyz on. The refresh short-circuits with ErrNotLoggedIn when the
// auth manager has not yet installed a client.
func NewRefresh(client *Client, log *slog.Logger, cfg RefreshConfig) scrape.RefreshFunc[Snapshot] {
	if cfg.ActivityLimit <= 0 {
		cfg.ActivityLimit = 30
	}
	return func(ctx context.Context) (*Snapshot, error) {
		gc := client.Get()
		if gc == nil {
			return nil, ErrNotLoggedIn
		}

		snap := &Snapshot{}
		now := time.Now()
		var attempts, failures int

		call := func(name string, fn func() error) {
			attempts++
			if err := fn(); err != nil {
				failures++
				log.Debug("garmin call failed", "endpoint", name, "err", err)
			}
		}

		call("UserSummary", func() error {
			v, err := gc.UserSummary(ctx, now)
			if err == nil {
				snap.UserSummary = v
			}
			return err
		})
		call("HeartRates", func() error {
			v, err := gc.HeartRates(ctx, now)
			if err == nil {
				snap.HeartRate = v
			}
			return err
		})
		call("SleepData", func() error {
			v, err := gc.SleepData(ctx, now)
			if err == nil {
				if snap.Sleep == nil {
					snap.Sleep = &Sleep{}
				}
				snap.Sleep.Data = v
			}
			return err
		})
		call("HRVData", func() error {
			v, err := gc.HRVData(ctx, now)
			if err == nil {
				if snap.Sleep == nil {
					snap.Sleep = &Sleep{}
				}
				snap.Sleep.HRV = v
			}
			return err
		})
		call("AllDayStress", func() error {
			v, err := gc.AllDayStress(ctx, now)
			if err == nil {
				snap.Stress = v
			}
			return err
		})
		call("SpO2", func() error {
			v, err := gc.SpO2(ctx, now)
			if err == nil {
				snap.SpO2 = v
			}
			return err
		})
		call("Respiration", func() error {
			v, err := gc.Respiration(ctx, now)
			if err == nil {
				snap.Respiration = v
			}
			return err
		})
		call("Hydration", func() error {
			v, err := gc.Hydration(ctx, now)
			if err == nil {
				snap.Hydration = v
			}
			return err
		})
		call("IntensityMinutes", func() error {
			v, err := gc.IntensityMinutes(ctx, now)
			if err == nil {
				snap.Intensity = v
			}
			return err
		})
		call("DailyWeighIns", func() error {
			v, err := gc.DailyWeighIns(ctx, now)
			if err == nil {
				snap.Body = v
			}
			return err
		})
		call("BodyComposition", func() error {
			v, err := gc.BodyComposition(ctx, now.AddDate(0, 0, -30), now)
			if err == nil {
				snap.BodyComposition = v
			}
			return err
		})
		call("BloodPressure", func() error {
			v, err := gc.BloodPressure(ctx, now.AddDate(0, -1, 0), now)
			if err == nil {
				snap.BloodPressure = v
			}
			return err
		})
		call("Devices", func() error {
			v, err := gc.Devices(ctx)
			if err == nil {
				snap.Devices = v
			}
			return err
		})
		call("Activities", func() error {
			activities := &Activities{}
			recent, err := gc.Activities(ctx, cfg.ActivityLimit)
			if err != nil {
				return err
			}
			activities.Recent = recent

			// Lifetime count is a secondary signal — failure here doesn't
			// invalidate the recent slice. Logged at debug and continues.
			if total, err := gc.ActivityCount(ctx); err != nil {
				log.Debug("garmin call failed", "endpoint", "ActivityCount", "err", err)
			} else {
				activities.Lifetime = total
			}
			snap.Activities = activities
			return nil
		})
		call("Goals+Badges", func() error {
			goals, err := gc.Goals(ctx, "active", 0, 100)
			if err != nil {
				return err
			}
			out := &Goals{Active: goals}
			if badges, err := gc.EarnedBadges(ctx); err != nil {
				log.Debug("garmin call failed", "endpoint", "EarnedBadges", "err", err)
			} else {
				out.Badges = badges
			}
			snap.Goals = out
			return nil
		})
		call("Gear", func() error {
			// Gear needs the user profile ID. Skip silently if UserSummary
			// hasn't loaded yet — gear isn't critical and will populate on
			// the next tick once UserSummary recovers.
			if snap.UserSummary == nil {
				return fmt.Errorf("gear: skipped, UserSummary not available")
			}
			v, err := gc.Gear(ctx, snap.UserSummary.UserProfileID)
			if err == nil {
				snap.Gear = v
			}
			return err
		})
		call("Golf", func() error {
			v, err := gc.GolfSummary(ctx, 0, 1)
			if err == nil {
				snap.Golf = v
			}
			return err
		})
		call("LactateThreshold", func() error {
			v, err := gc.LactateThreshold(ctx)
			if err == nil {
				snap.LactateThreshold = v
			}
			return err
		})
		call("PersonalRecords", func() error {
			v, err := gc.PersonalRecords(ctx)
			if err == nil {
				snap.PersonalRecords = v
			}
			return err
		})
		call("RunningTolerance", func() error {
			v, err := gc.RunningTolerance(ctx, now.AddDate(0, 0, -7), now)
			if err == nil {
				snap.RunningTolerance = v
			}
			return err
		})
		call("CyclingFTP", func() error {
			raw, err := gc.CyclingFTP(ctx)
			if err != nil {
				return err
			}
			if v, ok := parseCyclingFTP(raw); ok {
				snap.Cycling = &Cycling{FTPWatts: v}
			}
			return nil
		})
		call("TrainingStatus", func() error {
			v, err := gc.TrainingStatus(ctx, now)
			if err == nil {
				snap.TrainingStatus = v
			}
			return err
		})
		call("Training", func() error {
			t, err := collectTraining(ctx, gc, now, log)
			if err != nil {
				return err
			}
			snap.Training = t
			return nil
		})

		if attempts > 0 && failures == attempts {
			return nil, fmt.Errorf("garmin: all %d API calls failed", failures)
		}
		return snap, nil
	}
}

// parseCyclingFTP unpacks the FTP value from Garmin's CyclingFTP map response.
// Returns ok=false when the field is missing or zero.
func parseCyclingFTP(result map[string]json.RawMessage) (float64, bool) {
	raw, ok := result["mostRecentBiometric"]
	if !ok {
		return 0, false
	}
	var bio struct {
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal(raw, &bio); err != nil || bio.Value == 0 {
		return 0, false
	}
	return bio.Value, true
}

// collectTraining fans out the five training-related endpoints. Any individual
// failure logs at debug and leaves the corresponding field nil; the whole
// call only fails if every endpoint failed.
func collectTraining(ctx context.Context, gc *garminconnect.Client, now time.Time, log *slog.Logger) (*Training, error) {
	t := &Training{}
	var attempts, failures int

	sub := func(name string, fn func() error) {
		attempts++
		if err := fn(); err != nil {
			failures++
			log.Debug("garmin call failed", "endpoint", name, "err", err)
		}
	}

	sub("TrainingReadiness", func() error {
		v, err := gc.TrainingReadiness(ctx, now)
		if err == nil {
			t.Readiness = v
		}
		return err
	})
	sub("MaxMetrics", func() error {
		v, err := gc.MaxMetrics(ctx, now.AddDate(0, 0, -30), now)
		if err == nil {
			t.Max = v
		}
		return err
	})
	sub("RacePredictions", func() error {
		v, err := gc.RacePredictions(ctx)
		if err == nil {
			t.Race = v
		}
		return err
	})
	sub("EnduranceScore", func() error {
		raw, err := gc.EnduranceScore(ctx, now.AddDate(0, 0, -7), now)
		if err != nil {
			return err
		}
		var entries []garminconnect.EnduranceScoreEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			return err
		}
		t.Endurance = entries
		return nil
	})
	sub("HillScore", func() error {
		raw, err := gc.HillScore(ctx, now.AddDate(0, 0, -7), now)
		if err != nil {
			return err
		}
		var entries []garminconnect.HillScoreEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			return err
		}
		t.Hill = entries
		return nil
	})

	if attempts > 0 && failures == attempts {
		return nil, fmt.Errorf("training: all %d sub-endpoints failed", failures)
	}
	return t, nil
}
