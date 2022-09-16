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

## todo
update deployment with goroutine and tokenbucket
