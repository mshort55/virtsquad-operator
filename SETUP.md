# Creating a Kubernetes Operator with Kubebuilder - VirtSquad CRD

## High-Level Steps for Building VirtSquad Operator

### 1. Environment Setup
- Install Go (1.19+)
- Install Docker
- Install kubectl and access to Kubernetes cluster
- Install kubebuilder CLI tool
- Install kustomize

### 2. Initialize the Operator Project
```bash
mkdir virtsquad-operator
cd virtsquad-operator
kubebuilder init --domain mshort55.io --repo github.com/mshort55/virtsquad-operator
```

### 3. Initialize Git Repository and Push to GitHub
```bash
git init
git add .
git commit -m "Initial kubebuilder project setup"
git branch -M main
git remote add origin https://github.com/mshort55/virtsquad-operator.git
git push -u origin main
```

### 4. Create the VirtSquad CRD and Controller
```bash
kubebuilder create api --group apps --version v1 --kind VirtSquad --resource --controller
```

### 5. Define the VirtSquad CRD Spec
- Edit `api/v1/virtsquad_types.go`
- Add spec fields for team members:
  - Oksana (bool)
  - Kurtis (bool) 
  - Matt (bool)
  - Kike (bool)
- Define status fields for tracking created pods
- Run `make generate` to update generated code
- Run `make manifests` to update CRD manifests

### 6. Implement Controller Logic
- Edit `controllers/virtsquad_controller.go`
- Implement reconciliation logic:
  - Read VirtSquad CR spec
  - For each enabled team member, create/update corresponding pod
  - Update VirtSquad status with pod information
  - Handle pod cleanup when team members are disabled

### 7. Build and Test Locally
```bash
make generate
make manifests
make fmt
make vet
make test
```

### 8. Deploy CRD to Kubernetes
```bash
make install
```

### 9. Run Controller Locally (for testing)
```bash
make run
```

### 10. Create Sample VirtSquad Resource
- Create YAML file with VirtSquad CR
- Set desired team members to true in spec
- Apply to cluster: `kubectl apply -f sample-virtsquad.yaml`

### 11. Verify Operation
- Check that pods are created: `kubectl get pods`
- Verify pod names contain team member names
- Check VirtSquad resource status: `kubectl get virtsquad -o yaml`

### 12. Build and Deploy Operator Image (Optional)
```bash
make docker-build IMG=controller:latest
make docker-push IMG=controller:latest
make deploy IMG=controller:latest
```

### 13. Clean Up and Testing
- Test enabling/disabling different team members
- Verify pods are created/deleted accordingly
- Test edge cases and error handling

## Key Files to Modify
- `api/v1/virtsquad_types.go` - CRD definition
- `controllers/virtsquad_controller.go` - Controller logic
- `config/samples/` - Sample CR for testing

## Expected Outcome
After completing these steps, you'll have a working Kubernetes operator that:
- Manages VirtSquad custom resources
- Creates pods with team member names when specified in the CR spec
- Automatically reconciles desired state vs actual state
- Can be deployed and tested in any Kubernetes cluster