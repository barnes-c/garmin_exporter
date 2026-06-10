package datasource

import (
	"github.com/barnes-c/ovs-exporter/collector"
	"github.com/barnes-c/ovs-exporter/internal/ovsdb"
	"github.com/barnes-c/ovs-exporter/internal/scrape"
	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

// Compile-time check that DataSource implements collector.DataSource.
var _ collector.DataSource = (*DataSource)(nil)

type DataSource struct {
	ovs       *ovsdb.Client
	ovsScrape *scrape.Scraper[unixctl.OVSSnapshot]
}

func NewDataSource(ovs *ovsdb.Client, ovsScrape *scrape.Scraper[unixctl.OVSSnapshot]) *DataSource {
	return &DataSource{ovs: ovs, ovsScrape: ovsScrape}
}

func (d *DataSource) OVS() collector.OVSView {
	if d.ovs == nil {
		return nil
	}
	return d.ovs.View()
}

func (d *DataSource) UnixCtlOVS() *unixctl.OVSSnapshot {
	if d.ovsScrape == nil {
		return nil
	}
	return d.ovsScrape.Snapshot()
}
