FROM golang:1.19-alpine3.16 AS builder

RUN apk -U --no-cache --upgrade --latest add build-base git gcc bash

WORKDIR /go/src/github.com/aeneasr/go-crossorigin-proxy

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /usr/bin/go-crossorigin-proxy

FROM alpine:3.15

COPY --from=builder /usr/bin/go-crossorigin-proxy /usr/bin/go-crossorigin-proxy

EXPOSE 1313

ENTRYPOINT ["go-crossorigin-proxy"]
CMD ["serve"]


