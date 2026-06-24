package garmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/barnes-c/go-garminconnect/garminconnect"

	otelpkg "github.com/barnes-c/garmin_exporter/internal/otel"
	"github.com/barnes-c/garmin_exporter/internal/scrape"
)

type RefreshConfig struct {
	// ActivityLimit caps the number of recent activities fetched per tick.
	// Mirrors the --activity-limit flag.
	ActivityLimit  int
	OnUnauthorized func()
}

// NewRefresh returns a scrape.RefreshFunc that calls every Garmin Connect
// endpoint backing a default-on collector and assembles the parsed results
// into a Snapshot.
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
		var unauthorized bool

		call := func(name string, fn func(ctx context.Context) error) {
			attempts++
			childCtx, span := otel.Tracer(otelpkg.ScopeName).Start(ctx, "garmin."+name)
			err := fn(childCtx)
			endSpan(span, err)
			if err != nil {
				failures++
				if errors.Is(err, garminconnect.ErrUnauthorized) {
					unauthorized = true
				}
				log.Debug("garmin call failed", "endpoint", name, "err", err)
			}
		}

		call("UserSummary", func(ctx context.Context) error {
			v, err := gc.UserSummary(ctx, now)
			if err == nil {
				snap.UserSummary = v
			}
			return err
		})
		call("HeartRates", func(ctx context.Context) error {
			v, err := gc.HeartRates(ctx, now)
			if err == nil {
				snap.HeartRate = v
			}
			return err
		})
		call("SleepData", func(ctx context.Context) error {
			v, err := gc.SleepData(ctx, now)
			if err == nil {
				if snap.Sleep == nil {
					snap.Sleep = &Sleep{}
				}
				snap.Sleep.Data = v
			}
			return err
		})
		call("HRVData", func(ctx context.Context) error {
			v, err := gc.HRVData(ctx, now)
			if err == nil {
				if snap.Sleep == nil {
					snap.Sleep = &Sleep{}
				}
				snap.Sleep.HRV = v
			}
			return err
		})
		call("AllDayStress", func(ctx context.Context) error {
			v, err := gc.AllDayStress(ctx, now)
			if err == nil {
				snap.Stress = v
			}
			return err
		})
		call("SpO2", func(ctx context.Context) error {
			v, err := gc.SpO2(ctx, now)
			if err == nil {
				snap.SpO2 = v
			}
			return err
		})
		call("Respiration", func(ctx context.Context) error {
			v, err := gc.Respiration(ctx, now)
			if err == nil {
				snap.Respiration = v
			}
			return err
		})
		call("Hydration", func(ctx context.Context) error {
			v, err := gc.Hydration(ctx, now)
			if err == nil {
				snap.Hydration = v
			}
			return err
		})
		call("IntensityMinutes", func(ctx context.Context) error {
			v, err := gc.IntensityMinutes(ctx, now)
			if err == nil {
				snap.Intensity = v
			}
			return err
		})
		call("DailyWeighIns", func(ctx context.Context) error {
			v, err := gc.DailyWeighIns(ctx, now)
			if err == nil {
				snap.Body = v
			}
			return err
		})
		call("BodyComposition", func(ctx context.Context) error {
			v, err := gc.BodyComposition(ctx, now.AddDate(0, 0, -30), now)
			if err == nil {
				snap.BodyComposition = v
			}
			return err
		})
		call("BloodPressure", func(ctx context.Context) error {
			v, err := gc.BloodPressure(ctx, now.AddDate(0, -1, 0), now)
			if err == nil {
				snap.BloodPressure = v
			}
			return err
		})
		call("Devices", func(ctx context.Context) error {
			v, err := gc.Devices(ctx)
			if err == nil {
				snap.Devices = v
			}
			return err
		})
		call("Activities", func(ctx context.Context) error {
			activities := &Activities{}
			recent, err := gc.Activities(ctx, 0, cfg.ActivityLimit)
			if err != nil {
				return err
			}
			activities.Recent = recent

			if total, err := gc.ActivityCount(ctx); err != nil {
				log.Debug("garmin call failed", "endpoint", "ActivityCount", "err", err)
			} else {
				activities.Lifetime = total
			}
			snap.Activities = activities
			return nil
		})
		call("Goals+Badges", func(ctx context.Context) error {
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
		call("Gear", func(ctx context.Context) error {
			if snap.UserSummary == nil {
				return fmt.Errorf("gear: skipped, UserSummary not available")
			}
			v, err := gc.Gear(ctx, snap.UserSummary.UserProfileID)
			if err == nil {
				snap.Gear = v
			}
			return err
		})
		call("Golf", func(ctx context.Context) error {
			v, err := gc.GolfSummary(ctx, 0, 1)
			if err == nil {
				snap.Golf = v
			}
			return err
		})
		call("LactateThreshold", func(ctx context.Context) error {
			v, err := gc.LactateThreshold(ctx)
			if err == nil {
				snap.LactateThreshold = v
			}
			return err
		})
		call("PersonalRecords", func(ctx context.Context) error {
			v, err := gc.PersonalRecords(ctx)
			if err == nil {
				snap.PersonalRecords = v
			}
			return err
		})
		call("RunningTolerance", func(ctx context.Context) error {
			v, err := gc.RunningTolerance(ctx, now.AddDate(0, 0, -7), now)
			if err == nil {
				snap.RunningTolerance = v
			}
			return err
		})
		call("CyclingFTP", func(ctx context.Context) error {
			raw, err := gc.CyclingFTP(ctx)
			if err != nil {
				return err
			}
			if v, ok := parseCyclingFTP(raw); ok {
				snap.Cycling = &Cycling{FTPWatts: v}
			}
			return nil
		})
		call("TrainingStatus", func(ctx context.Context) error {
			v, err := gc.TrainingStatus(ctx, now)
			if err == nil {
				snap.TrainingStatus = v
			}
			return err
		})
		call("Training", func(ctx context.Context) error {
			t, err := collectTraining(ctx, gc, now, log)
			if err != nil {
				return err
			}
			snap.Training = t
			return nil
		})

		if unauthorized && cfg.OnUnauthorized != nil {
			cfg.OnUnauthorized()
		}
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

	sub := func(name string, fn func(ctx context.Context) error) {
		attempts++
		childCtx, span := otel.Tracer(otelpkg.ScopeName).Start(ctx, "garmin."+name)
		err := fn(childCtx)
		endSpan(span, err)
		if err != nil {
			failures++
			log.Debug("garmin call failed", "endpoint", name, "err", err)
		}
	}

	sub("TrainingReadiness", func(ctx context.Context) error {
		v, err := gc.TrainingReadiness(ctx, now)
		if err == nil {
			t.Readiness = v
		}
		return err
	})
	sub("MaxMetrics", func(ctx context.Context) error {
		v, err := gc.MaxMetrics(ctx, now.AddDate(0, 0, -30), now)
		if err == nil {
			t.Max = v
		}
		return err
	})
	sub("RacePredictions", func(ctx context.Context) error {
		v, err := gc.RacePredictions(ctx)
		if err == nil {
			t.Race = v
		}
		return err
	})
	sub("EnduranceScore", func(ctx context.Context) error {
		entries, err := gc.EnduranceScore(ctx, now.AddDate(0, 0, -7), now)
		if err != nil {
			return err
		}
		t.Endurance = entries
		return nil
	})
	sub("HillScore", func(ctx context.Context) error {
		entries, err := gc.HillScore(ctx, now.AddDate(0, 0, -7), now)
		if err != nil {
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

func endSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
