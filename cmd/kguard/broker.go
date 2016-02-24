package main

import (
	"time"

	"github.com/funkygao/gafka/zk"
)

// MonitorBrokers monitors
type MonitorBrokers struct {
	zkzone *zk.ZkZone
	stop   chan struct{}
	tick   time.Duration
}

func (this *MonitorBrokers) Run() {
	ticker := time.NewTicker(this.tick)
	defer ticker.Stop()

	for {
		select {
		case <-this.stop:
			return

		case <-ticker.C:

		}
	}
}
