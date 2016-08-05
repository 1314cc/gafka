# kguard

Kafka clusters body guard that emits health info to InfluxDB.

### Architecture

    
                       👥 ✉  here I am
                        |              
        +---------------------------------+
        |               |                 |
    +--------+     +----------+      +----------+
    | zabbix |     | InfluxDB |      | OpenTSDB |
    +--------+     +----------+      +----------+
        |            |                   |
        |            +-------------------+
        |               ^
        | periodically  |
        | call          | flush
        | RESTful       |
        |          +------------+
        |          | telementry |
        |          +------------+
        |               | collect
        |               V
        |    +-------------------+
        |    | in-memory metrics |
        |    +-------------------+                                       +- external scripts(plugin)
        |               ^                                                |- F5 latency
        |               | write                                          |- zone wide servers
        |       +-----------------------------------+                    |- influx query
        V       |            |                      |                    |- influxdb server
    +-------------+   +--------------+   +----------------------------+  |- pubsub
    | HTTP server |   | SOS receiver |   | Watchers/MonitorAggregator |--|- kafka
    +-------------+   +--------------+   +----------------------------+  |- zk
        |                    |                      |                    +- ...
        +-------------------------------------------+
                             | contains
                     +---------------+          +-----------------+     +-----------------+
                     | kguard leader |          | kguard standby1 |     | kguard standby2 | 
                     +---------------+          +-----------------+     +-----------------+
                             |                          |                       |
                             +--------------------------------------------------+
                                                  | election
                                          +--------------------+
                                          | zookeeper ensemble |
                                          +--------------------+

