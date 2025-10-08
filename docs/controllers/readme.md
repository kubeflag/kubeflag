# Kubeflag Controllers

Kubeflag's core logic is implemented through Kubernetes controllers using [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime).  
Each controller reconciles a specific Custom Resource (CRD), ensuring the actual cluster state matches the desired state described in that CRD.

## Controllers Overview

| Controller | CRD | Responsibility |
|-------------|-----|----------------|
| **Challenge** | `Challenge` | Defines templates of challenges, handles namespace creation, and triggers DataSyncer. |
| **ChallengeInstance** | `ChallengeInstance` | Deploys and manages instances of challenges per tenant/player. |
| **DataSyncer** | Annotated `Secret` / `ConfigMap` | Synchronizes data objects across namespaces. |
| **Tenant** | `Tenant` | Enforces multi-tenancy limits and policies. |
| **Consumer** | `Consumer` | Manages API consumers and authentication tokens. |

Each controller is registered in the Kubeflag `controller-manager` and runs as part of the same deployment.

Controllers communicate indirectly through the Kubernetes API (e.g., Challenge creation triggers the DataSyncer through annotations).

Refer to the individual pages below for deep dives:

- [Challenge Controller](challenge.md)
- [ChallengeInstance Controller](challengeinstance.md)
- [DataSyncer Controller](datasyncer.md)
- [Tenant Controller](tenant.md)
- [Consumer Controller](consumer.md)
