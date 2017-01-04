package influxquery

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/funkygao/gafka/cmd/kguard/monitor"
	"github.com/funkygao/gafka/zk"
	"github.com/funkygao/go-metrics"
	log "github.com/funkygao/log4go"
)

func init() {
	monitor.RegisterWatcher("ngx.err", func() monitor.Watcher {
		return &WatchNgxErr{
			Tick: time.Minute,
		}
	})
}

type WatchNgxErr struct {
	Zkzone *zk.ZkZone
	Stop   <-chan struct{}
	Tick   time.Duration
	Wg     *sync.WaitGroup

	addr string
	db   string
}

func (this *WatchNgxErr) Init(ctx monitor.Context) {
	this.Zkzone = ctx.ZkZone()
	this.Stop = ctx.StopChan()
	this.Wg = ctx.Inflight()

	this.addr = ctx.InfluxAddr()
	this.db = "kfk_prod"
}

func (this *WatchNgxErr) Run() {
	defer this.Wg.Done()

	if this.addr == "" || this.db == "" {
		log.Warn("empty addr or db, quit...")
		return
	}

	ticker := time.NewTicker(this.Tick)
	defer ticker.Stop()

	ngerr := metrics.NewRegisteredGauge("ngx.err", nil)
	for {
		select {
		case <-this.Stop:
			log.Info("ngx.err stopped")
			return

		case <-ticker.C:
			n, err := this.ngixErrSum()
			if err != nil {
				log.Error("ngx.err: %v", err)
			} else {
				ngerr.Update(int64(n))
			}
		}
	}
}

func (this *WatchNgxErr) ngixErrSum() (int, error) {
	res, err := queryInfluxDB(this.addr, this.db,
		fmt.Sprintf(`SELECT mean("m5") FROM "pub.qps.meter" WHERE "appid" = 'logstash' AND "topic" = 'nginx_errlog_intra' AND time > now() - 1m GROUP BY time(1m) fill(0)`))
	if err != nil {
		return 0, err
	}

	total := 0
	for _, row := range res {
		for _, x := range row.Series {
			for _, val := range x.Values {
				// val[0] is time
				n, _ := val[1].(json.Number).Float64()
				total += int(n)
			}
		}
	}

	res, err = queryInfluxDB(this.addr, this.db,
		fmt.Sprintf(`SELECT mean("m5") FROM "pub.qps.meter" WHERE "appid" = 'logstash' AND "topic" = 'nginx_errlog_extra' AND time > now() - 1m GROUP BY time(1m) fill(0)`))
	if err != nil {
		return 0, err
	}

	for _, row := range res {
		for _, x := range row.Series {
			for _, val := range x.Values {
				// val[0] is time
				n, _ := val[1].(json.Number).Float64()
				total += int(n)
			}
		}
	}

	return total, nil
}
