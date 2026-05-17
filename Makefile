IMAGOR_BASE_IMAGE ?= ghcr.io/cshum/imagor-base:vips8.18.2-r5-magick-ffmpeg

build:
	CGO_CFLAGS_ALLOW=-Xpreprocessor go build -o bin/imagorvideo ./cmd/imagorvideo/main.go

test:
	go clean -testcache && CGO_CFLAGS_ALLOW=-Xpreprocessor go test -coverprofile=profile.cov ./...

dev: build
	./bin/imagorvideo -debug -imagor-unsafe

help: build
	./bin/imagorvideo -h

get:
	go get -v -t -d ./...

docker-dev-build:
	docker build --build-arg BASE_IMAGE=$(IMAGOR_BASE_IMAGE) --build-arg DEV_BASE_IMAGE=$(IMAGOR_BASE_IMAGE)-dev -t imagorvideo:dev .

docker-dev-run:
	touch .env
	docker run --rm -p 8000:8000 --env-file .env imagorvideo:dev -debug -imagor-unsafe

docker-dev: docker-dev-build docker-dev-run

reset-golden:
	git rm -rf testdata/golden
	git commit -m  "test: reset golden"
	git push
