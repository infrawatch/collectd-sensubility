#!/bin/env bash
# CI script for CentOS8 jobs
set -ex

# enable required repo(s)
yum install -y epel-release

# Locale setting in CentOS8 is broken without this package
yum install -y glibc-langpack-en

# install Go
yum install -y golang
export GOBIN=$GOPATH/bin
export PATH=$PATH:$GOBIN

# install RPM dependencies
yum install -y qpid-proton-c-devel python3-qpid-proton git /usr/bin/pkill

# qpid-proton is pinned because latest version makes electron panic with:
# cannot marshal string: overflow: not enough space to encode
dnf downgrade -y https://cbs.centos.org/kojifiles/packages/qpid-proton/0.35.0/3.el8s/x86_64/qpid-proton-c-0.35.0-3.el8s.x86_64.rpm https://cbs.centos.org/kojifiles/packages/qpid-proton/0.35.0/3.el8s/x86_64/qpid-proton-c-devel-0.35.0-3.el8s.x86_64.rpm https://cbs.centos.org/kojifiles/packages/qpid-proton/0.35.0/3.el8s/x86_64/python3-qpid-proton-0.35.0-3.el8s.x86_64.rpm

# install go.mod dependencies
go mod tidy

# check if apputils repository has same topic branch
BRANCH="$(echo ${GITHUB_HEAD_REF:-${GITHUB_REF#refs/heads/}})"
if git ls-remote --exit-code --heads https://github.com/infrawatch/apputils.git $BRANCH; then
    pushd ..
    git clone -b $BRANCH https://github.com/infrawatch/apputils.git
    popd
    echo $(grep -m1 github.com/infrawatch/apputils go.mod | awk '{print "replace ", $1, " ", $2, " => ../apputils"}') >> go.mod
fi

# install collectd-sensubility
go build -o $GOBIN/collectd-sensubility main/main.go

# install Python part of CI
export LC_ALL=en_US.UTF-8
export LANG=en_US.UTF-8
yum install -y python3 python3-pip
pip3 install Cython
pip3 install -r ci/mocks/sensu/requirements.txt
pip3 install -r ci/mocks/amqp1/requirements.txt

# verify sensubility's behaviour on Sensu side
python3 ci/mocks/sensu/sensu_scheduler.py &
sleep 5

export COLLECTD_SENSUBILITY_CONFIG=$PWD/ci/mocks/sensu/collectd-sensubility.conf
touch sensubility-ci.log
collectd-sensubility --debug --log=sensubility-ci.log &

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
