# publisher
TaskQ Publisher

# Provisioning

## Docker/PodMan

```bash
podman run \
  --network host \
  --interactive \
  --tty \
  --detach \
  --rm \
  -e "REDIS_ADDRESS=10.32.0.238" \
  --name taskq-publisher \
  ghcr.io/taskq/publisher/publisher:1.0.0
```


# Prometheus Metrics

```bash
curl http://127.0.0.1:8080/metrics
```

```prometheus
# TYPE taskq_publisher_channel_len counter
# HELP Last checked Redis channel length
taskq_publisher_channel_len{channel="beep"} 112
taskq_publisher_channel_len{channel="junk"} 9027
# TYPE taskq_publisher_requests counter
# HELP Number of the requests to the TaskQ Publisher by type
taskq_publisher_requests{method="put"} 2
# TYPE taskq_publisher_errors counter
# HELP Number of the raised errors
taskq_publisher_errors 0
# TYPE taskq_publisher_index counter
# HELP Number of the requests to /
taskq_publisher_index 0
```
