SHELL := /bin/bash
APP := MEGAFLOYD.app
BIN := $(APP)/Contents/MacOS/MEGAFLOYD

.PHONY: icon build verify smoke smoke-ui run clean distclean

icon:
	python3 desktop/scripts/make-icon.py

build:
	node desktop/scripts/build.mjs

verify:
	node desktop/scripts/verify.mjs

smoke:
	rm -rf /tmp/megafloyd-smoke
	MEGAFLOYD_HOME=/tmp/megafloyd-smoke ./$(BIN) --smoke

smoke-ui:
	rm -rf /tmp/megafloyd-smoke-ui
	MEGAFLOYD_HOME=/tmp/megafloyd-smoke-ui ./$(BIN) --smoke-ui

run:
	open -n ./$(APP)

clean:
	rm -rf build

distclean: clean
	rm -rf $(APP) desktop/assets/MEGAFLOYD.iconset
