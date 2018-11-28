FROM golang:1.11-alpine

WORKDIR /go/src/gotee
COPY . .

RUN apk add --no-cache git mercurial \
    && go get -d -v ./... \
    && apk del git mercurial
RUN go install -v ./...

ENTRYPOINT [ "gotee" ]
CMD [ "--listen", "2003" ]

