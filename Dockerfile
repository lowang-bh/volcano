#build stage
FROM golang:1.19.4-dlv
WORKDIR /go/src/github.com/volcano-sh/volcano
EXPOSE 8080 2345
ADD . /go/src/github.com/volcano-sh/volcano

CMD ["dlv", "debug", "/go/src/github.com/volcano-sh/volcano/cmd/controller-manager", "--headless", "--listen=:2345", "--api-version=2", "--log"]
