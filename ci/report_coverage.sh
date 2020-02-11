#!/bin/env bash
# CI script for CentOS7 jobs
set -x

go get -u golang.org/x/tools/cmd/cover
go get -u github.com/mattn/goveralls

cat > coverage.out <<EOF
mode: set
EOF

for pkg in $(go list ./...); do
  go test -cover -coverpkg "$pkg" -coverprofile coverfragment.out ./tests/... && \
  grep -h -v "mode: set" coverfragment.out >> coverage.out
done

goveralls -service=travis-ci -repotoken=$COVERALLS_TOKEN -coverprofile=coverage.out
EXIT_CODE=$?
cat coverage.out

exit $EXIT_CODE
