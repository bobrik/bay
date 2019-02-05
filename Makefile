-include .env
.DEFAULT_GOAL := help
PROJECTNAME := $(shell basename "$(PWD)")
BINS := proxy tracker
CMDSEP := ;

# Go related variables.
GOBASE := $(shell pwd)
GOPATH := $(GOBASE)/vendor:$(GOBASE)
GOBIN := $(GOBASE)/bin

# Redirect error output to a file, so we can show it in development mode.
STDERR := /tmp/.$(PROJECTNAME)-stderr.txt

# PID file will keep the process id of the server
PID=/tmp/.$(PROJECTNAME).pid

# Make is verbose in Linux. Make it silent.
MAKEFLAGS += --silent

## install: Install missing dependencies. Runs `go get` internally. e.g; make install get=github.com/foo/bar
install:
	$(foreach pkg,$(BINS),$(MAKE) go-get-$(pkg) $(CMDSEP))

## compile: Compile the binary.
compile: install
	@-touch $(STDERR)
	@-rm $(STDERR)
	$(foreach pkg,$(BINS),$(MAKE) go-build-$(pkg) 2> $(STDERR) $(CMDSEP))
	@cat $(STDERR) | sed -e '1s/.*/\nError:\n/'  | sed 's/make\[.*/ /' | sed "/^/s/^/     /" 1>&2

## clean: Clean build files. Runs `go clean` internally.
clean:
	@-rm $(GOBIN)/$(PROJECTNAME)-* 2> /dev/null
	$(foreach pkg,$(BINS),$(MAKE) go-clean-$(pkg) $(CMDSEP))

go-compile-%:
	@$(MAKE) go-get-$*
	@$(MAKE) go-build-$*

go-build-%:
	@echo "  >  Building $* binary..."
	@cd $* && GOPATH=$(GOPATH) GOBIN=$(GOBIN) go build -o $(GOBIN)/$(PROJECTNAME)-$*

go-generate-%:
	@echo "  >  Generating $* dependency files..."
	@cd $* && GOPATH=$(GOPATH) GOBIN=$(GOBIN) go generate $(generate)

go-get-%:
	@echo "  >  Checking if there is any missing $* dependencies..."
	@cd $* && GOPATH=$(GOPATH) GOBIN=$(GOBIN) go get -t -d -v $(get)

go-install-%:
	@cd $* && GOPATH=$(GOPATH) GOBIN=$(GOBIN) go install

go-clean-%:
	@echo "  >  Cleaning $* build cache"
	@cd $* && GOPATH=$(GOPATH) GOBIN=$(GOBIN) go clean

.PHONY: help
all: help
help: Makefile
	@echo
	@echo " Choose a command run in "$(PROJECTNAME)":"
	@echo
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo
