# Copyright 2022 Mitchell Kember. Subject to the MIT License.

define usage
Targets:
	all        Build sim
	help       Show this help message
	install    Install sim in $$XDG_BIN_HOME
	uninstall  Uninstall sim
	check      Format, lint, and build
	fmt        Format code
	lint       Lint code
	clean      Remove build output
endef

.PHONY: all help install uninstall check fmt lint clean

XDG_BIN_HOME ?= ~/.local/bin

bin := out/sim
dest := $(XDG_BIN_HOME)/$(notdir $(bin))

.SUFFIXES:

all: $(bin)

help:
	$(info $(usage))
	@:

install: $(dest)

check: fmt lint all

fmt:
	go fmt

lint:
	go fix
	go vet
	go mod tidy

clean:
	rm -rf out

$(bin): go.mod $(wildcard *.go)
	go build -o $@

$(XDG_BIN_HOME):
	mkdir -p $@

$(dest): $(bin) | $(XDG_BIN_HOME)
	$< install $<

ifneq (,$(wildcard $(dest)))
uninstall: $(dest)
	$< remove $<
endif
