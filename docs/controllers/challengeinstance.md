# ChallengeInstance Controller

The **ChallengeInstance Controller** is responsible for creating, updating, and deleting challenge instances for players.

## Responsibilities

1. **Instance Provisioning**
   - Deploys Kubernetes workloads defined in the parent Challenge’s template.

2. **Exposure**
   - Exposes workloads via a Service of type NodePort (default).
   - Future: may support Ingress or LoadBalancer.

3. **Trigger Death**
   - Once we reach the TTL (duration), the challenge instance must kill itself