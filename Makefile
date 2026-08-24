AXIS_DIR ?= ../axis
PREFIX ?= $(HOME)/.local
SYSTEMD_DIR ?= $(HOME)/.config/systemd/user
ENVIRONMENT ?= $(HOME)/.config/axis/environment

.PHONY: build configure install update restart stop status logs uninstall

build:
	cd $(AXIS_DIR) && go build -o bin/axis .
	wails3 build

configure:
	mkdir -p $(dir $(ENVIRONMENT))
	test -e $(ENVIRONMENT) || install -m 600 systemd/environment $(ENVIRONMENT)

install: build
	test -f $(ENVIRONMENT)
	install -Dm755 $(AXIS_DIR)/bin/axis $(PREFIX)/bin/.axis.new
	install -Dm755 bin/axi $(PREFIX)/bin/.axi.new
	mv $(PREFIX)/bin/.axis.new $(PREFIX)/bin/axis
	mv $(PREFIX)/bin/.axi.new $(PREFIX)/bin/axi
	install -Dm644 systemd/axis.service $(SYSTEMD_DIR)/axis.service
	install -Dm644 systemd/axi-web.service $(SYSTEMD_DIR)/axi-web.service
	systemctl --user daemon-reload
	systemctl --user enable --now axis.service axi-web.service

update: build
	install -Dm755 $(AXIS_DIR)/bin/axis $(PREFIX)/bin/.axis.new
	install -Dm755 bin/axi $(PREFIX)/bin/.axi.new
	mv $(PREFIX)/bin/.axis.new $(PREFIX)/bin/axis
	mv $(PREFIX)/bin/.axi.new $(PREFIX)/bin/axi
	systemctl --user restart axis.service axi-web.service

restart:
	systemctl --user restart axis.service axi-web.service

stop:
	systemctl --user stop axi-web.service axis.service

status:
	systemctl --user status axis.service axi-web.service

logs:
	journalctl --user -u axis.service -u axi-web.service -f

uninstall:
	systemctl --user disable --now axi-web.service axis.service
	rm -f $(SYSTEMD_DIR)/axi-web.service $(SYSTEMD_DIR)/axis.service
	systemctl --user daemon-reload
