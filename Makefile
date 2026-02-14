.DELETE_ON_ERROR: clean

EXECUTABLES = go zip shasum
K := $(foreach exec,$(EXECUTABLES),\
  $(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH)))

# Add .SHELLFLAGS to ensure that shell errors are propagated
.SHELLFLAGS := -e -c

# this is used to rename the repository when is created from the template
# we will use the git remote url to get the repository name
GIT_REPOSITORY_NAME            ?= $(shell git remote get-url origin | cut -d '/' -f 2 | cut -d '.' -f 1)
GIT_REPOSITORY_NAME_UNDERSCORE := $(subst -,_,$(GIT_REPOSITORY_NAME))

PROJECT_NAME      ?= $(shell grep module go.mod | cut -d '/' -f 3)
PROJECT_NAMESPACE ?= $(shell grep module go.mod | cut -d '/' -f 2 )
PROJECT_MODULES_PATH := $(shell ls -d cmd/*)
PROJECT_MODULES_NAME := $(foreach dir_name, $(PROJECT_MODULES_PATH), $(shell basename $(dir_name)) )
PROJECT_DEPENDENCIES := $(shell go list -m -f '{{if not (or .Indirect .Main)}}{{.Path}}{{end}}' all)

BUILD_DIR       := ./build
DIST_DIR        := ./dist
DIST_ASSETS_DIR := $(DIST_DIR)/assets

PROJECT_COVERAGE_FILE ?= $(BUILD_DIR)/coverage.txt
PROJECT_COVERAGE_MODE	?= atomic
PROJECT_COVERAGE_TAGS ?= unit
PROJECT_INTEGRATION_COVERAGE_TAGS ?= integration

GIT_VERSION     ?= $(shell git rev-parse --abbrev-ref HEAD | cut -d "/" -f 2)
GIT_COMMIT      ?= $(shell git rev-parse HEAD | tr -d '\040\011\012\015\n')
GIT_BRANCH      ?= $(shell git rev-parse --abbrev-ref HEAD | tr -d '\040\011\012\015\n')
GIT_USER        := $(shell git config --get user.name | tr -d '\040\011\012\015\n')
GIT_USER_EMAIL  := $(shell git config --get user.email | tr -d '\040\011\012\015\n')
BUILD_DATE      := $(shell date +'%Y-%m-%dT%H:%M:%SZ')

GO_LDFLAGS_OPTIONS ?= -s -w
define EXTRA_GO_LDFLAGS_OPTIONS
-X '"'github.com/$(PROJECT_NAMESPACE)/$(PROJECT_NAME)/internal/version.Version=$(GIT_VERSION)'"' \
-X '"'github.com/$(PROJECT_NAMESPACE)/$(PROJECT_NAME)/internal/version.BuildDate=$(BUILD_DATE)'"' \
-X '"'github.com/$(PROJECT_NAMESPACE)/$(PROJECT_NAME)/internal/version.GitCommit=$(GIT_COMMIT)'"' \
-X '"'github.com/$(PROJECT_NAMESPACE)/$(PROJECT_NAME)/internal/version.GitBranch=$(GIT_BRANCH)'"' \
-X '"'github.com/$(PROJECT_NAMESPACE)/$(PROJECT_NAME)/internal/version.BuildUser=$(GIT_USER_EMAIL)'"'
endef

GO_LDFLAGS     := -ldflags "$(GO_LDFLAGS_OPTIONS) $(EXTRA_GO_LDFLAGS_OPTIONS)"
GO_CGO_ENABLED ?= 0
GO_OPTS        ?= -v
GO_OS          ?= linux darwin windows
GO_ARCH        ?= arm64 amd64
# avoid mocks in tests
GO_FILES       := $(shell go list ./... | grep -v mocks | grep -v docs)
GO_GRAPH_FILE  := $(BUILD_DIR)/go-mod-graph.txt

######## Functions ########
# this is a function that will execute a command and print a message
# MAKE_DEBUG=true make <target> will print the command
# MAKE_STOP_ON_ERRORS=true make any fail will stop the execution if the command fails, this is useful for CI
# NOTE: if the command has a > it will print the output into the original redirect of the command
MAKE_STOP_ON_ERRORS ?= false
MAKE_DEBUG          ?= false

define exec_cmd
$(if $(filter $(MAKE_DEBUG),true),\
	${1} \
, \
	$(if $(filter $(MAKE_STOP_ON_ERRORS),true),\
		@${1}  > /dev/null && printf "  🤞 ${1} ✅\n" || (printf "  ${1} ❌ 🖕\n"; exit 1) \
	, \
		$(if $(findstring >, $1),\
			@${1} 2>/dev/null; _exit_code=$$?; if [ $$_exit_code -eq 0 ]; then printf "  🤞 ${1} ✅\n"; else printf "  ${1} ❌ 🖕\n"; fi; exit $$_exit_code \
		, \
			@${1} > /dev/null 2>&1; _exit_code=$$?; if [ $$_exit_code -eq 0 ]; then printf '  🤞 ${1} ✅\n'; else printf '  ${1} ❌ 🖕\n'; fi; exit $$_exit_code \
		) \
	) \
)

endef # don't remove the white line before endef

###############################################################################
######## Targets ##############################################################
##@ Default command
.PHONY: all
all: clean build ## Clean, test and build the application.  Execute by default when make is called without arguments

###############################################################################
##@ Golang commands
.PHONY: go-fmt
go-fmt: ## Format go code
	@printf "👉 Formatting go code...\n"
	$(call exec_cmd, go fmt ./... )

.PHONY: go-vet
go-vet: ## Vet go code
	@printf "👉 Vet go code...\n"
	$(call exec_cmd, go vet  ./... )

.PHONY: go-generate
go-generate: ## Generate go code
	@printf "👉 Generating go code...\n"
	$(call exec_cmd, go generate ./... )

.PHONY: go-mod-tidy
go-mod-tidy: ## Clean go.mod and go.sum
	@printf "👉 Cleaning go.mod and go.sum...\n"
	$(call exec_cmd, go mod tidy)

.PHONY: go-mod-update
go-mod-update: go-mod-tidy ## Update go.mod and go.sum
	@printf "👉 Updating go.mod and go.sum...\n"
	$(foreach DEP, $(PROJECT_DEPENDENCIES), \
		$(call exec_cmd, go get -u $(DEP)) \
	)
	$(call exec_cmd, go mod tidy)

.PHONY: go-mod-vendor
go-mod-vendor: ## Create mod vendor
	@printf "👉 Creating mod vendor...\n"
	$(call exec_cmd, go mod vendor)

.PHONY: go-mod-verify
go-mod-verify: ## Verify go.mod and go.sum
	@printf "👉 Verifying go.mod and go.sum...\n"
	$(call exec_cmd, go mod verify)

.PHONY: go-mod-download
go-mod-download: ## Download go dependencies
	@printf "👉 Downloading go dependencies...\n"
	$(call exec_cmd, go mod download)

.PHONY: go-mod-graph
go-mod-graph: ## Create a file with the go dependencies graph in build dir
	@printf "👉 Printing go dependencies graph...\n"
	$(call exec_cmd, go mod graph > $(GO_GRAPH_FILE))

.PHONY: go-betteralign
go-betteralign: install-betteralign ## Align go code with betteralign
	@printf "👉 Aligning go code with betteralign...\n"
	$(call exec_cmd, betteralign -apply ./... )

# this target is needed to create the dist folder and the coverage file
$(PROJECT_COVERAGE_FILE):
	@printf "👉 Creating coverage file...\n"
	$(call exec_cmd, mkdir -p $(BUILD_DIR) )
	$(call exec_cmd, touch $(PROJECT_COVERAGE_FILE) )

.PHONY: go-test-coverage
go-test-coverage: test ## Shows in you browser the test coverage report per package
	@printf "👉 Running got tool coverage...\n"
	$(call exec_cmd, go tool cover -html=$(PROJECT_COVERAGE_FILE))

###############################################################################
##@ Test commands
.PHONY: test
test: $(PROJECT_COVERAGE_FILE) go-generate go-mod-tidy go-fmt go-vet ## Run tests
	@printf "👉 Running tests...\n"
	$(call exec_cmd, go test \
		-v -race \
		-coverprofile=$(PROJECT_COVERAGE_FILE) \
		-covermode=$(PROJECT_COVERAGE_MODE) \
		-tags=$(PROJECT_COVERAGE_TAGS) \
		./... \
	)

.PHONY: test-coverage
test-coverage: install-go-test-coverage ## Run tests and show coverage
	@printf "👉 Running tests and showing coverage...\n"
	$(call exec_cmd, go-test-coverage --config=./.testcoverage.yml )

###############################################################################
##@ Build commands
.PHONY: build
build: go-generate go-fmt go-vet ## Build the API service only
	@printf "👉 Building...\n"
	$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
		$(call exec_cmd, CGO_ENABLED=$(GO_CGO_ENABLED) go build $(GO_LDFLAGS) $(GO_OPTS) -o $(BUILD_DIR)/$(proj_mod) ./cmd/$(proj_mod)/ ) \
		$(call exec_cmd, chmod +x $(BUILD_DIR)/$(proj_mod) ) \
	)

.PHONY: build-all
build-all: lint vulncheck go-generate go-fmt go-vet ## Build all the application including the API service and the CLI
	@printf "👉 Building and lintering...\n"
	$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
		$(call exec_cmd, CGO_ENABLED=$(GO_CGO_ENABLED) go build $(GO_LDFLAGS) $(GO_OPTS) -o $(BUILD_DIR)/$(proj_mod) ./cmd/$(proj_mod)/ ) \
		$(call exec_cmd, chmod +x $(BUILD_DIR)/$(proj_mod) ) \
	)

.PHONY: build-dist
build-dist: ## Build the application for all platforms defined in GO_OS and GO_ARCH in this Makefile
	@printf "👉 Building application for different platforms...\n"
	$(foreach GOOS, $(GO_OS), \
		$(foreach GOARCH, $(GO_ARCH), \
			$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
				$(call exec_cmd, GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(GO_CGO_ENABLED) go build $(GO_LDFLAGS) $(GO_OPTS) -o $(DIST_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH) ./cmd/$(proj_mod)/ ) \
				$(call exec_cmd, chmod +x $(DIST_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH)) \
			)\
		)\
	)

.PHONY: build-dist-zip
build-dist-zip: ## Build the application for all platforms defined in GO_OS and GO_ARCH in this Makefile and create a zip file for each binary. Requires make build-dist
	@printf "👉 Creating zip files for distribution...\n"
	$(call exec_cmd, mkdir -p $(DIST_ASSETS_DIR))
	$(foreach GOOS, $(GO_OS), \
		$(foreach GOARCH, $(GO_ARCH), \
			$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
				$(call exec_cmd, zip --junk-paths -r $(DIST_ASSETS_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH).zip $(DIST_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH) ) \
				$(call exec_cmd, shasum -a 256 $(DIST_ASSETS_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH).zip | cut -d ' ' -f 1 > $(DIST_ASSETS_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH).sha256 ) \
			) \
		) \
	)

###############################################################################
##@ Check commands
.PHONY: lint
lint: install-golangci-lint ## Run linters
	@printf "👉 Running linters...\n"
	$(call exec_cmd, golangci-lint run ./...)

.PHONY: vulncheck
vulncheck: install-govulncheck ## Check vulnerabilities
	@printf "👉 Checking vulnerabilities...\n"
	$(call exec_cmd, govulncheck ./...)

###############################################################################
##@ Tools commands
.PHONY: install-go-test-coverage
install-go-test-coverage: ## Install got tool for test coverage (https://github.com/vladopajic/go-test-coverage)
	@printf "👉 Installing got tool for test coverage...\n"
	$(call exec_cmd, go install github.com/vladopajic/go-test-coverage/v2@latest )

.PHONY: install-govulncheck
install-govulncheck: ## Install govulncheck for vulnerabilities check (https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck#section-documentation)
	@printf "👉 Installing govulncheck...\n"
	$(call exec_cmd, go install golang.org/x/vuln/cmd/govulncheck@latest )

.PHONY: install-golangci-lint
install-golangci-lint: ## Install golangci-lint for linting (https://golangci-lint.run/)
	@printf "👉 Installing golangci-lint...\n"
	$(call exec_cmd, go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2 )

.PHONY: install-betteralign
install-betteralign: ## Install betteralign for code alignment (https://github.com/dkorunic/betteralign)
	@printf "👉 Installing betteralign...\n"
	$(call exec_cmd, go install github.com/dkorunic/betteralign/cmd/betteralign@latest )

###############################################################################
##@ Support Commands
.PHONY: clean
clean: ## Clean the environment
	@printf "👉 Cleaning environment...\n"
	$(call exec_cmd, go clean -n -x -i)
	$(call exec_cmd, rm -rf $(BUILD_DIR) $(DIST_DIR) )

# Test target to verify error handling
.PHONY: test-fail
test-fail: ## Test target that always fails (for testing error handling)
	@printf "👉 Testing error handling...\n"
	$(call exec_cmd, false)  # 'false' command always returns exit code 1
	@printf "This should not be printed if MAKE_STOP_ON_ERRORS=true\n"

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##";                                             \
		printf "Usage: make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ \
		{ printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2 } /^##@/            \
		{ printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } '                  \
		$(MAKEFILE_LIST)

