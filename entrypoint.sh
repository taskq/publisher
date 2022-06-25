#!/bin/sh
#
# Docker container Entrypoint script
#


[ -z "${LISTEN_ADDRESS}" ] && LISTEN_ADDRESS="0.0.0.0"
[ -z "${LISTEN_PORT}" ] && LISTEN_PORT="8080"

[ -z "${REDIS_ADDRESS}" ] && REDIS_ADDRESS="127.0.0.1"
[ -z "${REDIS_PORT}" ] && REDIS_PORT="6379"


./publisher -verbose -bind "${LISTEN_ADDRESS}:${LISTEN_PORT}" -redis-address "${REDIS_ADDRESS}:${REDIS_PORT}"
