# GitHub Secrets Configuration

## Required Secrets for CI/CD

The `deploy-dev.yml` workflow requires these repository secrets:

| Secret | Description | Source |
|--------|-------------|--------|
| `AZURE_CREDENTIALS` | Azure SP JSON for ACR login | ✅ Auto-set from petaverse-keyvault |
| `FEISHU_WEBHOOK` | Feishu notification webhook URL | ✅ Auto-set from petaverse-keyvault |
| `MANIFESTS_TOKEN` | GitHub PAT with write access to pv-k8s-manifests | See below |

## MANIFESTS_TOKEN Setup

Create a fine-grained Personal Access Token with:
- Repository access: `petaverse-cloud/pv-k8s-manifests` only
- Permissions: Contents (Read and Write)

Steps:
1. Go to: https://github.com/settings/tokens?type=beta
2. Click "Generate new token"
3. Token name: `global-sync-service-manifests`
4. Expiration: 90 days (or your preference)
5. Repository access: Select repositories → `petaverse-cloud/pv-k8s-manifests`
6. Permissions → Repository permissions → Contents: Read and write
7. Generate token and copy the value
8. Set the secret:

```bash
cd ~/pv-global-sync-service
gh secret set MANIFESTS_TOKEN --body "ghp_xxxxxxxx"
```

## How the CI/CD Pipeline Works

```
push to main
    │
    ├── build job: build Docker image → push to ACR (via Azure SP)
    │
    └── update-manifests job:
            ├── checkout pv-k8s-manifests (using MANIFESTS_TOKEN PAT)
            ├── update kustomization.yaml with new image tag
            └── push to pv-k8s-manifests/main
                    │
                    └── sync-argocd workflow (in pv-k8s-manifests):
                            └── argocd app sync → deploy to K8s
```

## ArgoCD Secrets (pv-k8s-manifests repo - already configured)

| Secret | Status |
|--------|--------|
| `ARGOCD_SERVER` | ✅ Set (https://argocd.verse4.pet) |
| `ARGOCD_TOKEN` | ✅ Refreshed 2026-04-10 |
