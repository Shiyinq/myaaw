#!/bin/bash

mkdir -p tmp

pkill -f "consumer-myaaw"

sleep 1

go run cmd/consumer/consumer-myaaw.go > tmp/consumer-myaaw.log 2>&1 &
echo $! > tmp/consumer-myaaw.pid