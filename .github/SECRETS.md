# GitHub Secrets Configuration

## Required Secrets for CI/CD

The `deploy-dev.yml` workflow requires these repository secrets:

| Secret | Description | How to Get |
|--------|-------------|------------|
| `AZURE_CREDENTIALS` | Azure service principal JSON for ACR login | Copy from pv-wigowago-api repo secrets |
| `FEISHU_WEBHOOK` | Feishu notification webhook URL | Copy from pv-wigowago-api repo secrets |
| `ARGOCD_APP_ID` | GitHub App ID for pushing to pv-k8s-manifests | Copy from pv-wigowago-api repo secrets |
| `ARGOCD_APP_PRIVATE_KEY` | GitHub App private key (PEM) | Copy from pv-wigowago-api repo secrets |

## How to Set Secrets

1. Go to: https://github.com/petaverse-cloud/pv-global-sync-service/settings/secrets/actions
2. Click "New repository secret"
3. For each secret above, copy the value from pv-wigowago-api and paste it here

## Alternative: Copy from pv-wigowago-api

```bash
# In a terminal with gh CLI authenticated:
cd pv-global-sync-service

# You'll need to manually copy values from:
# https://github.com/petaverse-cloud/pv-wigowago-api/settings/secrets/actions

gh secret set AZURE_CREDENTIALS --body "..."
gh secret set FEISHU_WEBHOOK --body "..."
gh secret set ARGOCD_APP_ID --body "..."
gh secret set ARGOCD_APP_PRIVATE_KEY --body "..."
```

## How the CI/CD Pipeline Works

```
push to main
    │
    ├── build job: build Docker image → push to ACR
    │
    └── update-manifests job:
            ├── checkout pv-k8s-manifests (using GitHub App token)
            ├── update kustomization.yaml with new image tag
            └── push to pv-k8s-manifests/main
                    │
                    └── sync-argocd workflow (in pv-k8s-manifests):
                            └── argocd app sync → deploy to K8s
```

## ArgoCD Secrets (pv-k8s-manifests repo - already configured)

| Secret | Value | Status |
|--------|-------|--------|
| `ARGOCD_SERVER` | `https://argocd.verse4.pet` | ✅ Set |
| `ARGOCD_TOKEN` | Generated from argocd CLI | ✅ Refreshed 2026-04-10 |
