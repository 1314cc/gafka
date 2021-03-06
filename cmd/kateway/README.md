# kateway

A fully-managed real-time secure and reliable geo-replicated RESTful Cloud Pub/Sub streaming message/job service.

    _/    _/              _/
       _/  _/      _/_/_/  _/_/_/_/    _/_/    _/      _/      _/    _/_/_/  _/    _/
      _/_/      _/    _/    _/      _/_/_/_/  _/      _/      _/  _/    _/  _/    _/
     _/  _/    _/    _/    _/      _/          _/  _/  _/  _/    _/    _/  _/    _/
    _/    _/    _/_/_/      _/_/    _/_/_/      _/      _/        _/_/_/    _/_/_/
                                                                               _/

### Alternatives

- Google Cloud Pub/Sub
- Amazon kenesis/SQS
  - kenesis is aws kafka
  - SQS is aws beanstalkd
- Azure EventHub
- Yahoo Pulsar
- IBM Bluemix Message Hub
- uber Cherami
- qcloud CMQ
- aliyun MNS
  - order not guaranteed
  - backtracking not supported
- misc
  - pubnub
  - pusher
  - firebase
  - parse
  - pubsubhubbub
  - aliyun ONS

### Features

- REST API
  - http/https/websocket/http2 interface for Pub/Sub
- Support both FIFO and Schedulable queue
- Flexible delivery options
  - Both push- and pull-style subscriptions supported
- Communication can be 
  - one-to-many (fan-out)
  - many-to-one (fan-in)
  - many-to-many
- Systemic Quality Requirements
  - Performance & Throughput
    - > 100K msg/sec delivery on a single host without batch
    - fully benchmark tested and profiler'ed
  - Scalability
    - scales to 1M msg/sec
    - elastic scales
  - Latency
    - < 1s delivery
  - Availability
    - Graceful shutdown without downtime
  - Long polling
  - Graceful Degrade
    - throttle
    - circuit breaker
    - hinted handoff
- Fully-managed
  - Discovery
  - Create versioned topics, subscribe to topics
  - Rich real-time tagged metrics, fully-functional dashboard and alarming
  - Easy trouble shooting
  - Controlled GC
  - Visualize message flow
  - Managed integration service via Webhooks
  - Hot configurable
  - Multi-tenant
  - Authentication
  - Authorization
  - Quotas
  - Optional hardware isolation
  - REST API for provisioning, admin and stats
  - swagger documentation
- Mirror across data centers
- Replicated storage and guaranteed at-least-once message delivery
- Functional Features
  - schedulable message
  - server side message filter by tag
  - managed message routing
  - avro based message schema registration and versioning
  - retry|dead queue
  - sub in batch
  - message backtracking
  - hot dryrun topic
  - multi-tenant metrics
  - self-servicable topic scaling and message rentention SLA
  - user can check sub status/lag
  - topic owners can check subscribers and their status
  - configurable lag alerting
- Enables sophisticated streaming data processing
- Load balancer friendly
- [ ] Quotas and rate limit, QoS
  - Flow control: Dynamic rate limiting
- [ ] Encryption of all message data on the wire


### Common scenarios

- Balancing workloads in network clusters
- Implementing asynchronous workflows
- Distributing event notifications
- Refreshing distributed caches
- Logging to multiple systems
- Data streaming from various processes or devices
- Reliability improvement

### APIs

#### Pub

    POST    /v1/msgs/:topic/:ver
    POST /v1/ws/msgs/:topic/:ver

    POST    /v1/jobs/:topic/:ver
    DELETE  /v1/jobs/:topic/:ver

#### Sub

    GET    /v1/msgs/:appid/:topic/:ver
    GET /v1/ws/msgs/:appid/:topic/:ver

    POST   /v1/shadow/:appid/:topic/:ver/:group
    DELETE /v1/groups/:appid/:topic/:ver/:group

    GET /v1/subd/:topic/:ver
    GET /v1/status/:appid/:topic/:ver

### The Big Picture

                +-----------+
                | VirtualIP |
                +-----------+                                
                      |
              +--------------+                                 Alert   SOS   Dashboard
              |              |                                    |     |       |              gk
     +-------------------------------------------------------------------------------------------+
     |        |              |                                    |     |       |                |
     |  +----------+    +----------+                              |     |       |                |
     |  | ehaproxy |    | ehaproxy |                              |     V       |                |
     |  +----------+    +----------+                              |     |       |                |
     |      |                |  | discovery                       |     |       |                |
     |      +----------------+  |                                 +-------------+                |
     |            | LB          |                                        |                       |
     |            |             |   +--------------------+           +--------+                  |
     |            |             +---|                    | election  |        | watch            |
     |            |keepalive        | zookeeper ensemble |-----------| kguard |-------------+    |
     |            |             +---|                    |           |        | aggragator  |    |
     |            |             |   +--------------------+           +--------+             |    |
     |            |     +-------+           |                                               |    |
     |            |     | registry          | orchestration                                 |    |      +- Pub
     |      +---------------+               |-----------+                      +---------+  |    | REST |
     |      |               |               |           |                      | kateway |--|----|------|
     |  +---------+    +---------+      +--------+    +--------+               +---------+  |    |      |
     |  | kateway |    | kateway |      | actord |    | actord |                            |    |      +- Sub
     |  +---------+    +---------+      +--------+    +--------+                            |    |
     |    | hh |         | hh |             | executor                                      |    |
     |    +----+         +----+          +--------------+                                   |    |
     |                      |            |              |                                   |    |  
     |                      |       +---------+    +---------+  push                        |    |  
     |             +--------+       | JobTube |    | Webhook |------------>-----------------|----|---Endpoints
     |             |        |       +---------+    +---------+                              |    |
     |             |        |           | scheduler     | sub                               |    |
     |        auth |        |job WAL    | dispatch      |                                   |    |
     |      +------+        +---------------------------+------------------+                |    |
     |      |               | tenant shard              | pubsub           | flush          |    |
     |  +----------+    +---------+                 +-------+           +------+            |    |
     |  | auth DB  |    | DB Farm |                 | kafka |           | TSDB |            |    |
     |  +----------+    +---------+                 +-------+           +------+            |    |
     |      |               |                           |                  |                |    |   
     |      |               +----------------------------------------------+                |    |  
     |      |                                   |                                           |    | 
     |      |                                   +-------------------------------------------+    |
     |      |                                                                                    |  
     |      |                                                                               zone |   
     +-------------------------------------------------------------------------------------------+
            |
        WebConsole 

#### Management

    GET    /alive
    GET    /v1/status
    GET    /v1/clusters
    GET    /v1/clients
    GET    /v1/partitions/:cluster/:appid/:topic/:ver
    POST   /v1/topics/:cluster/:appid/:topic/:ver
    DELETE /v1/counter/:name

### FAQ

- why named kateway?

  Admittedly, it is not a good name. Just short for kafka gateway

- how to batch messages in Pub?

  It is http client's job to put the variant length data into json array

- how to consume multiple messages in Sub?

  add param `batch` when Sub.
  kateway uses chunked transfer encoding and client MUST use TLV to decode.

- http header size limit?

  4KB

- what is limit of a pub message in size?

  1 ~ 512KB

- what is limit of a job message in size?

  1 ~ 16KB

- if sub with no arriving message, how long do client get http 204?

  30s

### Dependencies

- github.com/samuel/go-zookeeper

35a5e3bf880ea78fbebad4c7f349d4852c2f75a6

- github.com/Shopify/sarama

d37c73f2b2bce85f7fa16b6a550d26c5372892ef

### TODO

- [ ] kguard watch GC for zk/kafka
- [ ] tag move from body to key, only store hash(tag)
- [ ] kw id replaced by zk sequence
- [ ] cleanup of the idle->active pub clients during shutdown
- [ ] sub status display raw kafka offset status
- [ ] mirror, when destination dies stop consuming
- [ ] tagged metrics
- [ ] Content-Length if batch pub
- [ ] haproxy metrics export to InfluxDB
  http://user:pass@localhost:10890/stats?stats;csv
- [ ] raft consensus
- [ ] hh should use AsyncPub method instead of SyncPub
- [ ] hh index
- [ ] integration: bad Pub/Sub rate limit
  - [X] Pub
  - [ ] Sub
- [ ] when consumer group decision didn't change, refuse to rebalance
- [X] Pub pool will create up to 100 * (count(topic) + count(partition)) goroutines
- [ ] mysql slave of manager to data sync with manager in memory
- [ ] kguard apperr.go Line:128 restart if kafka conn broken
- [X] kguard watch for same group consuming multiple topics
- [X] when startup, hh Empty?
- [X] man /v1/clusters director of cluster distribution
- [ ] bug: /Users/funky/gopkg/src/github.com/samuel/go-zookeeper/zk/conn.go 511 
  panic: non-positive interval for NewTicker
- [ ] go over all zk watchers, handle expire
  - ehaproxy
  - kateway registry
  - actord
  - kguard
- [ ] ehaproxy sometimes haproxy process didn't exit normally
- [ ] bug: when shutdown, consumer group is not sync'ed with sub server stop
- [X] 1 consumer group can only Sub 1(not 2 or more) topic
  - lessen the thundering hurd because rebalance when group members change
  - better manageable
- [ ] when delete a topic, remove its all consumer groups
- [X] bug: kateway gone, but kguard kateway.pubsub.fail didn't notice
- [X] zk session expires and ephemeral znodes?
- [X] IO load balance for hinted handoff
- [ ] metrics for hh
- [ ] swagger
- [ ] reject Sub when the group is being used for another topic
- [X] pause a webhook
- [X] kafka timeout 1ms can work with hinted handoff

- [ ] DFS/Kahn webhook dead loop detection
- [ ] job
  - pause/resume a job
  - job state machine
  - partition table?
- [X] deregister before web listener closed
- [ ] github.com/funkygao/go-metrics/sample.go:151 heap
- [X] failure tolerance
  - pub breaker
  - migration partition from a to b
  - what if a broker killed
- [ ] async pub mem pool
- [ ] sub: what if broker moved
- [X] topic name obfuscation
- [ ] sub with delayed ack
  - StatusNotModified
  - what if rebalanced, and ack buffered p/o
- [ ] pub/sub a disabled topic, discard?
- [ ] features confirm
  - delayed job
  - bury
  - msg tag
  - avro schema registry
- [ ] fetchShadowQueueRecords enable
- [ ] Plugins
  - authentication and authorization
  - transform
  - hooks
- [ ] check hack pkg
- [ ] https, outer ip must https
- [ ] Update to glibc 2.20 or higher
- [ ] compress
  - gzip sub response
  - Pub HTTP POST Request compress
  - compression in kafka
