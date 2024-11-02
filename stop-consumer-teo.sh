#!/bin/bash

pkill -f "consumer-teo"

if [ -f tmp/consumer-teo.pid ]; then
    rm tmp/consumer-teo.pid
fi