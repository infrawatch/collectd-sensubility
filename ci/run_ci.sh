#!/bin/env bash
# CI script for CentOS7 jobs
set -ex

# enable required repo(s)
yum install -y epel-release

# install Go part of CI
yum install -y golang
export GOPATH=/root/go
export GOBIN=$GOPATH/bin
export PATH=$PATH:$GOBIN

# install dep
go get -u github.com/golang/dep/...
dep ensure -v

# install collectd-sensubility
go build -o $GOBIN/collectd-sensubility main/main.go

# install Python part of CI
export LC_ALL=en_US.UTF-8
export LANG=en_US.UTF-8
yum install -y python3 python3-pip
pip3 install -r ci/mocks/sensu/requirements.txt

# run mocked Sensu scheduler
python3 ci/mocks/sensu/sensu_scheduler.py &
sleep 5

# run sensubility
export COLLECTD_SENSUBILITY_CONFIG=$PWD/ci/collectd-sensubility.conf
collectd-sensubility --debug --log sensubility-ci.log &

# verify sensubility's behaviour
python3 ci/mocks/sensu/sensu_verify.py
EXIT_CODE=$?
cat sensubility-ci.log

exit $EXIT_CODE
