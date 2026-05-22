# Kubernetes

These routes target Kubernetes environments and generally expect `?env=<environmentId>`.

## Pods

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/pods/` | Lists pods. |
| GET | `/api/pods/{namespace}/{name}` | Returns one pod. |
| DELETE | `/api/pods/{namespace}/{name}` | Deletes a pod. |
| GET | `/api/pods/{namespace}/{name}/logs` | Returns pod logs. |

## Deployments

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/deployments/` | Lists deployments. |
| GET | `/api/deployments/{namespace}/{name}` | Returns one deployment. |
| DELETE | `/api/deployments/{namespace}/{name}` | Deletes a deployment. |
| POST | `/api/deployments/{namespace}/{name}/scale` | Scales a deployment. |
| POST | `/api/deployments/{namespace}/{name}/restart` | Triggers a rollout restart. |

## Services

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/k8s-services/` | Lists Kubernetes services. |
| GET | `/api/k8s-services/{namespace}/{name}` | Returns one Kubernetes service. |
| DELETE | `/api/k8s-services/{namespace}/{name}` | Deletes a Kubernetes service. |

## Namespaces

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/namespaces/` | Lists namespaces. |
| GET | `/api/namespaces/{name}` | Returns one namespace. |

## Notes

- McHarbor exposes Kubernetes resources through the same auth and RBAC model as Docker routes.
- Namespace and resource path parameters are part of the URL, not query parameters.
