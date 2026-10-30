SHELL := /bin/bash
APP := MEGAFLOYD.app
BIN := $(APP)/Contents/MacOS/MEGAFLOYD

.PHONY: icon build test verify smoke smoke-ui run clean distclean upstream

icon:
	python3 desktop/scripts/make-icon.py

upstream:
	node desktop/scripts/check-upstream.mjs

build:
	node desktop/scripts/build.mjs

test:
	node --experimental-vm-modules --test desktop/test/*.test.mjs

verify:
	node desktop/scripts/verify.mjs

smoke:
	rm -rf /tmp/megafloyd-smoke
	MEGAFLOYD_HOME=/tmp/megafloyd-smoke ./$(BIN) --smoke
	node desktop/scripts/check-log-secrets.mjs /tmp/megafloyd-smoke

smoke-ui:
	rm -rf /tmp/megafloyd-smoke-ui
	MEGAFLOYD_HOME=/tmp/megafloyd-smoke-ui ./$(BIN) --smoke-ui
	node desktop/scripts/check-log-secrets.mjs /tmp/megafloyd-smoke-ui

run:
	open -n ./$(APP)

clean:
	rm -rf build

distclean: clean
	rm -rf $(APP) desktop/assets/MEGAFLOYD.iconset
