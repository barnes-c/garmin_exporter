# Garmin

Package garmin wraps the `github.com/barnes-c/go-garminconnect/garminconnect` client with the supporting types the exporter needs: an atomic handle that can be swapped on re-auth, a typed Snapshot consumed by collectors, and a best-effort Refresh function that drives one Garmin API fan-out per scrape tick.
