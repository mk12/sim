XDG_BIN_HOME ?= ~/.local/bin

built := out/sim
installed := $(XDG_BIN_HOME)/$(notdir $(built))

.PHONY: all install uninstall

all: $(built)

install: $(installed)

uninstall: $(built)
	$< remove --self

$(built): go.mod $(wildcard *.go)
	go build -o $@

$(installed): | $(built)
	$| install $|
