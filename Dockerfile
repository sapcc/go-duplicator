FROM alpine:3.5

# WORKDIR /go/src/gotee
# COPY client.go tee.go ./

# RUN apk add --no-cache go musl-dev git mercurial \
#     && GOPATH=/go go get -d -v ./... \
#     && GOPATH=/go CGO_ENABLED=0 go install -v ./... \
#     && mv /go/bin/gotee /usr/local/bin \
#     && apk del go musl-dev git mercurial

COPY gotee /usr/local/bin/gotee

ENTRYPOINT [ "gotee" ]
CMD [ "--listen", "2003", "-1", "4001", "-2", "4002" ]
