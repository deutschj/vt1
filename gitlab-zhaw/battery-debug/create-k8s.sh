kn service create battery-debug \
  --image docker.io/juliandeutsch/battery-debug:0.0.1 \
  --env POWER_STATUS_URL=http://battery-simulator-svc.monitoring.svc.cluster.local:8080/status
