# Datasource

Package datasource adapts the `libovsdb` client and `unixctl` scrape orchestrator to the `collector.DataSource` interface. It is the production data-path wiring; tests in the collector package inject their own fakes against the same interface.
