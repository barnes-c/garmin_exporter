package garmin

import "github.com/barnes-c/go-garminconnect/garminconnect"

type Source interface {
	Snapshot() *Snapshot
}

// Snapshot holds the parsed responses fetched from Garmin Connect on each
// refresh. Fields are populated on a best-effort basis; an absent or failed
// fetch leaves the corresponding pointer nil so collectors can degrade
// gracefully.
type Snapshot struct {
	Activities       *Activities
	BloodPressure    *garminconnect.BloodPressureSummary
	Body             *garminconnect.WeighInsResponse
	BodyComposition  *garminconnect.BodyComposition
	Cycling          *Cycling
	Devices          []garminconnect.Device
	Gear             []garminconnect.Gear
	Goals            *Goals
	Golf             []garminconnect.GolfScorecard
	HeartRate        *garminconnect.HeartRates
	Hydration        *garminconnect.HydrationData
	Intensity        *garminconnect.IntensityMinutesData
	LactateThreshold []garminconnect.LactateThresholdEntry
	PersonalRecords  []garminconnect.PersonalRecord
	Respiration      *garminconnect.RespirationData
	RunningTolerance []garminconnect.RunningToleranceEntry
	Sleep            *Sleep
	SpO2             *garminconnect.SpO2Data
	Stress           *garminconnect.StressData
	Training         *Training
	TrainingStatus   *garminconnect.TrainingStatusResponse
	UserSummary      *garminconnect.UserSummary
}

// Activities pairs the recent-activities slice with the lifetime count, which
// share one collector but two API calls.
type Activities struct {
	Lifetime int
	Recent   []garminconnect.Activity
}

// Cycling carries the parsed cycling FTP value. The Garmin endpoint returns a
// map; we extract the only field we currently consume so collectors don't
// have to repeat the unmarshal.
type Cycling struct {
	FTPWatts float64
}

// Goals bundles the two endpoints feeding the goals collector.
type Goals struct {
	Active []garminconnect.Goal
	Badges []garminconnect.Badge
}

// Sleep bundles the two endpoints feeding the sleep collector.
type Sleep struct {
	Data *garminconnect.SleepData
	HRV  *garminconnect.HRVData
}

// Training bundles the five endpoints feeding the training collector.
type Training struct {
	Readiness []garminconnect.TrainingReadiness
	Max       []garminconnect.MaxMetricsEntry
	Race      *garminconnect.LatestRacePredictions
	Endurance []garminconnect.EnduranceScoreEntry
	Hill      []garminconnect.HillScoreEntry
}
