#!/bin/bash

mkdir -p tmp

pkill -f "consumer-teo"

sleep 1

go run cmd/consumer/consumer-teo.go > tmp/consumer-teo.log 2>&1 &
echo $! > tmp/consumer-teo.pid