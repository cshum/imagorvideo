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
	docker build -t imagorvideo:dev .

docker-dev-run:
	touch .env
	docker run --rm -p 8000:8000 --env-file .env imagorvideo:dev -debug -imagor-unsafe

docker-dev: docker-dev-build docker-dev-run
