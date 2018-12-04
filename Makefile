ARCH=amd64
OS=linux
IMAGE=mx3d/gotee
VERSION=v0.2

build:
	GOOS=$(OS) GOARCH=$(ARCH) go build
	docker build -t $(IMAGE):$(VERSION) . 

push:
	docker push $(IMAGE):$(VERSION)
