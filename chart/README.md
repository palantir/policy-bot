# policy-bot Helm Chart

A Helm chart for deploying [policy-bot](https://github.com/palantir/policy-bot) to Kubernetes.

> **Note:** This chart is currently **experimental** and may change in future releases.

## Installation

We recommend deploying policy-bot in its own namespace:

```bash
# Add your configuration
cp values.yaml my-values.yaml
# Edit my-values.yaml with your settings

# Install the chart in a dedicated namespace
helm install policy-bot ./chart -f my-values.yaml -n policy-bot --create-namespace
```

## Configuration

### GitHub App Configuration

Before deploying policy-bot, you need to create a GitHub App. See the [main documentation](https://github.com/palantir/policy-bot#github-app-configuration) for detailed instructions.

You'll need the following values from your GitHub App:
- **Integration ID** (App ID)
- **Webhook Secret**
- **Private Key** (PEM format)
- **OAuth Client ID**
- **OAuth Client Secret**

### Providing Secrets

There are two ways to provide sensitive configuration:

#### Option 1: Let the chart create the Secret (default)

```yaml
secrets:
  create: true
  github:
    integrationId: "12345"
    webhookSecret: "your-webhook-secret"
    privateKey: |
      -----BEGIN RSA PRIVATE KEY-----
      ...
      -----END RSA PRIVATE KEY-----
  oauth:
    clientId: "your-client-id"
    clientSecret: "your-client-secret"
  session:
    key: "a-random-string-at-least-32-characters"
```

#### Option 2: Use an existing Secret

Create a Secret with the following keys:
- `github-integration-id`
- `github-webhook-secret`
- `github-private-key`
- `oauth-client-id`
- `oauth-client-secret`
- `session-key`

Then reference it:

```yaml
secrets:
  create: false
  existingSecret: "my-policy-bot-secret"
```

### Providing Configuration

#### Option 1: Let the chart create the ConfigMap (default)

Configure options under the `config` key in values.yaml:

```yaml
config:
  create: true
  server:
    address: "0.0.0.0"
    port: 8080
    publicUrl: "https://policy-bot.example.com"
  github:
    webUrl: "https://github.com"
    v3ApiUrl: "https://api.github.com"
    v4ApiUrl: "https://api.github.com/graphql"
  # ... see values.yaml for all options
```

#### Option 2: Use an existing ConfigMap

If you need configuration options not exposed in the chart or want to manage the ConfigMap yourself:

```yaml
config:
  create: false
  existingConfigMap: "my-policy-bot-config"
  key: "policy-bot.yml"  # key in the ConfigMap containing the config
```

### GitHub Enterprise

For GitHub Enterprise, update the GitHub URLs:

```yaml
config:
  github:
    webUrl: "https://github.mycompany.com"
    v3ApiUrl: "https://github.mycompany.com/api/v3"
    v4ApiUrl: "https://github.mycompany.com/api/graphql"
```

### Ingress

To expose policy-bot via Ingress:

```yaml
ingress:
  enabled: true
  className: nginx
  annotations: {}
  hosts:
    - host: policy-bot.example.com
      paths:
        - path: /
          pathType: Prefix
```

### Metrics

Policy-bot exposes Prometheus metrics at `/api/metrics`

## Parameters

### Deployment Settings

| Name               | Description                                                                               | Value                             |
| ------------------ | ----------------------------------------------------------------------------------------- | --------------------------------- |
| `replicaCount`     | Number of replicas to deploy. policy-bot is stateless and safe to run multiple instances. | `2`                               |
| `image.repository` | Docker image repository                                                                   | `palantirtechnologies/policy-bot` |
| `image.pullPolicy` | Image pull policy (Always, IfNotPresent, Never)                                           | `IfNotPresent`                    |
| `image.tag`        | Overrides the image tag whose default is the chart appVersion                             | `""`                              |
| `imagePullSecrets` | Image pull secrets for private registries                                                 | `[]`                              |
| `nameOverride`     | Override the name of the chart (used in resource names)                                   | `""`                              |
| `fullnameOverride` | Override the full name of the chart (used in resource names)                              | `""`                              |

### Service Account

| Name                         | Description                                                     | Value  |
| ---------------------------- | --------------------------------------------------------------- | ------ |
| `serviceAccount.create`      | Create a service account for the deployment                     | `true` |
| `serviceAccount.name`        | Name of the service account (defaults to fullname)              | `""`   |
| `serviceAccount.annotations` | Annotations to add to the service account (e.g., for IAM roles) | `{}`   |

### Pod Settings

| Name                                       | Description                                   | Value            |
| ------------------------------------------ | --------------------------------------------- | ---------------- |
| `podAnnotations`                           | Additional annotations to add to pods         | `{}`             |
| `podSecurityContext`                       | Pod-level security context                    | `{}`             |
| `securityContext.readOnlyRootFilesystem`   | Prevents container from writing to filesystem | `true`           |
| `securityContext.runAsNonRoot`             | Ensures container doesn't run as root         | `true`           |
| `securityContext.runAsUser`                | Runs as non-privileged user (UID 1000)        | `1000`           |
| `securityContext.runAsGroup`               | Runs as non-privileged group (GID 1000)       | `1000`           |
| `securityContext.allowPrivilegeEscalation` | Prevents gaining additional privileges        | `false`          |
| `securityContext.capabilities.drop`        | Linux capabilities to drop                    | `["ALL"]`        |
| `securityContext.seccompProfile.type`      | Seccomp profile type                          | `RuntimeDefault` |
| `terminationGracePeriodSeconds`            | Time to allow for graceful shutdown           | `60`             |
| `resources.limits.memory`                  | Memory limit                                  | `2048Mi`         |
| `resources.requests.cpu`                   | CPU request                                   | `100m`           |
| `resources.requests.memory`                | Memory request                                | `2048Mi`         |
| `nodeSelector`                             | Node selector for pod assignment              | `{}`             |
| `tolerations`                              | Tolerations for pod assignment                | `[]`             |
| `affinity`                                 | Affinity rules for pod assignment             | `{}`             |

### Service Settings

| Name                  | Description                                      | Value       |
| --------------------- | ------------------------------------------------ | ----------- |
| `service.type`        | Service type (ClusterIP, NodePort, LoadBalancer) | `ClusterIP` |
| `service.port`        | Service port                                     | `8080`      |
| `service.annotations` | Additional service annotations                   | `{}`        |

### Ingress Settings

| Name                                 | Description                                                   | Value              |
| ------------------------------------ | ------------------------------------------------------------- | ------------------ |
| `ingress.enabled`                    | Enable ingress resource creation                              | `false`            |
| `ingress.className`                  | Ingress class name (e.g., nginx, traefik, alb)                | `""`               |
| `ingress.annotations`                | Ingress annotations. Use for cert-manager or nginx config.    | `{}`               |
| `ingress.hosts[0].host`              | Hostname for the ingress                                      | `policy-bot.local` |
| `ingress.hosts[0].paths[0].path`     | Path for the ingress                                          | `/`                |
| `ingress.hosts[0].paths[0].pathType` | Path type (Prefix, Exact, ImplementationSpecific)             | `Prefix`           |
| `ingress.tls.enabled`                | Enable TLS termination at ingress                             | `false`            |
| `ingress.tls.certificate`            | Base64 encoded TLS certificate (Option 1: provide directly)   | `""`               |
| `ingress.tls.privateKey`             | Base64 encoded TLS private key (Option 1: provide directly)   | `""`               |
| `ingress.tls.existingSecret`         | Name of existing TLS secret (Option 2: use existing)          | `""`               |
| `ingress.tls.host`                   | Hostname for TLS certificate (defaults to first ingress host) | `""`               |

### Policy-bot Configuration

| Name                                     | Description                                                                    | Value                            |
| ---------------------------------------- | ------------------------------------------------------------------------------ | -------------------------------- |
| `config.create`                          | Create a ConfigMap for the config file                                         | `true`                           |
| `config.existingConfigMap`               | Name of existing ConfigMap to use (when create is false)                       | `""`                             |
| `config.key`                             | Key in existing ConfigMap containing the config file                           | `policy-bot.yml`                 |
| `config.server.address`                  | Server listen address                                                          | `0.0.0.0`                        |
| `config.server.port`                     | Server port. The container and service will use this port.                     | `8080`                           |
| `config.server.publicUrl`                | Public URL for OAuth callbacks and links. Auto-detected from ingress if empty. | `""`                             |
| `config.logging.text`                    | Use human-readable text logs instead of JSON                                   | `false`                          |
| `config.logging.level`                   | Log level (debug, info, warn, error)                                           | `info`                           |
| `config.cache.maxSize`                   | Maximum size for the in-memory cache (e.g., 50MB)                              | `50MB`                           |
| `config.workers.workers`                 | Number of workers processing webhook events                                    | `10`                             |
| `config.workers.queueSize`               | Queue size for buffering incoming webhook events                               | `100`                            |
| `config.workers.githubTimeout`           | Timeout for GitHub API requests (e.g., 10s, 1m)                                | `10s`                            |
| `config.github.webUrl`                   | GitHub web URL. For GitHub Enterprise, use your instance URL.                  | `https://github.com`             |
| `config.github.v3ApiUrl`                 | GitHub REST API (v3) URL                                                       | `https://api.github.com`         |
| `config.github.v4ApiUrl`                 | GitHub GraphQL API (v4) URL                                                    | `https://api.github.com/graphql` |
| `config.options.policyPath`              | Path to policy file in repositories                                            | `.policy.yml`                    |
| `config.options.sharedRepository`        | Shared repository name for organization-wide policies                          | `.github`                        |
| `config.options.sharedPolicyPath`        | Path to policy file in shared repository                                       | `policy.yml`                     |
| `config.options.statusCheckContext`      | Context name for status checks on pull requests                                | `policy-bot`                     |
| `config.options.expandRequiredReviewers` | Expand required reviewers to show individual users                             | `false`                          |
| `config.options.forceSharedPolicy`       | Force use of shared policy for all repositories                                | `false`                          |
| `config.metrics.quantiles.enabled`       | Enable custom quantile configuration                                           | `false`                          |
| `config.metrics.quantiles.histogram`     | Histogram quantiles to track                                                   | `[]`                             |
| `config.metrics.quantiles.timer`         | Timer quantiles to track                                                       | `[]`                             |
| `config.metrics.labels`                  | Additional labels to add to metrics                                            | `{}`                             |

### Secrets Configuration

| Name                           | Description                                           | Value  |
| ------------------------------ | ----------------------------------------------------- | ------ |
| `secrets.create`               | Create a Kubernetes Secret for sensitive values       | `true` |
| `secrets.existingSecret`       | Name of existing Secret to use (when create is false) | `""`   |
| `secrets.github.integrationId` | GitHub App ID (Integration ID)                        | `""`   |
| `secrets.github.webhookSecret` | Webhook secret configured in your GitHub App          | `""`   |
| `secrets.github.privateKey`    | GitHub App private key in PEM format                  | `""`   |
| `secrets.oauth.clientId`       | OAuth Client ID from your GitHub App                  | `""`   |
| `secrets.oauth.clientSecret`   | OAuth Client Secret from your GitHub App              | `""`   |
| `secrets.session.key`          | Session signing key for encrypting user sessions      | `""`   |

### Additional Configuration

| Name                | Description                                              | Value |
| ------------------- | -------------------------------------------------------- | ----- |
| `extraEnv`          | Additional environment variables to add to the container | `[]`  |
| `extraVolumes`      | Additional volumes to add to the pod                     | `[]`  |
| `extraVolumeMounts` | Additional volume mounts to add to the container         | `[]`  |

## Upgrading

```bash
helm upgrade policy-bot ./chart -f my-values.yaml
```

## Uninstalling

```bash
helm uninstall policy-bot
```
