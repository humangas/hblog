VERSION = $(shell gobump show -r)

.DEFAULT_GOAL := help

.PHONY: all init help setup deps build upload release install

all:

## Setup devlopment machine environment
init:
	go get github.com/Songmu/make2help/cmd/make2help
	go get github.com/motemen/gobump/cmd/gobump
	go get github.com/Songmu/ghch/cmd/ghch
	go get github.com/Songmu/goxz/cmd/goxz
	go get github.com/tcnksm/ghr

help: 
	@make2help $(MAKEFILE_LIST)

setup:
	go get github.com/golang/dep/cmd/dep

## Install dependencies
deps: setup
	dep ensure

build:
	goxz -pv=v$(VERSION) -os darwin -d=./dist/v$(VERSION)

upload:
	ghr v$(VERSION) dist/v$(VERSION)

## Release to GitHub releases
release:
	@make build
	@make upload
	
## Install/Upgrade hblog
install:
	go get -u github.com/motemen/blogsync
	go get -u github.com/humangas/hblog
