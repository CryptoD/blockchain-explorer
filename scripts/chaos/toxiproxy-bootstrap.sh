#!/bin/sh
# Starts toxiproxy-server and registers a proxy: listen :6379 -> upstream redis:6379.
# Used by scripts/chaos/docker-compose.yml (see docs/CHAOS_TESTING.md).
set -e
toxiproxy-server &
sleep 2
toxiproxy-cli create -l 0.0.0.0:6379 -u redis:6379 redis
wait
