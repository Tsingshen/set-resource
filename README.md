## Contoller the resource of deployment be limited

### config.yaml模板
```
includeNamespace:
- beta1
resource:
  requests:
    cpu: 10m
    memory: 56Mi
  limits:
    cpu: 2000m
    memory: 2048Mi
ekletDeployment:
  deployment:
  - shencq/centos
  nodeSelector:
    "ops.lzwk.com/node-type": eklet
  prefix: "eklet-"
```

## ignore deployment
set deployment annotations to ignore
```
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    not-reset-resources: "true"
```

