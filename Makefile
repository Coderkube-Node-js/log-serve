BINARY=server-logger
VERSION=1.0.0
BUILD_DIR=build
PKGROOT=$(BUILD_DIR)/pkgroot

.PHONY: build build-amd64 build-arm64 clean run deb

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X 'github.com/ashraf/log-serve/internal/server.Version=$(VERSION)'" -o $(BINARY) ./cmd

build-amd64:
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X 'github.com/ashraf/log-serve/internal/server.Version=$(VERSION)'" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd

build-arm64:
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X 'github.com/ashraf/log-serve/internal/server.Version=$(VERSION)'" -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd

run:
	go run ./cmd

deb: build-amd64
	rm -rf $(PKGROOT)
	mkdir -p $(PKGROOT)/DEBIAN $(PKGROOT)/usr/local/bin $(PKGROOT)/etc/server-logger $(PKGROOT)/etc/systemd/system $(PKGROOT)/var/lib/server-logger $(PKGROOT)/var/log/server-logger
	cp $(BUILD_DIR)/$(BINARY)-linux-amd64 $(PKGROOT)/usr/local/bin/$(BINARY)
	cp configs/config.yaml $(PKGROOT)/etc/server-logger/config.yaml
	cp service/server-logger.service $(PKGROOT)/etc/systemd/system/server-logger.service
	install -m 0755 packaging/postinst $(PKGROOT)/DEBIAN/postinst
	install -m 0755 packaging/prerm $(PKGROOT)/DEBIAN/prerm
	sed "s/{{VERSION}}/$(VERSION)/g" packaging/control > $(PKGROOT)/DEBIAN/control
	dpkg-deb --build $(PKGROOT) $(BUILD_DIR)/server-logger_$(VERSION)_amd64.deb

clean:
	rm -rf $(BUILD_DIR) $(BINARY)
