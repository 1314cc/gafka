package zk

import (
	"bytes"
	"fmt"
	"time"

	"github.com/funkygao/gafka/zk"
	log "github.com/funkygao/log4go"
	zklib "github.com/samuel/go-zookeeper/zk"
)

type zkreg struct {
	zkzone *zk.ZkZone
}

func New(zkzone *zk.ZkZone) *zkreg {
	this := &zkreg{
		zkzone: zkzone,
	}

	return this
}

func (this *zkreg) mypath(id string) string {
	return fmt.Sprintf("%s/%s/%s", zk.KatewayIdsRoot, this.zkzone.Name(), id)
}

func (this *zkreg) Name() string {
	return "zookeeper"
}

func (this *zkreg) Register(id string, data []byte) {
	// https://issues.apache.org/jira/browse/ZOOKEEPER-1740
	//
	// might cause dead loop, but we accept it
	loops := 0
	backoffN := 0
	path := this.mypath(id)
	for {
		loops++

		if err := this.zkzone.CreateEphemeralZnode(path, data); err != nil {
			if err == zklib.ErrNodeExists {
				// An ephemeral node may still exist even after its corresponding session has expired
				// due to a Zookeeper bug, in this case we need to retry writing until the previous node is deleted
				// and hence the write succeeds without ZkNodeExistsException
				storedData, _, e := this.zkzone.Conn().Get(path)
				log.Trace("#%d get %s: %v", loops, path, e)
				if e == nil {
					if bytes.Equal(data, storedData) {
						backoffN++
						if backoffN == 3 {
							log.Warn("backoff %d times, give up retrying, might been registered", backoffN-1)
							return
						}

						log.Trace("#%d %s backoff %s awaiting previous node gone", loops, path, this.zkzone.SessionTimeout())
						time.Sleep(this.zkzone.SessionTimeout())
					} else {
						log.Error("conflict[%s] found, give up retry registering. expected: %s, got %s",
							id, string(data), string(storedData))
						return
					}
				} else {
					log.Error("#%d %s get data: %v", loops, path, e)
					continue
				}
			} else {
				// retry
				log.Error("#%d %s: %v", loops, path, err)
				continue
			}
		} else {
			// didn't encounter zk bug, happy ending
			log.Trace("#%d %s created", loops, path)
			return
		}
	}

}

func (this *zkreg) createEphemeralPathExpectConflict(path string, data []byte) error {
	err := this.zkzone.CreateEphemeralZnode(path, data)
	if err == nil {
		return nil
	}

	log.Warn("%s %v", path, err)

	if err == zklib.ErrNodeExists {
		_, _, e := this.zkzone.Conn().Get(path)
		log.Debug("%s %v", path, e)
		if e == zklib.ErrNoNode {
			// the node disappeared; treat as if node exists
			return err
		} else if e != nil {
			return e
		}
	}

	return err
}

func (this *zkreg) Deregister(id string, oldData []byte) error {
	data, _, err := this.zkzone.Conn().Get(this.mypath(id))
	if err != nil {
		return fmt.Errorf("%s %v", this.mypath(id), err)
	}

	// ensure I own this znode
	if !bytes.Equal(data, oldData) {
		return fmt.Errorf("registry[%s] exp %s, got %s", id, string(oldData), string(data))
	}

	return this.zkzone.Conn().Delete(this.mypath(id), -1)
}

func (this *zkreg) WatchInstances() ([]string, <-chan zklib.Event, error) {
	path := fmt.Sprintf("%s/%s", zk.KatewayIdsRoot, this.zkzone.Name())
	ids, _, ch, err := this.zkzone.Conn().ChildrenW(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%s %v", path, err)
	}

	instancePaths := make([]string, 0, len(ids))
	for _, id := range ids {
		instancePaths = append(instancePaths, fmt.Sprintf("%s/%s", path, id))
	}

	return instancePaths, ch, nil
}
