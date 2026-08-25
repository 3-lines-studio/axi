AXIS_DIR ?= ../axis
SLAXI_DIR ?= ../slaxi
ATTACHX_DIR ?= ../attachx
PREFIX ?= $(HOME)/.local

.PHONY: build install update uninstall

build:
	cd $(AXIS_DIR) && go build -o bin/axis .
	cd $(SLAXI_DIR) && go build -o bin/slaxi .
	cd $(ATTACHX_DIR) && go build -o bin/attachx .
	wails3 build

install: build
	install -Dm755 $(AXIS_DIR)/bin/axis $(PREFIX)/bin/.axis.new
	install -Dm755 bin/axi $(PREFIX)/bin/.axi.new
	install -Dm755 $(SLAXI_DIR)/bin/slaxi $(PREFIX)/bin/.slaxi.new
	install -Dm755 $(ATTACHX_DIR)/bin/attachx $(PREFIX)/bin/.attachx.new
	mv $(PREFIX)/bin/.axis.new $(PREFIX)/bin/axis
	mv $(PREFIX)/bin/.axi.new $(PREFIX)/bin/axi
	mv $(PREFIX)/bin/.slaxi.new $(PREFIX)/bin/slaxi
	mv $(PREFIX)/bin/.attachx.new $(PREFIX)/bin/attachx

update: install

uninstall:
	rm -f $(PREFIX)/bin/axi $(PREFIX)/bin/axis $(PREFIX)/bin/slaxi $(PREFIX)/bin/attachx
