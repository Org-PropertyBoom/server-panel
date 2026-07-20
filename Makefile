APP_NAME := ppt-server-panel
CTL_NAME := pptctl
GO_BUILD_FLAGS := -buildvcs=false
GO_PACKAGES := . ./routes/... ./services/...

.PHONY: run dev build build-app build-ctl test fmt tidy clean

run:
	go run $(GO_BUILD_FLAGS) .

dev:
	./scripts/dev.sh

build: build-app build-ctl

build-app:
	mkdir -p public/dist
	go build $(GO_BUILD_FLAGS) -o public/dist/$(APP_NAME) .

build-ctl:
	mkdir -p public/dist
	go build $(GO_BUILD_FLAGS) -tags ctl -o public/dist/$(CTL_NAME) .

test:
	go test $(GO_PACKAGES)

fmt:
	go fmt ./...

tidy:
	go mod tidy

clean:
	rm -rf public/dist tmp
