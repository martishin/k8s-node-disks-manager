.PHONY: tools generate manifests codegen fmt tidy test minikube-up minikube-delete images images-load helm-install helm-uninstall demo help

PROFILE ?= node-disks-dev
NAMESPACE ?= node-disks-system
CONTROLLER_IMG ?= node-disks-operator:dev
AGENT_IMG ?= node-disks-agent:dev

LOCALBIN ?= $(shell pwd)/bin
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
CONTROLLER_TOOLS_VERSION ?= v0.19.0

help: ## Show available targets
	@awk 'BEGIN {FS=":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "%-25s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

tools: $(CONTROLLER_GEN) ## Install local build tools

generate: tools ## Generate deepcopy code
	$(CONTROLLER_GEN) object:headerFile="/dev/null" paths="./..."

manifests: tools ## Generate CRD directly into Helm chart (source of truth)
	rm -f charts/node-disks-manager/crds/*.yaml
	$(CONTROLLER_GEN) crd:maxDescLen=0 paths="./api/..." output:crd:artifacts:config=charts/node-disks-manager/crds

codegen: generate manifests ## Run all code/config generation for API changes

fmt: ## Format Go code
	go fmt ./...

tidy: ## Tidy modules
	go mod tidy

test: ## Run Go tests
	go test ./...

minikube-up: ## Start local multi-node minikube cluster
	minikube start -p $(PROFILE) --kubernetes-version=v1.32.0 --nodes=4
	kubectl config use-context $(PROFILE)
	kubectl taint nodes -l node-role.kubernetes.io/control-plane node-role.kubernetes.io/control-plane=:NoSchedule --overwrite || true
	kubectl taint nodes -l node-role.kubernetes.io/master node-role.kubernetes.io/master=:NoSchedule --overwrite || true

minikube-delete: ## Delete minikube profile
	minikube delete -p $(PROFILE)

images: ## Build images once in local Docker
	docker build -t $(CONTROLLER_IMG) -f Dockerfile.controller .
	docker build -t $(AGENT_IMG) -f Dockerfile.agent .

images-load: images ## Load locally built images into the minikube profile
	minikube -p $(PROFILE) image load $(CONTROLLER_IMG)
	minikube -p $(PROFILE) image load $(AGENT_IMG)

helm-install: ## Install chart
	helm upgrade --install node-disks-manager charts/node-disks-manager -n $(NAMESPACE) --create-namespace \
		--set controller.image.repository=$(word 1,$(subst :, ,$(CONTROLLER_IMG))) \
		--set controller.image.tag=$(word 2,$(subst :, ,$(CONTROLLER_IMG))) \
		--set agent.image.repository=$(word 1,$(subst :, ,$(AGENT_IMG))) \
		--set agent.image.tag=$(word 2,$(subst :, ,$(AGENT_IMG)))

helm-uninstall: ## Uninstall chart
	helm uninstall node-disks-manager -n $(NAMESPACE) || true

demo: minikube-up images-load helm-install ## End-to-end demo setup
	kubectl -n $(NAMESPACE) get pods -o wide
	kubectl get nodedisks
