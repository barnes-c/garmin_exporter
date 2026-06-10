# Scrape

Package scrape is the TTL refresh orchestrator for unixctl-backed data sources. ovsdb is monitor-cached by libovsdb (push updates from the server keep the local cache fresh) so it doesn't need a scraper;
the unixctl protocol has no monitor concept, so collectors that consume appctl output read from an atomic.Pointer snapshot refreshed here.

A Scraper is generic over the snapshot type T so domain-specific structs (e.g. an OVSSnapshot composed of coverage / memory / upcall fields) can live in their own packages next to their parsers.
