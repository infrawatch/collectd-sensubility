# collectd-sensubility

[![Coverage Status](https://coveralls.io/repos/github/infrawatch/collectd-sensubility/badge.svg?branch=master)](https://coveralls.io/github/infrawatch/collectd-sensubility?branch=master)
[![Build Status](https://travis-ci.org/infrawatch/collectd-sensubility.svg?branch=master)](https://travis-ci.org/infrawatch/collectd-sensubility)

## Project's goal

Sensubility's goal is to provide smooth transition from Ruby Sensu-based availability monitoring solution to STF-based
(Service Telemetry Framework) availability monitoring.

Sensubility can be run either as standalone service, but it also behaves well with collectd and it's collectd-exec plugin.
The agent behaves exactly the same as Ruby-based sensu-client does, eg. it connects to RabbitMQ message bus and listens
for check execution requests from Sensu server. Sensubility then executes the requested command and reports the result back
to proper channel in RabbitMQ message bus.

## Installation

### From source
```
git clone https://github.com/infrawatch/collectd-sensubility
cd collectd-sensubility
go get -u github.com/golang/dep/...
dep ensure -v -vendor-only
go build -o /usr/bin/collectd-sensubility main/main.go
```

### On CentOS7
```
sudo yum install -y centos-release-opstools
sudo yum install -y collectd-sensubility
```

## Configuration

Configuration file lives in /etc/collectd-sensubility.conf file by default. You can change the default path
via COLLECTD_SENSUBILITY_CONFIG environment variable.

Following is a example of configuration file:

```
[default]
log_file=/var/log/collectd-sensubility.log
log_level=WARNING

[sensu]
connection=amqp://sensu:sensu@172.1.2.114:5672//sensu
subscriptions=all,overcloud-ceilometer-aodh-api,overcloud-ceilometer-aodh-evaluator
client_name=controller-0.internalapi.redhat.local
client_address=172.1.2.3
worker_count=2
checks={
  "check-container-health":{
    "command":"
      output=''
      for i in $(systemctl list-timers --no-pager --no-legend \"tripleo*healthcheck.timer\" | awk '{print $14}'); do
        i=${i%.timer}
        if result=$(systemctl show $i --property=ActiveState | awk '{split($0,a,/=/); print a[2]}'); then
          if [ \"$result\" == 'failed' ]; then
            timestamp=$(systemctl show $i --property=InactiveEnterTimestamp | awk '{print $2, $3}' )
            log=$(journalctl -u $i -t podman --since \"${timestamp}\" --no-pager --output=cat --directory /var/log/journal)
            if [ !  -z  \"$output\" ]; then
              output=\"$i: $log ; $output\"
            else
              output=\"$i: $log\"
            fi
          fi
        fi
      done
      if [ ! -z \"${output}\" ]; then
        echo ${output:3} && exit 2;
      fi",
    "handlers":[],
    "interval":10,
    "occurrences":3,
    "refresh":90,
    "standalone":true}}

[amqp1]
connection=amqp://saf:saf@127.0.0.1:5666/collectd
```

As you can see it is possible to also configure standalone checks (checks scheduled on the client side) with sensubility as you could with sensu-client.
The check configuration is compatible with the Sensu format supporting most of the configuration keys.

To enable running sensubility with collectd, you need to use collectd-exec plugin with following configuration:

```
<LoadPlugin exec>
  Globals false
</LoadPlugin>

<Plugin exec>
  Exec "collectd:collectd" "collectd-sensubility"
</Plugin>
```

Or you can run sensubility as standalone daemon: `collectd-sensubility &`
