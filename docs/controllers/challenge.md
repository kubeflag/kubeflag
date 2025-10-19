# Challenge Controller

The **Challenge Controller** watches `Challenge` CRDs and ensures that each challenge template is properly set up in the cluster.

## Responsibilities

1. **Validate Challenge Definition**
   - Ensures referenced data objects (Secrets/ConfigMaps) exist.
   - Checks if the tenant (if defined) is valid.

2. **Namespace Creation**
   - When a new `Challenge` is created, a dedicated namespace may be created for managing resources associated with that challenge.

3. **Data Object Annotation**
   - Annotates referenced Secrets/ConfigMaps with metadata:
     ```
     kubeflag.io/challenges: "challenge-name"
     kubeflag.io/sync-targets: "namespace-list"
     ```
   - This triggers the DataSyncer controller to replicate data objects to instance namespaces.


4. **Lifecycle Management**
   - Handles updates or deletions of Challenges (e.g., cleanup annotations, optional namespace removal).
   - Updates the existing challenge instances when the template is updated. 
   - Cleanup the existing challenge instances if the Main challenge is deleted