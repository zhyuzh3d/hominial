BINARY_NAME := Hominial-Elli
APP_BUNDLE := Hominial-Elli.app
APP_CONTENTS := $(APP_BUNDLE)/Contents
APP_MACOS := $(APP_CONTENTS)/MacOS
APP_RESOURCES := $(APP_CONTENTS)/Resources
APP_CMD := ./cmd/eibanban
CODESIGN_IDENTITY ?= $(shell security find-identity -v -p codesigning 2>/dev/null | awk '/Codexide Local Code Signing/ { print $$2; exit }')

.PHONY: build run test clean

build:
	rm -rf $(APP_BUNDLE)
	mkdir -p $(APP_MACOS) $(APP_RESOURCES)
	go build -o $(APP_MACOS)/$(BINARY_NAME) $(APP_CMD)
	printf '%s\n' \
		'<?xml version="1.0" encoding="UTF-8"?>' \
		'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
		'<plist version="1.0">' \
		'<dict>' \
		'  <key>CFBundleExecutable</key>' \
		'  <string>$(BINARY_NAME)</string>' \
		'  <key>CFBundleIdentifier</key>' \
		'  <string>life.hominial.elli</string>' \
		'  <key>CFBundleName</key>' \
		'  <string>$(BINARY_NAME)</string>' \
		'  <key>CFBundleDisplayName</key>' \
		'  <string>$(BINARY_NAME)</string>' \
		'  <key>CFBundlePackageType</key>' \
		'  <string>APPL</string>' \
		'  <key>CFBundleShortVersionString</key>' \
		'  <string>0.3.0</string>' \
		'  <key>CFBundleVersion</key>' \
		'  <string>0.3.0</string>' \
		'  <key>LSMinimumSystemVersion</key>' \
		'  <string>12.0</string>' \
		'  <key>NSHighResolutionCapable</key>' \
		'  <true/>' \
		'</dict>' \
		'</plist>' \
		> $(APP_CONTENTS)/Info.plist
	@if command -v xattr >/dev/null 2>&1; then find $(APP_BUNDLE) -exec xattr -c {} \; 2>/dev/null || true; fi
	@if command -v xattr >/dev/null 2>&1; then xattr -d com.apple.FinderInfo $(APP_BUNDLE) 2>/dev/null || true; fi
	@if command -v xattr >/dev/null 2>&1; then xattr -d 'com.apple.fileprovider.fpfs#P' $(APP_BUNDLE) 2>/dev/null || true; fi
	@if command -v codesign >/dev/null 2>&1; then \
		if [ -n "$(CODESIGN_IDENTITY)" ]; then codesign --force --deep --sign "$(CODESIGN_IDENTITY)" $(APP_BUNDLE); else false; fi || \
		codesign --force --deep --sign - $(APP_BUNDLE) || true; \
	fi

run:
	open $(APP_BUNDLE)

test:
	go test ./...

clean:
	rm -rf $(BINARY_NAME) $(APP_BUNDLE)
