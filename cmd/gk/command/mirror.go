package command

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Shopify/sarama"
	"github.com/funkygao/gafka"
	"github.com/funkygao/gafka/ctx"
	"github.com/funkygao/gafka/zk"
	"github.com/funkygao/gocli"
	"github.com/funkygao/golib/gofmt"
	"github.com/funkygao/golib/ratelimiter"
	"github.com/funkygao/golib/signal"
	"github.com/funkygao/kafka-cg/consumergroup"
	log "github.com/funkygao/log4go"
)

type Mirror struct {
	Ui  cli.Ui
	Cmd string

	quit chan struct{}
	once sync.Once

	zone1, zone2       string
	cluster1, cluster2 string
	excludes           string
	topicsExcluded     map[string]struct{}
	debug              bool
	compress           string
	autoCommit         bool

	transferN     int64
	transferBytes int64

	bandwidthLimit       int64
	bandwidthRateLimiter *ratelimiter.LeakyBucket
	progressStep         int64
}

func (this *Mirror) Run(args []string) (exitCode int) {
	cmdFlags := flag.NewFlagSet("mirror", flag.ContinueOnError)
	cmdFlags.Usage = func() { this.Ui.Output(this.Help()) }
	cmdFlags.StringVar(&this.zone1, "z1", "", "")
	cmdFlags.StringVar(&this.zone2, "z2", "", "")
	cmdFlags.StringVar(&this.cluster1, "c1", "", "")
	cmdFlags.StringVar(&this.cluster2, "c2", "", "")
	cmdFlags.StringVar(&this.excludes, "excluded", "", "")
	cmdFlags.BoolVar(&this.debug, "debug", false, "")
	cmdFlags.StringVar(&this.compress, "compress", "", "")
	cmdFlags.Int64Var(&this.bandwidthLimit, "net", 100, "")
	cmdFlags.BoolVar(&this.autoCommit, "commit", true, "")
	cmdFlags.Int64Var(&this.progressStep, "step", 5000, "")
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	if validateArgs(this, this.Ui).
		require("-z1", "-z2", "-c1", "-c2").
		invalid(args) {
		return 2
	}

	this.topicsExcluded = make(map[string]struct{})
	for _, e := range strings.Split(this.excludes, ",") {
		this.topicsExcluded[e] = struct{}{}
	}

	this.quit = make(chan struct{})
	limit := (1 << 20) * this.bandwidthLimit / 8
	this.bandwidthRateLimiter = ratelimiter.NewLeakyBucket(limit, time.Second)
	log.Info("start [%s/%s] -> [%s/%s] with bandwidth %sbps",
		this.zone1, this.cluster1,
		this.zone2, this.cluster2,
		gofmt.Comma(int64(limit*8)))
	signal.RegisterHandler(func(sig os.Signal) {
		log.Info("received signal: %s", strings.ToUpper(sig.String()))
		log.Info("quiting...")

		this.once.Do(func() {
			close(this.quit)
		})
	}, syscall.SIGINT, syscall.SIGTERM)

	z1 := zk.NewZkZone(zk.DefaultConfig(this.zone1, ctx.ZoneZkAddrs(this.zone1)))
	z2 := zk.NewZkZone(zk.DefaultConfig(this.zone2, ctx.ZoneZkAddrs(this.zone2)))
	c1 := z1.NewCluster(this.cluster1)
	c2 := z2.NewCluster(this.cluster2)

	log.Info("starting %s", gafka.BuildId)
	this.runMirror(c1, c2)

	log.Info("bye %s", gafka.BuildId)
	log.Close()

	return
}

func (this *Mirror) runMirror(c1, c2 *zk.ZkCluster) {
	pub, err := this.makePub(c2)
	swallow(err)

	topics, topicsChanges, err := c1.WatchTopics()
	swallow(err)

	log.Info("[%s/%s] topics: %+v", c1.ZkZone().Name(), c1.Name(), topics)

	group := this.groupName(c1, c2)
	sub, err := this.makeSub(c1, group, topics)
	swallow(err)

	// pprof
	go http.ListenAndServe("localhost:10009", nil)

LOOP:
	for {
		sub, err = this.makeSub(c1, group, topics)
		if err != nil {
			// TODO how to handle this err?
			log.Error(err)
			time.Sleep(time.Second * 10)
		}

		pumpStopper := make(chan struct{})
		pumpStopped := make(chan struct{})
		go this.pump(sub, pub, pumpStopper, pumpStopped)

		select {
		case <-topicsChanges:
			log.Warn("[%s/%s] topics changed, stopping pump...", c1.Name(), c2.Name())
			pumpStopper <- struct{}{} // stop pump
			<-pumpStopped             // await pump cleanup

			// refresh c1 topics
			topics, err = c1.Topics()
			if err != nil {
				// TODO how to handle this err?
				log.Error(err)
			}

			log.Info("[%s/%s] topics: %+v", c1.ZkZone().Name(), c1.Name(), topics)

		case <-this.quit:
			log.Info("awaiting pump cleanup...")
			<-pumpStopped
			log.Info("total transferred: %s %smsgs",
				gofmt.ByteSize(this.transferBytes),
				gofmt.Comma(this.transferN))
			break LOOP

		case <-pumpStopped:
			// pump encounters problems, just retry
			log.Warn("pump stopped for ?")
		}
	}

	log.Info("closing pub...")
	pub.Close()
}

func (this *Mirror) groupName(c1, c2 *zk.ZkCluster) string {
	return fmt.Sprintf("_mirror_.%s.%s.%s.%s", c1.ZkZone().Name(), c1.Name(), c2.ZkZone().Name(), c2.Name())
}

func (this *Mirror) makePub(c2 *zk.ZkCluster) (sarama.AsyncProducer, error) {
	cf := sarama.NewConfig()
	cf.Metadata.RefreshFrequency = time.Minute * 10
	cf.Metadata.Retry.Max = 3
	cf.Metadata.Retry.Backoff = time.Millisecond * 10

	cf.Producer.Flush.Frequency = time.Second * 10 // TODO
	cf.Producer.Flush.Messages = 1000
	cf.Producer.Flush.MaxMessages = 0 // unlimited

	cf.Producer.RequiredAcks = sarama.NoResponse
	cf.Producer.Retry.Backoff = time.Millisecond * 10 // gk migrate will trigger this backoff
	cf.Producer.Retry.Max = 3

	switch this.compress {
	case "gzip":
		cf.Producer.Compression = sarama.CompressionGZIP

	case "snappy":
		cf.Producer.Compression = sarama.CompressionSnappy
	}
	return sarama.NewAsyncProducer(c2.BrokerList(), cf)
}

func (this *Mirror) makeSub(c1 *zk.ZkCluster, group string, topics []string) (*consumergroup.ConsumerGroup, error) {
	cf := consumergroup.NewConfig()
	cf.Zookeeper.Chroot = c1.Chroot()
	cf.Offsets.CommitInterval = time.Second * 10
	cf.Offsets.ProcessingTimeout = time.Second
	cf.ChannelBufferSize = 100
	cf.Consumer.Return.Errors = true
	cf.OneToOne = false

	sub, err := consumergroup.JoinConsumerGroup(group, topics, c1.ZkZone().ZkAddrList(), cf)
	return sub, err
}

func (this *Mirror) pump(sub *consumergroup.ConsumerGroup, pub sarama.AsyncProducer,
	stop, stopped chan struct{}) {
	defer func() {
		log.Info("closing sub...")
		sub.Close()

		stopped <- struct{}{} // notify others I'm done
	}()

	go func(pub sarama.AsyncProducer) {
		for {
			select {
			case <-this.quit:
				return

			case err := <-pub.Errors():
				log.Error("pub %v", err)
			}
		}
	}(pub)

	log.Info("start pumping")
	active := false
	backoff := time.Second * 2
	for {
		select {
		case <-this.quit:
			return

		case <-stop:
			// yes sir!
			return

		case <-time.After(time.Second * 10):
			active = false
			log.Info("idle 10s waiting for new msg")

		case msg := <-sub.Messages():
			if !active || this.debug {
				log.Info("<-[%d] T:%s M:%s", this.transferN, msg.Topic, string(msg.Value))
			}
			active = true

			pub.Input() <- &sarama.ProducerMessage{
				Topic: msg.Topic,
				Key:   sarama.ByteEncoder(msg.Key),
				Value: sarama.ByteEncoder(msg.Value),
			}
			if this.autoCommit {
				sub.CommitUpto(msg)
			}

			// rate limit, never overflood the limited bandwidth between IDCs
			// FIXME when compressed, the bandwidth calculation is wrong
			bytesN := len(msg.Topic) + len(msg.Key) + len(msg.Value) + 20 // payload overhead
			if !this.bandwidthRateLimiter.Pour(bytesN) {
				time.Sleep(backoff)
				log.Warn("%s -> bandwidth reached, backoff %s", gofmt.ByteSize(this.transferBytes), backoff)
			}
			this.transferBytes += int64(bytesN)

			this.transferN++
			if this.transferN%this.progressStep == 0 {
				log.Info("%s %s %s", msg.Topic, gofmt.Comma(this.transferN), gofmt.ByteSize(this.transferBytes))
			}

		case err, ok := <-sub.Errors():
			if ok {
				log.Error("sub %v", err)
			}
		}
	}
}

func (*Mirror) Synopsis() string {
	return "Continuously copy data between two Kafka clusters"
}

func (this *Mirror) Help() string {
	help := fmt.Sprintf(`
Usage: %s mirror [options]

    %s

    e,g.
    gk mirror -z1 prod -c1 logstash -z2 mirror -c2 aggregator -net 100 -step 2000

Options:

    -z1 from zone

    -z2 to zone

    -c1 from cluster

    -c2 to cluster

    -exclude comma seperated topic names

    -net bandwidth limit in Mbps
      Defaults 100Mbps.

    -step n
      Defaults 5000.

    -debug

    -compress <gzip|snappy>
      Defaults none.

    -commit
      Auto commit the checkpoint offset.
      Defaults true.

`, this.Cmd, this.Synopsis())
	return strings.TrimSpace(help)
}
