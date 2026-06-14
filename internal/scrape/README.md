# Scrape

Package scrape is the TTL refresh orchestrator for poll-based data sources. The Garmin Connect API has no push channel, so collectors read their data from an `atomic.Pointer` snapshot refreshed here on the configured interval.

A Scraper is generic over the snapshot type T so the domain-specific snapshot struct (e.g. a `garmin.Snapshot` composed of HeartRate / Sleep / Activities / … fields) lives in its own package next to the code that fetches it.
