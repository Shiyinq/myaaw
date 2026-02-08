#!/bin/bash

pkill -f "consumer-myaaw"

if [ -f tmp/consumer-myaaw.pid ]; then
    rm tmp/consumer-myaaw.pid
fi