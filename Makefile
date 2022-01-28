GITV != git describe --tags
GITC != git rev-parse --verify HEAD
SRC  != find . -type f -name '*.go' ! -name '*_test.go'
TEST != find . -type f -name '*_test.go'
MODPATH != head -n1 go.mod | sed 's/module //'

PREFIX  ?= /usr/local
VERSION ?= $(GITV)
COMMIT  ?= $(GITC)
BUILDER ?= Makefile

GO      := go
INSTALL := install
RM      := rm

whatsup: go.mod go.sum $(SRC)
	GO111MODULE=on CGO_ENABLED=0 \
	$(GO) build -o $@ \
	-ldflags="-s -w -X $(MODPATH)/version.version=$(VERSION) -X $(MODPATH)/version.commit=$(COMMIT) -X $(MODPATH)/version.builtBy=$(BUILDER)"

.PHONY: clean
clean:
	$(RM) -f whatsup

.PHONY: install
install: whatsup
	$(INSTALL) -d $(DESTDIR)$(PREFIX)/bin/
	$(INSTALL) -m 755 whatsup $(DESTDIR)$(PREFIX)/bin/whatsup

.PHONY: uninstall
uninstall:
	$(RM) -f $(DESTDIR)$(PREFIX)/bin/whatsup


# Development helpers

.PHONY: fmt
fmt:
	$(GO) fmt ./...
