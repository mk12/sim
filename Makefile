XDG_BIN_HOME ?= ~/.local/bin

built := out/sim
installed := $(XDG_BIN_HOME)/$(notdir $(built))

.PHONY: all help install uninstall

all: $(built)

help:
	@echo "Targets:"
	@echo "help       show this help message"
	@echo "all        build sim"
	@echo "install    install sim in $$XDG_BIN_HOME"
	@echo "uninstall  uninstall sim"

install: $(installed)

$(built): go.mod $(wildcard *.go)
	go build -o $@

$(installed): $(built)
	$< install $<

ifneq (,$(wildcard $(installed)))
uninstall: $(installed)
	$< remove $<
endif
