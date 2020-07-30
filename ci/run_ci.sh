#!/bin/env bash
# CI script for CentOS7 jobs
set -ex

# enable required repo(s)
yum install -y epel-release

# install Go
yum install -y golang
export GOPATH=/root/go
export GOBIN=$GOPATH/bin
export PATH=$PATH:$GOBIN

# install dep and sensubility dependencies
yum install -y qpid-proton-c-devel
go get -u github.com/golang/dep/...
dep ensure -v

# install collectd-sensubility
go build -o $GOBIN/collectd-sensubility main/main.go

# install Python part of CI
export LC_ALL=en_US.UTF-8
export LANG=en_US.UTF-8
yum install -y Cython python3 python3-pip python36-Cython python3-qpid-proton
pip3 install -r ci/mocks/sensu/requirements.txt
pip3 install -r ci/mocks/amqp1/requirements.txt

# verify sensubility's behaviour on Sensu side
python3 ci/mocks/sensu/sensu_scheduler.py &
sleep 5

export COLLECTD_SENSUBILITY_CONFIG=$PWD/ci/mocks/sensu/collectd-sensubility.conf
touch sensubility-ci.log
collectd-sensubility --debug --log sensubility-ci.log &

python3 ci/mocks/sensu/sensu_verify.py --timeout 120
EXIT_CODE=$?
pkill -f collectd-sensubility
cat sensubility-ci.log
echo "Response to Sensu server side verified with result: $EXIT_CODE"

# verify sensubility's behaviour on AMQP1.0 side
export COLLECTD_SENSUBILITY_CONFIG=$PWD/ci/mocks/amqp1/collectd-sensubility.conf
echo "" > sensubility-ci.log
collectd-sensubility --debug --log sensubility-ci.log &

python3 ci/mocks/amqp1/amqp1_verify.py --timeout 120
EXIT_CODE+=$?
pkill -f collectd-sensubility
cat sensubility-ci.log
echo "Response to AMQP1.0 message bus verified with result: $EXIT_CODE"

cat sensubility-ci.log
exit $EXIT_CODE
