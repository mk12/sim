XDG_BIN_HOME ?= ~/.local/bin

program := out/sim

.PHONY: all install

all: $(program)

$(program): go.mod $(wildcard *.go)
	go build -o $@

install: $(program)
	ln -sf $(XDG_BIN_HOME)/$(basename $@) $(abspath $^)
