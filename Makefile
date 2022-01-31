XDG_BIN_HOME ?= ~/.local/bin

built := out/sim
installed := $(XDG_BIN_HOME)/$(notdir $(built))

define usage
Targets:
	all        Build sim
	help       Show this help message
	install    Install sim in $$XDG_BIN_HOME
	uninstall  Uninstall sim
endef

.PHONY: all help install uninstall

all: $(built)

help:
	$(info $(usage))
	@:

install: $(installed)

$(built): go.mod $(wildcard *.go)
	go build -o $@

$(XDG_BIN_HOME):
	mkdir -p $@

$(installed): $(built) | $(XDG_BIN_HOME)
	$< install $<

ifneq (,$(wildcard $(installed)))
uninstall: $(installed)
	$< remove $<
endif
